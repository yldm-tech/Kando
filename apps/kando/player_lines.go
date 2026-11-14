package main

import (
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/live"
	"kando/packages/core"
)

// The player's moves by subtitle line: previous, next and replay, and the loop that plays one line over and over. Lines are in the subtitles' time; the picture is the subtitle delay (filmUI.delay) later.

// playerLineRewind is how far into a line Previous Line goes back to that line's start rather than to the line before, as a music player's previous track does.
const playerLineRewind = time.Second

// playerLoopShortest is the least of the video a loop plays before it goes back.
const playerLoopShortest = 250 * time.Millisecond

// playerCues is the open film's subtitle lines, or nil while its subtitles are loading, failed to load or have none.
func (a *app) playerCues() []core.Segment {
	subs := a.player.subs
	if a.openKey == "" || subs.key != a.openKey || !subs.ready || subs.err != "" {
		return nil
	}
	return subs.cues
}

// playerLineAt is where the picture is, in the subtitles' time.
func (a *app) playerLineAt() time.Duration { return a.PlayHead() - a.player.film.delay }

// PrevLine is A: back to the start of the line playing once it has played for more than playerLineRewind, else to the start of the line before it. Between lines it goes to the start of the line just heard.
func (a *app) PrevLine() {
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	at := a.playerLineAt()
	prev, cur, _ := film.CueNear(cues, at)
	switch {
	case cur >= 0 && (at-cues[cur].Start > playerLineRewind || prev < 0):
		a.playerGoToLine(cues, cur)
	case prev >= 0:
		a.playerGoToLine(cues, prev)
	default:
		a.flash(a.messages().FilmNoEarlierLine())
	}
}

// NextLine is D: on to the start of the next line.
func (a *app) NextLine() {
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	if _, _, next := film.CueNear(cues, a.playerLineAt()); next >= 0 {
		a.playerGoToLine(cues, next)
		return
	}
	a.flash(a.messages().FilmNoLaterLine())
}

// ReplayLine is R: the line playing again from its start, or between lines the line just heard.
func (a *app) ReplayLine() {
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	prev, cur, _ := film.CueNear(cues, a.playerLineAt())
	if cur < 0 {
		cur = prev
	}
	if cur < 0 {
		a.flash(a.messages().FilmNoEarlierLine())
		return
	}
	a.playerGoToLine(cues, cur)
}

// LoopLine reports whether the player loops a line of the open film, for the transport's toggle and the menu bar's check mark. A loop made for a film that is no longer open does not count (playerLooping drops it).
func (a *app) LoopLine() bool {
	loop := a.player.film.loop
	return loop.on && loop.key == a.openKey
}

// playerLooping reports whether the player loops a line of the open film, and drops a loop left over from a film opened before, as when another film opened playing (OpenAt), which keeps the film's state as a seek does, or the same film opened again from elsewhere.
func (a *app) playerLooping() bool {
	if a.player.film.loop.on && !a.LoopLine() {
		a.player.film.loop = playerLoop{}
	}
	return a.player.film.loop.on
}

// ToggleLoopLine is L: it loops the line playing, or between lines the one just heard, as R replays it, or before the first line the one coming, or stops looping. Looping is the open film's only and is not saved.
func (a *app) ToggleLoopLine() {
	if a.playerLooping() {
		a.player.film.loop = playerLoop{}
		a.flash(a.messages().FilmLoopOff())
		return
	}
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	prev, cur, next := film.CueNear(cues, a.playerLineAt())
	switch {
	case cur >= 0:
		a.player.film.loop = playerLoopOf(a.openKey, cues, cur)
	case prev >= 0:
		// Between lines, and after the last, the picture goes back to the line just heard.
		a.player.film.loop = playerLoopOf(a.openKey, cues, prev)
		a.seekPlayback(max(cues[prev].Start+a.player.film.delay, 0))
	default:
		// Before the first line, the picture plays on into it and loops it from there.
		a.player.film.loop = playerLoopOf(a.openKey, cues, next)
	}
	a.flash(a.messages().FilmLoopOn())
}

// playerGoToLine plays from the start of line i and says which line it is.
func (a *app) playerGoToLine(cues []core.Segment, i int) {
	a.playerSeek(max(cues[i].Start+a.player.film.delay, 0))
	a.flash(a.messages().FilmLineOf(i+1, len(cues)))
}

// playerSeek plays from at, picked by the user: a click on the scrubber or a line, a skip, a move by line. The loop follows it (playerLoopFollow). The loop's own return to its start seeks directly (playerKeepLoop).
func (a *app) playerSeek(at time.Duration) {
	a.playerLoopFollow(at)
	a.seekPlayback(at)
}

// playerLoopOpening is what opening film key playing from at (OpenAt) does to the loop: in the open film, as a line picked in the command palette, the loop follows at as a seek has it; any other film starts without one.
func (a *app) playerLoopOpening(key string, at time.Duration) {
	if key != a.openKey {
		a.player.film.loop = playerLoop{}
		return
	}
	a.playerLoopFollow(at)
}

// playerLoopFollow moves the loop to the line at at, where the picture is going, or the line coming when at is between lines, and stops it after the last line, so that going elsewhere does not bring the picture back to the line looped before. A move within the looped line keeps it.
func (a *app) playerLoopFollow(at time.Duration) {
	if !a.playerLooping() {
		return
	}
	loop := &a.player.film.loop
	line := at - a.player.film.delay
	if line >= loop.start && (loop.end == 0 || line < loop.end) {
		return
	}
	cues := a.playerCues()
	_, cur, next := film.CueNear(cues, line)
	switch {
	case cur >= 0:
		*loop = playerLoopOf(a.openKey, cues, cur)
	case next >= 0:
		*loop = playerLoopOf(a.openKey, cues, next)
	default:
		*loop = playerLoop{}
	}
}

// playerLoopOf is the loop of line i of film key: from its start to its end, or, for a line without an end, to the start of the line after it or the end of the video.
func playerLoopOf(key string, cues []core.Segment, i int) playerLoop {
	loop := playerLoop{on: true, key: key, start: cues[i].Start, end: cues[i].End}
	if loop.end > loop.start {
		return loop
	}
	loop.end = 0
	for _, cue := range cues[i+1:] {
		if cue.Start > loop.start {
			loop.end = cue.Start
			break
		}
	}
	return loop
}

// playerKeepLoop takes a playing film back to the start of the looped line once it reaches the line's end, or the video's end; a timer set for the line's end (playerLoopSchedule), the system's player saying where it is (playerProgress) and reaching the end (playerEnded) run it, in the background too, where it plays on. A held film stays where it is. The return is not the user's seek, so where the film continues from is not written again on every pass (playerKeep). A line the subtitle delay has pushed so near the video's end or start that less than playerLoopShortest of it is in the video cannot loop, and looping stops, rather than go back to the same place over and over.
func (a *app) playerKeepLoop() {
	play := a.player.play
	if !a.playerLooping() || play == nil || !play.loaded || a.openKey == "" {
		return
	}
	loop, delay := a.player.film.loop, a.player.film.delay
	start := max(loop.start+delay, 0)
	end := loop.end + delay
	if play.duration > 0 && start > play.duration-playerLoopShortest || loop.end > 0 && end < start+playerLoopShortest {
		a.player.film.loop = playerLoop{}
		a.flash(a.messages().FilmLoopOff())
		return
	}
	switch {
	case play.finished:
	case play.paused:
		return
	case loop.end > 0 && play.playhead() >= end:
	default:
		return
	}
	a.seekTo(start, false)
}

// playerLoopLate is how long after the looped line's end, by the app's clock, its timer runs, so the playhead has reached the end when playerKeepLoop looks, whatever the rounding of the film's time at its speed.
const playerLoopLate = time.Millisecond

// playerLoopSchedule times the return of a playing film to the start of the looped line (playerKeepLoop) for when it reaches the line's end, so it goes back on time rather than at the system's player's next report, up to a quarter second later. A timer already set for that moment stays; a held film, or one with no line looped or a loop to the video's end, which the player's end takes back, has none. The app's changes (playerAfterChange) and the player's reports (playerProgress) run it, as they move the playhead, the speed or the loop; without the app's scope, as in tests that start no app, the reports alone take the film back.
func (a *app) playerLoopSchedule() {
	if a.cx == nil {
		return
	}
	play := a.player.play
	if play == nil || !play.Playing() || a.openKey == "" || !a.playerLooping() || a.player.film.loop.end == 0 {
		if a.player.loopTimer != nil {
			a.player.loopTimer.Stop()
		}
		return
	}
	end := a.player.film.loop.end + a.player.film.delay
	wait := playerWallTime(max(end-play.playhead(), 0), play.speed()) + playerLoopLate
	if a.player.loopTimer == nil {
		a.player.loopTimer = film.NewTimer(a.cx)
	}
	// The playhead runs on the app's clock between reports, so the same end is the same moment until a report, a seek or the speed moves it; a moment within playerLoopNear of the one set keeps the timer.
	if at := a.player.loopTimer.At(); !at.IsZero() && (at.Sub(a.playerNow().Add(wait))).Abs() < playerLoopNear {
		return
	}
	a.player.loopTimer.Set(wait, live.Do(a, func() {
		a.playerKeepLoop()
		a.playerLoopSchedule()
	}))
}

// playerLoopNear is how far the moment the looped line ends may move before its timer is set again (playerLoopSchedule).
const playerLoopNear = 20 * time.Millisecond
