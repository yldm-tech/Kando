package main

import (
	"math"
	"slices"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
)

// Rate is how fast films play, one of film.Rates, for the transport's speed button (film.App).
func (a *app) Rate() float64 { return a.player.rate }

// SpeedMenu is whether the transport's speed menu is open.
func (a *app) SpeedMenu() *bool { return &a.player.film.speedMenu }

// SetRate plays films at rate, the nearest of film.Rates, says so on the picture and on the speed button at once, and has the open film take it up and the settings keep it once the speed has rested (playerRateChanged).
func (a *app) SetRate(rate float64) {
	rate = playerNearestRate(rate)
	a.flash(locales.PlayerOsdSpeed(film.RateText(rate)))
	if rate == a.player.rate {
		return
	}
	a.player.rate = rate
	a.playerRateChanged()
}

// playerRateSettle is how long the speed has to stay put before the sound starts again at it.
const playerRateSettle = 250 * time.Millisecond

// playerRateChanged has the open film play at the new speed and keeps it in the settings, once the speed has rested for playerRateSettle: the sound plays in a process that takes its speed only when it starts, so a key held down waits until it rests instead of starting the sound again at every step. Picture and sound change together, so the two stay level meanwhile. Without an update, as before the window exists, it does so at once.
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
	a.playerApplyRate()
	if err := a.saveSettings(); err != nil {
		a.flash(locales.PlayerSaveSpeedFailed())
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

// playerApplyRate has the open film play at the player's speed, once a speed changed has rested (playerRateChanged). A film whose sound plays on while its picture may stand still, in the background or just back from it, keeps the speed its sound started at until its picture has caught up, as the clock that keeps the two level counts at that speed; the hook after each frame (playerAfterFrame) brings it to the player's speed then.
func (a *app) playerApplyRate() {
	p := a.player.play
	if p == nil || a.player.rateSettling || !p.backgroundAt.IsZero() || p.returning != nil {
		return
	}
	p.setRate(a.player.rate)
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
