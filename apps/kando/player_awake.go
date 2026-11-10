package main

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo"
)

// playerAwakeReady gives the player mygo.Power.KeepAwake and has it let the display sleep while the window is minimized or hidden, where only the film's sound plays on, and keep it awake again once the window is back. playerReady runs it once the main window exists.
func (a *app) playerAwakeReady() {
	a.player.keepAwake = mygo.Power.KeepAwake
	a.win.OnMinimize(func() { a.playerWindowAway(true) })
	a.win.OnRestore(func() { a.playerWindowAway(false) })
	a.win.OnHide(func() { a.playerWindowAway(true) })
	a.win.OnShow(func() { a.playerWindowAway(false) })
}

// playerWindowAway notes that the window was minimized or hidden (away), or came back, and takes or gives back the display hold to match at once, as a window away draws no frames.
func (a *app) playerWindowAway(away bool) {
	a.player.windowAway = away
	a.playerKeepAwake()
}

// playerKeepAwake keeps the display from turning off while the open film plays in sight, and lets it again once the film is paused, has ended or was closed, or plays on only as sound: while the window is minimized or hidden, or the picture stood still behind it (filmPlay.unseen). The frame a film starts, pauses or ends in runs it, as the picture moves on only in frames. A seek keeps the hold while it opens the film again, so skipping does not give it back and take it again each time. It does nothing until playerReady has set keepAwake, so tests, which never run ready, take no hold on the machine running them.
func (a *app) playerKeepAwake() {
	p := a.player.play
	playing := p != nil && p.Playing() && !p.unseen && !a.player.windowAway
	reopening := p == nil && a.player.playPath != "" && a.player.playErr == "" && !a.player.windowAway
	switch {
	case playing && a.player.wake == nil && a.player.keepAwake != nil:
		a.player.wake = a.player.keepAwake(locales.PlayerKeepAwake(), true)
	case !playing && !reopening:
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
