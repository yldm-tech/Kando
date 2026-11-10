package main

import (
	"sync/atomic"
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
		a.player.film.loop = playerLoopOf(a.openKey, cues, cur)
	case prev >= 0:
		// Between lines, and after the last, the picture goes back to the line just heard.
		a.player.film.loop = playerLoopOf(a.openKey, cues, prev)
		a.seekPlayback(max(cues[prev].Start+a.player.film.delay, 0))
	default:
		// Before the first line, the picture plays on into it and loops it from there.
		a.player.film.loop = playerLoopOf(a.openKey, cues, next)
	}
	a.flash(locales.FilmLoopOn())
}

// playerGoToLine plays from the start of line i and says which line it is.
func (a *app) playerGoToLine(cues []core.Segment, i int) {
	a.playerSeek(max(cues[i].Start+a.player.film.delay, 0))
	a.flash(locales.FilmLineOf(i+1, len(cues)))
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

// playerKeepLoop takes a playing film back to the start of the looped line once its sound reaches the line's end, or the video's end. The sound is what counts: while the window is out of sight the picture stands still and the sound plays on (playback_background.go), and playerLoopAway calls this then, as no frames are built. A held film stays where it is. The seek opens the video again, so a.player.play is nil until it has, and this does not seek twice; the return does not count as stopping there, so where the film continues from is not written again on every pass. A line the subtitle delay has pushed so near the video's end or start that less than playerLoopShortest of it is in the video cannot loop, and looping stops, rather than open the video again at the same place over and over.
func (a *app) playerKeepLoop() {
	play := a.player.play
	if !a.playerLooping() || play == nil || a.openKey == "" {
		return
	}
	loop, delay := a.player.film.loop, a.player.film.delay
	start := max(loop.start+delay, 0)
	end := loop.end + delay
	if play.duration > 0 && start > play.duration-playerLoopShortest || loop.end > 0 && end < start+playerLoopShortest {
		a.player.film.loop = playerLoop{}
		a.flash(locales.FilmLoopOff())
		return
	}
	away := !play.backgroundAt.IsZero()
	head := play.soundAt(time.Now())
	switch {
	case play.finished:
	case play.paused:
		return
	case loop.end > 0 && head >= end:
	case loop.end == 0 && away && play.duration > 0 && head >= play.duration-backgroundDrift:
		// The sound clock in the background stops just short of the video's end (soundClock), where a last line without an end ends.
	default:
		return
	}
	play.played = false
	a.seekPlayback(start)
}

// playerLoopAway keeps a looped line looping while the window is out of sight, the app inactive or the main window put away (playerSight), when the window may build no frames to run playerKeepLoop after: it checks every backgroundPoll until playerLoopBack. A film that does not loop as the window goes out of sight has nothing to check.
func (a *app) playerLoopAway() {
	a.playerLoopBack()
	if !a.LoopLine() || a.update == nil {
		return
	}
	stop := make(chan struct{})
	a.player.loopAway = stop
	update := a.update
	go func() {
		tick := time.NewTicker(backgroundPoll)
		defer tick.Stop()
		// One check waits on the UI thread at a time, so a busy thread is not handed a pile of them.
		var waiting atomic.Bool
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if waiting.CompareAndSwap(false, true) {
					update(func() {
						waiting.Store(false)
						a.playerKeepLoop()
					})
				}
			}
		}
	}()
}

// playerLoopBack stops the checks playerLoopAway started, as the window is back in sight and builds frames again.
func (a *app) playerLoopBack() {
	if a.player.loopAway != nil {
		close(a.player.loopAway)
		a.player.loopAway = nil
	}
}
