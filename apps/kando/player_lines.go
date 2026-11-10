package main

import (
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
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
		a.flash(locales.FilmNoLines())
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
		a.flash(locales.FilmNoEarlierLine())
	}
}

// NextLine is D: on to the start of the next line.
func (a *app) NextLine() {
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(locales.FilmNoLines())
		return
	}
	if _, _, next := film.CueNear(cues, a.playerLineAt()); next >= 0 {
		a.playerGoToLine(cues, next)
		return
	}
	a.flash(locales.FilmNoLaterLine())
}

// ReplayLine is R: the line playing again from its start, or between lines the line just heard.
func (a *app) ReplayLine() {
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(locales.FilmNoLines())
		return
	}
	prev, cur, _ := film.CueNear(cues, a.playerLineAt())
	if cur < 0 {
		cur = prev
	}
	if cur < 0 {
		a.flash(locales.FilmNoEarlierLine())
		return
	}
	a.playerGoToLine(cues, cur)
}

// LoopLine reports whether the player loops a line, for the transport's toggle.
func (a *app) LoopLine() bool { return a.player.film.loop.on }

// ToggleLoopLine is L: it loops the line playing, or between lines the one coming, or after the last line that one, or stops looping. Looping is the open film's only and is not saved.
func (a *app) ToggleLoopLine() {
	if a.player.film.loop.on {
		a.player.film.loop = playerLoop{}
		a.flash(locales.FilmLoopOff())
		return
	}
	cues := a.playerCues()
	if len(cues) == 0 {
		a.flash(locales.FilmNoLines())
		return
	}
	prev, cur, next := film.CueNear(cues, a.playerLineAt())
	switch {
	case cur >= 0:
		a.player.film.loop = playerLoopOf(cues, cur)
	case next >= 0:
		// The picture plays on into the line coming and loops it from there.
		a.player.film.loop = playerLoopOf(cues, next)
	default:
		// After the last line, the picture goes back to it.
		a.player.film.loop = playerLoopOf(cues, prev)
		a.seekPlayback(max(cues[prev].Start+a.player.film.delay, 0))
	}
	a.flash(locales.FilmLoopOn())
}

// playerGoToLine plays from the start of line i and says which line it is.
func (a *app) playerGoToLine(cues []core.Segment, i int) {
	a.playerSeek(max(cues[i].Start+a.player.film.delay, 0))
	a.flash(locales.FilmLineOf(i+1, len(cues)))
}

// playerSeek plays from at, picked by the user: a click on the scrubber or a line, a skip, a move by line. A loop moves with it to the line at at, or the line coming when at is between lines, and stops after the last line, so that going elsewhere does not bring the picture back to the line looped before. The loop's own return to its start seeks directly (playerKeepLoop).
func (a *app) playerSeek(at time.Duration) {
	if loop := &a.player.film.loop; loop.on {
		line := at - a.player.film.delay
		if line < loop.start || loop.end > 0 && line >= loop.end {
			cues := a.playerCues()
			_, cur, next := film.CueNear(cues, line)
			switch {
			case cur >= 0:
				*loop = playerLoopOf(cues, cur)
			case next >= 0:
				*loop = playerLoopOf(cues, next)
			default:
				*loop = playerLoop{}
			}
		}
	}
	a.seekPlayback(at)
}

// playerLoopOf is the loop of line i: from its start to its end, or, for a line without an end, to the start of the line after it or the end of the video.
func playerLoopOf(cues []core.Segment, i int) playerLoop {
	loop := playerLoop{on: true, start: cues[i].Start, end: cues[i].End}
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

// playerKeepLoop takes a playing picture back to the start of the looped line once it reaches the line's end, or the video's end. A held picture stays where it is. The seek opens the video again, so a.player.play is nil until it has, and this does not seek twice. A line that starts within playerLoopShortest of the video's end, or after it as with a large delay, cannot loop, and looping stops, rather than open the video again at its end over and over.
func (a *app) playerKeepLoop() {
	loop, play := a.player.film.loop, a.player.play
	if !loop.on || play == nil || a.openKey == "" {
		return
	}
	start := max(loop.start+a.player.film.delay, 0)
	if play.duration > 0 && start > play.duration-playerLoopShortest {
		a.player.film.loop = playerLoop{}
		return
	}
	switch {
	case play.finished:
	case play.paused:
		return
	case loop.end > 0 && play.playhead() >= loop.end+a.player.film.delay:
	default:
		return
	}
	a.seekPlayback(start)
}
