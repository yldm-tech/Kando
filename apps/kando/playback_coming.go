package main

import (
	"os"
	"time"

	"kando/apps/kando/films"

	ns "github.com/nsgo-dev/nsgo/core"
)

// A link's video comes after its sound: its job downloads the sound alone first, makes the subtitles from it while the video downloads beside (core.Job.SoundPath), and the film plays that sound until then, by the path its video will have (films.PlayPath), the stage saying the picture follows (PictureComing). Once the video is in, the film plays it from where the sound was, held or playing as it was; its place, its subtitle delay and what else is kept by its path move to the video's (queueSoundToVideo). A film opened before anything of it is in, as a link waiting in the queue, which opening puts first (queuePreferOpen), plays once its sound is.

// playerStandIn opens the open film's sound in the system's player when path, which no file is at, is the one it goes by until its video comes, held at at, or playing from there when jump, as startPlayback opens a file. It reports whether it did.
func (a *app) playerStandIn(path string, at time.Duration, jump bool) bool {
	item, ok := a.openFilm()
	if !ok || item.VideoPath != "" || item.SoundPath == "" || films.PlayPath(item) != path {
		return false
	}
	if _, err := os.Stat(item.SoundPath); err != nil {
		return false
	}
	a.playerOpen(path, item.SoundPath, at, jump).standIn = item.SoundPath
	return true
}

// PictureComing reports whether the open film plays its sound until its video comes (playerStandIn).
func (a *app) PictureComing() bool { return a.player.play != nil && a.player.play.standIn != "" }

// ComingCover is the site's thumbnail of the open film while it plays its sound until its video comes, none else.
func (a *app) ComingCover() ns.ImageSource {
	item, ok := a.openFilm()
	if !ok || !a.PictureComing() {
		return ns.ImageSource{}
	}
	return a.thumbnailOf(item)
}

// playerComingFollow plays the open film's video once it has come in place of its sound, from where the sound was, held or playing as it was, and plays a film opened before anything of it was in once its sound or its video is. It reports whether it did (playerAfterChange).
func (a *app) playerComingFollow() bool {
	item, ok := a.openFilm()
	if !ok {
		return false
	}
	p := a.player.play
	switch {
	case p != nil && p.standIn != "" && item.VideoPath != "":
		at, jump := p.head, !p.paused && !p.finished
		a.dropPlayback()
		a.playerNowPlayingClear()
		a.player.film.video = item.VideoPath
		a.playerOpen(item.VideoPath, item.VideoPath, at, jump)
		a.readFilmInfo(item.VideoPath)
		return true
	case p == nil && a.player.playPath == "" && a.player.playErr == "" && films.Watchable(item) && a.player.film.video != films.PlayPath(item):
		a.Open(item)
		return true
	}
	return false
}
