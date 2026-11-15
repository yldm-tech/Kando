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
	// frame is where the main window was last while neither maximized nor in full screen, as desktop.Frame reports it, and maximized and fullScreen whether it was last maximized or in full screen, which settings.json keeps for the next launch (savedWindow).
	frame                 core.WindowFrame
	maximized, fullScreen bool
}

func newWindowUI() windowUI { return windowUI{} }

// savedWindow is the main window's part of settings.json: its frame and whether it was maximized or in full screen, which MyGo's StateKey used to keep in a file of its own.
type savedWindow struct {
	MainWindow *savedFrame `json:"mainWindow,omitempty"`
}

func (s *savedWindow) clean() { s.MainWindow = s.MainWindow.cleaned() }

// savedFrame is how settings.json keeps a window: its frame while neither maximized nor in full screen, in points from the main screen's top left, positioned false where the platform reported only its size (Linux); and for the main window whether it was maximized or in full screen. Kando owns these keys, so a field NSGo renames in core.WindowFrame does not change the file.
type savedFrame struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Positioned bool    `json:"positioned,omitempty"`
	Maximized  bool    `json:"maximized,omitempty"`
	FullScreen bool    `json:"fullScreen,omitempty"`
}

// savedFrameOf is f as settings.json keeps it, nil for a frame without a size, as before the window first reported one.
func savedFrameOf(f core.WindowFrame) *savedFrame {
	if f.Width <= 0 || f.Height <= 0 {
		return nil
	}
	return &savedFrame{X: float64(f.X), Y: float64(f.Y), Width: float64(f.Width), Height: float64(f.Height), Positioned: f.Move}
}

// windowFrame is the frame s keeps, zero for none.
func (s *savedFrame) windowFrame() core.WindowFrame {
	if s == nil {
		return core.WindowFrame{}
	}
	return core.WindowFrame{X: core.Points(s.X), Y: core.Points(s.Y), Move: s.Positioned, Width: core.Points(s.Width), Height: core.Points(s.Height)}
}

// cleaned is s, or nil when it holds no size a window could open at.
func (s *savedFrame) cleaned() *savedFrame {
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	return s
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
			if f := savedFrameOf(a.window.frame); f != nil {
				f.Maximized, f.FullScreen = a.window.maximized, a.window.fullScreen
				saved.MainWindow = f
			}
		},
		restore: func(saved savedSettings) {
			if f := saved.MainWindow; f != nil {
				a.window.frame = f.windowFrame()
				a.window.maximized, a.window.fullScreen = f.Maximized, f.FullScreen
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

// windowFollow follows the main window from its view's scope cx: its frame and whether it is maximized or in full screen, kept for the next launch as it changes (saved with the settings, and as the app quits when they changed), whether it shows, which the areas hear about (windowSight), and a window closed in full screen, which hides once it has left it. A window that was maximized or in full screen when the app last quit goes back to it once it shows.
func (a *app) windowFollow(cx *core.Scope) {
	a.window.cx = cx
	maximized, fullScreen := a.window.maximized, a.window.fullScreen
	cx.Effect(func() {
		f := desktop.Frame(cx)
		a.window.maximized, a.window.fullScreen = desktop.Maximized(cx), desktop.FullScreen(cx)
		if a.window.maximized || a.window.fullScreen || f.Width <= 0 || f.Height <= 0 {
			return
		}
		// Linux reports no position (Move false), and the window opens centered there.
		f.Animated = false
		a.window.frame = f
	})
	// The state goes back once the platform has reported the window's frame, so the window is there to take it, and once only.
	restoring := maximized || fullScreen
	cx.Effect(func() {
		if !restoring || desktop.Frame(cx).Width <= 0 {
			return
		}
		restoring = false
		cx.Post(func() {
			switch {
			case fullScreen && !desktop.FullScreen(cx):
				desktop.ToggleFullScreen(cx)
			case !fullScreen && !desktop.Maximized(cx):
				desktop.ToggleMaximize(cx)
			}
		})
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
	cx.OnQuit(func(context.Context) { _ = a.saveSettingsChanged() })
}

// windowWatch listens to the main window's close button. It runs once, from ready.
func (a *app) windowWatch() {
	w := desktop.MainWindow(a.cx)
	a.window.main = w
	// TODO(nsgo#109): hear the main window's close button on Windows and Linux, so a running job asks before the app quits (queueBeforeQuit) as it does on macOS; NSGo's backends there close the window without asking OnClosing https://github.com/nsgo-dev/nsgo/issues/109
	w.OnClosing(a.cx, live.On(a, a.windowClosing))
	// The areas hear about it under the app's scope (core.Scope.Run), not under the effect, whose next run would dispose of what they made; the bindings and the menu bar that read whether the window is hidden follow when that changed.
	a.cx.Effect(func() {
		shown := w.Visible()
		changed := false
		a.cx.Run(func() { changed = a.windowSight(shown || w == nil) })
		if changed {
			a.Changed()
		}
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

// windowSight records whether the main window is in sight, and tells the areas that react to it (areaHooks.windowShown) when that changed, which it reports. Its callers in handlers are followed by Changed already (package live); windowWatch says so itself.
func (a *app) windowSight(shown bool) bool {
	if a.window.hidden == !shown {
		return false
	}
	a.window.hidden = !shown
	a.areasWindowShown(shown)
	return true
}
