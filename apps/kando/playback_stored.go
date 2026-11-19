package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"time"

	"kando/apps/kando/storage"
)

// playerStored opens the video at path from the storage location it was moved to (state_stored.go), as startPlayback opens a file, held at at or playing from there when jump: off the engine's goroutine the location is opened and the video looked up there, and the system's player then plays it through the loopback proxy (storage.Proxy). It reports false for a video that was not moved. A location no longer in the settings, one that cannot be reached and one without the video say so on the page (playErr), which then shows the film's lines alone, as for a missing file.
func (a *app) playerStored(path string, at time.Duration, jump bool) bool {
	s, ok := a.storedOf(path)
	if !ok {
		return false
	}
	p := a.playerOpen(path, "", at, jump)
	p.stored = true
	t, found := a.storedTargetOf(s.Location)
	switch {
	case !found:
		a.player.playErr = playerErrStoredGone
		return true
	case a.update == nil || (t.Store != nil && a.privacy.offline):
		// Offline mode keeps the internet, and with it an object store, closed.
		a.player.playErr = playerErrStoredUnreachable
		return true
	}
	pool, update := a.stored.pool, a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), storage.PoolOpenTimeout)
		defer cancel()
		url, err := pool.URL(ctx, t, s.Path)
		update(func() {
			if a.player.play != p {
				return
			}
			switch {
			case errors.Is(err, storage.ErrNotExist):
				a.player.playErr = playerErrStoredMissing
			case err != nil:
				log.Printf("player: could not open the storage location of %s: %v", filepath.Base(path), err)
				a.player.playErr = playerErrStoredUnreachable
			default:
				p.source = url
				return
			}
			a.playerNowPlayingClear()
		})
	}()
	return true
}
