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

// Peek is the index of the line the picture shows both lines of, whatever the subtitle mode, or -1 (PeekLine).
func (a *app) Peek() int { return a.player.film.peek - 1 }

// PeekLine is T: the line playing shows on the picture with both its lines, the translation and the original in their order, whatever the subtitle mode and with the subtitles off as well, so a learner listening to the original alone can check one line without changing the mode for the whole film. It lasts while that line plays, until the user moves the picture, and T again hides it. Between lines there is no line to show.
func (a *app) PeekLine() {
	if a.player.film.peek != 0 {
		a.player.film.peek = 0
		return
	}
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	if _, cur, _ := film.CueNear(cues, a.playerLineAt()); cur >= 0 {
		a.player.film.peek = cur + 1
	}
}

// LoopSpan is the span the player loops in the open film, in the subtitles' time, end 0 for the video's end, for the scrubber and the subtitle list to show; ok is false while it loops none.
func (a *app) LoopSpan() (start, end time.Duration, ok bool) {
	if !a.LoopLine() {
		return 0, 0, false
	}
	loop := a.player.film.loop
	return loop.start, loop.end, true
}

// ExtendLoop is ⇧L: the loop takes in the line after it as well, so a whole sentence or exchange plays over and over; without a loop it loops the line L would. Moves within the span (A, D, R, a click on one of its lines) keep it (playerLoopFollow). A loop that runs to the video's end has no line after it.
func (a *app) ExtendLoop() {
	if !a.playerLooping() {
		a.ToggleLoopLine()
		return
	}
	cues := a.playerCues()
	loop := &a.player.film.loop
	for i, cue := range cues {
		if cue.Start < loop.start || loop.end == 0 {
			continue
		}
		if end := playerLoopOf(a.openKey, cues, i).end; end == 0 || end > loop.end {
			loop.end = end
			a.playerFlashLoop(cues)
			return
		}
	}
	a.flash(a.messages().FilmNoLaterLine())
}

// LoopThrough is a click on line i of the subtitle list with Shift: the loop runs on through that line, or, for a line before it, from that line, so a span of lines plays over and over. Without a loop it loops from the line L would loop through line i, and the picture goes to the span's start when it is outside it.
func (a *app) LoopThrough(i int) {
	cues := a.playerCues()
	if i < 0 || i >= len(cues) {
		return
	}
	line := playerLoopOf(a.openKey, cues, i)
	loop := a.player.film.loop
	if !a.playerLooping() {
		// The line L would loop: the one playing, between lines the one just heard, before the first the one coming.
		prev, from, next := film.CueNear(cues, a.playerLineAt())
		if from < 0 {
			from = prev
		}
		if from < 0 {
			from = next
		}
		loop = playerLoopOf(a.openKey, cues, from)
	}
	loop.start = min(loop.start, line.start)
	if loop.end != 0 && (line.end == 0 || line.end > loop.end) {
		loop.end = line.end
	}
	a.player.film.loop = loop
	if at := a.playerLineAt(); at < loop.start || loop.end != 0 && at >= loop.end {
		a.seekPlayback(max(loop.start+a.player.film.delay, 0))
	}
	a.playerFlashLoop(cues)
}

// playerFlashLoop says on the picture which lines loop: Loop Line: On for one, else the first and the last by their numbers.
func (a *app) playerFlashLoop(cues []core.Segment) {
	loop := a.player.film.loop
	from, to := -1, -1
	for i, cue := range cues {
		if cue.Start >= loop.start && (loop.end == 0 || cue.Start < loop.end) {
			if from < 0 {
				from = i
			}
			to = i
		}
	}
	if from < 0 || from == to {
		a.flash(a.messages().FilmLoopOn())
		return
	}
	a.flash(a.messages().FilmLoopLines(from+1, to+1))
}

// playerGoToLine plays from the start of line i and says which line it is.
func (a *app) playerGoToLine(cues []core.Segment, i int) {
	a.playerSeek(max(cues[i].Start+a.player.film.delay, 0))
	a.flash(a.messages().FilmLineOf(i+1, len(cues)))
}

// playerSeek plays from at, picked by the user: a click on the scrubber or a line, a skip, a move by line. The loop follows it (playerLoopFollow). The loop's own return to its start seeks directly (playerKeepLoop).
func (a *app) playerSeek(at time.Duration) {
	a.player.film.jumps++
	a.player.film.peek = 0
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
		// A looped line's gap is over: it plays again.
		if stop := a.player.film.stop; !a.playerStopped(play) || stop.resume.IsZero() || a.playerNow().Before(stop.resume) {
			return
		}
	case loop.end > 0 && play.playhead() >= end:
		// The line stops at its end once each pass: Pause After Each Line pauses it until Space, and Pause Between Repeats holds it for as long as it played. Played on past the stop, it goes back.
		if a.playerStopped(play) {
			break
		}
		switch {
		case a.player.pauseLines:
			a.playerStopAt(play, loop.end, time.Time{})
			return
		case a.player.loopGap == playerLoopGapLine:
			a.playerStopAt(play, loop.end, a.playerNow().Add(playerWallTime(loop.end-loop.start, play.speed())))
			return
		}
	default:
		return
	}
	a.seekTo(start, false)
}

// playerLoopGapLine is Pause Between Repeats: a looped line holds after each pass for as long as it played, so the learner can say it after it, and then plays again by itself.
const playerLoopGapLine = "line"

// PauseLines reports whether the player pauses at the end of each line (P), for the transport's toggle and the menu bar's check mark.
func (a *app) PauseLines() bool { return a.player.pauseLines }

// TogglePauseLines is P: the film pauses as each subtitle line ends, for shadowing it or writing it down, and Space plays on to the next one's end; a looped line pauses before each repeat. It says so on the picture and is kept in the settings.
func (a *app) TogglePauseLines() {
	a.player.pauseLines = !a.player.pauseLines
	a.player.film.stop = playerStop{}
	if a.player.pauseLines {
		a.flash(a.messages().FilmPauseLinesOn())
	} else {
		a.flash(a.messages().FilmPauseLinesOff())
	}
	a.playerSaveSettings()
}

// playerLoopGap reports whether a looped line holds after each pass (Pause Between Repeats), for the menu bar's check mark.
func (a *app) playerLoopGap() bool { return a.player.loopGap == playerLoopGapLine }

// playerToggleLoopGap is the Playback menu's Pause Between Repeats, kept in the settings.
func (a *app) playerToggleLoopGap() {
	if a.playerLoopGap() {
		a.player.loopGap = ""
		a.flash(a.messages().FilmLoopGapOff())
	} else {
		a.player.loopGap = playerLoopGapLine
		a.flash(a.messages().FilmLoopGapOn())
	}
	a.player.film.stop = playerStop{}
	a.playerSaveSettings()
}

// playerStopped reports whether the film stopped at a line's end on the pass through it that play is on now (filmUI.stop).
func (a *app) playerStopped(play *filmPlay) bool {
	stop := a.player.film.stop
	return stop.on && stop.seeks == play.seeks
}

// playerInGap reports whether play holds in a looped line's gap, a pause of the app's own rather than the user's.
func (a *app) playerInGap(play *filmPlay) bool {
	return play != nil && a.playerStopped(play) && !a.player.film.stop.resume.IsZero()
}

// playerStopAt pauses a playing film at the end of the line ending at end, in the subtitles' time, until resume, or until the user plays it on when resume is zero.
func (a *app) playerStopAt(play *filmPlay, end time.Duration, resume time.Time) {
	if play.Playing() {
		play.toggle()
	}
	a.player.film.stop = playerStop{on: true, seeks: play.seeks, end: end, resume: resume}
}

// playerPauseAtLineEnd pauses a playing film once it reaches the end of the line Pause After Each Line waits for (filmUI.lineEnd), which playerTimerWait picks; a looped line stops in playerKeepLoop instead. It reports whether it paused the film.
func (a *app) playerPauseAtLineEnd() bool {
	play, end := a.player.play, a.player.film.lineEnd
	if !a.player.pauseLines || play == nil || !play.Playing() || end == 0 || a.playerLooping() || play.playhead()-a.player.film.delay < end {
		return false
	}
	if a.playerStopped(play) && a.player.film.stop.end >= end {
		return false
	}
	a.playerStopAt(play, end, time.Time{})
	return true
}

// playerNextLineEnd is the end, in the subtitles' time, of the line play reaches next: the one playing, or between lines the one coming, past the line it stopped at on this pass; 0 when none is left or the line has no end.
func (a *app) playerNextLineEnd(play *filmPlay) time.Duration {
	at := play.playhead() - a.player.film.delay
	if a.playerStopped(play) {
		at = max(at, a.player.film.stop.end)
	}
	for _, cue := range a.playerCues() {
		if cue.End > at {
			return cue.End
		}
	}
	return 0
}

// playerLoopLate is how long after the moment it waits for, a line's end by the app's clock, the player's timer runs, so the playhead has reached it when the timer looks, whatever the rounding of the film's time at its speed.
const playerLoopLate = time.Millisecond

// playerLoopSchedule times the player's stops at a line's end for when the film reaches it, so they come on time rather than at the system's player's next report, up to a quarter second later: the return of a looped line to its start (playerKeepLoop), Pause After Each Line pausing the film (playerPauseAtLineEnd) and the end of a looped line's gap. A timer already set for that moment stays; with nothing to wait for there is none (playerTimerWait). The app's changes (playerAfterChange) and the player's reports (playerProgress) run it, as they move the playhead, the speed or the loop; without the app's scope, as in tests that start no app, the reports alone take the film back.
func (a *app) playerLoopSchedule() {
	if a.cx == nil {
		return
	}
	wait, ok := a.playerTimerWait()
	if !ok {
		if a.player.loopTimer != nil {
			a.player.loopTimer.Stop()
		}
		return
	}
	if a.player.loopTimer == nil {
		a.player.loopTimer = film.NewTimer(a.cx)
	}
	// The playhead runs on the app's clock between reports, so the same end is the same moment until a report, a seek or the speed moves it; a moment within playerLoopNear of the one set keeps the timer.
	if at := a.player.loopTimer.At(); !at.IsZero() && (at.Sub(a.playerNow().Add(wait))).Abs() < playerLoopNear {
		return
	}
	a.player.loopTimer.Set(wait, live.Do(a, func() {
		a.playerKeepLoop()
		a.playerPauseAtLineEnd()
		a.playerLoopSchedule()
	}))
}

// playerTimerWait is how long from now the player's timer waits (playerLoopSchedule): to the end of the looped line, or of its gap while it holds in one; or with Pause After Each Line to the end of the next line, which it keeps as filmUI.lineEnd. ok is false with nothing to wait for: no film playing, no line looped and Pause After Each Line off, or a loop to the video's end, which the player's end takes back.
func (a *app) playerTimerWait() (wait time.Duration, ok bool) {
	play := a.player.play
	a.player.film.lineEnd = 0
	if play == nil || a.openKey == "" {
		return 0, false
	}
	delay := a.player.film.delay
	if a.playerLooping() {
		if play.paused && a.playerInGap(play) {
			return max(a.player.film.stop.resume.Sub(a.playerNow()), 0) + playerLoopLate, true
		}
		end := a.player.film.loop.end
		if !play.Playing() || end == 0 {
			return 0, false
		}
		return playerWallTime(max(end+delay-play.playhead(), 0), play.speed()) + playerLoopLate, true
	}
	if !a.player.pauseLines || !play.Playing() {
		return 0, false
	}
	end := a.playerNextLineEnd(play)
	if end == 0 {
		return 0, false
	}
	a.player.film.lineEnd = end
	return playerWallTime(max(end+delay-play.playhead(), 0), play.speed()) + playerLoopLate, true
}

// playerLoopNear is how far the moment the player's timer waits for may move before it is set again (playerLoopSchedule).
const playerLoopNear = 20 * time.Millisecond
