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
	// backgroundAt and backgroundHead are the clock of a film that played as the app went inactive: the playhead then, and when. The sound plays on in the background, while the picture stops as long as the window is out of sight.
	backgroundAt   time.Time
	backgroundHead time.Duration
	// background decodes the picture level with the sound while the window is out of sight (playback_background.go); nil while the app is active.
	background *backgroundPicture
	// returning is where the sound is while the app, just back, puts the picture there: from playerBackInSight until the picture decoded in the background or opened again takes over, or does not come.
	returning func() time.Duration
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
	a.startPlaybackWith(path, nil, nil)
}

// startPlaybackWith opens a new decoder. keep, a film whose picture fell behind its sound in the background, stays on screen with its sound playing while the new picture opens where that sound is by then (sound), and the new picture takes that sound over, so the sound never stops or starts again.
func (a *app) startPlaybackWith(path string, keep *filmPlay, sound func() time.Duration) {
	at := a.player.playFrom
	jump, same := a.player.playJump, a.player.playSame
	a.player.playFrom = 0
	a.player.playJump, a.player.playSame = false, false
	if keep == nil {
		a.stopPlayback()
	}
	// A seek opens the same video again and keeps the film's state; anything else is a film opening, the hero's 播放, 继续 and 从头看 and the palette's jump into another film included.
	if !same {
		a.resetFilm()
		a.player.film.video = path
		a.player.film.delay = a.player.delays[path]
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
		if keep != nil {
			openAt = sound()
		}
		frames, err := media.OpenFramesAt(ffmpeg, path, openAt)
		if err != nil {
			update(func() {
				// A film coming back plays on as it was.
				if keep != nil {
					keep.returning = nil
				}
				if a.player.playGen != gen || keep != nil {
					return
				}
				a.player.playErr = playerErrUnplayable
			})
			return
		}
		var audio *pcmAudio
		if keep != nil {
			// The sound played on while ffmpeg opened the picture.
			if now := sound(); now > openAt {
				if err := skipPlaybackFrames(frames, now-openAt); err != nil {
					frames.Close()
					update(func() { keep.returning = nil })
					return
				}
				openAt = now
			}
		} else if volume > 0 {
			// A silent film has no sound to play, muted or turned down to 0.
			audio, _ = openPCM(ffmpeg, path, openAt, volume)
		}
		update(func() {
			if keep != nil {
				keep.returning = nil
			}
			// A film paused or ended while its picture opened again keeps the picture it has, as its sound no longer runs on.
			if a.player.playGen != gen || keep != nil && (a.player.play != keep || keep.paused || keep.finished) {
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
				if keep == nil {
					a.player.playErr = playerErrUnplayable
				}
				return
			}
			if keep != nil {
				// The sound handed over plays at the volume keep had, which setVolume kept up to date while the picture opened.
				audio, keep.audio = keep.audio, nil
				volume = keep.volume
			} else if now := a.playerVolume(); now != volume {
				// The volume may have changed while the video was opening. The sound has not started yet, so it is made again at the new volume rather than started and then restarted.
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
				// Start does nothing to the sound a film coming back hands over, which plays already.
				if audio != nil {
					audio.Start()
				}
				player.Play()
				paused = false
			} else {
				player.Pause()
			}
			next := &filmPlay{
				player: player, frames: dec, audio: audio, ffmpeg: ffmpeg, path: path,
				duration: frames.Duration(), paused: paused, played: jump, volume: volume,
			}
			a.playReplace(next)
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
	a.player.playJump, a.player.playSame = true, true
	a.startPlayback(path)
}

// stopPlayback ends the open video and, when it played (filmPlay.played), keeps where it stopped. It does nothing when the film page is not playing, but for saving a subtitle delay that changed since the last save.
func (a *app) stopPlayback() {
	defer a.playerSaveDelay()
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
	play.stopBackground()
	if play.audio != nil {
		play.audio.Close()
	}
	play.player.Close()
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

// soundAt is where the sound is at now: where the picture is, except while the app is in the background (backgroundAt) or just back from it (returning), when the picture may stand still while the sound has played on.
func (p *filmPlay) soundAt(now time.Time) time.Duration {
	if !p.backgroundAt.IsZero() {
		return soundClock(p.backgroundHead, p.backgroundAt, p.duration, now)
	}
	if p.returning != nil {
		return p.returning()
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
	// held is a frame read ahead of the decoder, which Next gives first: the frame a picture decoded in the background stood on when it was handed over.
	held *ui.VideoFrame
	// sound, when set, is the clock of the sound the picture follows from the frame after held on. VideoPlayer counts its time only from when the window shows, which on macOS comes a frame or more after the app is back, and only then starts decoding; held shows until then, and the frame after it is the one the sound is in by then, for what is left of it. Without this the picture stood on held for its whole length once the window showed, then trailed the sound by the time the window took to show.
	sound func() time.Duration
	mu    sync.Mutex
	shown time.Duration
}

func (d *frameDecoder) Next() (ui.VideoFrame, error) {
	if f := d.held; f != nil {
		d.held = nil
		d.mu.Lock()
		d.shown += f.Duration
		d.mu.Unlock()
		shows := *f
		if d.sound != nil {
			shows.Duration = time.Millisecond
		}
		return shows, nil
	}
	img, dur, err := d.frames.Next()
	if err != nil {
		return ui.VideoFrame{}, err
	}
	d.mu.Lock()
	start := d.shown
	d.mu.Unlock()
	shows := dur
	if d.sound != nil {
		at := d.sound()
		d.sound = nil
		for start+dur <= at {
			start += dur
			if img, dur, err = d.frames.Next(); err != nil {
				d.mu.Lock()
				d.shown = start
				d.mu.Unlock()
				return ui.VideoFrame{}, err
			}
		}
		shows = max(start+dur-max(at, start), time.Millisecond)
	}
	d.mu.Lock()
	d.shown = start + dur
	d.mu.Unlock()
	return ui.VideoFrame{Image: img, Duration: shows}, nil
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
