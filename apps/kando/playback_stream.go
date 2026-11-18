package main

import (
	"log"
	"path/filepath"
	"time"

	"kando/apps/kando/film"
	"kando/packages/core"
	"kando/packages/media"
)

// A film that keeps no video (core.Job.SubtitlesOnly, 仅字幕) streams from its site when it is opened: its job's tools look the stream up off the engine's goroutine (media.Tools.StreamURL), and the system's player plays it as it plays a file, with the film's subtitles over it, its place kept by the path its sound was downloaded to, as a downloaded film's is by its video. Offline mode blocks it, and where it cannot stream, as any site but YouTube or any system but macOS for now (media.Streams), or when the stream fails, the player offers to download its video (DownloadVideo), which makes it a downloaded film (core.RedoVideo). These are the player's film.Streamer.

// playerStream opens the open film's stream when it keeps no video and path, its video, is the one its sound was downloaded to, held at at, or playing from there when jump, as startPlayback opens a file; or, when it cannot stream now, says why on the player (film.StreamErr). Any other film is left without a picture.
func (a *app) playerStream(path string, at time.Duration, jump bool) {
	item, ok := a.openFilm()
	if !ok || !item.SubtitlesOnly || path == "" || item.VideoPath != path {
		return
	}
	switch {
	case a.privacy.offline:
		a.player.playErr = film.ErrStreamOffline
		return
	case !a.playerStreams(item.Source):
		a.player.playErr = film.ErrStreamNone
		return
	}
	job, found := a.library.Find(item.JobID)
	tools, isTools := a.queuePipeline(job).(media.Tools)
	if !found || !isTools || a.update == nil {
		a.player.playErr = film.ErrStreamFailed
		return
	}
	p := a.playerOpen(path, "", at, jump)
	p.stream = true
	// Offline mode turned on meanwhile stops the lookup (privacyStopNetwork).
	ctx, update, source := a.privacyContext(), a.update, job.Source
	go func() {
		url, err := tools.StreamURL(ctx, source)
		update(func() {
			if a.player.play != p {
				return
			}
			if err != nil {
				log.Printf("player: could not look up the stream of %s: %v", filepath.Base(path), err)
				a.playerStreamFailed(p)
				return
			}
			p.source = url
		})
	}()
}

// playerStreams reports whether a film that keeps no video, from source, streams on this computer (playerUI.streams).
func (a *app) playerStreams(source string) bool {
	if a.player.streams != nil {
		return a.player.streams(source)
	}
	return media.Streams(source)
}

// playerStreamFailed is the open film's stream, p, failing to be looked up or played: the player says so and offers to try again or to download the video.
func (a *app) playerStreamFailed(p *filmPlay) {
	a.player.playErr = film.ErrStreamFailed
	p.source, p.loaded = "", false
	a.playerNowPlayingClear()
}

// playerStreamFollow opens the open film again once what kept it from playing is gone: its video was downloaded (core.RedoVideo), or offline mode was turned off elsewhere than on its card. It reports whether it did (playerAfterChange).
func (a *app) playerStreamFollow() bool {
	if !film.StreamErr(a.player.playErr) {
		return false
	}
	item, ok := a.openFilm()
	if !ok || (item.SubtitlesOnly && !(a.player.playErr == film.ErrStreamOffline && !a.privacy.offline)) {
		return false
	}
	a.Open(item)
	return true
}

// FilmStreams reports whether the open film, one that keeps no video, streams on this computer.
func (a *app) FilmStreams() bool {
	item, ok := a.openFilm()
	return ok && a.playerStreams(item.Source)
}

// DownloadVideo downloads the open film's video, as the player's cards and 信息 offer for a film that keeps none (queueVideo).
func (a *app) DownloadVideo() {
	if item, ok := a.openFilm(); ok {
		a.queueVideo(item.JobID)
	}
}

// StreamOnline turns offline mode off, as the card of a film opened offline offers, and opens the film again, which streams it.
func (a *app) StreamOnline() {
	a.privacySetOffline(false)
	a.StreamAgain()
}

// StreamAgain opens the open film again, which looks its stream up anew.
func (a *app) StreamAgain() {
	if item, ok := a.openFilm(); ok {
		a.Open(item)
	}
}

// queueVideo queues done film id, which keeps no video, to download it beside its subtitles (core.RedoVideo), at the quality Settings › 通用 › 在线视频 names, or the best where that is 仅字幕. The film stays done meanwhile, and one whose download fails or is stopped is put back as it was (queueRedoEnded). While offline mode is on the job waits until it is off (privacyHolds). A film with its video, or whose job waits or runs, is left alone.
func (a *app) queueVideo(id string) {
	job, ok := a.library.Find(id)
	if !ok || !job.SubtitlesOnly || job.Stage != core.StageComplete {
		return
	}
	job.Redo, job.RedoLanguage = core.RedoVideo, ""
	job.Quality = a.onlineQuality()
	if !job.Quality.KeepsVideo() {
		job.Quality = core.QualityBest
	}
	a.queueAdd(job)
}
