package main

import (
	"context"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/nowplaying"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// filmPlay is the video open on the film page, which the system's player plays, picture and sound (core.Video, mounted by film.View): the app keeps what it asked the player for and what the player reported, and the page binds the one and hears the other.
type filmPlay struct {
	// path is the film's video, as the library keeps it: where it continues from, its subtitle delay and what the system shows of it go by it. source is what the system's player plays: path, or a copy of it the player can open (playerRemux), "" while that copy is being made.
	path   string
	source string
	// remux is the step the copy in source was made by, 0 for the video itself.
	remux film.RemuxStep
	// loaded is set once the system's player has opened the source and knows its length, duration.
	loaded   bool
	duration time.Duration
	// head is where the picture was when the player last said, or where it opens or a seek went; headAt is when, on the app's clock. While the film plays, the picture is that far on since, at its speed (playhead).
	head   time.Duration
	headAt time.Time
	// seeking is set from a seek until the player reports a time where the seek lands (film.SeekLanded), so a report it sent before the seek landed does not take the picture back. seekFrom is where the picture was as it seeked, seekTo where it went and seekAt when.
	seeking  bool
	seekFrom time.Duration
	seekTo   time.Duration
	seekAt   time.Time
	// seeks counts the seeks of this playback, and keepSeek is whether the last was the user's, which playerKeep keeps on disk where it lands, rather than a looped line going back to its start.
	seeks    int
	keepSeek bool
	paused   bool
	// rate is how fast the system's player plays the film, picture and sound, as a multiple of the speed it was recorded at (playerSpeed); 0 counts as 1.
	rate float64
	// finished is set once the picture reaches the file's end, so the clock stops there.
	finished bool
	// played is set once this playback has played: opened playing at a point picked for it (继续, 从头看), started with play, or moved by a seek. stopPlayback keeps where it stopped only then, so opening a film from its poster, which holds it on its first frame, and going back leaves where it continues from as it was.
	played bool
	// nowPlaying is where toggle and finish tell the system whether the film plays (playback_nowplaying.go), or nil while the system is not shown the film.
	nowPlaying nowplaying.Center
	// clock is the app's clock, which the playhead runs on between the player's reports: the engine's, a test's manual one in tests. seek has the system's player seek (app.player.ref), for toggle to start a finished film over; a filmPlay a test makes may have neither.
	clock func() time.Time
	seek  func(at time.Duration)
	// restart holds the system's player paused while a seek away from the video's end reaches it (holdForSeek), which later lets go of once that update has gone to the system: later runs a function on the engine's goroutine after the update under way, and has the bindings follow.
	restart bool
	later   func(func())
}

// seekGiveUp is how long a seek may take before any report is believed again (filmPlay.seeking), as for a seek past the end, which lands at the end.
const seekGiveUp = 3 * time.Second

func (a *app) ffmpegPath() string {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return ""
	}
	return tools.FFmpeg
}

// playerNow is the app's clock, the engine's once it runs: a test's manual one moves with nsgotest's Advance.
func (a *app) playerNow() time.Time {
	if a.cx != nil {
		return a.cx.Now()
	}
	return time.Now()
}

// startPlayback opens path on the film page, in the system's player, held on its first frame or, for 继续, 从头看 and a jump to a line, playing from where it was asked to (playFrom, playJump). A missing file leaves the page as a subtitle list. A jump within the open film's video (playSame) seeks it rather than open it again.
func (a *app) startPlayback(path string) {
	at := a.player.playFrom
	jump, same := a.player.playJump, a.player.playSame
	a.player.playFrom = 0
	a.player.playJump, a.player.playSame = false, false
	if same && a.player.play != nil && a.player.play.path == path {
		a.seekTo(at, true)
		return
	}
	a.dropPlayback()
	// Anything but a jump within the open film is a film opening, the hero's 播放, 继续 and 从头看 and the palette's jump into another film included.
	if !same {
		a.resetFilm()
		a.player.film.video = path
		a.player.film.delay = a.player.delays[path]
		a.readFilmInfo(path)
	}
	a.player.opening = at
	opens := path != ""
	if opens {
		if _, err := os.Stat(path); err != nil {
			opens = false
		}
	}
	a.playerNowPlayingClear()
	if !opens {
		return
	}
	a.player.playGen++
	a.player.playPath = path
	a.player.play = &filmPlay{path: path, source: path, head: at, headAt: a.playerNow(), paused: !jump, played: jump, rate: a.playerSpeed(), clock: a.playerNow, seek: a.player.ref.Seek, later: a.playerLater}
}

// playerLater runs fn on the engine's goroutine in a pass of its own, after the update under way has gone to the system's player, and has the bindings follow. Without the app's scope, as in tests that start no app, it runs fn at once.
func (a *app) playerLater(fn func()) {
	if a.cx == nil {
		fn()
		return
	}
	a.cx.Post(func() {
		fn()
		a.Changed()
	})
}

// playerVideo is what the system's player plays and how: the open video's source, held until it has loaded, so it starts where it was asked to, and at the player's volume and speed. A volume of 0 plays muted, as the system's player plays a volume of 0 at full volume.
func (a *app) playerVideo() film.Video {
	p := a.player.play
	if p == nil {
		return film.Video{}
	}
	volume := float32(a.player.volume)
	return film.Video{
		Source: p.source,
		Paused: !p.loaded || p.paused || p.finished || p.restart,
		Muted:  a.player.muted || volume <= 0,
		Volume: volume,
		Rate:   float32(playerRate(a.playerSpeed())),
	}
}

// playerSpeed is how fast the system's player plays the open film, which the playhead, the subtitles' timers and the loop follow: the player's speed (app.player.rate), but 1 where the system's player plays every film at the speed it was recorded at (film.RateWorks), Linux, which has no speed button and no speed keys then. The settings keep the speed picked elsewhere all the same.
func (a *app) playerSpeed() float64 {
	if a.cx != nil && !film.RateWorks(a.cx) {
		return 1
	}
	return a.player.rate
}

// playerLoad is the system's player having opened the open video: its length is known, and it seeks to where it opens, for a film opened at a point, and plays when it was opened playing.
func (a *app) playerLoad(info ns.VideoInfo) {
	p := a.player.play
	if p == nil || p.loaded {
		return
	}
	p.loaded = true
	if info.Duration > 0 {
		p.duration = info.Duration
		a.player.length = info.Duration
	}
	if p.head > 0 {
		// The player has opened the video at its start, from which it seeks.
		p.startSeek(0, p.head, a.playerNow())
		a.player.ref.Seek(p.head)
	}
	p.headAt = a.playerNow()
	a.playerNowPlayingShow(p)
	a.loadClipFrames()
}

// playerProgress is the system's player saying where the open video is, about four times a second while it plays and as it pauses or ends. It moves the playhead there, unless a seek has not landed yet; keeps where the film is on disk now and then (playerKeep), takes a looped line back to its start once it reaches its end (playerKeepLoop) and tells the areas that follow the film's time (areasPlaying). It reports whether that changed what a binding reads beyond the playhead.
func (a *app) playerProgress(position, duration time.Duration) bool {
	p := a.player.play
	if p == nil || !p.loaded {
		return false
	}
	now := a.playerNow()
	if duration > 0 && duration != p.duration {
		p.duration = duration
		a.player.length = duration
	}
	if p.seeking {
		ran := time.Duration(0)
		if !p.paused && !p.finished {
			ran = playerFilmTime(now.Sub(p.seekAt), p.speed())
		}
		if !film.SeekLanded(p.seekFrom, p.seekTo, ran, position) && now.Sub(p.seekAt) < seekGiveUp {
			return false
		}
		p.seeking = false
	}
	p.head, p.headAt = position, now
	gen := a.player.playGen
	changed := a.playerKeep(now)
	a.playerKeepLoop()
	a.playerLoopSchedule()
	// every area hears each report, so the areas run before the result is put together rather than inside a short-circuited ||
	followed := a.areasPlaying()
	return changed || followed || a.player.playGen != gen
}

// playerEnded is the system's player reaching the end of the open video: the clock stops there, and the system learns the film is paused at its end, so its play key starts it over.
func (a *app) playerEnded() {
	p := a.player.play
	if p == nil {
		return
	}
	p.finish()
	a.playerKeepLoop()
}

// playerFailed is the system's player saying it cannot play the open video. A video in a container or of a codec it does not take, as an MKV or a WebM on macOS, plays from a copy ffmpeg makes (playerRemux), first with its streams as they are and then encoded anew; without ffmpeg, or when neither copy plays, the page says the video cannot be played.
func (a *app) playerFailed(err error) {
	p := a.player.play
	if p == nil {
		return
	}
	log.Printf("player: the system's player cannot play %s: %v", filepath.Base(p.source), err)
	next := film.RemuxCopy
	if p.remux == film.RemuxCopy {
		next = film.RemuxEncode
	}
	if p.remux == film.RemuxEncode || !a.playerRemux(p, next) {
		a.player.playErr = playerErrUnplayable
		p.source = ""
		a.playerNowPlayingClear()
	}
}

// playerRemux makes the copy of p's video by step off the engine's goroutine and plays it once made, keeping where p opens at. It reports false when there is no ffmpeg to make it with. A copy of the streams as they are that fails is made again encoded anew, and when that fails too the page says the video cannot be played.
func (a *app) playerRemux(p *filmPlay, step film.RemuxStep) bool {
	ffmpeg := a.ffmpegPath()
	if ffmpeg == "" || a.update == nil {
		return false
	}
	dir := a.player.remuxDir
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return false
		}
		dir = filepath.Join(cache, "Kando", "playback")
	}
	dest, err := film.RemuxPath(dir, p.path, step)
	if err != nil {
		return false
	}
	// TODO(nsgo#107): play every video ffmpeg decodes in the app's own player, as MyGo's VideoPlayer drew the frames ffmpeg decoded, rather than a copy the system's player can open https://github.com/nsgo-dev/nsgo/issues/107
	p.source, p.remux, p.loaded = "", step, false
	a.playerStopRemux()
	ctx, stop := context.WithCancel(context.Background())
	a.player.remuxStop = stop
	update, src := a.update, p.path
	go func() {
		err := film.Remux(ctx, ffmpeg, src, dest, step)
		update(func() {
			// A seek meanwhile keeps the film; another film, or none, has stopped the copy.
			if a.player.play != p || p.remux != step {
				return
			}
			a.player.remuxStop = nil
			if err != nil {
				log.Printf("player: could not copy %s for the system's player: %v", filepath.Base(src), err)
				// Streams an MP4 cannot hold as they are, as a WebM's VP8 and Vorbis, fail the copy; encoded anew they play.
				if step == film.RemuxCopy && a.playerRemux(p, film.RemuxEncode) {
					return
				}
				a.player.playErr = playerErrUnplayable
				a.playerNowPlayingClear()
				return
			}
			p.source = dest
		})
	}()
	return true
}

// playerStopRemux stops the copy being made for the open video, if one is.
func (a *app) playerStopRemux() {
	if a.player.remuxStop != nil {
		a.player.remuxStop()
		a.player.remuxStop = nil
	}
}

// readFilmInfo lists the folder of the video at path off the engine's goroutine, for the player's 信息 tab. A film without a video has nothing to list.
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

// seekPlayback plays the open video from at, as the user asked: a click on the scrubber or a line, a skip, a move by line.
func (a *app) seekPlayback(at time.Duration) { a.seekTo(at, true) }

// seekTo moves the open video to at and plays it from there, the system learning so at once. kept says it was the user's, which playerKeep keeps on disk where it lands; a looped line going back to its start is not. Before the system's player has loaded the video, it opens there instead.
func (a *app) seekTo(at time.Duration, kept bool) {
	p := a.player.play
	if p == nil || at < 0 {
		return
	}
	if p.duration > 0 {
		at = min(at, p.duration)
	}
	now := a.playerNow()
	from := p.playhead()
	a.player.playGen++
	a.player.opening = at
	p.head, p.headAt = at, now
	ended := p.finished
	p.paused, p.finished = false, false
	p.played = true
	p.seeks++
	p.keepSeek = kept
	if p.loaded {
		p.startSeek(from, at, now)
		a.player.ref.Seek(at)
		if ended {
			p.holdForSeek()
		}
	}
	a.playerNowPlayingSeeking(p.path, at)
}

// stopPlayback ends the open video and, when it played (filmPlay.played), keeps where it stopped, and takes it off where the system shows what plays. It does nothing when the film page is not playing, but for saving a subtitle delay that changed since the last save.
func (a *app) stopPlayback() {
	a.dropPlayback()
	a.playerNowPlayingClear()
}

// dropPlayback is stopPlayback but for what the system shows, which startPlayback clears itself. The page unmounts the system's player once there is no video to play.
func (a *app) dropPlayback() {
	defer a.playerSaveDelay()
	if a.player.play == nil && a.player.playPath == "" && a.player.playErr == "" {
		return
	}
	a.player.playGen++
	a.playerStopRemux()
	play := a.player.play
	a.player.play = nil
	a.player.playPath = ""
	a.player.playErr = ""
	if play != nil && play.played && play.loaded {
		a.rememberPosition(play)
	}
}

func (p *filmPlay) Playing() bool { return p.loaded && !p.paused && !p.finished }

func (p *filmPlay) Toggle() { p.toggle() }

func (p *filmPlay) Head() time.Duration { return p.playhead() }

func (p *filmPlay) Length() time.Duration { return p.duration }

// playhead is how far the picture has gotten: where the player last said, and while it plays the time since at its speed, up to the end of the video; it stays at the end once the file ends.
func (p *filmPlay) playhead() time.Duration {
	if p.finished && p.duration > 0 {
		return p.duration
	}
	at := p.head
	if p.Playing() && !p.seeking && p.clock != nil {
		at += playerFilmTime(max(p.clock().Sub(p.headAt), 0), p.speed())
	}
	if p.duration > 0 && at > p.duration {
		return p.duration
	}
	return at
}

// finish stops the clock at the end of the video, which the system learns.
func (p *filmPlay) finish() {
	if p.finished {
		return
	}
	p.head = p.playhead()
	p.finished = true
	p.paused = true
	p.reportPlayback()
}

// toggle plays or holds the film, which the system learns. Play after the end starts over from the start.
func (p *filmPlay) toggle() {
	now := time.Now()
	if p.clock != nil {
		now = p.clock()
	}
	if p.finished {
		from := p.playhead()
		p.finished, p.paused, p.played = false, false, true
		p.head, p.headAt = 0, now
		p.seeks++
		p.keepSeek = false
		if p.loaded {
			p.startSeek(from, 0, now)
			if p.seek != nil {
				p.seek(0)
			}
			p.holdForSeek()
		}
		p.reportPlaybackAt(0)
		return
	}
	p.head, p.headAt = p.playhead(), now
	if p.Playing() {
		p.paused = true
	} else {
		p.paused = false
		p.played = true
	}
	p.reportPlayback()
}

// holdForSeek keeps the system's player paused through the update that seeks a film away from its end, and lets it play from the next one (later), so the player has jumped before it is told to play: told to play while it still sits at the end, the system's player on macOS (AVPlayer) reports the end again at once and stays where it is, and the film froze while its button said it played. Without later, as for a filmPlay a test makes, there is nothing to hold.
func (p *filmPlay) holdForSeek() {
	if p.later == nil {
		return
	}
	// TODO(nsgo#206): send a video's Ref.Seek to the platform ahead of the props changed in the same update, or have the Apple player seek a finished item before it plays it again, so playing a video from its end in one update (Paused false and a seek) starts it over; NSGo's commit puts the commands (the seek) after the props (setRate), and this hold, which costs a commit, could go https://github.com/nsgo-dev/nsgo/issues/206
	p.restart = true
	p.later(func() { p.restart = false })
}

// startSeek notes a seek from where the picture is, from, to to at now, so reports the player sent before it landed are dropped (playerProgress).
func (p *filmPlay) startSeek(from, to time.Duration, now time.Time) {
	p.seeking, p.seekFrom, p.seekTo, p.seekAt = true, from, to, now
}

// speed is how fast the film plays: rate, or 1 for a filmPlay made without one.
func (p *filmPlay) speed() float64 { return playerRate(p.rate) }

// setRate plays the film at rate from now on, the playhead moving at it from where it is.
func (p *filmPlay) setRate(rate float64) {
	if p.speed() == playerRate(rate) {
		return
	}
	if p.clock != nil {
		p.head, p.headAt = p.playhead(), p.clock()
	}
	p.rate = rate
}

// playerRate is rate, or 1 for none (0), as for a film made without one.
func playerRate(rate float64) float64 {
	if rate <= 0 {
		return 1
	}
	return rate
}

// playerFilmTime is how much of a film playing at rate plays in d on the wall clock.
func playerFilmTime(d time.Duration, rate float64) time.Duration {
	rate = playerRate(rate)
	if rate == 1 {
		return d
	}
	return time.Duration(math.Round(float64(d) * rate))
}

// playerWallTime is how long d of a film playing at rate takes on the wall clock.
func playerWallTime(d time.Duration, rate float64) time.Duration {
	rate = playerRate(rate)
	if rate == 1 {
		return d
	}
	return time.Duration(math.Round(float64(d) / rate))
}
