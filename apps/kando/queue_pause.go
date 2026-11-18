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
		// A job a phone held keeps the phone's name (queueHold), which the pipeline's copy of it does not have.
		job.Stage, job.Progress, job.Err, job.PausedBy = core.StageQueued, core.Progress{}, nil, was.PausedBy
		a.queue.order = slices.Insert(slices.DeleteFunc(a.queue.order, func(queued string) bool { return queued == id }), 0, id)
		a.show(job)
	}
	if a.queue.active == id {
		a.queue.active, a.queue.cancel, a.queue.done, a.queue.whisperModel = "", nil, nil, ""
	}
	a.pump()
	a.queueKeepAwake()
}

// Pausing one job, from a paired phone (protocol v2's jobs.pause, state_phonesync_jobs.go): a job waiting or running is held where it is, named by the phone that paused it (core.Job.PausedBy), which its card says with 在这台电脑上继续. A running one stops and goes back to the queue as the whole queue's pause sends it back (queuePausedJob), keeping what its stages reached, and the jobs behind it go on. A held job keeps its place in the queue but is not started until it is let go on, from the phone (jobs.resume) or its card (queueLetGo), and it is held across a restart, as library.json keeps the name.

// errHeld is why queueHold cannot hold a job: one that is not waiting or running; errNoJob is a job the shelf does not have.
var (
	errHeld  = errors.New("only a job waiting or running can be paused")
	errNoJob = errors.New("no such job")
)

// queueHold holds job id where it is for the phone named by, as phone sync's jobs.pause does, and reports the job as held. A running job stops, and goes back to the queue once its pipeline has returned. A job already held is left held by the phone that held it.
func (a *app) queueHold(id, by string) (core.Job, error) {
	job, ok := a.library.Find(id)
	if !ok {
		return core.Job{}, errNoJob
	}
	if job.PausedBy != "" {
		return job, nil
	}
	if job.Stage != core.StageQueued && id != a.queue.active {
		return core.Job{}, errHeld
	}
	job.PausedBy = by
	a.queueKeepHeld(job)
	if id == a.queue.active {
		a.queue.stopActive(errPausing)
	}
	return job, nil
}

// queueLetGo lets job id, which a phone held, wait for its turn again, and starts it if nothing runs, as 在这台电脑上继续 on its card and phone sync's jobs.resume do. A job not held is left as it is.
func (a *app) queueLetGo(id string) {
	job, ok := a.library.Find(id)
	if !ok || job.PausedBy == "" {
		return
	}
	job.PausedBy = ""
	a.queueKeepHeld(job)
	a.pump()
}

// queueKeepHeld keeps job, whose PausedBy changed, on the shelf. show leaves the name as the shelf has it while a job runs, so the running pipeline's copy of a job, which is older, cannot let it go on (show).
func (a *app) queueKeepHeld(job core.Job) {
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().QueueSaveFailed)
	}
	a.areasShelfChanged()
	a.queueTellSystem()
}

// queueHeld reports whether job waits held by a phone (core.Job.PausedBy), which the queue does not start.
func queueHeld(job core.Job) bool { return job.PausedBy != "" && job.Stage == core.StageQueued }
