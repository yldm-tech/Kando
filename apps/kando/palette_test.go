package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
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
func paletteLibrary(t *testing.T, p media.Pipeline) (*app, *ui.Tester, chan func()) {
	t.Helper()
	dir := t.TempDir()
	a, tt, updates := newTestApp(p)
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
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: name, Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: dir})
	}
	a.palette.furthest = map[string]time.Duration{filepath.Join(dir, "rivers", "video.mp4"): 31 * time.Second}
	tt.Frame()
	return a, tt, updates
}

// paletteOpenFilm opens film id from the library and waits for its subtitles.
func paletteOpenFilm(t *testing.T, a *app, tt *ui.Tester, updates chan func(), id string) {
	t.Helper()
	item, ok := a.paletteFilm(id)
	if !ok {
		t.Fatalf("no film %q in %v", id, a.films())
	}
	a.Open(item)
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
	tt.Frame()
}

// paletteOpen presses ⌘K and waits for the subtitle files the palette reads.
func paletteOpen(t *testing.T, a *app, tt *ui.Tester, updates chan func()) {
	t.Helper()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !a.palette.open {
		t.Fatal("⌘K did not open the palette")
	}
	waitUntil(t, updates, func() bool { return !a.palette.reading })
	tt.Frame()
}

func paletteType(tt *ui.Tester, text string) {
	tt.Type(text)
	tt.Frame()
	tt.Frame()
}

// TestPaletteOpensAnywhereAndEscapeCloses checks that ⌘K opens the palette over the library with its commands and the field focused, that the window's menu commands wait behind it as behind any modal dialog, and that Escape closes it and gives the menu back.
func TestPaletteOpensAnywhereAndEscapeCloses(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	for _, want := range []string{"命令", "Esc", "打开本地视频…"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if got := a.paletteView().Placeholder; got != "命令或看过的台词…" {
		t.Fatalf("library placeholder %q", got)
	}
	if !a.modalOpen() {
		t.Fatal("the menu bar's commands stay enabled under the palette")
	}
	if got := len(a.paletteView().Commands); got != paletteFirstCommands {
		t.Fatalf("empty query lists %d commands, want %d", got, paletteFirstCommands)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.palette.open || tt.HasText("Esc") {
		t.Fatalf("Escape left the palette open: %q", tt.Texts())
	}
	if a.modalOpen() {
		t.Fatal("the palette kept the window's modal flag after it closed")
	}
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	if got := a.paletteView().Placeholder; got != "命令、台词，或直接问这部影片…" {
		t.Fatalf("player placeholder %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if a.palette.open {
		t.Fatal("a second ⌘K left the palette open")
	}
}

// TestPaletteSearchesOnlyWatchedLines checks that, in the player, typing finds the open film's lines in its exported subtitle file, in the translation or the original, but only those up to the furthest point it was played to, so the palette does not give away what comes later.
func TestPaletteSearchesOnlyWatchedLines(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河")
	v := a.paletteView()
	if len(v.Lines) != 2 || v.Lines[0].At != "0:00.4" || v.Lines[1].At != "0:30.4" {
		t.Fatalf("lines %+v", v.Lines)
	}
	if v.Lines[0].Translation != "每条河都始于高山上的一股细流。" || v.Lines[0].Original != "Every river begins as a trickle high in the mountains." {
		t.Fatalf("line %+v", v.Lines[0])
	}
	if !tt.HasText("台词") {
		t.Fatalf("no 台词 group in %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	paletteType(tt, "STONES")
	if v := a.paletteView(); len(v.Lines) != 1 || v.Lines[0].At != "0:10.4" {
		t.Fatalf("searching the original: %+v", v.Lines)
	}
}

// TestPaletteFindsLinesAcrossTheLibrary checks that, over the library, the palette searches the subtitle files of every film in it, each up to where it was played, and names the film of each line; choosing one opens that film and starts it at the line.
func TestPaletteFindsLinesAcrossTheLibrary(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "河")
	v := a.paletteView()
	if len(v.Lines) != 2 || v.Lines[1].Film != "rivers" {
		t.Fatalf("lines %+v", v.Lines)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
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
	v := a.paletteView()
	if len(v.Lines) != 0 || len(v.Commands) != 1 || v.Commands[0].Label != "设置 › AI 服务" {
		t.Fatalf("commands %+v", v.Commands)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	wantSettings(t, a, settings.SectionAI)
	if a.palette.open {
		t.Fatal("running a command left the palette open")
	}
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "快捷键")
	if err := tt.Click("快捷键速查"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.overlay.cheatsheet || a.palette.open {
		t.Fatalf("cheat sheet %v palette %v", a.overlay.cheatsheet, a.palette.open)
	}
}

// TestPaletteCommandsFollowTheScreen checks that the player's commands are listed only while a film is open, with the subtitle modes it is not in, and that the screen shown now is not offered.
func TestPaletteCommandsFollowTheScreen(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	labels := func() []string {
		var out []string
		for _, c := range a.paletteCommands() {
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
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.player.subMode != "translation" {
		t.Fatalf("sub mode %q", a.player.subMode)
	}
}

// TestPaletteArrowsMoveAcrossGroups checks that ↓ moves from the last line to the first command and stops at the last row, and ↑ comes back.
func TestPaletteArrowsMoveAcrossGroups(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "字幕")
	v := a.paletteView()
	if len(v.Lines) != 1 || len(v.Commands) < 2 {
		t.Fatalf("view %+v", v)
	}
	for range len(v.Lines) {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
	}
	if a.palette.sel != len(v.Lines) {
		t.Fatalf("selected %d after %d lines", a.palette.sel, len(v.Lines))
	}
	for range len(v.Lines) + len(v.Commands) + 3 {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
	}
	if a.palette.sel != len(v.Lines)+len(v.Commands)-1 {
		t.Fatalf("selected %d of %d rows", a.palette.sel, len(v.Lines)+len(v.Commands))
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.palette.sel != len(v.Lines)+len(v.Commands)-2 {
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
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("正在问 m1…") {
		t.Fatalf("no progress while asking: %q", tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	tt.Frame()
	for _, want := range []string{"回答", "0:30 讲到河道越切越深。", "▶ 0:30", "只依据已经播放到的部分（到 0:31）· m1 回答"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("▶ 9:00") {
		t.Fatal("the answer jumps past what was watched")
	}
	sent := strings.Join(*seen, "\n")
	if !strings.Contains(sent, "数千年间，河道被切割得越来越深。") || !strings.Contains(sent, "河流为什么会越切越深") {
		t.Fatalf("sent %q", sent)
	}
	if strings.Contains(sent, "河曲") || strings.Contains(sent, "meanders") {
		t.Fatalf("sent a line after the furthest point played: %q", sent)
	}
	if err := tt.Click("跳到 0:30"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
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
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText(locales.PaletteNoAI()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("打开「AI 服务」设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wantSettings(t, a, settings.SectionAI)
	if a.palette.open {
		t.Fatal("the palette stayed open over the settings")
	}
}

// TestPaletteKeepsTheFurthestPointPlayed checks that how far each video was played is kept in library.json with the shelf, read back when the shelf is opened, and only ever grows.
func TestPaletteKeepsTheFurthestPointPlayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
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
	b, _, _ := newTestApp(&fakePipeline{})
	b.openLibrary(path)
	if b.palette.furthest["/v/a.mp4"] != 40*time.Second {
		t.Fatalf("read back %v", b.palette.furthest)
	}
}

// TestPaletteIsInTheMenuAndTheCheatsheet checks that ⌘K is a row of the shortcut table, so the menu bar and the cheat sheet list it as the prototype's sheet does.
func TestPaletteIsInTheMenuAndTheCheatsheet(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	it := findMenuItem(t, a.menuBar(), "学习", "问这部影片 / 命令…")
	if it.Accelerator != accelerator(ui.Cmd, ui.KeyK) {
		t.Fatalf("accelerator %q", it.Accelerator)
	}
	groups := a.cheatsheetGroups()
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

// stillPlay is the video at video open in the player with its picture at at, playing or paused, as startPlayback leaves it after opening the video there; a seek opens it again the same way, as another playback.
func stillPlay(t *testing.T, video string, at time.Duration, playing bool) *filmPlay {
	t.Helper()
	player, err := ui.NewVideoPlayer(stillFrames{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(player.Close)
	if playing {
		player.Play()
	} else {
		player.Pause()
	}
	return &filmPlay{player: player, frames: &frameDecoder{shown: at}, path: video, duration: 2 * time.Minute, paused: !playing, played: playing}
}

// TestPaletteNotesHowFarTheOpenVideoPlayed checks that a video playing on from its start moves the furthest point played with it, frame by frame, and that closing it writes that point into library.json beside its resume position.
func TestPaletteNotesHowFarTheOpenVideoPlayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a := openStill(t, path, "clip", "/v/clip.mp4", 0)
	a.player.play.player.Play()
	a.areasAfterFrame()
	for _, at := range []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second} {
		a.player.play.frames.shown = at
		a.areasAfterFrame()
	}
	if got := a.palette.furthest["/v/clip.mp4"]; got != 6*time.Second {
		t.Fatalf("noted %v", got)
	}
	a.stopPlayback()
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Furthest["/v/clip.mp4"] != 6*time.Second || lib.Positions["/v/clip.mp4"] != 6*time.Second {
		t.Fatalf("furthest %v positions %v", lib.Furthest, lib.Positions)
	}
}

// TestPaletteDoesNotCountASeekAsWatched checks that only playing on from what was watched moves the furthest point played: not a picture that moved while paused, not a seek past it, paused or playing, nor playing on from there, and that playing on from a seek back into what was watched moves it again. The palette then finds no line the seek passed, and library.json keeps the point watched.
func TestPaletteDoesNotCountASeekAsWatched(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	item, _ := a.paletteFilm("rivers")
	video := item.VideoPath
	watched := 31 * time.Second
	frames := func(play *filmPlay, at ...time.Duration) {
		t.Helper()
		a.player.play = play
		tt.Frame()
		for _, to := range at {
			play.frames.shown = to
			tt.Frame()
		}
		if got := a.palette.furthest[video]; got != watched {
			t.Fatalf("furthest %v, want %v", got, watched)
		}
	}
	paused := stillPlay(t, video, 20*time.Second, false)
	frames(paused, 75*time.Second)
	frames(stillPlay(t, video, 75*time.Second, false), 76*time.Second)
	frames(stillPlay(t, video, 72*time.Second, true), 73*time.Second, 74*time.Second, 90*time.Second)
	if h := a.paletteHorizon(item); h != watched {
		t.Fatalf("horizon %v with the picture at 1:30", h)
	}
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "meanders")
	if v := a.paletteView(); len(v.Lines) != 0 {
		t.Fatalf("a line the seek passed was found: %+v", v.Lines)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	back := stillPlay(t, video, 25*time.Second, true)
	watched = 31 * time.Second
	frames(back, 28*time.Second, 31*time.Second)
	watched = 33 * time.Second
	frames(back, 33*time.Second)
	a.stopPlayback()
	if a.library.Furthest[video] != 33*time.Second {
		t.Fatalf("library.json keeps %v", a.library.Furthest[video])
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
	if v := a.paletteView(); len(v.Lines) != 2 {
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
	a, _, _ := newTestApp(&fakePipeline{})
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
	seen := []paletteCue{{start: 400 * time.Millisecond}, {start: 30*time.Second + 400*time.Millisecond}, {start: 62 * time.Second}}
	text, refs := paletteParseReply(`{"answer":"讲到了。","refs":["0:30","1:02","0:30","0:45","later"]}`, seen)
	if text != "讲到了。" || len(refs) != 2 || refs[0] != seen[1].start || refs[1] != seen[2].start {
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
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	tt.Frame()
	v := a.paletteView()
	if v.Answer == nil || !strings.HasPrefix(v.Answer.Text, "没有拿到回答：") || v.Answer.Note != "" || v.Answer.Action != locales.PaletteNoAIAction() {
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
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.palette.answer == nil || !a.palette.answer.busy || a.palette.cancel == nil {
		t.Fatalf("answer %+v", a.palette.answer)
	}
	paletteType(tt, "？")
	if a.palette.answer != nil || a.palette.cancel != nil {
		t.Fatalf("typing kept the question: %+v", a.palette.answer)
	}
	waitUntil(t, updates, func() bool { return true })
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
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
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText(everyAPISignedOut()) || !tt.HasText(locales.PaletteNoAIAction()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteSaysTheEveryAPICLIIsMissing checks that EveryAPI without a key on a computer without the everyapi program gets the words the sidebar uses for it, not the prompt to choose a model.
func TestPaletteSaysTheEveryAPICLIIsMissing(t *testing.T) {
	a, tt, updates := paletteLibrary(t, media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}})
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "这部影片讲了什么")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText(everyAPIMissing()) || !tt.HasText(locales.PaletteNoAIAction()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteSaysWhenNothingIsFound checks that a query that finds no line and no command says so, over the library, and in the player with the hint that Return asks the film.
func TestPaletteSaysWhenNothingIsFound(t *testing.T) {
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "火山")
	if !tt.HasText(locales.PaletteNothingFound()) {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	paletteType(tt, "火山")
	if !tt.HasText(locales.PaletteNothingFoundAsk()) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestPaletteInEnglish checks the palette in English: over the library its field, headings, footer and the note for a query that finds nothing; in the player its field and the card that asks for an AI service.
func TestPaletteInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, updates := paletteLibrary(t, &fakePipeline{})
	paletteOpen(t, a, tt, updates)
	if got := a.paletteView().Placeholder; got != "Commands or watched lines…" {
		t.Fatalf("library placeholder %q", got)
	}
	for _, want := range []string{"Commands", "Type to search commands and watched lines · ↑↓ to select · Return to run"} {
		wantText(t, tt, want)
	}
	paletteType(tt, "volcano")
	wantText(t, tt, "No commands or watched lines found.")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	paletteOpenFilm(t, a, tt, updates, "rivers")
	paletteOpen(t, a, tt, updates)
	if got := a.paletteView().Placeholder; got != "Commands, lines, or ask this film…" {
		t.Fatalf("player placeholder %q", got)
	}
	paletteType(tt, "what is this film about")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	for _, want := range []string{"Answer", "Answering questions needs an AI service. Choose a model in “AI Service” first.", "Open AI Service Settings"} {
		wantText(t, tt, want)
	}
	if tt.HasText("回答") || tt.HasText("命令") {
		t.Fatalf("the palette still reads Chinese: %q", tt.Texts())
	}
}
