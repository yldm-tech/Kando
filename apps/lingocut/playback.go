package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/packages/media"

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
	// finished is set once the picture reaches the file's end, so the clock stops there.
	finished bool
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
	at := a.player.playFrom
	jump := a.player.playJump
	a.player.playFrom = 0
	a.player.playJump = false
	a.stopPlayback()
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
		frames, err := media.OpenFramesAt(ffmpeg, path, at)
		if err != nil {
			update(func() {
				if a.player.playGen != gen {
					return
				}
				a.player.playErr = "这个视频播不了"
			})
			return
		}
		audio, _ := openPCM(ffmpeg, path, at)
		update(func() {
			if a.player.playGen != gen {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				return
			}
			dec := &frameDecoder{frames: frames, shown: at}
			player, err := ui.NewVideoPlayer(dec)
			if err != nil {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				a.player.playErr = "这个视频播不了"
				return
			}
			paused := true
			if jump {
				player.Play()
				if audio != nil {
					audio.Start()
				}
				paused = false
			} else {
				player.Pause()
			}
			a.player.play = &filmPlay{
				player: player, frames: dec, audio: audio, ffmpeg: ffmpeg, path: path,
				duration: frames.Duration(), paused: paused,
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

// stopPlayback ends the open video. It does nothing when the film page is not playing.
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
	a.rememberPosition(play)
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
		p.restartAudio()
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
	if p.audio != nil {
		p.audio.Play()
	} else {
		p.restartAudio()
	}
	p.player.Play()
}

func (p *filmPlay) restartAudio() {
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	audio, err := openPCM(p.ffmpeg, p.path, 0)
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
