package main

import (
	"image"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"
	"kando/packages/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

func TestVibrancyShowsThroughTheSidebarOnly(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.SetVibrancy(true)
	tt.Frame()
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The gutter left of the sidebar's card shows the material, the card below the last source lets it through, and the content below the shelf is opaque.
	// Only the card's soft shadow, a few percent, tints the gutter.
	if a := alphaAt(img, scale, 3, 400); a > 16 {
		t.Fatalf("gutter alpha = %d, want the window's material to show", a)
	}
	if a := alphaAt(img, scale, 120, 400); a == 0 || a == 255 {
		t.Fatalf("card alpha = %d, want it translucent", a)
	}
	if a := alphaAt(img, scale, 700, 680); a != 255 {
		t.Fatalf("content alpha = %d, want it opaque", a)
	}

	_, plain, _ := newTestApp(&fakePipeline{})
	plain.SetDark(true)
	plain.Frame()
	if a := alphaAt(plain.Image(), scale, 3, 400); a != 255 {
		t.Fatalf("gutter alpha without vibrancy = %d, want opaque", a)
	}
}

func alphaAt(img *image.RGBA, scale, x, y float32) uint8 {
	return img.RGBAAt(int(x*scale), int(y*scale)).A
}

func TestPrimaryButtonUsesTheAccent(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.Frame()
	r, ok := tt.Find("开始处理")
	if !ok {
		t.Fatalf("missing start button in %q", tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The label is padded inside the button; a point just before it is the button's face.
	px := img.RGBAAt(int((r.X-4)*scale), int((r.Y+r.H/2)*scale))
	// The face is Kando's blue as the platform fills an accent button, which theme.Palette gives the window: #0A66D8 itself on macOS, and on Windows the shade of it that WinUI's AccentFillColorDefault takes.
	var light *ui.Theme
	probe := ui.NewTester(func(c *ui.Context) { light, _ = theme.Palette(c) }, 10, 10)
	probe.Frame()
	want := light.Accent
	if breeze.Current() == breeze.Mac && want != ui.Hex("#0A66D8") {
		t.Fatalf("accent on macOS = %v, want Kando's #0A66D8", want)
	}
	for _, ch := range [][2]uint8{{px.R, want.R}, {px.G, want.G}, {px.B, want.B}} {
		if d := int(ch[0]) - int(ch[1]); d < -3 || d > 3 {
			t.Fatalf("button face = %v, want about %v", px, want)
		}
	}
}

func TestSidebarArrowKeysChangeTheSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	if err := tt.Click("全部影片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("page %q source %q after clicking the chosen item", a.page, a.side.filter)
	}
	// 浏览 sits between 全部影片 and YouTube.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.page != pageBrowse {
		t.Fatalf("page %q after Down, want the browser", a.page)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceYouTube {
		t.Fatalf("page %q source %q after Down, want %q", a.page, a.side.filter, sidebar.SourceYouTube)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("page %q source %q after Up, want %q", a.page, a.side.filter, sidebar.SourceAll)
	}
}

// TestSidebarSourceLeavesTheSettings checks that a source chosen while a settings page shows, where no source is chosen, goes back to the library filtered by it.
func TestSidebarSourceLeavesTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.OpenSettings(settings.SectionTranslate)
	tt.Frame()
	if err := tt.Click("本地文件"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceLocal {
		t.Fatalf("page %q source %q after choosing 本地文件 in the settings", a.page, a.side.filter)
	}
}

// TestWindowIsLightAndTheFilmDark checks the palette: the window is light with Kando's colors whatever the desktop's appearance, and an open film is drawn in the dark theme.
func TestWindowIsLightAndTheFilmDark(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.Frame()
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	if px := img.RGBAAt(int(700*scale), int(680*scale)); px.R != 255 || px.G != 255 || px.B != 255 {
		t.Fatalf("content = %v, want white", px)
	}

	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	a.openKey = "j"
	tt.Frame()
	r, ok := tt.Find("返回资料库")
	if !ok {
		t.Fatalf("film not open: %q", tt.Texts())
	}
	img = tt.Image()
	// Left of the back button is the film's own black, under its title bar.
	px := img.RGBAAt(int((r.X-6)*scale), int((r.Y+r.H/2)*scale))
	if px.R > 60 || px.G > 60 || px.B > 60 {
		t.Fatalf("film page = %v, want dark", px)
	}
}

func TestTitleBarClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 70})
	title, ok := tt.Find("videos")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	if title.Y < 28 {
		t.Fatalf("title sits under the traffic lights at y=%v", title.Y)
	}

	// Without a title bar the sidebar keeps no room for the traffic lights.
	_, plain, _ := newTestApp(&fakePipeline{})
	plain.Frame()
	plainTitle, ok := plain.Find("videos")
	if !ok || plainTitle.Y >= title.Y {
		t.Fatalf("title y=%v without a title bar, %v with one", plainTitle.Y, title.Y)
	}
}

// setWindowHidesOnClose sets windowHidesOnClose for the rest of t, to check each platform's close button.
func setWindowHidesOnClose(t *testing.T, hides bool) {
	t.Helper()
	old := windowHidesOnClose
	windowHidesOnClose = hides
	t.Cleanup(func() { windowHidesOnClose = old })
}

// TestClosingTheWindowOnMacKeepsKandoRunning checks that on macOS the main window's close button puts the window away without quitting, the job running on, that 窗口 › 主窗口 is offered only then and brings it back, and that a quit going ahead closes it.
func TestClosingTheWindowOnMacKeepsKandoRunning(t *testing.T) {
	setWindowHidesOnClose(t, true)
	quits := replaceQuitApp(t)
	g := newGatePipeline()
	a, _, updates := newTestApp(g)
	t.Cleanup(a.stop)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	row := func() shortcut {
		t.Helper()
		for _, s := range a.shortcuts() {
			if s.Group == groupWindow && s.Label == locales.WindowMenuMain() {
				return s
			}
		}
		t.Fatal("窗口 › 主窗口 is not in the shortcut table")
		return shortcut{}
	}
	if s := row(); s.Key != 0 || s.When() {
		t.Fatalf("主窗口 has key %v, enabled %v while the window shows", s.Key, s.When())
	}

	e := &mygo.CloseEvent{}
	a.windowClosing(e)
	if !e.DefaultPrevented() || !a.window.hidden || *quits != 0 || !a.running() {
		t.Fatalf("close button: prevented %v, hidden %v, quits %d, job running %v", e.DefaultPrevented(), a.window.hidden, *quits, a.running())
	}
	if !row().When() {
		t.Fatal("主窗口 is disabled while the window is put away")
	}
	row().Run()
	if a.window.hidden || row().When() {
		t.Fatalf("主窗口 left the window hidden %v", a.window.hidden)
	}

	// ⌘Q asks first; once agreed, the quit closes the window rather than putting it away.
	a.window.hidden = true
	e = &mygo.CloseEvent{}
	q := &mygo.QuitEvent{}
	a.areasBeforeQuit(q)
	if !q.DefaultPrevented() || a.window.hidden {
		t.Fatalf("⌘Q during a job: prevented %v, window hidden %v", q.DefaultPrevented(), a.window.hidden)
	}
	a.queue.quitConfirmed = true
	q = &mygo.QuitEvent{}
	a.areasBeforeQuit(q)
	a.windowClosing(e)
	if q.DefaultPrevented() || e.DefaultPrevented() || a.window.hidden {
		t.Fatalf("confirmed quit: quit prevented %v, close prevented %v, hidden %v", q.DefaultPrevented(), e.DefaultPrevented(), a.window.hidden)
	}
}

// TestClosingTheWindowElsewhereQuits checks that on Linux and Windows the main window's close button quits, through the quit that asks first while a job runs, and lets the window close once that quit goes ahead.
func TestClosingTheWindowElsewhereQuits(t *testing.T) {
	setWindowHidesOnClose(t, false)
	quits := replaceQuitApp(t)
	a, _, _ := newTestApp(&fakePipeline{})
	e := &mygo.CloseEvent{}
	a.windowClosing(e)
	if !e.DefaultPrevented() || a.window.hidden || *quits != 1 {
		t.Fatalf("close button: prevented %v, hidden %v, quits %d", e.DefaultPrevented(), a.window.hidden, *quits)
	}
	if s := a.windowKeys()[0]; s.When() {
		t.Fatal("主窗口 is offered where closing the window quits")
	}
	a.areasBeforeQuit(&mygo.QuitEvent{})
	e = &mygo.CloseEvent{}
	a.windowClosing(e)
	if e.DefaultPrevented() || *quits != 1 {
		t.Fatalf("the quit's own close: prevented %v, quits %d", e.DefaultPrevented(), *quits)
	}
}

// TestWindowMenuBringsTheWindowBack checks that a row of the Window menu, here 片库, shows the main window its close button put away before it acts on it, and that its put away and back are passed on to the player.
func TestWindowMenuBringsTheWindowBack(t *testing.T) {
	setWindowHidesOnClose(t, true)
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	a.windowClosing(&mygo.CloseEvent{})
	if !a.window.hidden || !a.player.windowHidden {
		t.Fatalf("close button: hidden %v, player told %v", a.window.hidden, a.player.windowHidden)
	}
	findMenuItem(t, a.menuBar(), "窗口", locales.MenuWindowLibrary()).Click(nil, nil)
	(<-updates)()
	if a.window.hidden || a.player.windowHidden {
		t.Fatalf("片库 left the window hidden %v, player told %v", a.window.hidden, a.player.windowHidden)
	}
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("片库 showed page %q open %q", a.page, a.openKey)
	}
}
