package main

import (
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/film"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// filmPlay is the video open on the film page. ffmpeg decodes the picture, and the sound (player_audio.go), which mygo/audio plays.
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
	// rate is how fast the film plays, picture and sound, as a multiple of the speed it was recorded at (app.player.rate); 0 counts as 1 (speed). The playhead and every clock of the film count in the film's own time, which moves on rate times as fast as the wall clock.
	rate float64
	// backgroundAt and backgroundHead are the clock of a film that played as the app went inactive: the playhead then, and when. The sound plays on in the background, while the picture stops as long as the window is out of sight.
	backgroundAt   time.Time
	backgroundHead time.Duration
	// background decodes the picture level with the sound while the window is out of sight (playback_background.go); nil while the app is active.
	background *backgroundPicture
	// unseen is set once background noticed the picture standing still, as the window is out of sight, so the display may sleep while only the sound plays on (playerKeepAwake); coming back clears it.
	unseen bool
	// returning is where the sound is while the app, just back, puts the picture there: from playerBackInSight until the picture decoded in the background or opened again takes over, or does not come.
	returning func() time.Duration
	// finished is set once the picture reaches the file's end, so the clock stops there.
	finished bool
	// played is set once this playback has played: opened playing at a point picked for it (a seek, 继续, 从头看) or started with play. stopPlayback keeps where it stopped only then, so opening a film from its poster, which holds it on its first frame, and going back leaves where it continues from as it was.
	played bool
	// soundSince and soundFrom are when the sound was last asked to play along with the picture, and where it was heard then, for catchUpSound to bring it level with the picture once it is heard to move; soundSince is zero once it did, or when there is nothing to check.
	soundSince time.Time
	soundFrom  time.Duration
	// nowPlaying is where toggle and finish tell the system whether the film plays (playback_nowplaying.go), or nil while the system is not shown the film.
	nowPlaying nowPlayingCenter
}

const (
	// playerSoundSettle is how long the sound plays before catchUpSound compares it with the picture: long enough for the system's audio output to open again, which it does on the first sound after five seconds of none, and for an ffmpeg just started to decode the first of it.
	playerSoundSettle = 250 * time.Millisecond
	// playerSoundGiveUp is how long catchUpSound waits for a sound that is not heard to move, such as one whose output does not open, before it leaves it as it is.
	playerSoundGiveUp = 3 * time.Second
	// playerSoundLag is how far the sound may trail the picture before catchUpSound moves it on. The output holds about 40 ms before it is heard, so a sound that keeps pace trails by that much from the start; the 60 ms beyond it are well within what is seen as in sync.
	playerSoundLag = 100 * time.Millisecond
)

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
	a.player.openHeld = jump && a.player.playHold
	a.player.playHold = false
	if keep == nil {
		a.dropPlayback()
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
	opens := path != "" && ffmpeg != ""
	if opens {
		if _, err := os.Stat(path); err != nil {
			opens = false
		}
	}
	if keep == nil {
		a.playerNowPlayingOpening(path, jump && opens)
	}
	if !opens {
		return
	}
	a.player.playGen++
	gen := a.player.playGen
	a.player.playPath = path
	volume := a.playerVolume()
	openRate := a.player.rate
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
				a.playerNowPlayingClear()
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
		} else {
			audio, err = openPCM(ffmpeg, path, openAt, volume, openRate)
			if err != nil {
				log.Printf("player: no sound for %s: %v", filepath.Base(path), err)
			}
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
			// A film coming back plays on at the speed of the sound it hands over; playerApplyRate brings it to a speed changed meanwhile once its picture has caught up.
			rate := a.player.rate
			if keep != nil {
				rate = keep.speed()
			}
			dec := &frameDecoder{frames: frames, shown: openAt, rate: rate}
			player, err := ui.NewVideoPlayer(dec)
			if err != nil {
				frames.Close()
				if audio != nil {
					audio.Close()
				}
				if keep == nil {
					a.player.playErr = playerErrUnplayable
					a.playerNowPlayingClear()
				}
				return
			}
			if keep != nil {
				// The sound handed over plays at the volume keep had, which setVolume kept up to date while the picture opened.
				audio, keep.audio = keep.audio, nil
				volume = keep.volume
			} else {
				// The speed may have changed while the video was opening. The sound has not started yet, so it is made again at the new one rather than started and then restarted.
				if rate != openRate && audio != nil {
					audio.Close()
					audio, err = openPCM(ffmpeg, path, openAt, a.playerVolume(), rate)
					if err != nil {
						log.Printf("player: no sound for %s: %v", filepath.Base(path), err)
					}
				}
				// The volume may have changed while the video was opening.
				volume = a.playerVolume()
				audio.setVolume(volume)
			}
			// A seek plays from where it opens, unless it was to hold there (openHeld).
			held := a.player.openHeld
			a.player.openHeld = false
			paused := true
			if jump && !held {
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
				duration: frames.Duration(), paused: paused, played: jump, volume: volume, rate: rate,
			}
			// A picture opened held (openHeld) has no sound playing to check.
			if !paused && keep == nil {
				next.watchSound(time.Now())
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
	if a.player.playPath == path {
		a.playerNowPlayingSeeking(path, at)
	}
}

// stopPlayback ends the open video and, when it played (filmPlay.played), keeps where it stopped, and takes it off where the system shows what plays. It does nothing when the film page is not playing, but for saving a subtitle delay that changed since the last save.
func (a *app) stopPlayback() {
	a.dropPlayback()
	a.playerNowPlayingClear()
}

// dropPlayback is stopPlayback but for what the system shows, which a video opening in its place (startPlaybackWith) keeps through a seek.
func (a *app) dropPlayback() {
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

// settlePlay, once a frame of the picture, brings a sound that just started level with it (catchUpSound), and stops the sound once the picture has played to the end.
func (p *filmPlay) settlePlay() {
	if p == nil || p.paused || p.player == nil {
		return
	}
	if p.player.Playing() {
		p.catchUpSound(time.Now())
		return
	}
	p.finish()
}

// watchSound notes that the sound was asked at now to play along with the picture, from where it is heard, for catchUpSound to check it once it plays.
func (p *filmPlay) watchSound(now time.Time) {
	at, ok := p.audio.heard()
	if !ok {
		p.soundSince = time.Time{}
		return
	}
	p.soundSince, p.soundFrom = now, at
}

// catchUpSound moves the sound on to where the picture is, once it has played for playerSoundSettle since watchSound, if it trails by more than playerSoundLag. The picture plays on the clock from the moment it is asked to, while the sound starts only once the system's audio output has opened again, which it does after five seconds without sound and so after most pauses, and once ffmpeg has decoded the first of it; left alone, each such start would leave the sound further behind. The picture's clock is the time since watchSound, as where the picture shows (playhead) runs a few frames ahead of what it shows.
func (p *filmPlay) catchUpSound(now time.Time) {
	if p.soundSince.IsZero() {
		return
	}
	played := now.Sub(p.soundSince)
	at, ok := p.audio.heard()
	if !ok || played > playerSoundGiveUp {
		p.soundSince = time.Time{}
		return
	}
	if played < playerSoundSettle || at <= p.soundFrom {
		return
	}
	p.soundSince = time.Time{}
	// The picture's clock runs at the film's speed, as the sound does.
	if want := p.soundFrom + filmTime(played, p.speed()); want-at > playerSoundLag {
		p.audio.seek(want)
	}
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
	p.reportPlayback()
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
		p.reportPlaybackAt(0)
		return
	}
	if p.player.Playing() {
		p.paused = true
		p.soundSince = time.Time{}
		p.player.Pause()
		if p.audio != nil {
			p.audio.Pause()
		}
		p.reportPlayback()
		return
	}
	p.paused = false
	p.played = true
	if p.audio != nil {
		p.audio.Play()
		p.watchSound(time.Now())
	} else {
		p.restartAudio()
	}
	p.player.Play()
	p.reportPlayback()
}

func (p *filmPlay) restartAudio() {
	p.restartAudioAt(p.soundAt(time.Now()))
}

// soundAt is where the sound is at now: where the picture is, except while the app is in the background (backgroundAt) or just back from it (returning), when the picture may stand still while the sound has played on.
func (p *filmPlay) soundAt(now time.Time) time.Duration {
	if !p.backgroundAt.IsZero() {
		return soundClock(p.backgroundHead, p.backgroundAt, p.duration, p.speed(), now)
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
	audio, err := openPCM(p.ffmpeg, p.path, at, p.volume, p.speed())
	if err != nil {
		log.Printf("player: no sound for %s: %v", filepath.Base(p.path), err)
		return
	}
	audio.Start()
	p.audio = audio
	// The picture plays on at once, while ffmpeg has only just started on the sound.
	p.watchSound(time.Now())
}

// setVolume plays the sound at volume from now on, as it plays: a muted film's sound plays on unheard, so that unmuting it is heard at once, where the picture is.
func (p *filmPlay) setVolume(volume float64) {
	p.volume = volume
	p.audio.setVolume(volume)
}

// speed is how fast the film plays: rate, or 1 for a filmPlay made without one.
func (p *filmPlay) speed() float64 { return playbackRate(p.rate) }

// setRate plays the film at rate from now on. The picture's frames show for their length over rate from the next one decoded. The sound's process takes its speed only when it starts, so a playing film's sound starts again where it is (soundAt), and a held one's is dropped for toggle to start at the new speed.
func (p *filmPlay) setRate(rate float64) {
	if p.speed() == playbackRate(rate) {
		return
	}
	at := p.soundAt(time.Now())
	p.rate = rate
	if p.frames != nil {
		p.frames.setRate(rate)
	}
	if p.audio != nil {
		p.audio.Close()
		p.audio = nil
	}
	if !p.paused && !p.finished && p.player != nil && p.player.Playing() {
		p.restartAudioAt(at)
	}
}

// playbackRate is rate, or 1 for none (0), as for a film or decoder made without one.
func playbackRate(rate float64) float64 {
	if rate <= 0 {
		return 1
	}
	return rate
}

// wallTime is how long d of a film playing at rate takes on the wall clock.
func wallTime(d time.Duration, rate float64) time.Duration {
	rate = playbackRate(rate)
	if rate == 1 {
		return d
	}
	return time.Duration(math.Round(float64(d) / rate))
}

// filmTime is how much of a film playing at rate plays in d on the wall clock.
func filmTime(d time.Duration, rate float64) time.Duration {
	rate = playbackRate(rate)
	if rate == 1 {
		return d
	}
	return time.Duration(math.Round(float64(d) * rate))
}

// frameDecoder is a FrameReader as a ui.VideoDecoder. Closing the player closes the reader.
// shown is the time of the frames decoded so far, including the one the player is about to show, in the film's own time: a film playing at a rate other than 1 (rate) shows each frame for its length over rate, and shown still counts the frame's length.
type frameDecoder struct {
	frames *media.FrameReader
	// held is a frame read ahead of the decoder, which Next gives first: the frame a picture decoded in the background stood on when it was handed over.
	held *ui.VideoFrame
	// sound, when set, is the clock of the sound the picture follows from the frame after held on. VideoPlayer counts its time only from when the window shows, which on macOS comes a frame or more after the app is back, and only then starts decoding; held shows until then, and the frame after it is the one the sound is in by then, for what is left of it. Without this the picture stood on held for its whole length once the window showed, then trailed the sound by the time the window took to show.
	sound func() time.Duration
	mu    sync.Mutex
	shown time.Duration
	// rate is how fast the picture plays (filmPlay.rate), which the UI thread may change while the player decodes; 0 counts as 1.
	rate float64
}

// setRate shows the frames decoded from now on for their length over rate.
func (d *frameDecoder) setRate(rate float64) {
	d.mu.Lock()
	d.rate = rate
	d.mu.Unlock()
}

func (d *frameDecoder) Next() (ui.VideoFrame, error) {
	if f := d.held; f != nil {
		d.held = nil
		d.mu.Lock()
		d.shown += f.Duration
		rate := d.rate
		d.mu.Unlock()
		shows := *f
		if d.sound != nil {
			shows.Duration = time.Millisecond
		} else {
			shows.Duration = wallTime(f.Duration, rate)
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
	rate := d.rate
	d.mu.Unlock()
	return ui.VideoFrame{Image: img, Duration: wallTime(shows, rate)}, nil
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
