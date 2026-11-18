package main

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/chrome"
	"kando/apps/kando/locales/testlocale"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// useNoMenuBar runs t's app without a menu bar of its own, as on Windows and Linux, whatever system the tests run on, until t ends.
func useNoMenuBar(t *testing.T) {
	t.Helper()
	prev := appMenuBar
	appMenuBar = false
	t.Cleanup(func() { appMenuBar = prev })
}

// windowCommands are the actions of the window commands tt's app sent, oldest first.
func windowCommands(tt *nsgotest.App) []ns.WindowAction {
	var out []ns.WindowAction
	for _, c := range tt.Window {
		out = append(out, c.Action)
	}
	return out
}

// lastTitleBar is the title bar the app last asked its window for: whether hidden and how tall.
func lastTitleBar(t *testing.T, tt *nsgotest.App) ns.WindowCommand {
	t.Helper()
	for i := len(tt.Window) - 1; i >= 0; i-- {
		if tt.Window[i].Action == ns.WindowSetTitleBar {
			return tt.Window[i]
		}
	}
	t.Fatal("the app set no title bar")
	return ns.WindowCommand{}
}

// TestChromeBarPerPlatform checks the main window's title bar on each system: none of the app's on macOS, which keeps its traffic lights in the sidebar and the palette's key in the menu bar; on Windows a 34-point bar with the window's title and the button that opens the palette, its key as Windows writes it, NSGo's caption buttons kept; on Linux a 46-point header bar with the palette's button, the title and round minimize, maximize and close buttons of its own, as GTK draws none. The status card's palette button gives way to the bar's.
func TestChromeBarPerPlatform(t *testing.T) {
	msgs := testlocale.Messages()
	useNoMenuBar(t)

	_, mac, _ := newTestApp(t, &fakePipeline{})
	if mac.Exists("id:chrome-bar") || mac.Exists("id:chrome-search") {
		t.Fatal("macOS has a title bar of the app's")
	}
	if bar := lastTitleBar(t, mac); bar.TitleBar != ns.TitleBarHidden || bar.Height != 0 {
		t.Fatalf("macOS title bar %+v", bar)
	}

	for _, c := range []struct {
		os      ns.OS
		height  ns.Points
		buttons bool
	}{{ns.OSWindows, 34, false}, {ns.OSLinux, 46, true}} {
		usePlatform(t, c.os)
		a, tt, _ := newTestApp(t, &fakePipeline{})
		if bar := lastTitleBar(t, tt); bar.TitleBar != ns.TitleBarHidden || bar.Height != c.height {
			t.Fatalf("%v: title bar %+v, want hidden and %v high", c.os, bar, c.height)
		}
		if got := tt.Find("id:chrome-bar").Frame.Size.Height; got != c.height {
			t.Fatalf("%v: the bar is %v high, want %v", c.os, got, c.height)
		}
		if got := tt.Find("id:chrome-title").Text; got != msgs.LibraryWindowTitle() {
			t.Fatalf("%v: the bar's title %q", c.os, got)
		}
		if got := tt.Find("id:chrome-search").Props.Label; got != msgs.ChromeSearch() {
			t.Fatalf("%v: the search button is named %q", c.os, got)
		}
		if got := tt.Find("id:chrome-search-key").Text; got != "Ctrl+K" {
			t.Fatalf("%v: the search button shows the key %q", c.os, got)
		}
		if tt.Exists("id:sidebar-palette") {
			t.Fatalf("%v: the status card keeps its palette button beside the bar's", c.os)
		}
		for _, id := range []string{"chrome-minimize", "chrome-maximize", "chrome-close"} {
			if tt.Exists("id:"+id) != c.buttons {
				t.Fatalf("%v: %s shows %v, want %v", c.os, id, !c.buttons, c.buttons)
			}
		}
		tt.Press("id:chrome-search")
		refresh(tt, a)
		if !a.palette.open {
			t.Fatalf("%v: the search button did not open the palette", c.os)
		}
		if v := a.PaletteView(a.cx); len(v.Commands) == 0 || v.Commands[0].Label != msgs.CheatsheetMenuItem() {
			t.Fatalf("%v: the palette opens with %+v", c.os, v.Commands)
		}
	}
}

// TestChromeLinuxWindowButtons checks the header bar's own window buttons on Linux: each sends its command to the window, close as the window's close button does, so windowClosing decides whether Kando keeps running or quits; maximize says restore while the window is maximized; and the whole bar goes in full screen, which has no title bar.
func TestChromeLinuxWindowButtons(t *testing.T) {
	msgs := testlocale.Messages()
	useNoMenuBar(t)
	usePlatform(t, ns.OSLinux)
	a, tt, _ := newTestApp(t, &fakePipeline{})
	before := len(tt.Window)
	for _, id := range []string{"chrome-minimize", "chrome-maximize", "chrome-close"} {
		tt.Press("id:" + id)
	}
	if got, want := windowCommands(tt)[before:], []ns.WindowAction{ns.WindowMinimize, ns.WindowToggleMaximize, ns.WindowClose}; !slices.Equal(got, want) {
		t.Fatalf("the buttons sent %v, want %v", got, want)
	}
	if got := tt.Find("id:chrome-maximize").Props.Label; got != msgs.ChromeMaximize() {
		t.Fatalf("maximize is named %q", got)
	}
	env := testEnvironment()
	env.Maximized = true
	tt.SetEnvironment(env)
	refresh(tt, a)
	if got := tt.Find("id:chrome-maximize").Props.Label; got != msgs.ChromeRestore() {
		t.Fatalf("maximized, the button is named %q", got)
	}
	env.FullScreen = true
	tt.SetEnvironment(env)
	refresh(tt, a)
	if tt.Exists("id:chrome-bar") {
		t.Fatal("the bar shows in full screen")
	}
}

// TestChromeContentClearsNoTitleBar checks that under the app's own bar the window's content keeps clear of nothing: on Windows desktop.TitleBar reports the caption buttons' width and the bar's height, which the bar keeps clear of itself, so the sidebar's card starts without the traffic lights' row and the library's header bar runs to the window's edge, as they do on macOS only for the traffic lights.
func TestChromeContentClearsNoTitleBar(t *testing.T) {
	useNoMenuBar(t)
	usePlatform(t, ns.OSWindows)
	a, tt, _ := paletteLibrary(t, &fakePipeline{})
	useTitleBar(tt, ns.Edges{Top: 34, Right: 112})
	refresh(tt, a)
	bar := pageFrame(tt.Find("id:chrome-bar"))
	if bar.Origin.Y != 0 || bar.Size.Height != 34 {
		t.Fatalf("the bar at %+v", bar)
	}
	search := pageFrame(tt.Find("id:chrome-search"))
	if right := bar.Origin.X + bar.Size.Width - 112; search.Origin.X+search.Size.Width > right {
		t.Fatalf("the search button reaches %v, under the caption buttons from %v", search.Origin.X+search.Size.Width, right)
	}
	sidebar := pageFrame(tt.Find("id:sidebar"))
	if sidebar.Origin.Y != 34 {
		t.Fatalf("the sidebar starts at %v, want under the bar", sidebar.Origin.Y)
	}
	if card := pageFrame(tt.Find("id:sidebar-switcher")); card.Origin.Y-sidebar.Origin.Y > 30 {
		t.Fatalf("the switcher starts %v under the sidebar's top, leaving the traffic lights' row", card.Origin.Y-sidebar.Origin.Y)
	}
	start := pageFrame(tt.Find("id:films-start"))
	if end := start.Origin.X + start.Size.Width; end < bar.Size.Width-14-1 {
		t.Fatalf("开始处理 ends at %v in a window %v wide, kept clear of the caption buttons under the bar", end, bar.Size.Width)
	}
}

// TestChromeSettingsWindowClose checks the settings window's own close button: on Linux, where GTK draws no controls in its hidden title bar, it is a round close button that closes the window it is in as its close button would; elsewhere the system's controls are there and there is none. The settings dialog over the main window gets none either way (settingsOverlay).
func TestChromeSettingsWindowClose(t *testing.T) {
	for _, c := range []struct {
		os   ns.OS
		want bool
	}{{ns.OSMacOS, false}, {ns.OSWindows, false}, {ns.OSLinux, true}} {
		usePlatform(t, c.os)
		tt := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
			return ns.View{Children: []ns.Node{chrome.Close(cx)}}
		}, nsgotest.Environment(testEnvironment()))
		if got := tt.Exists("id:chrome-window-close"); got != c.want {
			t.Fatalf("%v: the settings window's close button shows %v, want %v", c.os, got, c.want)
		}
		if !c.want {
			continue
		}
		before := len(tt.Window)
		tt.Press("id:chrome-window-close")
		if got := windowCommands(tt)[before:]; !slices.Equal(got, []ns.WindowAction{ns.WindowClose}) {
			t.Fatalf("%v: the close button sent %v", c.os, got)
		}
	}
}

// TestPaletteReachesEveryMenuCommand checks that where the app has no menu bar, as on Windows and Linux, the command palette lists every command the menu bar would carry: each item that does something, a submenu's items under its name, and the window's own commands (close, full screen, minimize, maximize), which the macOS menu bar takes from the system. The palette's own item is the palette itself, the text commands of the Edit menu stay with the text fields' keys, and macOS's About, Hide and Show All have no counterpart there.
func TestPaletteReachesEveryMenuCommand(t *testing.T) {
	msgs := testlocale.Messages()
	usePlatform(t, ns.OSWindows)
	useNoMenuBar(t)
	a, tt, _ := paletteLibrary(t, &fakePipeline{})
	open := a.snap().LibraryDir
	a.edit(func(s *session) { s.Libraries = []string{open, "/other.kando"} })
	refresh(tt, a)

	// The menu bar the app would show is the one macOS's has, built on this system.
	appMenuBar = true
	menu := a.menuBar(a.cx)
	appMenuBar = false
	var palette []string
	for _, c := range a.paletteCommands(a.cx) {
		palette = append(palette, c.label)
	}

	skip := map[nsdesktop.MenuRole]bool{
		nsdesktop.MenuAbout: true, nsdesktop.MenuHide: true, nsdesktop.MenuHideOthers: true, nsdesktop.MenuShowAll: true,
		nsdesktop.MenuUndo: true, nsdesktop.MenuRedo: true, nsdesktop.MenuCut: true, nsdesktop.MenuCopy: true, nsdesktop.MenuPaste: true, nsdesktop.MenuSelectAll: true,
	}
	var walk func(prefix string, items []nsdesktop.MenuItem)
	walk = func(prefix string, items []nsdesktop.MenuItem) {
		for _, it := range items {
			label := it.Label
			if prefix != "" {
				label = msgs.PaletteSubmenuItem(prefix, it.Label)
			}
			switch {
			case it.Separator, it.Disabled, it.Checked, skip[it.Role], it.Label == msgs.PaletteMenuItem():
			case it.Items != nil:
				walk(label, it.Items)
			case it.Role == nsdesktop.MenuZoom:
				if !slices.Contains(palette, msgs.ChromeMaximize()) {
					t.Errorf("the palette has no %q for %q: %q", msgs.ChromeMaximize(), label, palette)
				}
			default:
				if !slices.Contains(palette, label) {
					t.Errorf("the palette has no %q", label)
				}
			}
		}
	}
	for _, m := range menu {
		walk("", m.Items)
	}
	if want := msgs.PaletteSubmenuItem(msgs.MenuSwitchLibrary(), ""); !slices.ContainsFunc(palette, func(l string) bool { return strings.HasPrefix(l, want) }) {
		t.Fatalf("the palette cannot switch libraries: %q", palette)
	}
}

// TestPaletteRunsTheMenuOnlyCommands checks what the palette's rows for the menu bar's own items do where there is no menu bar: the window's go to the main window as commands, and a library of 切换资料库 opens, the one open not offered.
func TestPaletteRunsTheMenuOnlyCommands(t *testing.T) {
	msgs := testlocale.Messages()
	usePlatform(t, ns.OSLinux)
	useNoMenuBar(t)
	a, tt, _ := newTestApp(t, &fakePipeline{})
	other := t.TempDir() + "/乙.kando"
	a.edit(func(s *session) { s.Libraries = []string{"/videos", other} })
	refresh(tt, a)
	run := func(label string) {
		t.Helper()
		for _, c := range a.paletteCommands(a.cx) {
			if c.label == label {
				onEngine(a, c.run)
				refresh(tt, a)
				return
			}
		}
		t.Fatalf("the palette has no %q", label)
	}
	before := len(tt.Window)
	run(msgs.MenuMinimize())
	run(msgs.ChromeMaximize())
	run(msgs.MenuToggleFullScreen())
	run(msgs.MenuCloseWindow())
	if got, want := windowCommands(tt)[before:], []ns.WindowAction{ns.WindowMinimize, ns.WindowToggleMaximize, ns.WindowToggleFullScreen, ns.WindowClose}; !slices.Equal(got, want) {
		t.Fatalf("the window's rows sent %v, want %v", got, want)
	}
	for _, c := range a.paletteCommands(a.cx) {
		if strings.HasPrefix(c.label, msgs.PaletteSubmenuItem(msgs.MenuSwitchLibrary(), "")) && strings.Contains(c.label, "乙") {
			onEngine(a, c.run)
			refresh(tt, a)
		}
	}
	if a.snap().LibraryDir != other {
		t.Fatalf("切换资料库 opened %q, want %q", a.snap().LibraryDir, other)
	}
	if strings.Contains(strings.Join(paletteLabels(a), "\n"), msgs.PaletteSubmenuItem(msgs.MenuSwitchLibrary(), "乙")) {
		t.Fatal("the palette offers the library already open")
	}
}

// paletteLabels are the labels of the palette's commands now.
func paletteLabels(a *app) []string {
	var out []string
	for _, c := range a.paletteCommands(a.cx) {
		out = append(out, c.label)
	}
	return out
}

// TestPaletteQuitAndCloseAskWhileAJobRuns checks the palette's rows and the header bar's button that end the app where there is no menu bar: 退出看懂 quits through the app's own quit, which asks first while a job runs, as ⌘Q does; the header bar's close button and 关闭窗口 send the window its close, which, with 关掉窗口后继续在后台运行 off, quits through the same question; and 在文件管理器中显示资料库 shows the open library in the file manager.
func TestPaletteQuitAndCloseAskWhileAJobRuns(t *testing.T) {
	msgs := testlocale.Messages()
	usePlatform(t, ns.OSLinux)
	useNoMenuBar(t)
	setWindowHidesOnClose(t, false)
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	a.tray.keepRunning = false
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	refresh(tt, a)
	run := func(label string) {
		t.Helper()
		for _, c := range a.paletteCommands(a.cx) {
			if c.label == label {
				onEngine(a, c.run)
				refresh(tt, a)
				return
			}
		}
		t.Fatalf("the palette has no %q in %q", label, paletteLabels(a))
	}
	asked := func(what string) {
		t.Helper()
		if !a.queue.quitAsk || tt.Quits != 0 || !shows(tt, msgs.QueueQuitWhileRunningTitle()) {
			t.Fatalf("%s while a job runs: asked %v, quits %d", what, a.queue.quitAsk, tt.Quits)
		}
		a.queue.quitAsk = false
		if a.queue.quitRelease != nil {
			a.queue.quitRelease()
			a.queue.quitRelease = nil
		}
		refresh(tt, a)
	}

	run(msgs.MenuQuit())
	asked("退出看懂")

	before := len(tt.Window)
	tt.Press("id:chrome-close")
	run(msgs.MenuCloseWindow())
	if got, want := windowCommands(tt)[before:], []ns.WindowAction{ns.WindowClose, ns.WindowClose}; !slices.Equal(got, want) {
		t.Fatalf("close sent %v, want %v", got, want)
	}
	// The window's close the system reports back for that command, which the test app, opening no main window on Linux, does not.
	e := &ns.WindowClosing{}
	a.windowClosing(e)
	refresh(tt, a)
	if !windowClosingCanceled(e) {
		t.Fatal("the window closed while a job runs")
	}
	asked("closing the window")

	var revealed string
	prev := revealInFolder
	revealInFolder = func(_ *ns.Scope, path string, done func(error)) { revealed = path; done(nil) }
	t.Cleanup(func() { revealInFolder = prev })
	dir := t.TempDir()
	a.edit(func(s *session) { s.LibraryDir = dir })
	refresh(tt, a)
	run(msgs.MenuRevealLibrary(platformName()))
	if want := dir; revealed != want {
		t.Fatalf("the reveal row showed %q, want %q", revealed, want)
	}
}
