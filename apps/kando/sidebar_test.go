package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// TestSidebarKeepsContentBelowTheTitleBar checks the drag band over the content by the rule contentBand follows: a page that draws its own header bar at its top (pageOwnsTitleBar) owns the room under the title bar and gets no band over it, so nothing on it moves down as far as a band would push it; any other page gets the band, which moves its top down by exactly the band's height and nothing on it further (what is centered in the room left moves half as far): the title bar's height, and on macOS no more than the sidebar card's top row, though the real window's title bar is 64 points tall with the traffic lights moved into that row. It reads only where the content's texts sit, not what the pages show, so it holds for whichever page draws a header bar: the library page beside the sidebar, and an open film that hides the sidebar. The sidebar clears the traffic lights in its own card (TestSidebarCardClearsTheTrafficLights).
func TestSidebarKeepsContentBelowTheTitleBar(t *testing.T) {
	// 64 and 84 are what mygo's TitleBar reports for the real macOS window, whose 14-point traffic lights sit at {24, 25}.
	tall := float32(64)
	if runtime.GOOS == "darwin" {
		tall = sidebar.LightsRow
	}
	open := func(a *app) {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.openKey = "j"
	}
	for _, scene := range []struct {
		name  string
		setup func(*app)
	}{
		{"the library page", nil},
		{"an open film", open},
	} {
		for _, c := range []struct {
			bar  ui.TitleBar
			band float32
		}{
			{ui.TitleBar{Height: 28, Left: 80}, 28},
			{ui.TitleBar{Height: 64, Left: 84}, tall},
		} {
			owns, top, shifts := contentShifts(t, c.bar, scene.setup)
			if !owns && abs32(shifts[top]-c.band) > 0.5 {
				t.Fatalf("%s draws no header bar of its own, yet its top text %q moved %v under a %v-high title bar, want the drag band's %v", scene.name, top, shifts[top], c.bar.Height, c.band)
			}
			for text, d := range shifts {
				if owns && d >= c.band-0.5 {
					t.Fatalf("%s draws its own header bar, yet %q moved %v under a %v-high title bar, as far as a %v-high drag band over it would push it", scene.name, text, d, c.bar.Height, c.band)
				}
				if !owns && d > c.band+0.5 {
					t.Fatalf("%s draws no header bar of its own, yet %q moved %v under a %v-high title bar, further than the drag band's %v", scene.name, text, d, c.bar.Height, c.band)
				}
			}
		}
	}
}

// contentShifts renders the window without a title bar and with bar, after setup, and returns whether the page in the content column draws its own header bar (pageOwnsTitleBar), the column's top text without a title bar, and how far down bar moved each text in that column, right of the sidebar while it shows.
func contentShifts(t *testing.T, bar ui.TitleBar, setup func(*app)) (owns bool, top string, shifts map[string]float32) {
	t.Helper()
	plain, plainTT, _ := newTestApp(&fakePipeline{})
	barred, barredTT, _ := newTestApp(&fakePipeline{})
	barredTT.SetTitleBar(bar)
	if setup != nil {
		setup(plain)
		setup(barred)
	}
	plainTT.Frame()
	barredTT.Frame()
	left := float32(0)
	if plain.sidebarShown() {
		left = sidebar.Width
	}
	shifts = map[string]float32{}
	topY := float32(0)
	for _, text := range plainTT.Texts() {
		before, _ := plainTT.Find(text)
		if before.X < left {
			continue
		}
		if top == "" || before.Y < topY {
			top, topY = text, before.Y
		}
		after, ok := barredTT.Find(text)
		if !ok {
			t.Fatalf("%q shown without a title bar is missing under one: %q", text, barredTT.Texts())
		}
		shifts[text] = after.Y - before.Y
	}
	if len(shifts) == 0 {
		t.Fatalf("no text right of x=%v in %q", left, plainTT.Texts())
	}
	return barred.pageOwnsTitleBar(), top, shifts
}

// sidebarCount is the count the sidebar shows after label, the text that follows it.
func sidebarCount(t *testing.T, tt *ui.Tester, label string) string {
	t.Helper()
	texts := tt.Texts()
	at := slices.Index(texts, label)
	if at < 0 || at+1 >= len(texts) {
		t.Fatalf("no %q row in %q", label, texts)
	}
	return texts[at+1]
}

// chooseFromSwitcher opens the library switcher's menu and chooses item, or the library item whose name is item and whose count follows it.
func chooseFromSwitcher(t *testing.T, tt *ui.Tester, item string) {
	t.Helper()
	tt.Frame()
	pill := ""
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "切换资料库，当前：") {
			pill = text
		}
	}
	if pill == "" {
		t.Fatalf("no library switcher in %q", tt.Texts())
	}
	if err := tt.Click(pill); err != nil {
		t.Fatal(err)
	}
	label := ""
	for _, l := range tt.Menu() {
		if l == item || strings.HasPrefix(l, item+" · ") {
			label = l
		}
	}
	if label == "" {
		t.Fatalf("no %q in the switcher's menu %q", item, tt.Menu())
	}
	if err := tt.ChooseMenuItem(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// applyUntil runs the background updates, as the UI thread would, until done reports true.
func applyUntil(t *testing.T, tt *ui.Tester, updates chan func(), done func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatal("timed out waiting for a background update")
		}
	}
	tt.Frame()
}

func TestSidebarListsTheSourcesWithCounts(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "yt", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: "/videos/yt-clip/video.mp4"},
		{ID: "bili", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Stage: core.StageComplete, VideoPath: "/videos/bili-clip/video.mp4"},
		{ID: "file", Source: "file:///videos/file-clip/video.mp4", Stage: core.StageComplete, VideoPath: "/videos/file-clip/video.mp4"},
	}
	a.edit(func(s *session) {
		s.Videos = []media.Video{{Name: "loose-clip.mp4", Path: "/videos/loose/loose-clip.mp4"}}
	})
	tt.Frame()
	want := []struct{ label, count string }{{"全部影片", "4"}, {"YouTube", "1"}, {"哔哩哔哩", "1"}, {"本地文件", "2"}}
	last := -1
	for _, row := range want {
		if got := sidebarCount(t, tt, row.label); got != row.count {
			t.Errorf("%s shows %q, want %q", row.label, got, row.count)
		}
		at := slices.Index(tt.Texts(), row.label)
		if at < last {
			t.Errorf("%s is out of order in %q", row.label, tt.Texts())
		}
		last = at
	}
	// 其他网站 shows only while the library holds a film from another site, or while it is the source shown.
	if tt.HasText("其他网站") {
		t.Fatalf("其他网站 shown without a film from another site: %q", tt.Texts())
	}
	a.side.filter = sidebar.SourceWeb
	tt.Frame()
	if got := sidebarCount(t, tt, "其他网站"); got != "0" {
		t.Fatalf("其他网站 shows %q while chosen", got)
	}
	a.side.filter = sidebar.SourceAll
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "web", Source: "https://vimeo.com/76979871", Stage: core.StageComplete, VideoPath: "/videos/web-clip/video.mp4"})
	tt.Frame()
	if got := sidebarCount(t, tt, "其他网站"); got != "1" {
		t.Fatalf("其他网站 shows %q", got)
	}
	texts := tt.Texts()
	if web := slices.Index(texts, "其他网站"); web < slices.Index(texts, "哔哩哔哩") || web > slices.Index(texts, "本地文件") {
		t.Fatalf("其他网站 is out of order in %q", texts)
	}
	// No settings rows: the settings open from the gear. The sidebar's texts come before the status line's.
	side := texts[:slices.Index(texts, "状态")]
	for _, label := range []string{"通用", "语音识别", "AI 服务", "翻译", "账号"} {
		if slices.Contains(side, label) {
			t.Fatalf("settings row %q still in the sidebar: %q", label, side)
		}
	}
}

// sidebarCountInk returns the ink of the count the sidebar shows after label, the pixel of its frame in tt's last frame farthest from the frame's first one, the row behind it, at the frame's scale.
func sidebarCountInk(t *testing.T, tt *ui.Tester, label string) ui.Color {
	t.Helper()
	row, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q row in %q", label, tt.Texts())
	}
	r, ok := tt.Find(sidebarCount(t, tt, label))
	if !ok || r.X < row.X+row.W || r.Y+r.H/2-(row.Y+row.H/2) > 2 || row.Y+row.H/2-(r.Y+r.H/2) > 2 {
		t.Fatalf("the count of %q is not on its row: %v, the label at %v", label, r, row)
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	at := func(x, y int) ui.Color { px := img.RGBAAt(x, y); return ui.RGB(px.R, px.G, px.B) }
	bg := at(int(r.X*scale), int(r.Y*scale))
	ink, far := bg, 0
	for y := int(r.Y * scale); y < int((r.Y+r.H)*scale); y++ {
		for x := int(r.X * scale); x < int((r.X+r.W)*scale); x++ {
			if d := colorDistance(at(x, y), bg); d > far {
				ink, far = at(x, y), d
			}
		}
	}
	return ink
}

// colorDistance is the largest difference between a channel of a and the same channel of b.
func colorDistance(a, b ui.Color) int {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return max(d(a.R, b.R), d(a.G, b.G), d(a.B, b.B))
}

// TestSidebarCountFollowsTheSelection checks that a source's count stays readable on its chosen row. On macOS it is in the secondary text color without the focus, as the other rows' counts are, and once a click gives the sidebar the focus and the row is filled with the accent, near white as its label is (breeze.SidebarItemDetail, as AppKit draws a cell's secondary text there). On Windows and GNOME the focus leaves it as it was; there it is not compared with the other rows' counts, as GNOME dims it by opacity over the chosen row's darker fill. The count of a row not chosen stays as it was.
func TestSidebarCountFollowsTheSelection(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.SetScale(2)
	a.library.Jobs = []core.Job{
		{ID: "yt", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: "/videos/yt-clip/video.mp4"},
		{ID: "file", Source: "file:///videos/file-clip/video.mp4", Stage: core.StageComplete, VideoPath: "/videos/file-clip/video.mp4"},
	}
	tt.Frame()
	if got := sidebarCount(t, tt, "全部影片"); got != "2" {
		t.Fatalf("全部影片 shows %q, want 2", got)
	}
	mac := breeze.Current() == breeze.Mac
	secondary := sidebarCountInk(t, tt, "YouTube")
	unfocused := sidebarCountInk(t, tt, "全部影片")
	if mac && colorDistance(unfocused, secondary) > 8 {
		t.Fatalf("the chosen row's count without the focus is %v, the others' %v", unfocused, secondary)
	}
	if err := tt.Click("全部影片"); err != nil {
		t.Fatal(err)
	}
	tt.Move(1000, 700)
	tt.Frame()
	got := sidebarCountInk(t, tt, "全部影片")
	if mac {
		if white := ui.Hex("#FFFFFF"); colorDistance(got, white) > 32 {
			t.Fatalf("the chosen row's count with the focus is %v, want near %v", got, white)
		}
	} else if colorDistance(got, unfocused) > 8 {
		t.Fatalf("the chosen row's count with the focus is %v, want %v as without it", got, unfocused)
	}
	if other := sidebarCountInk(t, tt, "YouTube"); colorDistance(other, secondary) > 8 {
		t.Fatalf("a count of a row not chosen turned %v with the focus, want %v", other, secondary)
	}
}

// TestSidebarCardClearsTheTrafficLights checks that the card keeps an empty row for the traffic lights under a hidden title bar, and none without one.
func TestSidebarCardClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.Frame()
	caption, ok := tt.Find("资料库")
	if !ok {
		t.Fatalf("missing the switcher in %q", tt.Texts())
	}
	// The card is 8 points in from the window's top and its row for the traffic lights is 48 high.
	if caption.Y < 56 {
		t.Fatalf("switcher at y=%v, inside the traffic lights' row", caption.Y)
	}
	_, plain, _ := newTestApp(&fakePipeline{})
	plain.Frame()
	if r, _ := plain.Find("资料库"); r.Y >= 28 {
		t.Fatalf("switcher at y=%v without a title bar", r.Y)
	}
}

func TestSwitcherMenuListsTheLibrariesWithCounts(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "甲.kando")
	second := filepath.Join(parent, "乙.kando")
	for _, path := range []string{filepath.Join(first, "a", "video.mp4"), filepath.Join(second, "b", "video.mp4"), filepath.Join(second, "talk.mp4")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var revealed string
	prev := revealInFolder
	revealInFolder = func(path string) error {
		revealed = path
		return nil
	}
	t.Cleanup(func() { revealInFolder = prev })

	a, tt, updates := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	a.refreshLibrary()
	a.countLibraries()
	applyUntil(t, tt, updates, func() bool { _, ok := a.LibraryCount(second); return ok })
	if !tt.HasText("甲") || !tt.HasText("切换资料库，当前：甲") {
		t.Fatalf("switcher does not name the open library: %q", tt.Texts())
	}
	if err := tt.Click("切换资料库，当前：甲"); err != nil {
		t.Fatal(err)
	}
	want := []string{"甲 · 1", "乙 · 2", "-", "新建资料库…", "打开已有资料库…", "在访达中显示「甲」"}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("在访达中显示「甲」"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if revealed != first {
		t.Fatalf("revealed %q, want %q", revealed, first)
	}

	// Switching shows all of the new library's films, and the library left gets its count.
	a.SetSourceFilter(sidebar.SourceLocal)
	chooseFromSwitcher(t, tt, "乙")
	if a.snap().LibraryDir != second || a.side.filter != sidebar.SourceAll {
		t.Fatalf("library %q source %q after switching", a.snap().LibraryDir, a.side.filter)
	}
	applyUntil(t, tt, updates, func() bool { _, ok := a.LibraryCount(first); return ok })
	if err := tt.Click("切换资料库，当前：乙"); err != nil {
		t.Fatal(err)
	}
	want = []string{"甲 · 1", "乙 · 2", "-", "新建资料库…", "打开已有资料库…", "在访达中显示「乙」"}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	tt.CloseMenu()
}

// TestLibraryCountsKeepTheLatestRun checks that counts read before a library switch, arriving after the ones read after it, do not replace them and leave the library just left without a count.
func TestLibraryCountsKeepTheLatestRun(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "甲.kando")
	second := filepath.Join(parent, "乙.kando")
	for _, path := range []string{filepath.Join(first, "a.mp4"), filepath.Join(second, "b.mp4"), filepath.Join(second, "c.mp4")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, _, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	// The startup count, of 乙, is still running when the user switches to 乙 and 甲 is counted.
	a.countLibraries()
	var startup func()
	select {
	case startup = <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the startup count")
	}
	a.edit(func(s *session) { s.LibraryDir = second })
	a.countLibraries()
	waitUpdate(t, updates)
	startup()
	if n, ok := a.LibraryCount(first); !ok || n != 1 {
		t.Fatalf("count of the library just left = %d, %v; want 1", n, ok)
	}
}

func TestStatusLineSaysWhatIsMissing(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli"}
	withModel := found
	withModel.WhisperModel = model
	noCLI := withModel
	noCLI.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
	keyed := withModel
	keyed.Translator = media.ChatTranslator{BaseURL: "https://api.example.com/v1", Model: "gpt", APIKey: "k"}
	gone := found
	gone.WhisperModel = filepath.Join(t.TempDir(), "gone.bin")
	cases := []struct {
		name     string
		pipeline media.Pipeline
		want     string
		section  string
	}{
		{"no tools", media.Tools{FFmpeg: "/bin/ffmpeg"}, "缺少组件：yt-dlp、whisper-cli", settings.SectionGeneral},
		{"no model", found, noSpeechModel(), settings.SectionSpeech},
		{"model gone", gone, noSpeechModel(), settings.SectionSpeech},
		{"no translation model", withModel, noLLMModel(), settings.SectionAI},
		{"no everyapi program", noCLI, everyAPIMissing(), settings.SectionAI},
		{"ready", keyed, readyMessage(), settings.SectionGeneral},
		{"another pipeline", &fakePipeline{}, readyMessage(), settings.SectionGeneral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, tt, _ := newTestApp(c.pipeline)
			tt.Frame()
			if !tt.HasText(checkingTools()) {
				t.Fatalf("before the first check: %q", tt.Texts())
			}
			a.checkReadiness(true)
			tt.Frame()
			if !tt.HasText(c.want) {
				t.Fatalf("missing %q in %q", c.want, tt.Texts())
			}
			if err := tt.Click("设置"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			wantSettings(t, a, c.section)
		})
	}

	t.Run("no library", func(t *testing.T) {
		a, tt, _ := newTestApp(keyed)
		a.edit(func(s *session) { s.LibraryDir = "" })
		a.checkReadiness(true)
		tt.Frame()
		if !tt.HasText(noLibrary()) {
			t.Fatalf("missing %q in %q", noLibrary(), tt.Texts())
		}
	})
}

// TestStatusLineWaitsForTheEveryAPISignIn checks that the sign-in is asked off the UI thread, that its answer is kept until 重新检测, and that an answer arriving after the AI service changed does not hide the new state.
func TestStatusLineWaitsForTheEveryAPISignIn(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := make(chan error)
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, <-answers
	}}
	a, tt, updates := newTestApp(tools)
	check := func(signIn bool) {
		a.checkReadiness(signIn)
		tt.Frame()
	}
	check(true)
	if !tt.HasText(checkingSignIn()) {
		t.Fatalf("missing %q in %q", checkingSignIn(), tt.Texts())
	}
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(everyAPISignedOut()) {
		t.Fatalf("missing %q in %q", everyAPISignedOut(), tt.Texts())
	}
	// Choosing the speech model again keeps the answer instead of asking the CLI again.
	check(false)
	if !tt.HasText(everyAPISignedOut()) || len(updates) != 0 {
		t.Fatalf("asked again: texts %q, %d updates", tt.Texts(), len(updates))
	}
	// 重新检测 asks again.
	check(true)
	if !tt.HasText(checkingSignIn()) {
		t.Fatalf("missing %q in %q", checkingSignIn(), tt.Texts())
	}
	// While it waits, an API key replaces the sign-in; the late answer leaves the key's state.
	keyed := tools
	keyed.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, APIKey: "k"}
	a.pipeline = keyed
	check(false)
	if !tt.HasText(readyMessage()) {
		t.Fatalf("missing %q in %q", readyMessage(), tt.Texts())
	}
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(readyMessage()) {
		t.Fatalf("late sign-in answer replaced the state: %q", tt.Texts())
	}

	// Of two sign-in checks running at once, the later one's answer stands, whichever comes last.
	later := make(chan error)
	again := tools
	again.Translator.Credential = func(context.Context, bool) (media.Endpoint, error) { return media.Endpoint{}, <-later }
	a.pipeline = tools
	check(true)
	a.pipeline = again
	check(true)
	later <- nil
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(readyMessage()) {
		t.Fatalf("missing %q in %q", readyMessage(), tt.Texts())
	}
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(readyMessage()) {
		t.Fatalf("the earlier check's answer replaced the later one's: %q", tt.Texts())
	}
}

// TestStatusLineAsksForTheSignInOnce checks that the changes that do not ask again, as each keystroke in 「AI 服务」, wait for the sign-in check already running instead of starting another everyapi process each.
func TestStatusLineAsksForTheSignInOnce(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	answers := make(chan error)
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Credential: func(context.Context, bool) (media.Endpoint, error) {
		return media.Endpoint{}, <-answers
	}}
	a, tt, updates := newTestApp(tools)
	a.checkReadiness(true)
	asked := a.side.ready.gen
	for range 3 {
		a.checkReadiness(false)
		tt.Frame()
		if !tt.HasText(checkingSignIn()) {
			t.Fatalf("missing %q in %q", checkingSignIn(), tt.Texts())
		}
	}
	if a.side.ready.gen != asked {
		t.Fatalf("started %d more sign-in checks while one was running", a.side.ready.gen-asked)
	}
	answers <- media.ErrEveryAPISignedOut
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(everyAPISignedOut()) {
		t.Fatalf("the running check's answer was dropped: %q", tt.Texts())
	}
}

// TestLLMTestAnswersTheSignIn checks that 测试 in 「AI 服务」 succeeding through the EveryAPI CLI's key replaces a kept "not signed in", which only 重新检测 on another page would otherwise ask again.
func TestLLMTestAnswersTheSignIn(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer srv.Close()
	signedIn := false
	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model}
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Client: srv.Client(), Credential: func(context.Context, bool) (media.Endpoint, error) {
		if !signedIn {
			return media.Endpoint{}, media.ErrEveryAPISignedOut
		}
		return media.Endpoint{BaseURL: srv.URL + "/v1", APIKey: "relay"}, nil
	}}
	a, tt, updates := newTestApp(tools)
	a.checkReadiness(true)
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText(everyAPISignedOut()) {
		t.Fatalf("missing %q in %q", everyAPISignedOut(), tt.Texts())
	}

	// The user signs in to the CLI and presses 测试.
	signedIn = true
	a.testLLM()
	waitUpdate(t, updates)
	tt.Frame()
	if a.LLMResult() != "连接成功" || !tt.HasText(readyMessage()) {
		t.Fatalf("test result %q, texts %q", a.LLMResult(), tt.Texts())
	}
}

func TestStatusDotIsGreenWhenReady(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.checkReadiness(true)
	tt.Frame()
	r, ok := tt.Find(readyMessage())
	if !ok {
		t.Fatalf("missing %q in %q", readyMessage(), tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The 8-point dot ends 10 points before the message, centered on the row.
	px := img.RGBAAt(int((r.X-14)*scale), int((r.Y+r.H/2)*scale))
	want := ui.Hex("#30D158")
	for _, ch := range [][2]uint8{{px.R, want.R}, {px.G, want.G}, {px.B, want.B}} {
		if d := int(ch[0]) - int(ch[1]); d < -8 || d > 8 {
			t.Fatalf("dot = %v, want about %v", px, want)
		}
	}
}

// TestSidebarInEnglish checks the sidebar in English: its sources, the switcher and its menu, and a status line that follows the language as it changes, though it was found before the change.
func TestSidebarInEnglish(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.checkReadiness(true)
	tt.Frame()
	wantText(t, tt, "已就绪，可以开始处理")

	testlocale.Use(t, "en")
	tt.Frame()
	for _, want := range []string{"All Films", "YouTube", "Bilibili", "Local Files", "Library", "Ready to process"} {
		wantText(t, tt, want)
	}
	// The library page beside the sidebar is another area's, so only the sidebar's own words are checked.
	for _, chinese := range []string{"本地文件", "状态", "已就绪，可以开始处理"} {
		wantNoText(t, tt, chinese)
	}
	name := sidebar.Title(a.snap().LibraryDir)
	if err := tt.Click("Switch library, current: " + name); err != nil {
		t.Fatalf("switcher: %v; texts %q", err, tt.Texts())
	}
	menu := tt.Menu()
	for _, want := range []string{"New Library…", "Open Existing Library…", "Show “" + name + "” in Finder"} {
		if !slices.Contains(menu, want) {
			t.Fatalf("menu %q lacks %q", menu, want)
		}
	}
	tt.CloseMenu()

	b, btt, _ := newTestApp(media.Tools{FFmpeg: "/bin/ffmpeg"})
	b.checkReadiness(true)
	btt.Frame()
	wantText(t, btt, "Missing components: yt-dlp, whisper-cli")
}
