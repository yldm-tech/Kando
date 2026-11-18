package main

import (
	"context"
	"errors"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/packages/core"
	psync "kando/packages/sync"
)

// Phone Sync's protocol v2 (packages/sync, v2.go): a paired phone sends links to the desktop and follows and controls its queue. phoneJobs answers the calls that act on the queue, on the engine's goroutine (remote.OnUI); the jobs a phone follows are published to the server after every change of the app's state (phoneSyncPublish), and the server answers the list and the events from them. What the desktop does with a link a phone sends is the page's 接收手机发来的链接 and 收到后 (phoneUI.receiveOff, receiveAsk): queued at once, with 来自 <phone> on its card (core.Job.Device), put in the link field for the user to start, or refused. A locked library is never written: a link is refused while the open library is locked, and the jobs of a locked library are neither listed nor acted on (remoteQueue, remoteFindJob).

// phoneOps are the operations the desktop answers. Pausing and resuming a job are not among them yet: the queue cannot hold a job where it is.
var phoneOps = []string{psync.OpHello, psync.OpJobCreate, psync.OpJobs, psync.OpJob, psync.OpJobCancel, psync.OpJobRetry, psync.OpUpdateYTDLP, psync.OpEvents}

// phoneReceive is what the desktop does with a link a phone sends, as Hello says it.
func (a *app) phoneReceive() string {
	switch {
	case a.phone.receiveOff:
		return psync.ReceiveOff
	case a.phone.receiveAsk:
		return psync.ReceiveAsk
	}
	return psync.ReceiveQueue
}

// phoneSyncSetReceive turns 接收手机发来的链接 on or off, and saves it.
func (a *app) phoneSyncSetReceive(on bool) {
	a.phone.receiveOff = !on
	a.Save()
}

// phoneSyncSetAsk sets 收到后 to 先问我 (ask) or 直接加入队列, and saves it.
func (a *app) phoneSyncSetAsk(ask bool) {
	a.phone.receiveAsk = ask
	a.Save()
}

// phoneSyncPublish tells the server the jobs phones follow, when it runs: the queue as 处理中 shows it, of the open library and those the queue runs for others, and the jobs the queue ran to the end since launch, never one of a locked library (remoteQueue). It runs after every change of the app's state and changes nothing the window reads.
func (a *app) phoneSyncPublish() bool {
	srv := a.phone.server
	if srv == nil {
		return false
	}
	srv.PublishJobs(a.phoneJobList())
	return false
}

// phoneJobList is the jobs phones follow, as phoneSyncPublish tells them. UI thread only.
func (a *app) phoneJobList() []psync.Job {
	out := []psync.Job{}
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return out
	}
	running, waiting, finished := a.remoteQueue()
	for _, job := range slices.Concat(running, waiting, finished) {
		if job.Stage == core.StageComplete && !a.queue.ran[job.ID] {
			continue
		}
		out = append(out, a.phoneJobOf(job))
	}
	return out
}

// phoneJobOf is job as protocol v2 tells it, its failure in English as the control socket says it. A job that was processing a done film again and failed or was stopped is on the shelf as the done film it was, and is told as it ended, as the control socket tells it (remoteJobOf). UI thread only.
func (a *app) phoneJobOf(job core.Job) psync.Job {
	if ended, ok := a.queue.redoEnded[job.ID]; ok && job.Stage == core.StageComplete {
		job = ended
	}
	facts := phonesync.JobFacts{Title: a.libraryJobName(job), Library: phoneLibraryID(job.LibraryDir)}
	if job.Stage == core.StageFailed {
		m := remoteMessages()
		var section string
		facts.Reason, section = failureHint(m, films.FromJob(job, facts.Title, statusText(m, job)))
		facts.Setup = section != ""
	}
	return phonesync.JobOf(job, facts)
}

// phoneJobs is the app's queue as protocol v2's calls act on it (psync.Service).
type phoneJobs struct{ a *app }

func (j phoneJobs) Hello(ctx context.Context, _ psync.Caller) (psync.Hello, error) {
	a := j.a
	return remote.OnUI(ctx, a.update, func() (psync.Hello, error) {
		return psync.Hello{Ops: phoneOps, Options: []string{}, Receive: a.phoneReceive()}, nil
	})
}

// CreateJob does with the link what the page's setting says: queues it as 开始处理 does, with the phone's name on its card, puts it in the link field for the user to start (phoneSyncAsk), or refuses it. A video the library has a job for already answers that job. A link is refused while the open library is locked, and with the reason the library's message bar would give while a job cannot start.
func (j phoneJobs) CreateJob(ctx context.Context, caller psync.Caller, req psync.JobCreate) (psync.JobCreated, error) {
	a := j.a
	if req.Options != (psync.JobOptions{}) {
		return psync.JobCreated{}, psync.Errorf(psync.CodeUnsupported, "this desktop takes no job options yet")
	}
	link, ok := core.ParseLink(req.Link)
	if !ok || link.Platform == core.PlatformLocal {
		return psync.JobCreated{}, psync.Errorf(psync.CodeInvalid, "%q is not a link to a video on the web", req.Link)
	}
	return remote.OnUI(ctx, a.update, func() (psync.JobCreated, error) {
		m := remoteMessages()
		switch {
		case a.phone.receiveOff:
			return psync.JobCreated{}, psync.Errorf(psync.CodeLinksOff, "this desktop does not take links from phones; turn on Receive Links from Phones in Settings › Phone Sync")
		case a.lockShut():
			msg, _ := a.precheckReadyIn(m)
			return psync.JobCreated{}, psync.Errorf(psync.CodeNotReady, "%s", msg)
		}
		if msg, _ := a.precheckIn(m, []string{link.URL}); msg != "" {
			return psync.JobCreated{}, psync.Errorf(psync.CodeNotReady, "%s", msg)
		}
		if a.phone.receiveAsk {
			a.phoneSyncAsk(caller.Name, link.URL)
			return psync.JobCreated{Pending: true}, nil
		}
		if strings.TrimSpace(a.prefs.language) == "" {
			return psync.JobCreated{}, psync.Errorf(psync.CodeNotReady, "%s", m.QueueNoLanguage())
		}
		id, err := a.queueTake(link.URL)
		var dup *queueDuplicate
		switch {
		case errors.As(err, &dup):
			job, ok := a.library.Find(dup.ID)
			if !ok {
				return psync.JobCreated{Existing: true}, nil
			}
			pj := a.phoneJobOf(job)
			return psync.JobCreated{Job: &pj, Existing: true}, nil
		case err != nil:
			return psync.JobCreated{}, psync.Errorf(psync.CodeInvalid, "%s", sourceError(m, err))
		}
		job, _ := a.library.Find(id)
		job.Device = caller.Name
		a.show(job)
		a.tell(a.messages().PhoneSyncLinkQueued(caller.Name))
		pj := a.phoneJobOf(job)
		return psync.JobCreated{Job: &pj}, nil
	})
}

// phoneSyncAsk adds link, sent by the phone named device, to the link field above the library, after what is there already, for the user to start or not, and the message bar says where it came from. The window goes to the library unless a film is open, which is left playing.
func (a *app) phoneSyncAsk(device, link string) {
	if a.openKey == "" && a.page != pageFilms {
		a.Select(pageFilms)
	}
	// The field takes several links between spaces (Start), so the link joins what is there, the user's typing or links phones sent before, rather than replacing it.
	if !slices.Contains(strings.Fields(a.queue.source), link) {
		a.queue.source = strings.TrimSpace(strings.TrimSpace(a.queue.source) + " " + link)
	}
	a.queue.status, a.queue.linkFocus = "", true
	a.tell(a.messages().PhoneSyncLinkAsk(device))
	a.windowUnhide()
}

// ActOnJob cancels or retries a job as the control socket's jobs.cancel and jobs.retry do (remoteCancel, remoteRetry), which neither move the window; pausing and resuming are not answered yet. It answers the job as they leave it: a job of another library leaves 处理中 once it has stopped, so it is looked up on the shelf rather than with remoteFindJob, and one of a video found in the folder leaves the shelf once stopped, as it was (queueFolderEnded).
func (j phoneJobs) ActOnJob(ctx context.Context, _ psync.Caller, op, id string) (psync.Job, error) {
	a := j.a
	var err error
	switch op {
	case psync.OpJobCancel:
		_, err = a.remoteCancel(ctx, id)
	case psync.OpJobRetry:
		_, err = a.remoteRetry(ctx, id)
	default:
		return psync.Job{}, psync.Errorf(psync.CodeUnsupported, "%s is not answered by this desktop", op)
	}
	if err != nil {
		return psync.Job{}, phoneRemoteErr(err)
	}
	return remote.OnUI(ctx, a.update, func() (psync.Job, error) {
		job, ok := a.library.Find(id)
		if ended, done := a.queue.folderEnded[id]; !ok && done {
			return a.phoneJobOf(ended), nil
		}
		if !ok {
			return psync.Job{}, psync.Errorf(psync.CodeNotFound, "job %q was removed", id)
		}
		return a.phoneJobOf(job), nil
	})
}

// UpdateYTDLP starts 更新并重试 as a card of a download yt-dlp was too old for does (queueUpdateYTDLP): the downloads that failed for it run again once it is updated, as their events tell.
func (j phoneJobs) UpdateYTDLP(ctx context.Context, _ psync.Caller) (psync.ToolUpdate, error) {
	a := j.a
	return remote.OnUI(ctx, a.update, func() (psync.ToolUpdate, error) {
		if a.queue.ytdlp.updating {
			return psync.ToolUpdate{Started: true}, nil
		}
		a.queueUpdateYTDLP()
		if !a.queue.ytdlp.updating {
			return psync.ToolUpdate{}, psync.Errorf(psync.CodeNotReady, "%s", remoteMessages().PrecheckNoTools())
		}
		return psync.ToolUpdate{Started: true}, nil
	})
}

// phoneRemoteErr is an error of the control socket's (remote.Error) as protocol v2 says it.
func phoneRemoteErr(err error) error {
	var e *remote.Error
	if !errors.As(err, &e) {
		return err
	}
	code := psync.CodeFailed
	switch e.Code {
	case remote.CodeNotFound:
		code = psync.CodeNotFound
	case remote.CodeNotReady:
		code = psync.CodeNotReady
	case remote.CodeInvalid:
		code = psync.CodeConflict
	}
	return psync.Errorf(code, "%s", e.Message)
}
