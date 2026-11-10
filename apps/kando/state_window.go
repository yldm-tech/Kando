package main

import (
	"runtime"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo"
)

// windowUI is the state of the main window itself: whether its close button put it away while the app runs on, and whether a quit is closing it for good.
type windowUI struct {
	// hidden is set while the main window is put away by its close button (windowHidesOnClose), until the Dock icon, 窗口 › 主窗口 or anything else shows it again.
	hidden bool
	// leavingFullScreen is set while a window closed in full screen leaves it, so it hides once it has.
	leavingFullScreen bool
	// quitting is set once a quit goes ahead (areaHooks.quitStarting), so the window's close button no longer puts it away and the quit closes it.
	quitting bool
}

func newWindowUI() windowUI { return windowUI{} }

// windowHidesOnClose says the main window's close button only puts the window away and Kando runs on, its jobs and its sound with it, until the Dock icon brings the window back: on macOS, where an app outlives its windows. On Linux and Windows closing the main window quits, after the queue asks when a job is running. Tests set it to check both.
var windowHidesOnClose = runtime.GOOS == "darwin"

// quitApp quits the app as ⌘Q does, so the areas' beforeQuit hooks run first. Tests replace it.
var quitApp = mygo.App.Quit

// windowHooks: once the main window exists its close button is caught (windowClosing), and the app coming back from the Dock shows it again. A quit that goes ahead lets the window close.
func (a *app) windowHooks() areaHooks {
	return areaHooks{
		ready:        a.windowWatch,
		quitStarting: func() { a.window.quitting = true },
	}
}

// windowKeys are the main window's rows of the shortcut table: 窗口 › 主窗口, without a key, which brings the window back while its close button has put it away.
func (a *app) windowKeys() []shortcut {
	return []shortcut{
		{Group: groupWindow, Label: locales.WindowMenuMain(), When: func() bool { return a.window.hidden }, Run: a.windowShowMain},
	}
}

// windowWatch listens to the main window and to the app being opened again from the Dock. It runs once, from ready.
func (a *app) windowWatch() {
	win := a.win
	win.OnClose(a.windowClosing)
	win.OnLeaveFullScreen(func() {
		if a.window.leavingFullScreen {
			a.window.leavingFullScreen = false
			win.Hide()
		}
	})
	// Show is how the window comes back from anywhere, a second launch of the app included (remoteRaise).
	win.OnShow(func() { a.windowSight(true) })
	mygo.App.OnActivate(func(bool) {
		if a.window.hidden {
			a.windowShowMain()
		}
	})
}

// windowClosing decides what the main window's close button, ⌘W or 关闭窗口 do. A quit under way closes the window. Otherwise the window stays: on macOS it is put away while Kando runs on, and elsewhere the app quits, which asks first when a job is running (queueBeforeQuit) and closes the window once it may.
func (a *app) windowClosing(e *mygo.CloseEvent) {
	if a.window.quitting || e.DefaultPrevented() {
		return
	}
	e.PreventDefault()
	if !windowHidesOnClose {
		quitApp()
		return
	}
	a.windowHide()
}

// windowHide puts the main window away. A window in full screen leaves it first, as a closed window does, and hides once it has.
func (a *app) windowHide() {
	a.windowSight(false)
	if a.win == nil {
		return
	}
	if a.win.IsFullScreen() {
		a.window.leavingFullScreen = true
		a.win.SetFullScreen(false)
		return
	}
	a.win.Hide()
}

// windowShowMain brings the main window back to the front, restored when it was minimized: the Dock icon, 窗口 › 主窗口, and the queue asking before a quit show it so.
func (a *app) windowShowMain() {
	a.window.leavingFullScreen = false
	a.windowSight(true)
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	mygo.App.Focus()
}

// windowUnhide brings the main window back when its close button put it away, and leaves it alone otherwise: the rows of the Window menu call it before they act on the window (menu.go).
func (a *app) windowUnhide() {
	if a.window.hidden {
		a.windowShowMain()
	}
}

// windowSight records whether the main window is in sight, and tells the areas that react to it (areaHooks.windowShown) when that changed.
func (a *app) windowSight(shown bool) {
	if a.window.hidden == !shown {
		return
	}
	a.window.hidden = !shown
	a.areasWindowShown(shown)
}
