package main

import (
	"fmt"
	"slices"

	"kando/packages/core"
	"kando/packages/media"
)

// The film opened goes first: a film made while it plays is made nearest where it is watched (queueWatch), and so, across films, the one opened is made before the others waiting. Opening a film whose subtitles are not made yet puts its job at the head of the queue; a job running that stopping loses no more than the work it is doing, as Make makes it (media.MakesAPartAtATime), gives way to it, going back to the queue right after it, as a paused job does (queuePausedJob), and goes on where it was once the opened film is made. A job that stopping would set back, as the download of a film's video (core.RedoVideo), runs on, and the opened film runs next.

// errYielding is the cause the running job is stopped with to give way to the film opened. It is a pause (errPausing), so the job reports nothing more and goes back to the queue once its pipeline has returned.
var errYielding = fmt.Errorf("%w for the film opened", errPausing)

// queuePreferOpen puts the open film's job first when its subtitles are not made yet and it waits, once for each film opened, and reports whether that changed the queue.
func (a *app) queuePreferOpen() bool {
	id := a.openKey
	if id == "" || id == a.queue.preferred || id == a.queue.active || a.queue.paused {
		return false
	}
	job, ok := a.library.Find(id)
	// A job that cannot run now, as one of a library not open on this computer or a download while offline mode is on (queueCanRun), would not run first: the job running does not give way to it.
	if !ok || job.Stage != core.StageQueued || queueHeld(job) || job.Redo != "" || !a.queueCanRun(job) {
		return false
	}
	a.queue.preferred = id
	a.queue.order = slices.Insert(slices.DeleteFunc(a.queue.order, func(queued string) bool { return queued == id }), 0, id)
	if running, ok := a.library.Find(a.queue.active); ok && media.MakesAPartAtATime(running) {
		a.queue.stopActive(errYielding)
	}
	a.pump()
	return true
}

// queueAfterPreferred is where in the queue a paused job goes back to: its head, or right after the film opened (queuePreferOpen) when that is first, so the job that gave way to it runs next.
func (a *app) queueAfterPreferred(id string) int {
	if p := a.queue.preferred; p != "" && p != id && len(a.queue.order) > 0 && a.queue.order[0] == p {
		return 1
	}
	return 0
}
