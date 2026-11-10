package main

import "kando/apps/kando/locales"

// playerKeepAwake keeps the display from turning off while the open film plays, and lets it again once the film is paused, has ended or was closed. The frame a film starts, pauses or ends in runs it, as the picture moves on only in frames. It does nothing until playerReady has set keepAwake, so tests, which never run ready, take no hold on the machine running them.
func (a *app) playerKeepAwake() {
	playing := a.player.play != nil && a.player.play.Playing()
	switch {
	case playing && a.player.wake == nil && a.player.keepAwake != nil:
		a.player.wake = a.player.keepAwake(locales.PlayerKeepAwake(), true)
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
