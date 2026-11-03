package main

import (
	"os"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// filmPlay is the video open on the film page. The picture is decoded by ffmpeg and the sound by the system audio queue.
type filmPlay struct {
	player *ui.VideoPlayer
	audio  *pcmAudio
	ffmpeg string
	path   string
	anchor time.Time
	base   time.Duration
	paused bool
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
	a.stopPlayback()
	ffmpeg := a.ffmpegPath()
	if path == "" || ffmpeg == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	a.playGen++
	gen := a.playGen
	a.playPath = path
	update := a.update
	go func() {
		frames, err := media.OpenFrames(ffmpeg, path)
		if err != nil {
			update(func() {
				if a.playGen != gen {
					return
				}
				a.playErr = "这个视频播不了"
			})
			return
		}
		audio, _ := openPCM(ffmpeg, path)
		update(func() {
			if a.playGen != gen {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				return
			}
			player, err := ui.NewVideoPlayer(&frameDecoder{frames})
			if err != nil {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				a.playErr = "这个视频播不了"
				return
			}
			player.Pause()
			a.play = &filmPlay{player: player, audio: audio, ffmpeg: ffmpeg, path: path, paused: true}
		})
	}()
}

// stopPlayback ends the open video. It does nothing when the film page is not playing.
func (a *app) stopPlayback() {
	if a.play == nil && a.playPath == "" && a.playErr == "" {
		return
	}
	a.playGen++
	play := a.play
	a.play = nil
	a.playPath = ""
	a.playErr = ""
	if play == nil {
		return
	}
	if play.audio != nil {
		play.audio.Close()
	}
	play.player.Close()
}

func (p *filmPlay) playhead() time.Duration {
	if p.paused {
		return p.base
	}
	return p.base + time.Since(p.anchor)
}

func (p *filmPlay) toggle() {
	if p.player.Playing() {
		p.base += time.Since(p.anchor)
		p.paused = true
		p.player.Pause()
		if p.audio != nil {
			p.audio.Pause()
		}
		return
	}
	if !p.paused {
		p.restartAudio()
		p.base = 0
	} else if p.audio != nil {
		p.audio.Play()
	}
	p.player.Play()
	p.anchor = time.Now()
	p.paused = false
}

func (p *filmPlay) restartAudio() {
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	audio, err := openPCM(p.ffmpeg, p.path)
	if err != nil || audio == nil {
		return
	}
	audio.Start()
	p.audio = audio
}

// frameDecoder is a FrameReader as a ui.VideoDecoder. Closing the player closes the reader.
type frameDecoder struct {
	frames *media.FrameReader
}

func (d *frameDecoder) Next() (ui.VideoFrame, error) {
	img, dur, err := d.frames.Next()
	if err != nil {
		return ui.VideoFrame{}, err
	}
	return ui.VideoFrame{Image: img, Duration: dur}, nil
}

func (d *frameDecoder) Rewind() error { return d.frames.Rewind() }

func (d *frameDecoder) Close() error { return d.frames.Close() }

func cueAt(cues []core.Segment, at time.Duration) (core.Segment, bool) {
	var found core.Segment
	ok := false
	for _, cue := range cues {
		if cue.Start > at {
			break
		}
		if cue.End > at || cue.End == 0 {
			found = cue
			ok = true
		}
	}
	return found, ok
}
