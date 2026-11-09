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
	// backgroundAt and backgroundHead are the clock of a film that played as the app went inactive: the playhead then, and when. The sound plays on in the background, while the picture stops as long as the window is out of sight.
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
	a.startPlaybackWith(path, nil, nil)
}

// startPlaybackWith opens a new decoder. keep, a film whose picture fell behind its sound in the background, stays on screen with its sound playing while the new picture opens where that sound is by then (sound), and the new picture takes that sound over, so the sound never stops or starts again.
func (a *app) startPlaybackWith(path string, keep *filmPlay, sound func() time.Duration) {
	at := a.player.playFrom
	jump := a.player.playJump
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
				if a.player.playGen != gen || keep != nil {
					return
				}
				a.player.playErr = "这个视频播不了"
			})
			return
		}
		var audio *pcmAudio
		if keep != nil {
			// The sound played on while ffmpeg opened the picture.
			if now := sound(); now > openAt {
				if err := skipPlaybackFrames(frames, now-openAt); err != nil {
					frames.Close()
					return
				}
				openAt = now
			}
		} else {
			audio, _ = openPCM(ffmpeg, path, openAt)
		}
		update(func() {
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
					a.player.playErr = "这个视频播不了"
				}
				return
			}
			if keep != nil {
				audio, keep.audio = keep.audio, nil
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
				duration: frames.Duration(), paused: paused, played: jump,
			}
			if keep != nil && !keep.backgroundAt.IsZero() {
				// The app went inactive again while the picture opened: the new picture is level with the sound now.
				next.backgroundAt, next.backgroundHead = time.Now(), next.playhead()
			}
			old := a.player.play
			a.player.play = next
			if old != nil {
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

// backgroundDrift is how far the picture may trail the sound when the app comes back before the picture opens again where the sound is. The picture of a window that stayed in sight kept pace with the sound, give or take the frames decoded ahead, and opening it again would only make it jump.
const backgroundDrift = 200 * time.Millisecond

// playerDidResignActive notes the clock of the playing film as the app goes inactive. Its sound, a process of its own, plays on; VideoPlayer stops the picture only while the window is out of sight.
func (a *app) playerDidResignActive() {
	p := a.player.play
	if p == nil || p.finished || p.paused || !p.player.Playing() {
		return
	}
	p.backgroundAt = time.Now()
	p.backgroundHead = p.playhead()
}

// playerDidBecomeActive opens the picture again where the sound is, when the picture stood still behind the window while the sound played on. A picture that kept pace is left alone.
func (a *app) playerDidBecomeActive() {
	p := a.player.play
	if p == nil || p.backgroundAt.IsZero() {
		return
	}
	sound := p.backgroundSound()
	p.backgroundAt, p.backgroundHead = time.Time{}, 0
	if p.paused || p.finished || p.path == "" || sound()-p.playhead() < backgroundDrift {
		return
	}
	a.player.playFrom = sound()
	a.player.playJump = true
	a.startPlaybackWith(p.path, p, sound)
}

// backgroundSound is where the sound of p is at any moment since the app went inactive, held short of the film's end.
func (p *filmPlay) backgroundSound() func() time.Duration {
	head, since, length := p.backgroundHead, p.backgroundAt, p.duration
	return func() time.Duration {
		at := head + max(time.Since(since), 0)
		if length > 0 && at >= length {
			at = max(0, length-100*time.Millisecond)
		}
		return at
	}
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
	p.restartAudioAt(p.playhead())
}

func (p *filmPlay) restartAudioAt(at time.Duration) {
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	audio, err := openPCM(p.ffmpeg, p.path, at)
	if err != nil || audio == nil {
		return
	}
	audio.Start()
	p.audio = audio
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
