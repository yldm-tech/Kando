package main

import (
	"os"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/nowplaying"
)

// The open film where the system shows what plays: Now Playing in Control Center and the menu bar on macOS, the media controls of Windows and the MPRIS clients of Linux. The media keys of keyboards and headsets, and those controls, play, pause and move the film while it shows there, the window in sight or not.

// nowPlayingCommands are the commands the system's controls offer for a film: play, pause and the scrubber. A film has no next or previous track, and stopping is closing it in the window.
const nowPlayingCommands = nowplaying.Play | nowplaying.Pause | nowplaying.Seek

// playerNowPlayingConnect has the player show the open film through center, and play, pause and move it as the media keys and the system's controls ask. The commands may come on another thread and run as a background report does, through update, so the window shows what they changed.
func (a *app) playerNowPlayingConnect(center nowplaying.Center) {
	a.player.nowPlaying.center = center
	center.OnCommand(func(cmd nowplaying.Command, at time.Duration) {
		a.update(func() { a.playerMediaCommand(cmd, at) })
	})
}

// playerMediaCommand plays, pauses or moves the open film as a media key or the system's controls asked. A command that would leave it as it is, as play while it plays, tells the system again how it is instead. The system's player plays the picture with the sound whether the window is in sight or not, so a command behind the window does as one in front of it. While the system's player opens the film, play and pause have it open playing or held, and the system learns so at once, so the key is not lost; the scrubber has it open there. Without a film there is nothing to command.
func (a *app) playerMediaCommand(cmd nowplaying.Command, at time.Duration) {
	p := a.player.play
	if p == nil {
		return
	}
	if cmd == nowplaying.Seek {
		a.Seek(at)
		return
	}
	if !p.loaded {
		switch cmd {
		case nowplaying.Pause:
			p.paused = true
		case nowplaying.Play:
			p.paused, p.played = false, true
		default:
			return
		}
		state := nowplaying.Playing
		if p.paused {
			state = nowplaying.Paused
		}
		if np := &a.player.nowPlaying; np.center != nil && np.path == p.path {
			np.center.SetPlayback(state, p.playhead())
		}
		return
	}
	playing := p.playbackState() == nowplaying.Playing
	switch {
	case cmd == nowplaying.Play && !playing, cmd == nowplaying.Pause && playing:
		p.toggle()
	default:
		p.reportPlayback()
	}
}

// playerNowPlayingShow shows p, the picture the system's player just loaded, where the system shows what plays, and has p report its play and pause from now on. The same film opened again only says where it is; another film shows with its name and length, and its poster once read.
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
func (a *app) playerNowPlayingInfo(p *filmPlay, artwork []byte) nowplaying.Info {
	return nowplaying.Info{
		Title:    a.player.nowPlaying.title,
		Artwork:  artwork,
		Duration: p.duration,
		Position: p.playhead(),
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

// playerNowPlayingArtwork reads the poster of the video at path off the engine's goroutine and shows it with the film, while the system still shows that film. A video without a poster on disk shows without one.
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

// playerNowPlayingSeeking says the film shown plays from at on, as a seek moves its picture there, so the system's scrubber moves at once rather than once the picture has landed.
func (a *app) playerNowPlayingSeeking(path string, at time.Duration) {
	np := &a.player.nowPlaying
	if np.center == nil || np.path != path {
		return
	}
	np.center.SetPlayback(nowplaying.Playing, at)
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
func (p *filmPlay) playbackState() nowplaying.State {
	if p.Playing() {
		return nowplaying.Playing
	}
	return nowplaying.Paused
}

// reportPlayback tells the system whether p plays and where it is, so the play/pause key sends the command that changes that.
func (p *filmPlay) reportPlayback() { p.reportPlaybackAt(p.playhead()) }

// reportPlaybackAt is reportPlayback with p at at, for a moment its playhead does not know yet, as play after the end, which starts over from 0.
func (p *filmPlay) reportPlaybackAt(at time.Duration) {
	if p.nowPlaying == nil {
		return
	}
	p.nowPlaying.SetPlayback(p.playbackState(), at)
}
