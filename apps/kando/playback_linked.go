package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"time"

	"kando/apps/kando/mediasource"
	"kando/packages/media"
)

// playerLinked opens the video of the film at path, indexed from a media source (state_mediasource_import.go), as startPlayback opens a file, held at at or playing from there when jump, as playerStored does for a video moved to a storage location: off the engine's goroutine the source is reached and the video looked up there, and the system's player then plays it through the sources' loopback server (mediasource.Pool.URL). It reports false for a film that was not indexed. A source no longer in the settings, one that cannot be reached, one on the internet while offline mode is on, which the page says is offline mode's doing, and one without the video say so on the page (playErr), which then shows the film's lines alone. A file that changed on the source since it was indexed plays all the same, the bar over the picture saying so (linkedChanged).
func (a *app) playerLinked(path string, at time.Duration, jump bool) bool {
	l, ok := a.linkedOf(path)
	if !ok {
		return false
	}
	p := a.playerOpen(path, "", at, jump)
	p.linked = true
	i := a.mediaSourceIndex(l.Source)
	switch {
	case i < 0:
		a.player.playErr = playerErrLinkedGone
		return true
	case a.update == nil:
		a.player.playErr = playerErrLinkedUnreachable
		return true
	}
	a.mediaSourcePoolSync()
	s, pool, offline, local, update := a.sources.list[i], a.sources.pool, a.privacy.offline, a.sources.localHost, a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mediasource.PoolOpenTimeout)
		defer cancel()
		var e mediasource.Entry
		var url string
		err := mediaSourceOfflineCheck(ctx, offline, s, local)
		if err == nil {
			var d mediasource.Driver
			if d, err = pool.Driver(ctx, s.ID); err == nil {
				e, err = d.Stat(ctx, l.Path)
			}
		}
		if err == nil && e.Dir {
			err = mediasource.ErrNotExist
		}
		if err == nil {
			url, err = pool.URL(ctx, s.ID, l.Path)
		}
		update(func() {
			if a.player.play != p {
				// The page moved on while the video was looked up: nothing plays the address.
				if err == nil {
					pool.Revoke(url)
				}
				return
			}
			switch {
			case errors.Is(err, mediasource.ErrNotExist):
				a.player.playErr = playerErrLinkedMissing
			case errors.Is(err, media.ErrOffline):
				a.player.playErr = playerErrLinkedOffline
			case err != nil:
				log.Printf("player: could not reach the media source of %s: %v", filepath.Base(path), err)
				a.player.playErr = playerErrLinkedUnreachable
			default:
				p.source, p.storedURL = url, url
				if linkedChangedFrom(l, e) {
					a.player.linkedChanged = &linkedChange{path: path, now: e}
				}
				return
			}
			a.playerNowPlayingClear()
		})
	}()
	return true
}

// playerRetryLinked opens the open film's video from its media source again, where the picture was, as 重试 does once the source could not be reached.
func (a *app) playerRetryLinked() {
	path := a.player.playPath
	if _, ok := a.linkedOf(path); !ok || a.player.playErr == "" {
		return
	}
	a.dropPlayback()
	a.playerLinked(path, a.player.opening, false)
}
