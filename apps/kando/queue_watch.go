package main

import (
	"sync/atomic"
	"time"
)

// queueJump is how far the playhead must move between two of the player's reports, about a quarter of a second apart while it plays, to count as a jump the film being made gives way to (media.Watcher).
const queueJump = 3 * time.Second

// queueWatch is where the film the running job makes is watched (media.Watcher): where it will be watched from (core.Job.ResumeAt) until it is watched, then the playhead of the player watching it (queueWatched): this computer's, which its reports move (queuePlaying), or a phone's (queueWatchedBy). Make reads it on its own goroutine.
type queueWatch struct {
	at     atomic.Int64
	jumped chan struct{}
}

func newQueueWatch(at time.Duration) *queueWatch {
	w := &queueWatch{jumped: make(chan struct{}, 1)}
	w.at.Store(int64(at))
	return w
}

func (w *queueWatch) Playhead() time.Duration { return time.Duration(w.at.Load()) }

func (w *queueWatch) Jumped() <-chan struct{} { return w.jumped }

// move puts the playhead at at, and says it jumped when it moved further than playing moves it.
func (w *queueWatch) move(at time.Duration) {
	was := time.Duration(w.at.Swap(int64(at)))
	if d := at - was; d > queueJump || d < -queueJump {
		select {
		case w.jumped <- struct{}{}:
		default:
		}
	}
}

// keep puts the playhead at at without saying it jumped: where a phone that watches the film says it is, every few seconds (psync.WatchEvery), which is as far as playing moved it.
func (w *queueWatch) keep(at time.Duration) { w.at.Store(int64(at)) }

// queuePlaying notes this computer's player as it plays (queueDeskSeen), and follows its playhead into the running job's watch while the film watched is the open film and the one the job makes, so the lines nearest where it is watched are made first. It changes nothing a binding reads.
func (a *app) queuePlaying() bool {
	a.queueDeskSeen()
	if at, ok := a.queueWatched(); ok && a.queue.watch != nil && at.phone == "" && at.placed && at.film == a.queue.active {
		a.queue.watch.move(at.head)
	}
	return false
}
