package main

import (
	"math"
	"time"

	"kando/apps/kando/locales"
)

// playerVolumeStep is how much ↑ and ↓ change the volume.
const playerVolumeStep = 0.1

// playerVolumeSaveDelay is how long the volume has to stay put before the settings keep it, so that a key held down or a slider dragged writes settings.json once it rests rather than at every step. The sound follows every step at once.
const playerVolumeSaveDelay = 250 * time.Millisecond

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
	a.flash(playerVolumeText(a.player.volume))
	a.playerVolumeChanged()
}

// ToggleMute is M: it silences the films, or gives them back their volume. A film that is silent, muted or turned down to 0, is unmuted, and a volume of 0 becomes half, so that it can be heard.
func (a *app) ToggleMute() {
	if a.player.muted || a.player.volume == 0 {
		a.player.muted = false
		if a.player.volume == 0 {
			a.player.volume = 0.5
		}
		a.flash(playerVolumeText(a.player.volume))
	} else {
		a.player.muted = true
		a.flash(locales.PlayerOsdMuted())
	}
	a.playerVolumeChanged()
}

// playerNudgeVolume is ↑ and ↓: the volume a step louder or quieter, in whole steps, from the volume a mute kept, unmuting.
func (a *app) playerNudgeVolume(steps int) {
	a.SetVolume((math.Round(a.player.volume/playerVolumeStep) + float64(steps)) * playerVolumeStep)
}

// playerVolumeChanged plays the open film's sound at the new volume and keeps it in the settings once the volume has rested for playerVolumeSaveDelay. Without an update, as before the window exists, it keeps it at once.
func (a *app) playerVolumeChanged() {
	if a.player.play != nil {
		a.player.play.setVolume(a.playerVolume())
	}
	a.player.volumeUnsaved = true
	a.player.volumeGen++
	gen := a.player.volumeGen
	update := a.update
	if update == nil {
		a.playerSaveVolume()
		return
	}
	time.AfterFunc(playerVolumeSaveDelay, func() {
		update(func() {
			if a.player.volumeGen == gen {
				a.playerSaveVolume()
			}
		})
	})
}

// playerSaveVolume keeps a volume changed since it was last kept, saying so on the picture when it cannot be saved.
func (a *app) playerSaveVolume() {
	if !a.player.volumeUnsaved {
		return
	}
	a.player.volumeUnsaved = false
	if err := a.saveSettings(); err != nil {
		a.flash(locales.PlayerSaveVolumeFailed())
	}
}

// playerQuitting stops the open video and keeps a volume changed too shortly before quitting for playerVolumeChanged to have kept it.
func (a *app) playerQuitting() {
	a.playerSaveVolume()
	a.stopPlayback()
}

// playerVolumeText is what the picture says for volume, as "音量 70%".
func playerVolumeText(volume float64) string {
	return locales.PlayerOsdVolume(int(math.Round(volume * 100)))
}
