package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// paletteSRT is a film's exported bilingual subtitle file: each translation over its original, as the pipeline writes it.
const paletteSRT = `1
00:00:00,400 --> 00:00:05,000
每条河都始于高山上的一股细流。
Every river begins as a trickle high in the mountains.

2
00:00:10,400 --> 00:00:15,000
水往低处流，沿途裹挟起泥沙和石块。
As water flows downhill, it picks up sand and stones.

3
00:00:20,400 --> 00:00:25,000
字幕里的每个词都能查。
Every word in the subtitles can be looked up.

4
00:00:30,400 --> 00:00:35,000
数千年间，河道被切割得越来越深。
Over thousands of years, the channel cuts deeper.

5
00:01:10,400 --> 00:01:15,000
这些大幅度的弯道被称为河曲。
These sweeping curves are called meanders.

`

// paletteLibrary makes a library folder with two finished films, rivers and market, each a video with its exported subtitles, and an app on it. rivers has been played to 0:31, market never.
func paletteLibrary(t *testing.T, p media.Pipeline) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	dir := t.TempDir()
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.LibraryDir = dir })
	for _, name := range []string{"rivers", "market"} {
		video := filepath.Join(dir, name, "video.mp4")
		if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		srt := filepath.Join(dir, name, "video.bilingual.srt")
		if err := os.WriteFile(srt, []byte(paletteSRT), 0o644); err != nil {
			t.Fatal(err)
		}
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: name, Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: dir})
	}
	a.palette.furthest = map[string]time.Duration{filepath.Join(dir, "rivers", "video.mp4"): 31 * time.Second}
	refresh(tt, a)
	return a, tt, updates
}

// paletteOpenFilm opens film id from the library and waits for its subtitles.
func paletteOpenFilm(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id string) {
	t.Helper()
	item, ok := a.paletteFilm(id)
	if !ok {
		t.Fatalf("no film %q in %v", id, a.films())
	}
	a.Open(item)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
}

// paletteOpen presses ⌘K and waits for the subtitle files the palette reads.
func paletteOpen(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	tt.Key("mod+k")
	if !a.palette.open {
		t.Fatal("⌘K did not open the palette")
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.palette.reading })
}

// paletteType types text at the end of the palette's field.
func paletteType(tt *nsgotest.App, text string) { tt.Type("id:palette-field", text) }

// TestPaletteOpensAnywhereAndEscapeCloses checks that ⌘K opens the palette over the library with its commands and the field focused, that the window's menu commands wait behind it as behind any modal dialog, and that Escape closes it and gives the menu back.
func TestPaletteOpensAnywhereAndEscapeCloses(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	for _, want := range []string{"命令", "Esc", "打开本地视频…"} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if !tt.Find("id:palette-field").Focused {
		t.Fatal("the palette's field does not have the keyboard")
	}
	if got := a.PaletteView(a.cx).Placeholder; got != "影片、命令或看过的台词…" {
		t.Fatalf("library placeholder %q", got)
	}
	if !a.modalOpen() {
		t.Fatal("the menu bar's commands stay enabled under the palette")
	}
	if got := len(a.PaletteView(a.cx).Commands); got != paletteFirstCommands {
		t.Fatalf("empty query lists %d commands, want %d", got, paletteFirstCommands)
	}
	tt.Key("escape")
	if a.palette.open || hasText(tt, "Esc") {
		t.Fatalf("Escape left the palette open: %q", tt.Texts())
	}
	if a.modalOpen() {
		t.Fatal("the palette kept the window's modal flag after it closed")
	}
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	if got := a.PaletteView(a.cx).Placeholder; got != "命令、台词，或直接问这部影片…" {
		t.Fatalf("player placeholder %q", got)
	}
	tt.Key("mod+k")
	if a.palette.open {
		t.Fatal("a second ⌘K left the palette open")
	}
	// Escape in the palette closes the palette, not the film behind it, which the window's own Escape closes.
	paletteOpen(t, a, tt, updates)
	tt.Key("escape")
	if a.palette.open || a.openKey != "rivers" {
		t.Fatalf("Escape: palette open %v, film %q", a.palette.open, a.openKey)
	}
}

// TestPaletteSearchesOnlyWatchedLines checks that, in the player, typing finds the open film's lines in its exported subtitle file, in the translation or the original, but only those up to the furthest point it was played to, so the palette does not give away what comes later.
func TestPaletteSearchesOnlyWatchedLines(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河")
	v := a.PaletteView(a.cx)
	if len(v.Lines) != 2 || v.Lines[0].At != "0:00.4" || v.Lines[1].At != "0:30.4" {
		t.Fatalf("lines %+v", v.Lines)
	}
	if v.Lines[0].Translation != "每条河都始于高山上的一股细流。" || v.Lines[0].Original != "Every river begins as a trickle high in the mountains." {
		t.Fatalf("line %+v", v.Lines[0])
	}
	if !hasText(tt, "台词") {
		t.Fatalf("no 台词 group in %q", tt.Texts())
	}
	tt.Clear("id:palette-field")
	paletteType(tt, "STONES")
	if v := a.PaletteView(a.cx); len(v.Lines) != 1 || v.Lines[0].At != "0:10.4" {
		t.Fatalf("searching the original: %+v", v.Lines)
	}
}

// TestPaletteFindsLinesAcrossTheLibrary checks that, over the library, the palette searches the subtitle files of every film in it, each up to where it was played, and names the film of each line; choosing one opens that film and starts it at the line.
func TestPaletteFindsLinesAcrossTheLibrary(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河")
	v := a.PaletteView(a.cx)
	if len(v.Lines) != 2 || v.Lines[1].Film != "rivers" {
		t.Fatalf("lines %+v", v.Lines)
	}
	tt.Key("down")
	tt.Key("enter")
	if a.palette.open {
		t.Fatal("choosing a line left the palette open")
	}
	if a.openKey != "rivers" || a.player.opening != 30*time.Second+400*time.Millisecond {
		t.Fatalf("open %q at %v", a.openKey, a.player.opening)
	}
}

// TestPaletteRunsCommands checks that the palette lists the real commands, the shortcut table's and one for each settings section, finds them by their names, and runs the one chosen with Return.
func TestPaletteRunsCommands(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "AI 服务")
	v := a.PaletteView(a.cx)
	if len(v.Lines) != 0 || len(v.Commands) != 1 || v.Commands[0].Label != "设置 › AI 服务" {
		t.Fatalf("commands %+v", v.Commands)
	}
	tt.Key("enter")
	wantSettings(t, a, settings.SectionAI)
	if a.palette.open {
		t.Fatal("running a command left the palette open")
	}
	// The settings show over the main window here, as on Windows and Linux, and hold the window's keys, ⌘K among them (binds): Escape closes them first.
	tt.Key("escape")
	refresh(tt, a)
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "快捷键")
	tt.Press("快捷键速查")
	if !a.overlay.cheatsheet || a.palette.open {
		t.Fatalf("cheat sheet %v palette %v", a.overlay.cheatsheet, a.palette.open)
	}
}

// TestPaletteCommandsFollowTheScreen checks that the player's commands are listed only while a film is open, with the subtitle modes it is not in, and that the screen shown now is not offered.
func TestPaletteCommandsFollowTheScreen(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	labels := func() []string {
		var out []string
		for _, c := range a.paletteCommands(a.cx) {
			out = append(out, c.label)
		}
		return out
	}
	has := func(list []string, s string) bool {
		for _, l := range list {
			if l == s {
				return true
			}
		}
		return false
	}
	lib := labels()
	if has(lib, "显示/隐藏字幕") || has(lib, "切换为仅译文") || has(lib, "片库") || !has(lib, "打开本地视频…") || !has(lib, "设置 › 语音识别") {
		t.Fatalf("library commands %q", lib)
	}
	paletteOpenFilm(t, a, tt, updates, "rivers")
	player := labels()
	if !has(player, "显示/隐藏字幕") || !has(player, "切换为仅译文") || has(player, "切换为双语") || has(player, "播放器") || !has(player, "片库") {
		t.Fatalf("player commands %q", player)
	}
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "仅译文")
	tt.Key("down")
	tt.Key("enter")
	if a.player.subMode != "translation" {
		t.Fatalf("sub mode %q", a.player.subMode)
	}
}

// TestPaletteArrowsMoveAcrossGroups checks that ↓ moves from the row that asks the film over the lines to the first command and stops at the last row, and ↑ comes back.
func TestPaletteArrowsMoveAcrossGroups(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "字幕")
	v := a.PaletteView(a.cx)
	if len(v.Lines) != 1 || len(v.Commands) < 2 {
		t.Fatalf("view %+v", v)
	}
	rows := 1 + len(v.Lines) + len(v.Commands)
	for range 1 + len(v.Lines) {
		tt.Key("down")
	}
	if a.palette.sel != 1+len(v.Lines) {
		t.Fatalf("selected %d after the ask row and %d lines", a.palette.sel, len(v.Lines))
	}
	for range rows + 3 {
		tt.Key("down")
	}
	if a.palette.sel != rows-1 {
		t.Fatalf("selected %d of %d rows", a.palette.sel, rows)
	}
	tt.Key("up")
	if a.palette.sel != rows-2 {
		t.Fatalf("↑ selected %d", a.palette.sel)
	}
}

// fakeLLM is an OpenAI-compatible chat endpoint that answers every question with reply, recording the conversations it was sent.
func fakeLLM(t *testing.T, reply string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "unexpected", http.StatusUnauthorized)
			return
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, m := range req.Messages {
			seen = append(seen, m.Content)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// TestPaletteAsksTheConfiguredAI checks that Return with a question asks the AI service set in 「AI 服务」, sending the lines of the open film's subtitle file watched so far and nothing after them, and shows its answer with chips that jump to the moments it names, and how far the film had been watched.
func TestPaletteAsksTheConfiguredAI(t *testing.T) {
	srv, seen := fakeLLM(t, "```json\n{\"answer\":\"0:30 讲到河道越切越深。\",\"refs\":[\"0:30\",\"9:00\"]}\n```")
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河流为什么会越切越深")
	tt.Key("enter")
	if !hasText(tt, "正在问 m1…") {
		t.Fatalf("no progress while asking: %q", tt.Texts())
	}
	waitUntil(t, tt, a, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	for _, want := range []string{"回答", "0:30 讲到河道越切越深。", "▶ 0:30", "只依据已经播放到的部分（到 0:31）· m1 回答"} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if hasText(tt, "▶ 9:00") {
		t.Fatal("the answer jumps past what was watched")
	}
	sent := strings.Join(*seen, "\n")
	if !strings.Contains(sent, "数千年间，河道被切割得越来越深。") || !strings.Contains(sent, "河流为什么会越切越深") {
		t.Fatalf("sent %q", sent)
	}
	if strings.Contains(sent, "河曲") || strings.Contains(sent, "meanders") {
		t.Fatalf("sent a line after the furthest point played: %q", sent)
	}
	tt.Press("跳到 0:30")
	if a.palette.open || a.player.opening != 30*time.Second+400*time.Millisecond {
		t.Fatalf("palette open %v, opening %v", a.palette.open, a.player.opening)
	}
}

// TestPaletteWithoutAIOffersItsSettings checks that, with no AI service set, a question gets the prompt to set one, whose button opens Settings › AI 服务.
func TestPaletteWithoutAIOffersItsSettings(t *testing.T) {
	a, tt, updates := paletteLibrary(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "这部影片讲了什么")
	tt.Key("enter")
	if !hasText(tt, testlocale.Messages().PaletteNoAI()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Press("打开「AI 服务」设置")
	wantSettings(t, a, settings.SectionAI)
	if a.palette.open {
		t.Fatal("the palette stayed open over the settings")
	}
}

// TestPaletteKeepsTheFurthestPointPlayed checks that how far each video was played is kept in library.json with the shelf, read back when the shelf is opened, and only ever grows.
func TestPaletteKeepsTheFurthestPointPlayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.paletteNoteFurthest("/v/a.mp4", 40*time.Second)
	a.paletteNoteFurthest("/v/a.mp4", 10*time.Second)
	a.areasPositionsChanged()
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Furthest["/v/a.mp4"] != 40*time.Second {
		t.Fatalf("saved %v", lib.Furthest)
	}
	b, _, _ := newTestApp(t, &fakePipeline{})
	b.openLibrary(path)
	if b.palette.furthest["/v/a.mp4"] != 40*time.Second {
		t.Fatalf("read back %v", b.palette.furthest)
	}
}

// TestPaletteIsInTheMenuAndTheCheatsheet checks that ⌘K is a row of the shortcut table, so the menu bar and the cheat sheet list it as the prototype's sheet does.
func TestPaletteIsInTheMenuAndTheCheatsheet(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	it := findMenuItem(t, a.menuBar(a.cx), "学习", "问这部影片 / 命令…")
	if it.Shortcut != "mod+k" {
		t.Fatalf("shortcut %q", it.Shortcut)
	}
	groups := a.cheatsheetGroups(a.cx)
	for _, g := range groups {
		if g.Title != "窗口" {
			continue
		}
		if g.Keys[0].Label != "问这部影片 / 命令" {
			t.Fatalf("窗口 starts with %q", g.Keys[0].Label)
		}
		return
	}
	t.Fatal("no 窗口 group")
}

// TestPaletteNotesHowFarTheOpenVideoPlayed checks that a video playing on from its start moves the furthest point played with it, as the player reports its progress, and that the player's saving of its resume positions writes that point into library.json beside them.
func TestPaletteNotesHowFarTheOpenVideoPlayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	play := palettePlayback{play: new(filmPlay), gen: 1}
	for _, at := range []time.Duration{0, 2 * time.Second, 4 * time.Second, 6 * time.Second} {
		a.paletteProgress(play, "/v/clip.mp4", at, true)
	}
	if got := a.palette.furthest["/v/clip.mp4"]; got != 6*time.Second {
		t.Fatalf("noted %v", got)
	}
	a.areasPositionsChanged()
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Furthest["/v/clip.mp4"] != 6*time.Second {
		t.Fatalf("furthest %v", lib.Furthest)
	}
}

// TestPaletteDoesNotCountASeekAsWatched checks that only playing on from what was watched moves the furthest point played: not a picture that moved while paused, not a seek past it, paused or playing, nor playing on from there, and that playing on from a seek back into what was watched moves it again. A seek starts another playback (seekPlayback moves app.player.playGen on), in the same filmPlay or a new one. The palette then finds no line the seek passed, and library.json keeps the point watched.
func TestPaletteDoesNotCountASeekAsWatched(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	item, _ := a.paletteFilm("rivers")
	video := item.VideoPath
	watched := 31 * time.Second
	// pictures notes the picture of playback play at each of at in turn, playing or paused, and checks that the furthest point played stayed watched.
	pictures := func(play palettePlayback, playing bool, at ...time.Duration) {
		t.Helper()
		for _, to := range at {
			a.paletteProgress(play, video, to, playing)
		}
		if got := a.palette.furthest[video]; got != watched {
			t.Fatalf("furthest %v, want %v", got, watched)
		}
	}
	play := new(filmPlay)
	pictures(palettePlayback{play: play, gen: 1}, false, 20*time.Second, 75*time.Second)
	pictures(palettePlayback{play: play, gen: 2}, false, 75*time.Second, 76*time.Second)
	pictures(palettePlayback{play: play, gen: 3}, true, 72*time.Second, 73*time.Second, 74*time.Second, 90*time.Second)
	pictures(palettePlayback{play: new(filmPlay), gen: 4}, true, 72*time.Second, 73*time.Second)
	if h := a.paletteHorizon(item); h != watched {
		t.Fatalf("horizon %v with the picture at 1:30", h)
	}
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "meanders")
	if v := a.PaletteView(a.cx); len(v.Lines) != 0 {
		t.Fatalf("a line the seek passed was found: %+v", v.Lines)
	}
	tt.Key("escape")
	back := palettePlayback{play: play, gen: 5}
	pictures(back, true, 25*time.Second, 28*time.Second, 31*time.Second)
	watched = 33 * time.Second
	pictures(back, true, 33*time.Second)
	a.areasPositionsChanged()
	if a.library.Furthest[video] != 33*time.Second {
		t.Fatalf("library.json keeps %v", a.library.Furthest[video])
	}
}

// TestPaletteFollowsThePlayingFilmOnlyAtNewLines checks that the player's progress reports make the app follow them only when the open palette lists another line: when the furthest point played passes the start of a line the query finds, not at every report, and never while the palette is closed or its query empty.
func TestPaletteFollowsThePlayingFilmOnlyAtNewLines(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	item, _ := a.paletteFilm("rivers")
	video := item.VideoPath
	play := palettePlayback{play: a.player.play, gen: a.player.playGen}
	// progress reports the picture at each of at in turn, playing, and returns the reports that changed what the palette lists.
	progress := func(at ...time.Duration) []time.Duration {
		var changed []time.Duration
		for _, to := range at {
			if a.paletteProgress(play, video, to, true) {
				changed = append(changed, to)
			}
		}
		return changed
	}
	if got := progress(31*time.Second, 40*time.Second); got != nil {
		t.Fatalf("the closed palette was followed at %v", got)
	}
	paletteOpen(t, a, tt, updates)
	if got := progress(50 * time.Second); got != nil {
		t.Fatalf("the palette without a query was followed at %v", got)
	}
	paletteType(tt, "meanders")
	if v := a.PaletteView(a.cx); len(v.Lines) != 0 {
		t.Fatalf("a line not watched yet was found: %+v", v.Lines)
	}
	var at []time.Duration
	for s := 51; s <= 80; s++ {
		at = append(at, time.Duration(s)*time.Second)
	}
	// The line about meanders starts at 1:10.4, so it counts as seen from 1:09.9 on: the report at 1:10 lists it, and none of the others changes anything.
	if got := progress(at...); !slices.Equal(got, []time.Duration{70 * time.Second}) {
		t.Fatalf("followed at %v, want only at 1:10", got)
	}
	tt.Settle()
	if v := a.PaletteView(a.cx); len(v.Lines) != 1 {
		t.Fatalf("the line just watched is not found: %+v", v.Lines)
	}
}

// TestPaletteFollowsARenamedLibrary checks that what was watched of a film is still known after its library folder was renamed in Finder: the palette still finds its lines watched, in memory and in library.json.
func TestPaletteFollowsARenamedLibrary(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	path := filepath.Join(t.TempDir(), "library.json")
	jobs := a.library.Jobs
	a.openLibrary(path)
	a.library.Jobs = jobs
	dir := a.snap().LibraryDir
	old := filepath.Join(dir, "rivers", "video.mp4")
	a.palette.furthest = map[string]time.Duration{old: 31 * time.Second}
	a.library.Furthest = map[string]time.Duration{old: 31 * time.Second}
	next := dir + " renamed.kando"
	if err := os.Rename(dir, next); err != nil {
		t.Fatal(err)
	}
	a.renameLibraryPath(dir, next)
	video := filepath.Join(next, "rivers", "video.mp4")
	if a.palette.furthest[video] != 31*time.Second || len(a.palette.furthest) != 1 {
		t.Fatalf("furthest %v", a.palette.furthest)
	}
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Furthest[video] != 31*time.Second || len(lib.Furthest) != 1 {
		t.Fatalf("library.json keeps %v", lib.Furthest)
	}
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河")
	if v := a.PaletteView(a.cx); len(v.Lines) != 2 {
		t.Fatalf("lines %+v", v.Lines)
	}
}

// TestPaletteFollowsVideosGatheredIntoFolders checks that a loose video the library refresh moves into a folder of its own keeps how far it was played, under its new path.
func TestPaletteFollowsVideosGatheredIntoFolders(t *testing.T) {
	dir := t.TempDir()
	loose := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(loose, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.palette.furthest = map[string]time.Duration{loose: 12 * time.Second}
	a.library.Furthest = map[string]time.Duration{loose: 12 * time.Second}
	a.refreshLibrary()
	video := filepath.Join(dir, "talk", "video.mp4")
	if _, err := os.Stat(video); err != nil {
		t.Fatalf("the refresh did not gather the video: %v", err)
	}
	if a.palette.furthest[video] != 12*time.Second || len(a.palette.furthest) != 1 {
		t.Fatalf("furthest %v", a.palette.furthest)
	}
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Furthest[video] != 12*time.Second || len(lib.Furthest) != 1 {
		t.Fatalf("library.json keeps %v", lib.Furthest)
	}
}

// TestPaletteReadsTheModelsReply checks that a reply in the JSON asked for gives the answer and chips at the starts of the lines it names, that a moment no line seen starts at gives no chip, and that a reply in plain text is shown as it is.
func TestPaletteReadsTheModelsReply(t *testing.T) {
	seen := []lines.Line{{Start: 400 * time.Millisecond}, {Start: 30*time.Second + 400*time.Millisecond}, {Start: 62 * time.Second}}
	text, refs := paletteParseReply(`{"answer":"讲到了。","refs":["0:30","1:02","0:30","0:45","later"]}`, seen)
	if text != "讲到了。" || len(refs) != 2 || refs[0] != seen[1].Start || refs[1] != seen[2].Start {
		t.Fatalf("text %q refs %v", text, refs)
	}
	text, refs = paletteParseReply("  这部影片讲河流。 ", seen)
	if text != "这部影片讲河流。" || refs != nil {
		t.Fatalf("plain reply %q %v", text, refs)
	}
}

// TestPaletteSaysWhenTheServiceFails checks that an answer the AI service refuses shows why, without the note that names a model as having answered.
func TestPaletteSaysWhenTheServiceFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "wrong", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河流为什么会越切越深")
	tt.Key("enter")
	waitUntil(t, tt, a, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	v := a.PaletteView(a.cx)
	if v.Answer == nil || !strings.HasPrefix(v.Answer.Text, "没有拿到回答：") || v.Answer.Note != "" || v.Answer.Action != testlocale.Messages().PaletteNoAIAction() {
		t.Fatalf("answer %+v", v.Answer)
	}
}

// TestPaletteTypingDropsTheQuestion checks that typing again while a question is on its way stops it and drops its card, so a late reply does not show over the new query.
func TestPaletteTypingDropsTheQuestion(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(block); srv.Close() })
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "为什么")
	tt.Key("enter")
	if a.palette.answer == nil || !a.palette.answer.busy || a.palette.cancel == nil {
		t.Fatalf("answer %+v", a.palette.answer)
	}
	paletteType(tt, "？")
	if a.palette.answer != nil || a.palette.cancel != nil {
		t.Fatalf("typing kept the question: %+v", a.palette.answer)
	}
	waitUpdate(t, tt, a, updates)
	tt.Key("escape")
	if a.palette.open || a.modalOpen() {
		t.Fatal("Escape left the palette open")
	}
}

// TestPaletteTreatsASignedOutCLIAsNoService checks that when the sidebar's check found the EveryAPI CLI signed out, a question gets what to do about it and the button to the AI service's settings instead of a request bound to fail.
func TestPaletteTreatsASignedOutCLIAsNoService(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, media.ErrEveryAPISignedOut
	}}}
	a, tt, updates := paletteLibrary(t, tools)
	a.side.ready.signInDone, a.side.ready.signInErr = true, media.ErrEveryAPISignedOut
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "这部影片讲了什么")
	tt.Key("enter")
	if !hasText(tt, everyAPISignedOut(a.cx)) || !hasText(tt, testlocale.Messages().PaletteNoAIAction()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteSaysTheEveryAPICLIIsMissing checks that EveryAPI without a key on a computer without the everyapi program gets the words the sidebar uses for it, not the prompt to choose a model.
func TestPaletteSaysTheEveryAPICLIIsMissing(t *testing.T) {
	a, tt, updates := paletteLibrary(t, media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "这部影片讲了什么")
	tt.Key("enter")
	if !hasText(tt, everyAPIMissing(a.cx)) || !hasText(tt, testlocale.Messages().PaletteNoAIAction()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteSaysWhenNothingIsFound checks that a query that finds no line and no command says so, over the library, and in the player under the row that asks the film, which Return runs.
func TestPaletteSaysWhenNothingIsFound(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	msgs := testlocale.Messages()
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "火山")
	if !hasText(tt, msgs.PaletteNothingFound()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Key("escape")
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "火山")
	if !hasText(tt, msgs.PaletteNothingFound()) || !hasText(tt, msgs.PaletteAskRow("火山")) || a.PaletteView(a.cx).Selected != 0 {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteHighlightsWhatReturnDoes checks that in the player the row Return runs is the one highlighted: once something is typed, a row that asks the film comes first and is highlighted, and Return asks; with ↓ on a command, Return runs the command and asks nothing.
func TestPaletteHighlightsWhatReturnDoes(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "仅原文")
	ask := testlocale.Messages().PaletteAskRow("仅原文")
	v := a.PaletteView(a.cx)
	if v.Ask != ask || v.Selected != 0 || len(v.Lines) != 0 || len(v.Commands) != 1 {
		t.Fatalf("view %+v", v)
	}
	wantText(t, tt, ask)
	tt.Key("down")
	if v := a.PaletteView(a.cx); v.Selected != 1 {
		t.Fatalf("↓ selected %d", v.Selected)
	}
	tt.Key("enter")
	if a.palette.answer != nil || a.player.subMode != film.SubOriginal || a.palette.open {
		t.Fatalf("answer %+v, sub mode %q, palette open %v", a.palette.answer, a.player.subMode, a.palette.open)
	}
	// Over the library no row asks: Return runs the first row found.
	tt.Key("escape")
	refresh(tt, a)
	a.Select(pageFilms)
	refresh(tt, a)
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "打开本地")
	if v := a.PaletteView(a.cx); v.Ask != "" || v.Selected != 0 {
		t.Fatalf("library view %+v", v)
	}
}

// TestPaletteFindsCommandsAsTheShortcutsPageDoes checks that the palette finds a command by its key as written there, Esc for 返回资料库, and by every word of a query in its label, group or key, and that it lists first the commands whose label starts with the query, so Return runs the closest one.
func TestPaletteFindsCommandsAsTheShortcutsPageDoes(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	labels := func(query string) []string {
		a.palette.query = query
		_, cmds := a.paletteFound(a.cx)
		var out []string
		for _, c := range cmds {
			out = append(out, c.label)
		}
		return out
	}
	msgs := testlocale.Messages()
	if got := labels("esc"); !slices.Equal(got, []string{msgs.FilmBackToLibrary()}) {
		t.Fatalf("esc found %q", got)
	}
	if got := labels("显示 f"); !slices.Contains(got, "全屏") || slices.Contains(got, msgs.FilmBackToLibrary()) {
		t.Fatalf("显示 f found %q", got)
	}
	got := labels("字幕")
	if len(got) < 3 || !strings.HasPrefix(got[0], "字幕") || !slices.Contains(got, "显示/隐藏字幕") {
		t.Fatalf("字幕 found %q", got)
	}
	a.palette.query = ""
}

// TestPaletteListsTheCheatSheetFirst checks that the palette opens with the shortcut cheat sheet first among its commands, over the library and in the player, so the list of shortcuts is a press away on every platform.
func TestPaletteListsTheCheatSheetFirst(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	first := testlocale.Messages().CheatsheetMenuItem()
	paletteOpen(t, a, tt, updates)
	if v := a.PaletteView(a.cx); len(v.Commands) != paletteFirstCommands || v.Commands[0].Label != first {
		t.Fatalf("library commands %+v", v.Commands)
	}
	tt.Key("escape")
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	if v := a.PaletteView(a.cx); len(v.Commands) != paletteFirstCommands || v.Commands[0].Label != first {
		t.Fatalf("player commands %+v", v.Commands)
	}
}

// TestPaletteInEnglish checks the palette in English: over the library its field, headings, footer and the note for a query that finds nothing; in the player its field and the card that asks for an AI service.
func TestPaletteInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	if got := a.PaletteView(a.cx).Placeholder; got != "Films, commands or watched lines…" {
		t.Fatalf("library placeholder %q", got)
	}
	for _, want := range []string{"Commands", "Type to search films, commands and watched lines · ↑↓ to select · Return to run"} {
		wantText(t, tt, want)
	}
	paletteType(tt, "volcano")
	wantText(t, tt, "No films, commands or watched lines found.")
	tt.Key("escape")
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	if got := a.PaletteView(a.cx).Placeholder; got != "Commands, lines, or ask this film…" {
		t.Fatalf("player placeholder %q", got)
	}
	paletteType(tt, "what is this film about")
	tt.Key("enter")
	for _, want := range []string{"Answer", "Answering questions needs an AI service. Choose a model in “AI Service” first.", "Open AI Service Settings"} {
		wantText(t, tt, want)
	}
	if hasText(tt, "回答") || hasText(tt, "命令") {
		t.Fatalf("the palette still reads Chinese: %q", tt.Texts())
	}
}

// TestPaletteFindsFilmsByName checks that the palette lists the films whose names hold the query before the lines, none while nothing is typed and not the film open in the player, and that Return on one closes the palette and plays it.
func TestPaletteFindsFilmsByName(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	if v := a.PaletteView(a.cx); len(v.Films) != 0 {
		t.Fatalf("films %q with nothing typed", v.Films)
	}
	paletteType(tt, "MARK")
	if v := a.PaletteView(a.cx); !slices.Equal(v.Films, []string{"market"}) || v.Selected != 0 {
		t.Fatalf("films %q, selected %d", v.Films, v.Selected)
	}
	wantText(t, tt, testlocale.Messages().PaletteHeadingFilms())
	tt.Key("enter")
	if a.palette.open || a.openKey != "market" {
		t.Fatalf("Return on the film: palette open %v, film %q", a.palette.open, a.openKey)
	}
	// In the player the row that asks the film comes first, and the open film is not offered.
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "r")
	v := a.PaletteView(a.cx)
	if v.Ask == "" || !slices.Equal(v.Films, []string{"rivers"}) {
		t.Fatalf("player view: ask %q, films %q", v.Ask, v.Films)
	}
	tt.Key("down")
	tt.Key("enter")
	if a.palette.open || a.openKey != "rivers" {
		t.Fatalf("Return on the film in the player: palette open %v, film %q", a.palette.open, a.openKey)
	}
}

// TestPaletteExplainsTheLinePlaying checks that E, in a film with lines, opens the palette with the AI service explaining the line playing, with no question typed: it sends that line, past the furthest point played as well, with the lines just before it, and not the film's other lines, and the card shows the line's original over the explanation, a chip that plays the line and which model explained it.
func TestPaletteExplainsTheLinePlaying(t *testing.T) {
	srv, seen := fakeLLM(t, `{"answer":"河曲是河流的大弯道。","refs":["1:10"]}`)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	item, _ := a.paletteFilm("rivers")
	a.OpenAt(item, 71*time.Second)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	tt.Key("e")
	if !a.palette.open || a.palette.answer == nil || !a.palette.answer.busy {
		t.Fatalf("E: palette open %v answer %+v", a.palette.open, a.palette.answer)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	for _, want := range []string{"These sweeping curves are called meanders.\n\n河曲是河流的大弯道。", "▶ 1:10", "m1 讲解"} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if chips := a.palette.answer.refs; len(chips) != 1 || chips[0] != 70*time.Second+400*time.Millisecond {
		t.Fatalf("chips %v", chips)
	}
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"要讲解的一句：[1:10] These sweeping curves are called meanders.", "数千年间，河道被切割得越来越深。", "Every word in the subtitles can be looked up.", "语法"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
	if strings.Contains(sent, "Every river begins") {
		t.Fatalf("sent lines far from the one explained: %q", sent)
	}
	tt.Press("跳到 1:10")
	if a.palette.open {
		t.Fatal("the chip left the palette open")
	}
}

// TestPaletteAnswersInTheFilmsLanguage checks that Explain Line and a question about a film translated into another language than the one the settings name now are answered in the film's, the language of its subtitles on screen, and that a film translated into none is answered in the settings' one.
func TestPaletteAnswersInTheFilmsLanguage(t *testing.T) {
	srv, seen := fakeLLM(t, `{"answer":"說明。","refs":[]}`)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	a.prefs.language = "简体中文"
	a.library.Jobs[0].Language = "繁體中文"
	refresh(tt, a)
	paletteOpenFilm(t, a, tt, updates, "rivers")
	ask := func(run func(), want string) {
		t.Helper()
		*seen = nil
		apply(tt, a, run)
		waitUntil(t, tt, a, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
		if sent := strings.Join(*seen, "\n"); !strings.Contains(sent, "用"+want) {
			t.Fatalf("sent %q, want it answered in %s", sent, want)
		}
	}
	ask(func() { a.ExplainLine(1) }, "繁體中文")
	ask(func() { a.palette.query = "what is this film about"; a.paletteAsk() }, "繁體中文")
	a.library.Jobs[0].TranslationErr = "the AI service refused the key"
	refresh(tt, a)
	ask(func() { a.ExplainLine(1) }, "简体中文")
}

// TestPaletteExplainWithoutAIOffersItsSettings checks that Explain Line, with no AI service set, gets the palette's prompt to set one rather than a question sent nowhere.
func TestPaletteExplainWithoutAIOffersItsSettings(t *testing.T) {
	a, tt, updates := paletteLibrary(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	a.ExplainLine(1)
	refresh(tt, a)
	if !a.palette.open || !hasText(tt, testlocale.Messages().PaletteNoAI()) || !hasText(tt, testlocale.Messages().PaletteNoAIAction()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}
