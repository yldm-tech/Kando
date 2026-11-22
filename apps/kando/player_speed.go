package main

import (
	"math"
	"slices"
	"time"

	"kando/apps/kando/film"

	"github.com/nsgo-dev/nsgo/core"
)

// Rate is how fast films play, one of film.Rates, for the transport's speed button (film.App).
func (a *app) Rate() float64 { return a.player.rate }

// SpeedFixed is whether the open film plays only at the speed it was recorded at, as the system's player said when it opened it (filmPlay.speedFixed).
func (a *app) SpeedFixed() bool { return a.player.play != nil && a.player.play.speedFixed }

// SpeedMenu is whether the transport's speed menu is open.
func (a *app) SpeedMenu() *bool { return &a.player.film.speedMenu }

// SpeedKeys are the keys of the speed rows of the table on os under the preset chosen (presetKey), for the speed button's tooltip.
func (a *app) SpeedKeys(os core.OS) (slower, faster string) {
	return presetKey(a.prefs.shortcutPreset, os, groupPlayback, playerSlowerKey), presetKey(a.prefs.shortcutPreset, os, groupPlayback, playerFasterKey)
}

// playerSlowerKey and playerFasterKey are Kando's own keys that step the speed down and up (playerKeys).
const (
	playerSlowerKey = "shift+,"
	playerFasterKey = "shift+."
)

// SetRate plays films at rate, the nearest of film.Rates, says so on the picture and on the speed button at once, has the open film take it up at once and the settings keep it once the speed has rested (playerRateChanged).
func (a *app) SetRate(rate float64) {
	rate = playerNearestRate(rate)
	a.flash(a.messages().PlayerOsdSpeed(film.RateTextIn(a.messages(), rate)))
	if rate == a.player.rate {
		return
	}
	a.player.rate = rate
	a.playerApplyRate()
	a.playerRateChanged()
}

// playerRateSettle is how long the speed has to stay put before the settings keep it.
const playerRateSettle = 250 * time.Millisecond

// playerRateChanged keeps the speed in the settings once it has rested for playerRateSettle, so a key held down writes settings.json once it rests rather than at every step. Without an update, as before the window exists, it keeps it at once.
func (a *app) playerRateChanged() {
	a.player.rateGen++
	gen := a.player.rateGen
	update := a.update
	if update == nil {
		a.playerRateSettled()
		return
	}
	a.player.rateSettling = true
	time.AfterFunc(playerRateSettle, func() {
		update(func() {
			if a.player.rateGen == gen {
				a.playerRateSettled()
			}
		})
	})
}

func (a *app) playerRateSettled() {
	a.player.rateSettling = false
	if err := a.saveSettings(); err != nil {
		a.flash(a.messages().PlayerSaveSpeedFailed())
	}
}

// playerSaveRate keeps a speed changed too shortly before quitting for playerRateChanged to have kept it.
func (a *app) playerSaveRate() {
	if a.player.rateSettling {
		a.playerRateSettled()
	}
}

// playerStepRate is ⇧, and ⇧.: the next speed of film.Rates slower (steps < 0) or faster, staying at the slowest and the fastest.
func (a *app) playerStepRate(steps int) {
	rates := film.Rates[:]
	i := slices.Index(rates, playerNearestRate(a.player.rate)) + steps
	a.SetRate(rates[min(max(i, 0), len(rates)-1)])
}

// playerApplyRate has the open film's playhead move at the speed the system's player plays it at from where it is (playerSpeed, playerVideo).
func (a *app) playerApplyRate() {
	if p := a.player.play; p != nil {
		p.setRate(a.playerSpeed())
	}
}

// playerNearestRate is the speed of film.Rates nearest rate, the slower of two as near, a positive rate off either end being the end; a rate that is not a speed, not a number or not above 0, is 1.
func playerNearestRate(rate float64) float64 {
	if math.IsNaN(rate) || rate <= 0 {
		return 1
	}
	rate = min(max(rate, film.Rates[0]), film.Rates[len(film.Rates)-1])
	best := film.Rates[0]
	for _, r := range film.Rates {
		if math.Abs(r-rate) < math.Abs(best-rate) {
			best = r
		}
	}
	return best
}
