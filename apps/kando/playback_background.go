package main

import (
	"github.com/nsgo-dev/nsgo/core"
)

// The system's player plays a film's picture and sound together whether the window is in sight or not, so nothing here keeps the two level, as the app's own player had to when MyGo stopped the picture behind a window out of sight. What the window being out of sight changes is the display: the player keeps it awake only while the film can be seen (player_awake.go), which this file follows.

// playerAwakeLifecycle follows, under the app's scope, whether the main window shows, as the platform reports it for that window (core.LifecycleBackground: minimized, hidden or covered where the system says so), so the display may sleep while only the film's sound is heard, also while the settings window still shows. It reads the main view's scope (windowFollow) once the view has been built, the app's, every window's together, until then. playerAwakeReady runs it once the main window exists.
func (a *app) playerAwakeLifecycle() {
	a.cx.Run(func() {
		a.cx.Effect(func() {
			a.Track()
			win := a.window.cx
			if win == nil {
				win = a.cx
			}
			away := win.Env().Lifecycle() == core.LifecycleBackground
			core.Untracked(a.cx, func() bool {
				if away != a.player.windowAway {
					a.playerWindowAway(away)
				}
				return true
			})
		})
	})
}

// playerWindowHidden notes that the main window was put away by its close button, or shown again (playerHooks.windowShown), and takes or gives back the display hold to match.
func (a *app) playerWindowHidden(hidden bool) {
	a.player.windowHidden = hidden
	a.playerKeepAwake()
}

// playerOutOfSight is whether the window is out of sight, put away by its close button or with none of the app showing, so only the film's sound is heard.
func (a *app) playerOutOfSight() bool { return a.player.windowHidden || a.player.windowAway }
