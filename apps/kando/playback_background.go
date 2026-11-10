package main

import (
	"image"
	"sync/atomic"
	"time"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// The sound of a film plays on while the app is in the background, since mygo/audio mixes it on a thread of its own, but VideoPlayer stops the picture while the window is out of sight. This file keeps the two level: while the picture stands still behind the window, a second decoder follows the sound, and coming back hands its picture over at once instead of showing the frame the window was hidden on and then jumping.

// backgroundDrift is how far the picture may trail the sound before it counts as standing still. The picture of a window that stayed in sight keeps pace with the sound, give or take the frames decoded ahead, and opening it again would only make it jump.
const backgroundDrift = 200 * time.Millisecond

// backgroundPoll is how often a film in the background checks whether its picture stopped.
const backgroundPoll = 100 * time.Millisecond

// backgroundPicture is the decoder that follows the sound of a film in the background (followSound).
type backgroundPicture struct {
	// quit drops it; take, when the app came back, has it hand its picture over.
	quit chan struct{}
	take chan struct{}
	// opened is set once its picture stopped and it began decoding where the sound is. Before that the app coming back drops it, and opens the picture again itself if the picture fell behind meanwhile.
	opened atomic.Bool
}

// playerDidResignActive notes that the app went inactive (main's OnDidResignActive).
func (a *app) playerDidResignActive() { a.playerSight(func() { a.player.inactive = true }) }

// playerDidBecomeActive notes that the app is the active one again (main's OnDidBecomeActive).
func (a *app) playerDidBecomeActive() { a.playerSight(func() { a.player.inactive = false }) }

// playerWindowHidden notes that the main window was put away by its close button, or shown again (playerHooks.windowShown).
func (a *app) playerWindowHidden(hidden bool) {
	a.playerSight(func() { a.player.windowHidden = hidden })
}

// playerSight runs change, which marks the app inactive or active or the main window hidden or shown, and follows the sound of the playing film from when the window went out of sight, the app inactive or the window put away, until it is in sight again, both active and shown. A second reason to be out of sight while the first lasts changes nothing, so the sound's clock stays the one taken as the picture first went out of sight.
func (a *app) playerSight(change func()) {
	away := a.player.inactive || a.player.windowHidden
	change()
	switch now := a.player.inactive || a.player.windowHidden; {
	case now && !away:
		a.playerLeftSight()
	case !now && away:
		a.playerBackInSight()
	}
}

// playerLeftSight notes the clock of the playing film as the window goes out of sight and starts following its sound.
func (a *app) playerLeftSight() {
	p := a.player.play
	if p == nil || p.finished || p.paused || !p.player.Playing() {
		return
	}
	p.stopBackground()
	p.backgroundAt = time.Now()
	p.backgroundHead = p.playhead()
	p.background = a.followSound(p)
}

// playerBackInSight puts the picture where the sound is, when it stood still behind the window while the sound played on: the picture decoded in the background when there is one, or else the picture opened again from there. A picture that kept pace is left alone.
func (a *app) playerBackInSight() {
	p := a.player.play
	if p == nil || p.backgroundAt.IsZero() {
		return
	}
	p.unseen = false
	sound := p.backgroundSound()
	bg := p.background
	p.backgroundAt, p.backgroundHead, p.background = time.Time{}, 0, nil
	if bg != nil && bg.opened.Load() {
		p.returning = sound
		close(bg.take)
		return
	}
	if bg != nil {
		close(bg.quit)
	}
	if p.paused || p.finished || p.path == "" || sound()-p.playhead() < backgroundDrift {
		return
	}
	p.returning = sound
	a.player.playFrom = sound()
	a.player.playJump, a.player.playSame = true, true
	a.startPlaybackWith(p.path, p, sound)
}

// stopBackground drops the decoder following the sound of p, if any.
func (p *filmPlay) stopBackground() {
	if p.background != nil {
		close(p.background.quit)
		p.background = nil
	}
}

// backgroundSound is where the sound of p is at any moment since the app went inactive.
func (p *filmPlay) backgroundSound() func() time.Duration {
	head, since, length := p.backgroundHead, p.backgroundAt, p.duration
	return func() time.Duration { return soundClock(head, since, length, time.Now()) }
}

// soundClock is where a sound that was at head at since is at now, held short of the end of a film length long (0 when not known).
func soundClock(head time.Duration, since time.Time, length time.Duration, now time.Time) time.Duration {
	at := head + max(now.Sub(since), 0)
	if length > 0 && at >= length {
		at = max(0, length-100*time.Millisecond)
	}
	return at
}

// followSound watches, off the UI thread, whether the picture of p falls behind its sound, which happens only while the window is out of sight. Once it does, it opens a decoder where the sound is and keeps it on the frame the sound is in, until the app comes back (take) and the picture goes to the UI thread, or p is dropped (quit). A film without ffmpeg has nothing to follow.
func (a *app) followSound(p *filmPlay) *backgroundPicture {
	if p.ffmpeg == "" || p.path == "" || p.frames == nil || a.update == nil {
		return nil
	}
	bg := &backgroundPicture{quit: make(chan struct{}), take: make(chan struct{})}
	sound := p.backgroundSound()
	shown, ffmpeg, path, update, gen := p.frames, p.ffmpeg, p.path, a.update, a.player.playGen
	go func() {
		tick := time.NewTicker(backgroundPoll)
		for sound()-shown.position() < backgroundDrift {
			select {
			case <-bg.quit:
				tick.Stop()
				return
			case <-bg.take:
				tick.Stop()
				return
			case <-tick.C:
			}
		}
		tick.Stop()
		bg.opened.Store(true)
		// The window is out of sight and only the sound plays on, which needs no display awake (playerKeepAwake).
		update(func() {
			if a.player.play == p && p.background == bg {
				p.unseen = true
				a.playerKeepAwake()
			}
		})
		// Without a picture to hand over, the app coming back leaves the picture where it stood, and the sound clock with it.
		fail := func() {
			select {
			case <-bg.quit:
			case <-bg.take:
				update(func() { p.returning = nil })
			}
		}
		at := sound()
		frames, err := media.OpenFramesAt(ffmpeg, path, at)
		if err != nil {
			fail()
			return
		}
		img, dur, err := frames.Next()
		if err != nil {
			frames.Close()
			fail()
			return
		}
		ended := false
		wait := time.NewTimer(0)
		defer wait.Stop()
		for {
			// Move on to the frame the sound is in. The reader draws every frame into the same image, so the frame held is read last.
			for !ended && at+dur <= sound() {
				next, d, err := frames.Next()
				if err != nil {
					ended = true
					break
				}
				at, img, dur = at+dur, next, d
			}
			due := time.Hour
			if !ended {
				due = at + dur - sound()
			}
			wait.Reset(due)
			select {
			case <-bg.quit:
				frames.Close()
				return
			case <-bg.take:
				update(func() { a.takeBackgroundPicture(p, gen, frames, img, at, dur) })
				return
			case <-wait.C:
			}
		}
	}()
	return bg
}

// takeBackgroundPicture shows, in place of p's picture that stood still, the picture decoded in the background: frames, standing on img, which began at at and lasts dur. The new picture takes p's sound over.
func (a *app) takeBackgroundPicture(p *filmPlay, gen int, frames *media.FrameReader, img image.Image, at, dur time.Duration) {
	sound := p.returning
	p.returning = nil
	if a.player.playGen != gen || a.player.play != p || p.paused || p.finished {
		frames.Close()
		return
	}
	dec := &frameDecoder{frames: frames, shown: at, held: &ui.VideoFrame{Image: img, Duration: dur}, sound: sound}
	player, err := ui.NewVideoPlayer(dec)
	if err != nil {
		frames.Close()
		return
	}
	player.Play()
	audio := p.audio
	p.audio = nil
	a.playReplace(&filmPlay{
		player: player, frames: dec, audio: audio, ffmpeg: p.ffmpeg, path: p.path,
		duration: p.duration, played: true, volume: p.volume,
	})
}

// playReplace puts next in place of the film playing and closes that one, but for the sound next took over from it, and shows next where the system shows what plays. When the app went inactive again while next opened, next follows its sound from here, level with it now.
func (a *app) playReplace(next *filmPlay) {
	old := a.player.play
	a.player.play = next
	a.playerNowPlayingShow(next)
	if old == nil {
		return
	}
	if !old.backgroundAt.IsZero() && next.Playing() {
		next.backgroundAt, next.backgroundHead = time.Now(), next.playhead()
		next.background = a.followSound(next)
	}
	old.stopBackground()
	if old.audio != nil {
		old.audio.Close()
	}
	old.player.Close()
}

// skipPlaybackFrames reads frames for d, so the reader stands on where the sound got to while it opened.
func skipPlaybackFrames(frames *media.FrameReader, d time.Duration) error {
	for d > 0 {
		_, dur, err := frames.Next()
		if err != nil {
			return err
		}
		d -= dur
	}
	return nil
}
