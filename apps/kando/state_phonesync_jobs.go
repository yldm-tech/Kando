package main

import (
	"context"
	"errors"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/packages/core"
	psync "kando/packages/sync"
)

// Phone Sync's protocol v2 (packages/sync, v2.go): a paired phone sends links to the desktop and follows and controls its queue. phoneJobs answers the calls that act on the queue, on the engine's goroutine (remote.OnUI); the jobs a phone follows are published to the server after every change of the app's state (phoneSyncPublish), and the server answers the list and the events from them. What the desktop does with a link a phone sends is the page's 接收手机发来的链接 and 收到后 (phoneUI.receiveOff, receiveAsk): queued at once, with 来自 <phone> on its card (core.Job.Device), put in the link field for the user to start, or refused. A locked library is never written: a link is refused while the open library is locked, and the jobs of a locked library are neither listed nor acted on (remoteQueue, remoteFindJob).

// phoneOps are the operations the desktop answers, the learning state's (state_phonesync_state.go), the lookups and explanations (state_phonesync_study.go), the settings' (state_phonesync_settings.go), the subscriptions' (state_phonesync_subscribe.go), the films' (state_phonesync_filmops.go), their word marks (state_phonesync_marks.go) and the glossary's (state_phonesync_glossary.go) among them.
var phoneOps = []string{psync.OpHello, psync.OpJobCreate, psync.OpJobs, psync.OpJob, psync.OpJobCancel, psync.OpJobRetry, psync.OpJobPause, psync.OpJobResume, psync.OpUpdateYTDLP, psync.OpEvents, psync.OpStateGet, psync.OpStatePut, psync.OpStateResolve, psync.OpLookUp, psync.OpExplain, psync.OpExplainAsk, psync.OpRecap, psync.OpSettingsGet, psync.OpSettingsPut, psync.OpSubscriptions, psync.OpSubscribe, psync.OpSubscriptionSet, psync.OpSubscriptionTake, psync.OpUnsubscribe, psync.OpFilmRetranslate, psync.OpFilmExport, psync.OpFilmDelete, psync.OpFilmVideo, psync.OpFilmMarks, psync.OpTerms, psync.OpTermPut}

// phoneOptions are the job options a phone may send with a link (psync.Hello.Options): the picture its download asks for and where its original lines come from, which the desktop's Settings › 通用 › 在线视频 names for every link otherwise.
var phoneOptions = []string{psync.OptionQuality, psync.OptionSubtitles}

// phoneJobOptions is what the options a phone sent with a link make of its job: a quality and a source of original lines, each "" for the desktop's own (onlineFor). A value the protocol does not name is refused.
func phoneJobOptions(o psync.JobOptions) (core.Quality, core.SubtitleSource, error) {
	var q core.Quality
	var src core.SubtitleSource
	switch o.Quality {
	case "":
	case psync.QualityBest:
		q = core.QualityBest
	case psync.Quality720:
		q = core.Quality720
	default:
		return "", "", psync.Errorf(psync.CodeInvalid, "quality %q is not one this desktop downloads at", o.Quality)
	}
	switch o.Subtitles {
	case "":
	case psync.SubtitlesPlatform:
		src = core.SubtitlesPlatform
	case psync.SubtitlesRecognize:
		src = core.SubtitlesLocal
	default:
		return "", "", psync.Errorf(psync.CodeInvalid, "subtitles %q is not a source this desktop takes", o.Subtitles)
	}
	return q, src, nil
}

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
		return psync.Hello{Ops: phoneOps, Options: phoneOptions, State: phonesync.StateKinds, Receive: a.phoneReceive()}, nil
	})
}

// CreateJob does with the link what the page's setting says: queues it as 开始处理 does, with the phone's name on its card and the quality and source of original lines the phone chose (phoneJobOptions), puts it in the link field for the user to start (phoneSyncAsk), where it starts with the desktop's own settings, as the phone, told Hello.Receive, offers no options then, or refuses it. A video the library has a job for already answers that job. A link is refused while the open library is locked, and with the reason the library's message bar would give while a job cannot start.
func (j phoneJobs) CreateJob(ctx context.Context, caller psync.Caller, req psync.JobCreate) (psync.JobCreated, error) {
	a := j.a
	quality, subtitles, err := phoneJobOptions(req.Options)
	if err != nil {
		return psync.JobCreated{}, err
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
			a.phoneLog(caller.Name, settings.ActivityLink, link.URL, 0, 0)
			return psync.JobCreated{Pending: true}, nil
		}
		if strings.TrimSpace(a.prefs.language) == "" {
			return psync.JobCreated{}, psync.Errorf(psync.CodeNotReady, "%s", m.QueueNoLanguage())
		}
		id, err := a.queueTakeFrom(link.URL, caller.Name, quality, subtitles)
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
		a.phoneLog(caller.Name, settings.ActivityLink, link.URL, 0, 0)
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

// ActOnJob cancels or retries a job as the control socket's jobs.cancel and jobs.retry do (remoteCancel, remoteRetry), which neither move the window, and pauses one where it is in the phone's name or lets it go on (phoneHold). It answers the job as they leave it: a job of another library leaves 处理中 once it has stopped, so it is looked up on the shelf rather than with remoteFindJob, and one of a video found in the folder leaves the shelf once stopped, as it was (queueFolderEnded).
func (j phoneJobs) ActOnJob(ctx context.Context, caller psync.Caller, op, id string) (psync.Job, error) {
	a := j.a
	var err error
	switch op {
	case psync.OpJobCancel:
		_, err = a.remoteCancel(ctx, id)
	case psync.OpJobRetry:
		_, err = a.remoteRetry(ctx, id)
	case psync.OpJobPause, psync.OpJobResume:
		err = a.phoneHold(ctx, caller, op == psync.OpJobPause, id)
	default:
		return psync.Job{}, psync.Errorf(psync.CodeUnsupported, "%s is not answered by this desktop", op)
	}
	if err != nil {
		return psync.Job{}, phoneRemoteErr(err)
	}
	return remote.OnUI(ctx, a.update, func() (psync.Job, error) {
		job, ok := a.library.Find(id)
		if ended, done := a.queue.folderEnded[id]; !ok && done {
			job, ok = ended, true
		} else if !ok {
			return psync.Job{}, psync.Errorf(psync.CodeNotFound, "job %q was removed", id)
		}
		if kind, logged := phoneActivityOf[op]; logged {
			a.phoneLog(caller.Name, kind, a.libraryJobName(job), 0, 0)
		}
		return a.phoneJobOf(job), nil
	})
}

// phoneActivityOf is what the page's list of what the phones did says of each action on a job.
var phoneActivityOf = map[string]string{psync.OpJobCancel: settings.ActivityCanceled, psync.OpJobRetry: settings.ActivityRetried, psync.OpJobPause: settings.ActivityPaused, psync.OpJobResume: settings.ActivityResumed}

// phoneHold pauses job id where it is in the name of the phone calling (queueHold), or lets it go on (queueLetGo), and the message bar says which phone paused it. A job of a locked library, or one the phone does not follow, is not found (remoteFindJob); one done or failed cannot be paused.
func (a *app) phoneHold(ctx context.Context, caller psync.Caller, pause bool, id string) error {
	_, err := remote.OnUI(ctx, a.update, func() (struct{}, error) {
		if _, err := a.remoteFindJob(id); err != nil {
			return struct{}{}, phoneRemoteErr(err)
		}
		if !pause {
			a.queueLetGo(id)
			return struct{}{}, nil
		}
		job, err := a.queueHold(id, caller.Name)
		if err != nil {
			return struct{}{}, psync.Errorf(psync.CodeConflict, "%v", err)
		}
		a.tell(a.messages().PhoneSyncPaused(caller.Name, a.libraryJobName(job)))
		return struct{}{}, nil
	})
	return err
}

// UpdateYTDLP starts 更新并重试 as a card of a download yt-dlp was too old for does (queueUpdateYTDLP): the downloads that failed for it run again once it is updated, as their events tell.
func (j phoneJobs) UpdateYTDLP(ctx context.Context, caller psync.Caller) (psync.ToolUpdate, error) {
	a := j.a
	return remote.OnUI(ctx, a.update, func() (psync.ToolUpdate, error) {
		if a.queue.ytdlp.updating {
			return psync.ToolUpdate{Started: true}, nil
		}
		a.queueUpdateYTDLP()
		if !a.queue.ytdlp.updating {
			return psync.ToolUpdate{}, psync.Errorf(psync.CodeNotReady, "%s", remoteMessages().PrecheckNoTools())
		}
		a.phoneLog(caller.Name, settings.ActivityYTDLP, "", 0, 0)
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
