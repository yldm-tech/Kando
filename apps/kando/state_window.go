package main

import (
	"context"
	"runtime"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// windowUI is the state of the main window itself: where it is, whether its close button put it away while the app runs on, and whether a quit is closing it for good.
type windowUI struct {
	// main is the main window, nil before the app runs and where the platform opens no windows of the app's (Windows and Linux for now, tests), whose methods then do nothing; cx is the main view's scope, which the window's commands act through.
	main *core.Window
	cx   *core.Scope
	// hidden is set while the main window is put away by its close button (windowHidesOnClose), until the Dock icon, 窗口 › 主窗口 or anything else shows it again.
	hidden bool
	// leavingFullScreen is set while a window closed in full screen leaves it, so it hides once it has.
	leavingFullScreen bool
	// quitting is set once a quit goes ahead (areaHooks.quitStarting), so the window's close button no longer puts it away and the quit closes it.
	quitting bool
	// frame is where the main window was last while neither maximized nor in full screen, as desktop.Frame reports it, which settings.json keeps for the next launch (savedWindow).
	frame core.WindowFrame
}

func newWindowUI() windowUI { return windowUI{} }

// savedWindow is the main window's part of settings.json: its frame, which MyGo's StateKey used to keep in a file of its own.
type savedWindow struct {
	MainWindow *core.WindowFrame `json:"mainWindow,omitempty"`
}

func (s *savedWindow) clean() {
	if f := s.MainWindow; f != nil && (f.Width <= 0 || f.Height <= 0) {
		s.MainWindow = nil
	}
}

// windowFrame is the frame main opens the main window at: the one settings.json kept, false without one.
func (a *app) windowFrame() (core.WindowFrame, bool) {
	f := a.window.frame
	return f, f.Width > 0 && f.Height > 0
}

// windowHidesOnClose says the main window's close button only puts the window away and Kando runs on, its jobs and its sound with it, until the Dock icon brings the window back: on macOS, where an app outlives its windows. On Linux and Windows closing the main window quits, after the queue asks when a job is running. Tests set it to check both.
var windowHidesOnClose = runtime.GOOS == "darwin"

// windowHooks: the main window's frame is kept in settings.json; once the window exists its close button is caught (windowClosing). A quit that goes ahead lets the window close.
func (a *app) windowHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			if _, ok := a.windowFrame(); ok {
				f := a.window.frame
				saved.MainWindow = &f
			}
		},
		restore: func(saved savedSettings) {
			if f := saved.MainWindow; f != nil {
				a.window.frame = *f
			}
		},
		ready:        a.windowWatch,
		quitStarting: func() { a.window.quitting = true },
	}
}

// windowKeys are the main window's rows of the shortcut table: 窗口 › 主窗口, without a key, which brings the window back while its close button has put it away.
func (a *app) windowKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupWindow, Label: locales.WindowMenuMain(cx), When: func() bool { return a.window.hidden }, Run: a.windowShowMain},
	}
}

// windowFollow follows the main window from its view's scope cx: its frame, kept for the next launch as it moves (saved with the settings, and as the app quits), whether it shows, which the areas hear about (windowSight), and a window closed in full screen, which hides once it has left it.
func (a *app) windowFollow(cx *core.Scope) {
	a.window.cx = cx
	cx.Effect(func() {
		f := desktop.Frame(cx)
		if desktop.Maximized(cx) || desktop.FullScreen(cx) || f.Width <= 0 || f.Height <= 0 {
			return
		}
		// Linux reports no position (Move false), and the window opens centered there.
		f.Animated = false
		a.window.frame = f
	})
	cx.Effect(func() {
		if desktop.FullScreen(cx) || !a.window.leavingFullScreen {
			return
		}
		core.Untracked(cx, func() bool {
			a.window.leavingFullScreen = false
			a.window.main.Hide()
			return true
		})
	})
	cx.OnQuit(func(context.Context) { _ = a.saveSettings() })
}

// windowWatch listens to the main window's close button. It runs once, from ready.
func (a *app) windowWatch() {
	w := desktop.MainWindow(a.cx)
	a.window.main = w
	// TODO(nsgo#109): hear the main window's close button on Windows and Linux, so a running job asks before the app quits (queueBeforeQuit) as it does on macOS; NSGo's backends there close the window without asking OnClosing https://github.com/nsgo-dev/nsgo/issues/109
	w.OnClosing(a.cx, live.On(a, a.windowClosing))
	a.cx.Effect(func() {
		shown := w.Visible()
		core.Untracked(a.cx, func() bool {
			a.windowSight(shown || w == nil)
			return true
		})
	})
}

// windowClosing decides what the main window's close button, ⌘W or 关闭窗口 do. A quit under way closes the window. Otherwise the window stays: on macOS it is put away while Kando runs on, and elsewhere the app quits, which asks first when a job is running (queueBeforeQuit) and closes the window once it may.
func (a *app) windowClosing(c *core.WindowClosing) {
	if a.window.quitting {
		return
	}
	c.Cancel()
	if !windowHidesOnClose {
		a.requestQuit()
		return
	}
	a.windowHide()
}

// windowHide puts the main window away. A window in full screen leaves it first, as a closed window does, and hides once it has (windowFollow).
func (a *app) windowHide() {
	a.windowSight(false)
	if a.window.cx != nil && core.Untracked(a.window.cx, func() bool { return desktop.FullScreen(a.window.cx) }) {
		a.window.leavingFullScreen = true
		desktop.ToggleFullScreen(a.window.cx)
		return
	}
	a.window.main.Hide()
}

// windowShowMain brings the main window back to the front: the Dock icon, 窗口 › 主窗口, and the queue asking before a quit show it so.
func (a *app) windowShowMain() {
	a.window.leavingFullScreen = false
	a.windowSight(true)
	a.window.main.Focus()
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
