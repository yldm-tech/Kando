package main

import (
	"kando/apps/kando/desktop"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// playerAwakeReady gives the player desktop.KeepAwake, under the app's scope so the hold outlives the handler that took it, and has it let the display sleep while none of the app shows, where only the film's sound is heard, and keep it awake again once the window is back (playerAwakeLifecycle). playerReady runs it once the main window exists.
func (a *app) playerAwakeReady() {
	a.player.keepAwake = func(reason string, display bool) (release func()) {
		a.cx.Run(func() { release = desktop.KeepAwake(a.cx, reason, display) })
		return release
	}
	a.playerAwakeLifecycle()
}

// playerWindowAway notes that none of the app shows (away), or that it shows again, and takes or gives back the display hold to match at once.
func (a *app) playerWindowAway(away bool) {
	a.player.windowAway = away
	a.playerKeepAwake()
}

// playerKeepAwake keeps the display from turning off while the open film plays in sight, and lets it again once the film is paused, has ended or was closed, or is only heard: while the main window is put away or none of the app shows (playerOutOfSight). A change of the app's state, as the film starting, pausing or ending, runs it (playerAfterChange). A seek keeps the hold, as the film plays on from where it lands. It does nothing until playerReady has set keepAwake, so tests, which never run ready, take no hold on the machine running them.
func (a *app) playerKeepAwake() {
	p := a.player.play
	playing := p != nil && p.Playing() && !a.playerOutOfSight()
	switch {
	case playing && a.player.wake == nil && a.player.keepAwake != nil:
		a.player.wake = a.player.keepAwake(a.messages().PlayerKeepAwake(), true)
	case !playing:
		a.playerLetSleep()
	}
}

// playerLetSleep gives back the hold that keeps the display awake, if the player has one.
func (a *app) playerLetSleep() {
	if a.player.wake != nil {
		a.player.wake()
		a.player.wake = nil
	}
}

// windowScreen is the main window's full screen, as the player enters and leaves it: through the main view's scope (windowUI.cx), which knows the window; before the view is built there is no window to act on.
type windowScreen struct{ a *app }

func (w windowScreen) IsFullScreen() bool {
	cx := w.a.window.cx
	return cx != nil && core.Untracked(cx, func() bool { return nsdesktop.FullScreen(cx) })
}

func (w windowScreen) SetFullScreen(on bool) {
	if w.IsFullScreen() != on {
		w.ToggleFullScreen()
	}
}

func (w windowScreen) ToggleFullScreen() {
	if cx := w.a.window.cx; cx != nil {
		nsdesktop.ToggleFullScreen(cx)
	}
}
