package main

import (
	"math"
	"strconv"
	"time"
)

// volumeStep is how much ↑ and ↓ change the volume.
const volumeStep = 0.1

// volumeSettle is how long the volume has to stay put before the sound starts again at it. The sound plays in a process that takes its volume only when it starts, so a key held down or a slider dragged waits until it rests instead of cutting the sound at every step.
const volumeSettle = 250 * time.Millisecond

// playerVolume is how loud the sound plays now, from 0 to 1: nothing while muted.
func (a *app) playerVolume() float64 {
	if a.player.muted {
		return 0
	}
	return a.player.volume
}

// Volume and Muted are the player's volume and whether it is muted, for the transport's volume control (film.App).
func (a *app) Volume() float64 { return a.player.volume }

func (a *app) Muted() bool { return a.player.muted }

// SetVolume plays films at volume from 0 to 1, unmuting them, and says so on the picture.
func (a *app) SetVolume(volume float64) {
	a.player.volume = min(max(volume, 0), 1)
	a.player.muted = false
	a.flash(volumeText(a.player.volume))
	a.playerVolumeChanged()
}

// ToggleMute is M: it silences the films, or gives them back their volume. A film that is silent, muted or turned down to 0, is unmuted, and a volume of 0 becomes half, so that it can be heard.
func (a *app) ToggleMute() {
	if a.player.muted || a.player.volume == 0 {
		a.player.muted = false
		if a.player.volume == 0 {
			a.player.volume = 0.5
		}
		a.flash(volumeText(a.player.volume))
	} else {
		a.player.muted = true
		a.flash("静音")
	}
	a.playerVolumeChanged()
}

// nudgeVolume is ↑ and ↓: the volume a step louder or quieter, in whole steps, from the volume a mute kept, unmuting.
func (a *app) nudgeVolume(steps int) {
	a.SetVolume((math.Round(a.player.volume/volumeStep) + float64(steps)) * volumeStep)
}

// playerVolumeChanged starts the sound again at the new volume and keeps it in the settings, once the volume has rested for volumeSettle. Without an update, as before the window exists, it does so at once.
func (a *app) playerVolumeChanged() {
	a.player.volumeGen++
	gen := a.player.volumeGen
	update := a.update
	if update == nil {
		a.playerApplyVolume()
		return
	}
	time.AfterFunc(volumeSettle, func() {
		update(func() {
			if a.player.volumeGen == gen {
				a.playerApplyVolume()
			}
		})
	})
}

func (a *app) playerApplyVolume() {
	if a.player.play != nil {
		a.player.play.setVolume(a.playerVolume())
	}
	if err := a.saveSettings(); err != nil {
		a.flash("音量设置无法保存")
	}
}

// volumeText is what the picture says for volume, as "音量 70%".
func volumeText(volume float64) string {
	return "音量 " + strconv.Itoa(int(math.Round(volume*100))) + "%"
}
