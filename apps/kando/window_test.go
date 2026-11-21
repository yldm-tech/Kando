package main

import (
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// windowAlphaOf is how opaque a view's background is, from 0 (none, which shows what is behind it) to 1.
func windowAlphaOf(v *nsgotest.View) float64 {
	_, _, _, a := v.Style.Background.Components()
	return a
}

// windowRoot is the main view's root, the column that holds the title bar of Windows and Linux (package chrome) over the row of the sidebar and the content, and paints the window where it shows no material.
func windowRoot(t *testing.T, tt *nsgotest.App) *nsgotest.View {
	t.Helper()
	return tt.Find("id:main-window")
}

// windowBody is the row under the title bar that holds the sidebar and the content beside it.
func windowBody(t *testing.T, tt *nsgotest.App) *nsgotest.View {
	t.Helper()
	side := tt.Find("id:sidebar")
	if side.Parent == nil {
		t.Fatalf("the sidebar has no parent\n%s", tt.Dump())
	}
	return side.Parent
}

// windowChildOf is the child of root that holds v.
func windowChildOf(t *testing.T, root, v *nsgotest.View) *nsgotest.View {
	t.Helper()
	for c := v; c != nil; c = c.Parent {
		if c.Parent == root {
			return c
		}
	}
	t.Fatalf("view %d is not under view %d", v.ID, root.ID)
	return nil
}

// TestMaterialShowsThroughTheSidebarOnly checks that with the window's material on, as on Windows, the gutter around the sidebar's card shows it, the card lets it through, and the content beside it stays opaque, and that on Linux, without the material, the gutter is painted.
func TestMaterialShowsThroughTheSidebarOnly(t *testing.T) {
	usePlatform(t, ns.OSWindows)
	_, tt, _ := newTestApp(t, &fakePipeline{})
	env := testEnvironment()
	env.Dark = true
	env.TitleBar = ns.Edges{Top: 28, Left: 80}
	env.WindowMaterial = ns.WindowMaterialMica
	tt.SetEnvironment(env)
	root := windowRoot(t, tt)
	side := tt.Find("id:sidebar")
	content := windowChildOf(t, windowBody(t, tt), tt.Find(whole(testlocale.Messages().FilmsStart())))
	if a := windowAlphaOf(root); a != 0 {
		t.Fatalf("the window's root is painted at alpha %v over the material", a)
	}
	if a := windowAlphaOf(side); a != 0 {
		t.Fatalf("the sidebar's gutter is painted at alpha %v over the material", a)
	}
	card := sidebarCard(t, tt)
	if a := windowAlphaOf(card); a == 0 || a == 1 {
		t.Fatalf("card alpha = %v, want it translucent", a)
	}
	if card.Style.Glass != ns.GlassNone {
		t.Fatal("the card is glass over the window's material, want it to let the material through")
	}
	if a := windowAlphaOf(tt.Find("id:sidebar-wash")); a != 0 {
		t.Fatalf("the card is washed at alpha %v over the window's material, want it to let the material through", a)
	}
	if a := windowAlphaOf(content); a != 1 {
		t.Fatalf("content alpha = %v, want it opaque", a)
	}

	usePlatform(t, ns.OSLinux)
	_, plain, _ := newTestApp(t, &fakePipeline{})
	if a := windowAlphaOf(windowRoot(t, plain)); a != 1 {
		t.Fatalf("window alpha without the material = %v, want opaque", a)
	}
	if a := windowAlphaOf(plain.Find("id:sidebar")); a != 1 {
		t.Fatalf("gutter alpha without the material = %v, want opaque", a)
	}
}

// TestSidebarFloatsOverThePageOnMac checks that on macOS, whose window shows no material, the window is painted the page's ground, the sidebar's gutter leaves it showing, so the page's ground runs around the card, and the card stands as far in from the page as from the window's edges, as Finder's sidebar does on macOS 26, rather than a column under the page: glass in the light, a neutral gray a step lighter than the page in the dark.
func TestSidebarFloatsOverThePageOnMac(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	for _, dark := range []bool{false, true} {
		env := testEnvironment()
		env.Dark = dark
		tt.SetEnvironment(env)
		root, side, card := windowRoot(t, tt), tt.Find("id:sidebar"), sidebarCard(t, tt)
		if a := windowAlphaOf(root); a != 1 {
			t.Fatalf("dark %v: the window's root alpha = %v, want the page's ground opaque", dark, a)
		}
		if a := windowAlphaOf(side); a != 0 {
			t.Fatalf("dark %v: the sidebar's gutter is painted at alpha %v over the page's ground", dark, a)
		}
		if !dark {
			if card.Style.Glass == ns.GlassNone {
				t.Fatal("the sidebar's card is not glass in the light")
			}
			if a := windowAlphaOf(card); a == 0 || a == 1 {
				t.Fatalf("card tint alpha = %v in the light, want it translucent", a)
			}
			// The glass takes on the desktop's colors, which the card's own wash over it tones down to a near-white.
			if a := windowAlphaOf(tt.Find("id:sidebar-wash")); a < 0.5 || a == 1 {
				t.Fatalf("the wash over the glass is at alpha %v in the light, want most of the way to opaque", a)
			}
		} else {
			// In the dark the glass would show the desktop's colors as a gradient beside the page's gray, so the card is a neutral gray a step lighter than the page.
			if card.Style.Glass != ns.GlassNone {
				t.Fatal("the sidebar's card is glass in the dark, want the card's neutral gray")
			}
			if a := windowAlphaOf(tt.Find("id:sidebar-wash")); a != 0 {
				t.Fatalf("the card's gray is washed at alpha %v in the dark, want it as it is", a)
			}
			face, page := card.Style.Background, root.Style.Background
			if windowAlphaOf(card) != 1 || luminance(face) <= luminance(page) {
				t.Fatalf("card %#x over the page %#x in the dark, want it opaque and lighter", uint32(face), uint32(page))
			}
		}
		left := card.Frame.Origin.X - side.Frame.Origin.X
		right := side.Frame.Origin.X + side.Frame.Size.Width - (card.Frame.Origin.X + card.Frame.Size.Width)
		if left <= 0 || right != left {
			t.Fatalf("dark %v: the card stands %v in from the window's edge and %v from the page, want the same room on both", dark, left, right)
		}
	}
}

// sidebarCard is the card the sidebar draws its lists on, the sidebar's only child.
func sidebarCard(t *testing.T, tt *nsgotest.App) *nsgotest.View {
	t.Helper()
	side := tt.Find("id:sidebar")
	if len(side.Children) != 1 {
		t.Fatalf("the sidebar has %d children, want its card\n%s", len(side.Children), tt.Dump())
	}
	return side.Children[0]
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

// TestWindowFollowsTheSystemAppearance checks the palette: the window is light with Kando's colors while the system is, turns dark with the system, the sidebar, the library page and the settings window with it, and an open film is drawn in the dark theme either way.
func TestWindowFollowsTheSystemAppearance(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	content := func() *nsgotest.View {
		return windowChildOf(t, windowBody(t, tt), tt.Find(whole(testlocale.Messages().FilmsStart())))
	}
	status := func() ns.Color { return tt.Find("id:sidebar-status").TextStyle.Color }
	if bg := content().Style.Background; bg != ns.RGB(0xFFFFFF) {
		t.Fatalf("content = %#x in the light, want white", uint32(bg))
	}
	if c := status(); c != ns.RGB(0x3A3A3F) {
		t.Fatalf("sidebar status = %#x in the light, want #3A3A3F", uint32(c))
	}

	env := testEnvironment()
	env.Dark = true
	tt.SetEnvironment(env)
	if bg := content().Style.Background; !darkFace(bg) {
		t.Fatalf("content = %#x in the dark, want dark", uint32(bg))
	}
	if c := status(); !lightInk(c) {
		t.Fatalf("sidebar status = %#x in the dark, want light", uint32(c))
	}
	if face := tt.Find("id:sidebar-switcher").Style.Background; lightInk(face) {
		t.Fatalf("library switcher = %#x in the dark, want no white pill", uint32(face))
	}

	settingsOpenAt(a, tt, settings.SectionGeneral)
	if bg := tt.Find("id:settings-window").Style.Background; !darkFace(bg) {
		t.Fatalf("settings window = %#x in the dark, want dark", uint32(bg))
	}
	if c := tt.Find("id:settings-title").TextStyle.Color; !lightInk(c) {
		t.Fatalf("settings title = %#x in the dark, want light", uint32(c))
	}
	env.Dark = false
	tt.SetEnvironment(env)
	if bg := tt.Find("id:settings-window").Style.Background; bg != ns.RGB(0xF5F5F7) {
		t.Fatalf("settings window = %#x back in the light, want #F5F5F7", uint32(bg))
	}
	a.prefs.open = false
	env.Dark = true
	tt.SetEnvironment(env)

	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	a.openKey = "j"
	refresh(tt, a)
	back := tt.Find(testlocale.Messages().FilmBackToLibrary())
	// Under the back button is the film's own black, under its title bar. The button and the chips around it are frosted glass, a light tint over what is under them, so the first opaque background behind the button is the page's.
	under := ns.Color(0)
	for c := back.Parent; c != nil && under == 0; c = c.Parent {
		if _, _, _, alpha := c.Style.Background.Components(); alpha == 1 {
			under = c.Style.Background
		}
	}
	if !darkFace(under) {
		t.Fatalf("film page = %#x, want dark", uint32(under))
	}
}

// darkFace reports whether c is an opaque dark color, as the dark page's and window's are.
func darkFace(c ns.Color) bool {
	r, g, b, alpha := c.Components()
	return c != 0 && alpha == 1 && r <= 60.0/255 && g <= 60.0/255 && b <= 60.0/255
}

// lightInk reports whether c is a light color, as text on a dark page is.
func lightInk(c ns.Color) bool {
	r, g, b, _ := c.Components()
	return r >= 180.0/255 && g >= 180.0/255 && b >= 180.0/255
}

func TestTitleBarClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(tt, ns.Edges{Top: 28, Left: 70})
	// The library's name is the sidebar switcher's title; the library page's header names it too
	title := pageFrame(tt.Find("id:sidebar-switcher >> " + whole("videos")))
	if title.Origin.Y < 28 {
		t.Fatalf("title sits under the traffic lights at y=%v", title.Origin.Y)
	}

	// Without a title bar the sidebar keeps no room for the traffic lights.
	_, plain, _ := newTestApp(t, &fakePipeline{})
	if plainTitle := pageFrame(plain.Find("id:sidebar-switcher >> " + whole("videos"))); plainTitle.Origin.Y >= title.Origin.Y {
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
	if !e.Canceled() || !a.window.hidden || tt.Quits != 0 || !a.running() {
		t.Fatalf("close button: canceled %v, hidden %v, quits %d, job running %v", e.Canceled(), a.window.hidden, tt.Quits, a.running())
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
	if q.DefaultPrevented() || e.Canceled() || a.window.hidden {
		t.Fatalf("confirmed quit: quit prevented %v, close canceled %v, hidden %v", q.DefaultPrevented(), e.Canceled(), a.window.hidden)
	}
}

// TestClosingTheWindowElsewhereQuits checks that on Linux and Windows, with 关掉窗口后继续在后台运行 off, the main window's close button quits, through the quit that asks first while a job runs, and lets the window close once that quit goes ahead.
func TestClosingTheWindowElsewhereQuits(t *testing.T) {
	setWindowHidesOnClose(t, false)
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.tray.keepRunning = false
	e := &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if !e.Canceled() || a.window.hidden || tt.Quits != 1 {
		t.Fatalf("close button: canceled %v, hidden %v, quits %d", e.Canceled(), a.window.hidden, tt.Quits)
	}
	if s := a.windowKeys(a.cx)[0]; s.When() {
		t.Fatal("主窗口 is offered where closing the window quits")
	}
	a.areasBeforeQuit(&quitEvent{})
	e = &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if e.Canceled() || tt.Quits != 1 {
		t.Fatalf("the quit's own close: canceled %v, quits %d", e.Canceled(), tt.Quits)
	}
}

// TestWindowMenuBringsTheWindowBack checks that a row of the Window menu, here 资料库, shows the main window its close button put away before it acts on it, and that its put away and back are passed on to the player.
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

// The main window's text is in the design's font of the platform it runs on, in the interface's language as it changes, which a dialog drawn over the window takes too: the system's on macOS, Segoe UI Variable on Windows and Cantarell on Linux.
func TestWindowTextIsInThePlatformsFont(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("a target runs on its own system, which the environment does not change")
	}
	for _, os := range []ns.OS{ns.OSMacOS, ns.OSWindows, ns.OSLinux} {
		env := testEnvironment()
		env.OS, env.AppleKeyboard = os, os == ns.OSMacOS
		a, tt, _ := newTestApp(t, &fakePipeline{}, nsgotest.Environment(env))
		want := theme.FontFor(os, testlocale.Current())
		if got := tt.Find("id:sidebar-status").TextStyle.Font.Family; got != want {
			t.Errorf("%v: the sidebar's text in %q, want %q", os, got, want)
		}
		a.BeginNew()
		refresh(tt, a)
		if got := tt.Find("id:newlib-create >> text").TextStyle.Font.Family; got != want {
			t.Errorf("%v: the new-library dialog's text in %q, want %q", os, got, want)
		}
		// The font follows the interface's language through the root's binding: the sidebar is not built again, and its text takes the CJK family of Japanese.
		status := tt.Find("id:sidebar-status")
		a.cx.SetLocale("ja")
		refresh(tt, a)
		again := tt.Find("id:sidebar-status")
		if got, want := again.TextStyle.Font.Family, theme.FontFor(os, "ja"); got != want || again.ID != status.ID {
			t.Errorf("%v: in Japanese the sidebar's text in %q, want %q (built again: %v)", os, got, want, again.ID != status.ID)
		}
	}
}

// luminance is c's brightness, enough to say which of two grays is the lighter.
func luminance(c ns.Color) float64 {
	r, g, b, _ := c.Components()
	return 0.2126*r + 0.7152*g + 0.0722*b
}
