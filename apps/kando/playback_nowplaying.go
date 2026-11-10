package main

import (
	"os"
	"time"

	"kando/apps/kando/films"

	"github.com/egoist/mygo"
)

// The open film where the system shows what plays: Now Playing in Control Center and the menu bar on macOS, the media controls of Windows and the MPRIS clients of Linux. The media keys of keyboards and headsets, and those controls, play, pause and move the film while it shows there, the window in sight or not.

// nowPlayingCenter is the part of mygo.NowPlaying the player uses. Tests connect a recorder in its place (playerNowPlayingConnect).
type nowPlayingCenter interface {
	Set(info mygo.NowPlayingInfo)
	SetPlayback(state mygo.PlaybackState, position time.Duration)
	Clear()
	OnCommand(fn func(cmd mygo.MediaCommand, position time.Duration)) (off func())
}

// nowPlayingCommands are the commands the system's controls offer for a film: play, pause and the scrubber. A film has no next or previous track, and stopping is closing it in the window.
const nowPlayingCommands = mygo.MediaPlay | mygo.MediaPause | mygo.MediaSeek

// playerNowPlayingConnect has the player show the open film through center, and play, pause and move it as the media keys and the system's controls ask. The commands come on the main thread and run as a menu command does, through update, so the window draws what they changed.
func (a *app) playerNowPlayingConnect(center nowPlayingCenter) {
	a.player.nowPlaying.center = center
	center.OnCommand(func(cmd mygo.MediaCommand, at time.Duration) {
		a.update(func() { a.playerMediaCommand(cmd, at) })
	})
}

// playerMediaCommand plays, pauses or moves the open film as a media key or the system's controls asked. A command that would leave it as it is, as play while it plays, tells the system again how it is instead. While the window is out of sight, where only these commands reach the film, play and pause keep its picture level with its sound (playerToggleBehind). Without a picture there is nothing to command, but for the film shown while a seek opens its picture again (playerMediaReopening).
func (a *app) playerMediaCommand(cmd mygo.MediaCommand, at time.Duration) {
	p := a.player.play
	if p == nil {
		a.playerMediaReopening(cmd, at)
		return
	}
	playing := p.playbackState() == mygo.PlaybackPlaying
	switch {
	case cmd == mygo.MediaPlay && !playing, cmd == mygo.MediaPause && playing:
		if a.playerOutOfSight() {
			a.playerToggleBehind(p)
		} else {
			p.toggle()
		}
	case cmd == mygo.MediaSeek:
		a.Seek(at)
	default:
		p.reportPlayback()
	}
}

// playerMediaReopening answers a media command that comes while the film shown opens again at another point, as after a seek, when there is no picture to command yet: pause has the picture open held and play has it open playing, and the system learns so at once, so the key is not lost; the scrubber seeks again.
func (a *app) playerMediaReopening(cmd mygo.MediaCommand, at time.Duration) {
	np := &a.player.nowPlaying
	if np.center == nil || a.player.playPath == "" || a.player.playPath != np.path {
		return
	}
	switch cmd {
	case mygo.MediaPause:
		a.player.openHeld = true
	case mygo.MediaPlay:
		a.player.openHeld = false
	case mygo.MediaSeek:
		a.Seek(at)
		return
	default:
		return
	}
	state := mygo.PlaybackPlaying
	if a.player.openHeld {
		state = mygo.PlaybackPaused
	}
	np.center.SetPlayback(state, a.player.opening)
}

// playerNowPlayingShow shows p, the picture that just opened, where the system shows what plays, and has p report its play and pause from now on. The same film opened again, by a seek or as it came back from the background, only says where it is; another film shows with its name and length, and its poster once read.
func (a *app) playerNowPlayingShow(p *filmPlay) {
	np := &a.player.nowPlaying
	if np.center == nil {
		return
	}
	p.nowPlaying = np.center
	if np.path == p.path {
		p.reportPlayback()
		return
	}
	np.path, np.title = p.path, a.playerNowPlayingTitle(p.path)
	np.center.Set(a.playerNowPlayingInfo(p, nil))
	a.playerNowPlayingArtwork(p.path)
}

// playerNowPlayingInfo is what the system shows of p, with artwork, the film's poster, when there is one.
func (a *app) playerNowPlayingInfo(p *filmPlay, artwork []byte) mygo.NowPlayingInfo {
	return mygo.NowPlayingInfo{
		Title:    a.player.nowPlaying.title,
		Artwork:  artwork,
		Duration: p.duration,
		Position: p.soundAt(time.Now()),
		State:    p.playbackState(),
		Commands: nowPlayingCommands,
	}
}

// playerNowPlayingTitle is the name of the film whose video is at path, as the library names it, or the video's own name for one the library does not list. It is the film's name, not interface text, so it is not translated.
func (a *app) playerNowPlayingTitle(path string) string {
	if item, ok := films.Find(a.films(), a.openKey); ok && item.VideoPath == path {
		return filmName(item)
	}
	return displayVideoName(path)
}

// playerNowPlayingArtwork reads the poster of the video at path off the UI thread and shows it with the film, while the system still shows that film. A video without a poster on disk shows without one.
func (a *app) playerNowPlayingArtwork(path string) {
	dest := posterDest(a.lib.posterDir, path)
	if dest == "" || a.update == nil {
		return
	}
	update := a.update
	go func() {
		artwork, err := os.ReadFile(dest)
		if err != nil || len(artwork) == 0 {
			return
		}
		update(func() {
			np := &a.player.nowPlaying
			p := a.player.play
			if np.center == nil || np.path != path || p == nil || p.path != path {
				return
			}
			np.center.Set(a.playerNowPlayingInfo(p, artwork))
		})
	}()
}

// playerNowPlayingOpening takes the film off where the system shows it as its playback ends for the video at path to open, unless seek says it opens again in the same film at another point, which keeps showing it through the moment its picture takes to open.
func (a *app) playerNowPlayingOpening(path string, seek bool) {
	if !seek || path != a.player.nowPlaying.path {
		a.playerNowPlayingClear()
	}
}

// playerNowPlayingSeeking says the film shown plays from at on, as a seek opens its picture there, so the system's scrubber moves at once rather than once the picture has opened.
func (a *app) playerNowPlayingSeeking(path string, at time.Duration) {
	np := &a.player.nowPlaying
	if np.center == nil || np.path != path {
		return
	}
	np.center.SetPlayback(mygo.PlaybackPlaying, at)
}

// playerNowPlayingClear takes the film off where the system shows it, so the media keys go to other apps again.
func (a *app) playerNowPlayingClear() {
	np := &a.player.nowPlaying
	if np.center == nil || np.path == "" {
		return
	}
	np.path, np.title = "", ""
	np.center.Clear()
}

// playbackState is whether p plays, as the system shows it: a film held or at its end is paused there.
func (p *filmPlay) playbackState() mygo.PlaybackState {
	if p.Playing() && !p.paused && !p.finished {
		return mygo.PlaybackPlaying
	}
	return mygo.PlaybackPaused
}

// reportPlayback tells the system whether p plays and where it is, so the play/pause key sends the command that changes that. Where it is is where its sound is (soundAt), which in the background has played on past the picture standing still behind the window.
func (p *filmPlay) reportPlayback() { p.reportPlaybackAt(p.soundAt(time.Now())) }

// reportPlaybackAt is reportPlayback with p at at, for a moment its playhead does not know yet, as play after the end, which starts over from 0.
func (p *filmPlay) reportPlaybackAt(at time.Duration) {
	if p.nowPlaying == nil {
		return
	}
	p.nowPlaying.SetPlayback(p.playbackState(), at)
}
