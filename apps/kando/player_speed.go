package main

import (
	"math"
	"slices"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
)

// Rate is how fast films play, one of film.Rates, for the transport's speed button (film.App).
func (a *app) Rate() float64 { return a.player.rate }

// SpeedMenu is whether the transport's speed menu is open.
func (a *app) SpeedMenu() *bool { return &a.player.film.speedMenu }

// SetRate plays films at rate, the nearest of film.Rates, says so on the picture and keeps it in the settings. The open film takes it up at once (playerApplyRate).
func (a *app) SetRate(rate float64) {
	rate = playerNearestRate(rate)
	a.flash(locales.PlayerOsdSpeed(film.RateText(rate)))
	if rate == a.player.rate {
		return
	}
	a.player.rate = rate
	a.playerApplyRate()
	if err := a.saveSettings(); err != nil {
		a.flash(locales.PlayerSaveSpeedFailed())
	}
}

// playerStepRate is ⇧, and ⇧.: the next speed of film.Rates slower (steps < 0) or faster, staying at the slowest and the fastest.
func (a *app) playerStepRate(steps int) {
	rates := film.Rates[:]
	i := slices.Index(rates, playerNearestRate(a.player.rate)) + steps
	a.SetRate(rates[min(max(i, 0), len(rates)-1)])
}

// playerApplyRate has the open film play at the player's speed. A film whose sound plays on while its picture may stand still, in the background or just back from it, keeps the speed its sound started at until its picture has caught up, as the clock that keeps the two level counts at that speed; the hook after each frame (playerAfterFrame) brings it to the player's speed then.
func (a *app) playerApplyRate() {
	p := a.player.play
	if p == nil || !p.backgroundAt.IsZero() || p.returning != nil {
		return
	}
	p.setRate(a.player.rate)
}

// playerNearestRate is the speed of film.Rates nearest rate, the slower of two as near; a rate that is not a number is 1.
func playerNearestRate(rate float64) float64 {
	if math.IsNaN(rate) {
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
