package main

import (
	"github.com/nsgo-dev/nsgo/core"
)

// The system's player plays a film's picture and sound together whether the window is in sight or not, so nothing here keeps the two level, as the app's own player had to when MyGo stopped the picture behind a window out of sight. What the window being out of sight changes is the display: the player keeps it awake only while the film can be seen (player_awake.go), which this file follows.

// playerAwakeLifecycle follows, under the app's scope, whether any of the app shows, as the platform reports it (core.LifecycleBackground: the window minimized, hidden or covered where the system says so), so the display may sleep while only the film's sound is heard. playerAwakeReady runs it once the main window exists.
func (a *app) playerAwakeLifecycle() {
	// TODO(nsgo): per-window minimize, restore, hide and show events, so the display may sleep while the main window alone is minimized or hidden, as MyGo's Window.OnMinimize and OnHide had it; the app-wide LifecycleBackground keeps it awake while the settings window still shows
	a.cx.Run(func() {
		a.cx.Effect(func() {
			away := a.cx.Env().Lifecycle() == core.LifecycleBackground
			core.Untracked(a.cx, func() bool {
				a.playerWindowAway(away)
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
