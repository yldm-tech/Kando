package main

import (
	"reflect"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// alphaOf is how opaque a view's background is, from 0 (none, which shows what is behind it) to 1.
func alphaOf(v *nsgotest.View) float64 {
	_, _, _, a := v.Style.Background.Components()
	return a
}

// windowRoot is the main view's root, the row that holds the sidebar and the content beside it.
func windowRoot(t *testing.T, tt *nsgotest.App) *nsgotest.View {
	t.Helper()
	side := tt.Find("id:sidebar")
	if side.Parent == nil {
		t.Fatalf("the sidebar has no parent\n%s", tt.Dump())
	}
	return side.Parent
}

// childOf is the child of root that holds v.
func childOf(t *testing.T, root, v *nsgotest.View) *nsgotest.View {
	t.Helper()
	for c := v; c != nil; c = c.Parent {
		if c.Parent == root {
			return c
		}
	}
	t.Fatalf("view %d is not under view %d", v.ID, root.ID)
	return nil
}

// backgroundUnder is the first background drawn under v: its own, or the nearest of its ancestors'.
func backgroundUnder(v *nsgotest.View) ns.Color {
	for c := v; c != nil; c = c.Parent {
		if c.Style.Background != 0 {
			return c.Style.Background
		}
	}
	return 0
}

// TestMaterialShowsThroughTheSidebarOnly checks that with the window's material on, the gutter around the sidebar's card shows it, the card lets it through, and the content beside it stays opaque, and that without the material the gutter is painted.
func TestMaterialShowsThroughTheSidebarOnly(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	env := testEnvironment()
	env.Dark = true
	env.TitleBar = ns.Edges{Top: 28, Left: 80}
	env.WindowMaterial = ns.WindowMaterialSidebar
	tt.SetEnvironment(env)
	root := windowRoot(t, tt)
	side := tt.Find("id:sidebar")
	content := childOf(t, root, tt.Find(whole(testlocale.Messages().FilmsStart())))
	if a := alphaOf(root); a != 0 {
		t.Fatalf("the window's root is painted at alpha %v over the material", a)
	}
	if a := alphaOf(side); a != 0 {
		t.Fatalf("the sidebar's gutter is painted at alpha %v over the material", a)
	}
	if len(side.Children) == 0 {
		t.Fatalf("the sidebar has no card\n%s", tt.Dump())
	}
	if a := alphaOf(side.Children[0]); a == 0 || a == 1 {
		t.Fatalf("card alpha = %v, want it translucent", a)
	}
	if a := alphaOf(content); a != 1 {
		t.Fatalf("content alpha = %v, want it opaque", a)
	}

	_, plain, _ := newTestApp(t, &fakePipeline{})
	if a := alphaOf(windowRoot(t, plain)); a != 1 {
		t.Fatalf("window alpha without the material = %v, want opaque", a)
	}
	if a := alphaOf(plain.Find("id:sidebar")); a != 1 {
		t.Fatalf("gutter alpha without the material = %v, want opaque", a)
	}
}

// TestPrimaryButtonUsesTheAccent checks that 开始处理, the library's primary button, is filled with Kando's blue, #0A66D8, which theme.Kando gives both appearances in place of the desktop's accent color.
func TestPrimaryButtonUsesTheAccent(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	env := testEnvironment()
	env.Dark = true
	tt.SetEnvironment(env)
	if theme.Accent != ns.RGB(0x0A66D8) {
		t.Fatalf("Kando's accent is %#x, want #0A66D8", uint32(theme.Accent))
	}
	label := tt.Find(whole(testlocale.Messages().FilmsStart()))
	if face := backgroundUnder(label); face != theme.Accent {
		t.Fatalf("button face = %#x, want Kando's accent %#x", uint32(face), uint32(theme.Accent))
	}
}

func TestSidebarArrowKeysChangeTheSource(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt.Press("id:source-" + sidebar.SourceAll)
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("page %q source %q after clicking the chosen item", a.page, a.side.filter)
	}
	// 浏览 sits between 全部影片 and YouTube.
	tt.Key("down")
	if a.page != pageBrowse {
		t.Fatalf("page %q after Down, want the browser", a.page)
	}
	tt.Key("down")
	if a.page != pageFilms || a.side.filter != sidebar.SourceYouTube {
		t.Fatalf("page %q source %q after Down, want %q", a.page, a.side.filter, sidebar.SourceYouTube)
	}
	tt.Key("up")
	tt.Key("up")
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("page %q source %q after Up, want %q", a.page, a.side.filter, sidebar.SourceAll)
	}
}

// TestSidebarSourceLeavesTheSettings checks that a source chosen while the settings are open, where no source is chosen, goes back to the library filtered by it.
func TestSidebarSourceLeavesTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	settingsOpenAt(a, tt, settings.SectionTranslate)
	tt.Press("id:source-" + sidebar.SourceLocal)
	if a.page != pageFilms || a.side.filter != sidebar.SourceLocal {
		t.Fatalf("page %q source %q after choosing 本地文件 in the settings", a.page, a.side.filter)
	}
}

// TestWindowIsLightAndTheFilmDark checks the palette: the window is light with Kando's colors whatever the desktop's appearance, and an open film is drawn in the dark theme.
func TestWindowIsLightAndTheFilmDark(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	env := testEnvironment()
	env.Dark = true
	tt.SetEnvironment(env)
	content := childOf(t, windowRoot(t, tt), tt.Find(whole(testlocale.Messages().FilmsStart())))
	if bg := content.Style.Background; bg != ns.RGB(0xFFFFFF) {
		t.Fatalf("content = %#x, want white", uint32(bg))
	}

	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	a.openKey = "j"
	refresh(tt, a)
	back := tt.Find(testlocale.Messages().FilmBackToLibrary())
	// Under the back button is the film's own black, under its title bar.
	if r, g, b, _ := backgroundUnder(back).Components(); r > 60.0/255 || g > 60.0/255 || b > 60.0/255 {
		t.Fatalf("film page = %#x, want dark", uint32(backgroundUnder(back)))
	}
}

func TestTitleBarClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(tt, ns.Edges{Top: 28, Left: 70})
	title := pageFrame(tt.Find(whole("videos")))
	if title.Origin.Y < 28 {
		t.Fatalf("title sits under the traffic lights at y=%v", title.Origin.Y)
	}

	// Without a title bar the sidebar keeps no room for the traffic lights.
	_, plain, _ := newTestApp(t, &fakePipeline{})
	if plainTitle := pageFrame(plain.Find(whole("videos"))); plainTitle.Origin.Y >= title.Origin.Y {
		t.Fatalf("title y=%v without a title bar, %v with one", plainTitle.Origin.Y, title.Origin.Y)
	}
}

// setWindowHidesOnClose sets windowHidesOnClose for the rest of t, to check each platform's close button.
func setWindowHidesOnClose(t *testing.T, hides bool) {
	t.Helper()
	old := windowHidesOnClose
	windowHidesOnClose = hides
	t.Cleanup(func() { windowHidesOnClose = old })
}

// closeCanceled reports whether the close button's c was canceled, which keeps the window open. NSGo's WindowClosing only records it, for the window that asked, so the test reads what it recorded.
func closeCanceled(c *ns.WindowClosing) bool {
	return reflect.ValueOf(c).Elem().FieldByName("canceled").Bool()
}

// TestClosingTheWindowOnMacKeepsKandoRunning checks that on macOS the main window's close button puts the window away without quitting, the job running on, that 窗口 › 主窗口 is offered only then and brings it back, and that a quit going ahead closes it.
func TestClosingTheWindowOnMacKeepsKandoRunning(t *testing.T) {
	setWindowHidesOnClose(t, true)
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	row := func() shortcut {
		t.Helper()
		for _, s := range a.shortcuts(a.cx) {
			if s.Group == groupWindow && s.Label == testlocale.Messages().WindowMenuMain() {
				return s
			}
		}
		t.Fatal("窗口 › 主窗口 is not in the shortcut table")
		return shortcut{}
	}
	if s := row(); s.Key != "" || s.When() {
		t.Fatalf("主窗口 has key %q, enabled %v while the window shows", s.Key, s.When())
	}

	e := &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if !closeCanceled(e) || !a.window.hidden || tt.Quits != 0 || !a.running() {
		t.Fatalf("close button: canceled %v, hidden %v, quits %d, job running %v", closeCanceled(e), a.window.hidden, tt.Quits, a.running())
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
	e = &ns.WindowClosing{}
	q := &quitEvent{}
	a.areasBeforeQuit(q)
	if !q.DefaultPrevented() || a.window.hidden {
		t.Fatalf("⌘Q during a job: prevented %v, window hidden %v", q.DefaultPrevented(), a.window.hidden)
	}
	a.queue.quitConfirmed = true
	q = &quitEvent{}
	a.areasBeforeQuit(q)
	a.windowClosing(e)
	if q.DefaultPrevented() || closeCanceled(e) || a.window.hidden {
		t.Fatalf("confirmed quit: quit prevented %v, close canceled %v, hidden %v", q.DefaultPrevented(), closeCanceled(e), a.window.hidden)
	}
}

// TestClosingTheWindowElsewhereQuits checks that on Linux and Windows the main window's close button quits, through the quit that asks first while a job runs, and lets the window close once that quit goes ahead.
func TestClosingTheWindowElsewhereQuits(t *testing.T) {
	setWindowHidesOnClose(t, false)
	a, tt, _ := newTestApp(t, &fakePipeline{})
	e := &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if !closeCanceled(e) || a.window.hidden || tt.Quits != 1 {
		t.Fatalf("close button: canceled %v, hidden %v, quits %d", closeCanceled(e), a.window.hidden, tt.Quits)
	}
	if s := a.windowKeys(a.cx)[0]; s.When() {
		t.Fatal("主窗口 is offered where closing the window quits")
	}
	a.areasBeforeQuit(&quitEvent{})
	e = &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if closeCanceled(e) || tt.Quits != 1 {
		t.Fatalf("the quit's own close: canceled %v, quits %d", closeCanceled(e), tt.Quits)
	}
}

// TestWindowMenuBringsTheWindowBack checks that a row of the Window menu, here 片库, shows the main window its close button put away before it acts on it, and that its put away and back are passed on to the player.
func TestWindowMenuBringsTheWindowBack(t *testing.T) {
	setWindowHidesOnClose(t, true)
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.windowClosing(&ns.WindowClosing{})
	refresh(tt, a)
	if !a.window.hidden || !a.player.windowHidden {
		t.Fatalf("close button: hidden %v, player told %v", a.window.hidden, a.player.windowHidden)
	}
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupWindow), testlocale.Messages().MenuWindowLibrary()))
	if a.window.hidden || a.player.windowHidden {
		t.Fatalf("片库 left the window hidden %v, player told %v", a.window.hidden, a.player.windowHidden)
	}
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("片库 showed page %q open %q", a.page, a.openKey)
	}
}
