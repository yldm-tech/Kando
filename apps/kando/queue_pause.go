package main

import (
	"context"
	"errors"
	"slices"

	"kando/packages/core"
)

// Pausing the whole queue, from the tray icon's menu (state_tray.go) or 文件 › 暂停处理队列 (queueKeys): the job running stops and goes back to the head of the queue, as a job the app quit in the middle of is read back at the next launch, and no job starts until the queue is resumed, when that job starts again. The pause lasts until Kando quits.

// errPausing is the cause the running job is stopped with when the queue is paused. It is never shown: a job stopped with it reports nothing more, and goes back to the queue once its pipeline has returned (queuePausedJob).
var errPausing = errors.New("the queue is paused")

// queueTogglePause pauses the queue, or resumes it while it is paused.
func (a *app) queueTogglePause() { a.queueSetPaused(!a.queue.paused) }

// queueSetPaused pauses the queue (on) or resumes it. Pausing stops the running job; resuming starts the job at the head of the queue, unless the one paused has not returned yet, which then starts again as it does.
func (a *app) queueSetPaused(on bool) {
	if a.queue.paused == on {
		return
	}
	a.queue.paused = on
	if !on {
		a.pump()
		return
	}
	a.queue.stopActive(errPausing)
	a.queueTellSystem()
}

// queuePausedJob takes back job id, which the queue's pause stopped, once its pipeline has returned with result: a job that completed meanwhile is done as any other; one cancelled meanwhile (Cancel, Withdraw) ends cancelled, as it would have without the pause; and else it goes back to the head of the queue as queued, with what its stages reached and left on disk (result, whose reports the pause dropped), unless it was removed meanwhile. It starts again at once when the queue was resumed in the meantime.
func (a *app) queuePausedJob(id string, result core.Job) {
	defer a.queueTellSystem()
	if result.Stage == core.StageComplete {
		a.show(result)
		a.finished(id)
		return
	}
	cancelled := a.queue.active == id && a.queue.cancelled
	was, ok := a.library.Find(id)
	keep := ok && !a.queue.removed[id] && !was.Stage.Done()
	job := was
	if result.ID == id {
		job = result
	}
	switch {
	case cancelled:
		if keep {
			job.Stage = core.StageFailed
			if !errors.Is(job.Err, context.Canceled) {
				job.Err = context.Canceled
			}
			a.show(job)
		}
		a.finished(id)
		return
	case keep:
		job.Stage, job.Progress, job.Err = core.StageQueued, core.Progress{}, nil
		a.queue.order = slices.Insert(slices.DeleteFunc(a.queue.order, func(queued string) bool { return queued == id }), 0, id)
		a.show(job)
	}
	if a.queue.active == id {
		a.queue.active, a.queue.cancel, a.queue.done, a.queue.whisperModel = "", nil, nil, ""
	}
	a.pump()
	a.queueKeepAwake()
}
