package main

import (
	"time"

	"kando/apps/kando/phonesync"
	psync "kando/packages/sync"
)

// phoneWatched is told, on a request's goroutine, that a paired phone plays film f while it is made (psync.OpFilmWatch), at at, claimed by its user when claim is set: the queue makes it first and nearest where it is watched while it is the film watched (queueWatchedBy), as the film open on this computer, and at once rather than at the next change of the app (afterChange).
func (a *app) phoneWatched(caller psync.Caller, f phonesync.Film, at time.Duration, claim bool) {
	a.update(func() {
		a.queueWatchedBy(caller.DeviceID, f.ID, at, claim)
		a.queuePreferOpen()
	})
}
