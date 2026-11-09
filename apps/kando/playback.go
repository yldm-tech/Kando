package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/film"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// filmPlay is the video open on the film page. The picture is decoded by ffmpeg and the sound by ffplay.
type filmPlay struct {
	player   *ui.VideoPlayer
	frames   *frameDecoder
	audio    *pcmAudio
	ffmpeg   string
	path     string
	duration time.Duration
	paused   bool
	// volume is how loud the sound plays, from 0 to 1, muting included (app.playerVolume).
	volume float64
	// backgroundAt and backgroundHead remember a playing film's clock while
	// the app is inactive. The sound keeps playing; on return the picture is
	// reopened at the sound's expected position.
	backgroundAt   time.Time
	backgroundHead time.Duration
	// finished is set once the picture reaches the file's end, so the clock stops there.
	finished bool
	// played is set once this playback has played: opened playing at a point picked for it (a seek, 继续, 从头看) or started with play. stopPlayback keeps where it stopped only then, so opening a film from its poster, which holds it on its first frame, and going back leaves where it continues from as it was.
	played bool
}

func (a *app) ffmpegPath() string {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return ""
	}
	return tools.FFmpeg
}

// startPlayback opens path on the film page. A missing file or a pipeline without ffmpeg leaves the page as a subtitle list.
func (a *app) startPlayback(path string) {
	a.startPlaybackWith(path, nil)
}

// startPlaybackWith opens a new decoder. keep leaves the current player on
// screen until the replacement is ready, which is used when returning from
// the background so the UI does not flash the loading state.
func (a *app) startPlaybackWith(path string, keep *filmPlay) {
	at := a.player.playFrom
	jump := a.player.playJump
	resumeAt := time.Time{}
	resumeHead := time.Duration(0)
	if keep != nil {
		resumeAt = keep.backgroundAt
		resumeHead = keep.backgroundHead
	}
	a.player.playFrom = 0
	a.player.playJump = false
	if keep == nil {
		a.stopPlayback()
	}
	// A seek opens the same video again and keeps the film's state; anything else is a film opening.
	if !jump {
		a.resetFilm()
		a.readFilmInfo(path)
	}
	a.player.opening = at
	ffmpeg := a.ffmpegPath()
	if path == "" || ffmpeg == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	a.player.playGen++
	gen := a.player.playGen
	a.player.playPath = path
	volume := a.playerVolume()
	update := a.update
	go func() {
		openAt := at
		if !resumeAt.IsZero() {
			openAt = resumeHead + time.Since(resumeAt)
		}
		frames, err := media.OpenFramesAt(ffmpeg, path, openAt)
		if err != nil {
			update(func() {
				if a.player.playGen != gen {
					return
				}
				if keep != nil && a.player.play == keep {
					keep.backgroundAt = time.Time{}
					keep.backgroundHead = 0
					return
				}
				a.player.playErr = "这个视频播不了"
			})
			return
		}
		if !resumeAt.IsZero() {
			currentAt := resumeHead + time.Since(resumeAt)
			if currentAt > openAt {
				if err := skipPlaybackFrames(frames, currentAt-openAt); err != nil {
					frames.Close()
					return
				}
				openAt = currentAt
			}
		}
		// A silent film has no sound to play, muted or turned down to 0.
		var audio *pcmAudio
		if volume > 0 {
			audio, _ = openPCM(ffmpeg, path, openAt, volume)
		}
		update(func() {
			if a.player.playGen != gen {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				return
			}
			if keep != nil && a.player.play != keep {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				return
			}
			dec := &frameDecoder{frames: frames, shown: openAt}
			player, err := ui.NewVideoPlayer(dec)
			if err != nil {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				if keep != nil {
					keep.backgroundAt = time.Time{}
					keep.backgroundHead = 0
					return
				}
				a.player.playErr = "这个视频播不了"
				return
			}
			// The volume may have changed while the video was opening. The sound has not started yet, so it is made again at the new volume rather than started and then restarted.
			if now := a.playerVolume(); now != volume {
				if audio != nil {
					audio.Close()
					audio = nil
				}
				if now > 0 {
					audio, _ = openPCM(ffmpeg, path, openAt, now)
				}
				volume = now
			}
			paused := true
			if jump {
				if audio != nil {
					audio.Start()
				}
				player.Play()
				paused = false
			} else {
				player.Pause()
			}
			old := a.player.play
			a.player.play = &filmPlay{
				player: player, frames: dec, audio: audio, ffmpeg: ffmpeg, path: path,
				duration: frames.Duration(), paused: paused, played: jump, volume: volume,
			}
			if old != nil && old != a.player.play {
				old.backgroundAt = time.Time{}
				old.backgroundHead = 0
				if old.audio != nil {
					old.audio.Close()
				}
				old.player.Close()
			}
			if frames.Duration() > 0 {
				a.player.length = frames.Duration()
			}
		})
	}()
}

// readFilmInfo lists the folder of the video at path off the UI thread, for the player's 信息 tab. A film without a video has nothing to list.
func (a *app) readFilmInfo(path string) {
	if path == "" || a.update == nil {
		return
	}
	dir := filepath.Dir(path)
	a.player.film.infoFor = path
	update := a.update
	go func() {
		entries, err := os.ReadDir(dir)
		files := make([]film.File, 0, len(entries))
		for _, entry := range entries {
			if err != nil || entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			info, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			files = append(files, film.File{Name: entry.Name(), Size: info.Size()})
		}
		update(func() {
			if a.player.film.infoFor != path {
				return
			}
			a.player.film.info = film.Info{Folder: dir, Files: files}
		})
	}()
}

// seekPlayback opens path again at at and plays from there.
func (a *app) seekPlayback(at time.Duration) {
	path := a.player.playPath
	if a.player.play != nil && a.player.play.path != "" {
		path = a.player.play.path
	}
	if path == "" || at < 0 {
		return
	}
	a.player.playFrom = at
	a.player.playJump = true
	a.startPlayback(path)
}

// stopPlayback ends the open video and, when it played (filmPlay.played), keeps where it stopped. It does nothing when the film page is not playing.
func (a *app) stopPlayback() {
	if a.player.play == nil && a.player.playPath == "" && a.player.playErr == "" {
		return
	}
	a.player.playGen++
	play := a.player.play
	a.player.play = nil
	a.player.playPath = ""
	a.player.playErr = ""
	if play == nil {
		return
	}
	if play.played {
		a.rememberPosition(play)
	}
	if play.audio != nil {
		play.audio.Close()
	}
	play.player.Close()
}

// playerDidResignActive remembers the playing film's clocks while the app is
// in the background. VideoPlayer deliberately stops advancing when its
// surface is hidden, while audio is a separate process that keeps playing.
func (a *app) playerDidResignActive() {
	p := a.player.play
	if p == nil || p.finished || p.paused || !p.player.Playing() {
		return
	}
	p.backgroundAt = time.Now()
	p.backgroundHead = p.playhead()
}

// playerDidBecomeActive reopens the picture at the position the sound reached
// while the window was hidden, avoiding a burst of catch-up frames.
func (a *app) playerDidBecomeActive() {
	p := a.player.play
	if p == nil || p.backgroundAt.IsZero() {
		return
	}
	at := p.backgroundResumeAt(time.Now())
	path := p.path
	if at < 0 {
		at = 0
	}
	if p.duration > 0 && at >= p.duration {
		at = max(0, p.duration-100*time.Millisecond)
	}
	if path == "" {
		return
	}
	a.player.playFrom = at
	a.player.playJump = true
	a.startPlaybackWith(path, p)
}

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

func (p *filmPlay) backgroundResumeAt(now time.Time) time.Duration {
	if p == nil {
		return 0
	}
	if p.backgroundAt.IsZero() || now.Before(p.backgroundAt) {
		return p.backgroundHead
	}
	return p.backgroundHead + now.Sub(p.backgroundAt)
}

// playhead is how far the picture itself has gotten. It stays on the last frame once the file ends, instead of following the time since play was pressed.
func (p *filmPlay) Widget() *ui.VideoPlayer { return p.player }

func (p *filmPlay) Playing() bool { return p.player != nil && p.player.Playing() }

func (p *filmPlay) Settle() { p.settlePlay() }

func (p *filmPlay) Toggle() { p.toggle() }

func (p *filmPlay) Head() time.Duration { return p.playhead() }

func (p *filmPlay) Length() time.Duration { return p.duration }

func (p *filmPlay) playhead() time.Duration {
	if p.finished && p.duration > 0 {
		return p.duration
	}
	at := time.Duration(0)
	if p.frames != nil {
		at = p.frames.position()
	}
	if p.duration > 0 && at > p.duration {
		return p.duration
	}
	return at
}

// settlePlay stops the sound once the picture has played to the end.
func (p *filmPlay) settlePlay() {
	if p == nil || p.paused || p.player == nil || p.player.Playing() {
		return
	}
	p.finish()
}

func (p *filmPlay) finish() {
	if p.finished {
		return
	}
	p.finished = true
	p.paused = true
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
}

func (p *filmPlay) toggle() {
	if p.finished {
		p.finished = false
		p.paused = false
		p.played = true
		// VideoPlayer.Play rewinds after the last frame. Start its sound from
		// the same origin instead of the old end position.
		p.restartAudioAt(0)
		p.player.Play()
		return
	}
	if p.player.Playing() {
		p.paused = true
		p.player.Pause()
		if p.audio != nil {
			p.audio.Pause()
		}
		return
	}
	p.paused = false
	p.played = true
	if p.audio != nil {
		p.audio.Play()
	} else {
		p.restartAudio()
	}
	p.player.Play()
}

func (p *filmPlay) restartAudio() {
	p.restartAudioAt(p.soundAt(time.Now()))
}

// soundAt is where the sound is at now: where the picture is, except while the app is in the background or just back from it (backgroundAt), when the picture stands still and the sound has played on.
func (p *filmPlay) soundAt(now time.Time) time.Duration {
	if !p.backgroundAt.IsZero() {
		return p.backgroundResumeAt(now)
	}
	return p.playhead()
}

func (p *filmPlay) restartAudioAt(at time.Duration) {
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	if p.volume <= 0 {
		return
	}
	audio, err := openPCM(p.ffmpeg, p.path, at, p.volume)
	if err != nil || audio == nil {
		return
	}
	audio.Start()
	p.audio = audio
}

// setVolume plays the sound at volume from now on. Its process takes the volume only when it starts, so a playing film's sound starts again where it is (soundAt), and a held one's is dropped for toggle to start at the new volume. At 0 there is no sound to play.
func (p *filmPlay) setVolume(volume float64) {
	if p.volume == volume {
		return
	}
	p.volume = volume
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	if !p.paused && !p.finished && p.player.Playing() {
		p.restartAudio()
	}
}

// frameDecoder is a FrameReader as a ui.VideoDecoder. Closing the player closes the reader.
// shown is the time of the frames decoded so far, including the one the player is about to show.
type frameDecoder struct {
	frames *media.FrameReader
	mu     sync.Mutex
	shown  time.Duration
}

func (d *frameDecoder) Next() (ui.VideoFrame, error) {
	img, dur, err := d.frames.Next()
	if err != nil {
		return ui.VideoFrame{}, err
	}
	d.mu.Lock()
	d.shown += dur
	d.mu.Unlock()
	return ui.VideoFrame{Image: img, Duration: dur}, nil
}

func (d *frameDecoder) position() time.Duration {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.shown
}

func (d *frameDecoder) Rewind() error {
	if err := d.frames.Rewind(); err != nil {
		return err
	}
	d.mu.Lock()
	d.shown = 0
	d.mu.Unlock()
	return nil
}

func (d *frameDecoder) Close() error { return d.frames.Close() }
