package sync

import (
	"encoding/json"
	"net/http"
	"time"
)

// Protocol v2 (yldm-tech/Kando#208) lets a paired phone send links to the desktop and follow and control the desktop's queue. It is a set of messages rather than a set of paths, so that the same messages travel over any transport: on the local network a phone POSTs each Call to PathCall and reads the Reply as the answer, and the storage relay (yldm-tech/Kando#219) carries the same Call and Reply as files in a device's inbox and outbox, unchanged. What a transport adds is only who is calling (Caller), which on the local network is the device whose token the request carries.
//
// Negotiation. A phone that speaks v2 starts with OpHello. A desktop that speaks only v1 has no PathCall and answers it 404 not_found, which Client.Hello reports as ErrV1Only, and the phone keeps to v1's paths; a v1 phone never calls PathCall, and every v1 path stays as it was. A v2 desktop answers Hello with the versions it speaks, the operations it answers (Ops) and the job options it understands (Options): the phone offers only what is listed, and the desktop answers an operation it does not list with CodeUnsupported. New operations and options are added to the lists under the same version; a change that is not an addition makes a new version.
//
// Calls. Every Call has an ID the phone makes, unique among its calls (NewCallID): the desktop answers a call it has answered before, by the same device with the same ID, with the Reply it gave the first time, for a while (ReplyMemory), so a call sent twice, as a relay delivers at least once or a phone sends again after the network dropped, does its work once. A Call names the version it speaks (V, CallVersion) and an operation (Op) with its arguments (Args); the Reply names the same ID and Op, and carries either a Result or an Error, whose Code is one of v1's codes or v2's below. On the local network the answer's status is 200 for a Result, and StatusOf(code) for an Error.
//
// Jobs. A phone sends a link with OpJobCreate. The desktop's setting for links from phones decides what comes of it: queued at once (ReceiveQueue), put before the user on the desktop to start or not (ReceiveAsk, answered as Pending), or refused (ReceiveOff, CodeLinksOff). Nothing is ever added to a locked library, nor is a job of one listed or acted on: to a phone it is not found. OpJobs lists the jobs a phone follows, the desktop's queue (waiting, running, failed or canceled) and those that ended since the desktop started, each with its steps' progress and, for one that failed, a typed Failure. OpJobCancel, OpJobRetry, OpJobPause and OpJobResume act on one; OpUpdateYTDLP updates the downloader the desktop uses, after which the downloads that failed because it was too old run again.
//
// Read-only devices. A paired device may change everything by default; the desktop's user can make one read-only, which the desktop checks at every call, on every transport: the device reads its libraries, jobs, events and learning state, and every operation that changes something (Mutates) answers CodeReadOnly. Hello tells the device it is read-only (Hello.ReadOnly), so it offers none of them.
//
// Events. The desktop numbers every change of what it tells phones with a revision, counting up from 1 since it started, in an epoch named afresh at each start. OpEvents waits, up to the wait it is given (at most MaxEventWait), for changes after a revision and answers the newest of each (a job's last state, not each step it went through), with the revision to wait after next. Waiting with another epoch than the desktop's, as before the phone has listed the jobs, or after a revision the desktop no longer remembers, answers Reset at once: the phone lists the jobs again (OpJobs answers the revision its list is at) and waits after that. Event kinds a phone does not know are skipped, so later kinds can be added, as the learning state's (EventState) were.

// CallVersion is the version a Call speaks.
const CallVersion = 2

// PathCall takes a Call by POST and answers its Reply: protocol v2 on the local network.
const PathCall = "/v2/call"

// The versions of the protocol, as Hello lists them.
const (
	V1 = "v1"
	V2 = "v2"
)

// Call is a phone's request: its ID, the version it speaks, the operation and its arguments, one of the Op… types' JSON.
type Call struct {
	ID   string          `json:"id"`
	V    int             `json:"v"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Reply is the desktop's answer to the Call with the same ID: Result, the operation's answer's JSON, or Error.
type Reply struct {
	ID     string          `json:"id"`
	Op     string          `json:"op"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorBody      `json:"error,omitempty"`
}

// The operations of protocol v2, with what each takes as Call.Args and answers as Reply.Result.
const (
	// OpHello takes nothing and answers Hello.
	OpHello = "hello"
	// OpJobCreate takes JobCreate and answers JobCreated.
	OpJobCreate = "jobs.create"
	// OpJobs takes nothing and answers JobList.
	OpJobs = "jobs.list"
	// OpJob takes JobRef and answers Job.
	OpJob = "jobs.get"
	// OpJobCancel stops a job that is waiting or running; it takes JobRef and answers the Job as it is once stopped.
	OpJobCancel = "jobs.cancel"
	// OpJobRetry queues a job that failed or was canceled again; it takes JobRef and answers the Job once queued.
	OpJobRetry = "jobs.retry"
	// OpJobPause holds a job that is waiting or running where it is, without failing it; it takes JobRef and answers the Job.
	OpJobPause = "jobs.pause"
	// OpJobResume lets a paused job wait for its turn again; it takes JobRef and answers the Job.
	OpJobResume = "jobs.resume"
	// OpUpdateYTDLP takes nothing and answers ToolUpdate once the update has started; how it ended shows in the jobs it runs again.
	OpUpdateYTDLP = "tools.update_ytdlp"
	// OpEvents takes EventsWait and answers Events.
	OpEvents = "events.wait"
)

// Hello is what a desktop speaking v2 says of itself.
type Hello struct {
	// Desktop is the desktop's name.
	Desktop string `json:"desktop"`
	// Versions are the versions it speaks, as V1 and V2.
	Versions []string `json:"versions"`
	// Ops are the operations it answers; Options the job options (JobOptions' JSON names) it understands.
	Ops     []string `json:"ops"`
	Options []string `json:"options"`
	// State are the kinds of record of the learning state the desktop syncs (KindPosition, KindLine…), none when it does not serve it.
	State []string `json:"state"`
	// Receive is what it does with a link a phone sends: ReceiveQueue, ReceiveAsk or ReceiveOff.
	Receive string `json:"receive"`
	// ReadOnly is set when the desktop's user made the calling device read-only: it reads everything, and every operation that changes something answers CodeReadOnly, so the phone hides them.
	ReadOnly bool `json:"read_only,omitempty"`
	// Epoch names the desktop's run, which the revisions of its events count in.
	Epoch string `json:"epoch"`
}

// What the desktop does with a link a phone sends (Hello.Receive).
const (
	// ReceiveQueue queues it at once.
	ReceiveQueue = "queue"
	// ReceiveAsk puts it before the user on the desktop, who starts it or not; the phone hears JobCreated.Pending.
	ReceiveAsk = "ask"
	// ReceiveOff refuses it with CodeLinksOff.
	ReceiveOff = "off"
)

// JobCreate is a link to process, with the options it is processed with.
type JobCreate struct {
	Link    string     `json:"link"`
	Options JobOptions `json:"options"`
}

// JobOptions are how a job is processed, each "" for the desktop's own setting. A desktop refuses a job with an option it does not list in Hello.Options with CodeUnsupported, rather than process it otherwise than asked.
type JobOptions struct {
	// Quality is the video quality to download: QualityBest or a height such as "720p".
	Quality string `json:"quality,omitempty"`
	// Subtitles is where the original lines come from: SubtitlesPlatform, the site's own, or SubtitlesRecognize, the desktop's speech recognition.
	Subtitles string `json:"subtitles,omitempty"`
}

// The values of JobOptions: a desktop that lists OptionQuality takes QualityBest and Quality720, and one that lists OptionSubtitles takes SubtitlesPlatform and SubtitlesRecognize.
const (
	QualityBest        = "best"
	Quality720         = "720p"
	SubtitlesPlatform  = "platform"
	SubtitlesRecognize = "recognize"
)

// The job options a desktop may list in Hello.Options, as JobOptions' JSON names.
const (
	OptionQuality   = "quality"
	OptionSubtitles = "subtitles"
)

// JobCreated is the answer to JobCreate: the job queued, or Pending, when the desktop asks its user first and no job is made yet. A link the desktop has a job for already answers that job with Existing set, rather than a second job.
type JobCreated struct {
	Job      *Job `json:"job,omitempty"`
	Pending  bool `json:"pending,omitempty"`
	Existing bool `json:"existing,omitempty"`
}

// JobRef names a job.
type JobRef struct {
	ID string `json:"id"`
}

// JobList is the jobs a phone follows, as of revision Rev: the running one, the waiting ones in the order they will run, then the others, newest first.
type JobList struct {
	Epoch string `json:"epoch"`
	Rev   int64  `json:"rev"`
	Jobs  []Job  `json:"jobs"`
}

// Job is a job of the desktop's queue.
type Job struct {
	ID string `json:"id"`
	// Rev is the revision at which the job last changed.
	Rev  int64  `json:"rev"`
	Link string `json:"link"`
	// Title is the video's title once the desktop knows it, else what the desktop calls the job by.
	Title string `json:"title"`
	// Library is the ID of the library the job saves into (Library.ID).
	Library string `json:"library"`
	// Device is the name of the paired phone that sent the job's link, "" for a job added on the desktop.
	Device string `json:"device,omitempty"`
	State  string `json:"state"`
	// Steps are the job's steps in order: StepDownload, StepTranscribe and StepTranslate.
	Steps []Step `json:"steps"`
	// Failure is why a job in JobFailed failed; nil otherwise.
	Failure *Failure `json:"failure,omitempty"`
	// Film is the ID of the film the job made (Film.ID), "" until it is done.
	Film string `json:"film,omitempty"`
}

// The states of a Job.
const (
	JobWaiting  = "waiting"
	JobRunning  = "running"
	JobPaused   = "paused"
	JobDone     = "done"
	JobFailed   = "failed"
	JobCanceled = "canceled"
)

// Step is a step of a job: what it is, where it stands and how far it has gone.
type Step struct {
	Name  string `json:"name"`
	State string `json:"state"`
	// Progress is how far a running step has gone, from 0 to 1, or -1 when its tool has not said; 1 for a step done, 0 for one waiting.
	Progress float64 `json:"progress"`
}

// The names of the Steps.
const (
	StepDownload   = "download"
	StepTranscribe = "transcribe"
	StepTranslate  = "translate"
)

// The states of a Step. StepSkipped is a step the job does not need: the download of a video already on the desktop, or the translation of a film heard in the language its subtitles are for.
const (
	StepWaiting = "waiting"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Failure is why a job failed: the step that failed, a code the phone tells the user by, a message in the desktop's words for logs, and Remedy, what the phone can offer to do about it. A failure of the desktop's AI service or of a translation source carries no Message: neither what the service answered, which may hold its prices or links to buy credit, nor the desktop's own words of it, which may tell its user to top an account up, reach a phone, which tells the user by the code alone (FailAIUnavailable, FailTranslation, FailDesktopSetup).
type Failure struct {
	Step    string `json:"step"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Remedy  string `json:"remedy,omitempty"`
}

// The codes of Failure. A phone that does not know a code tells the user Message, and its own words for a failure it does not know (FailOther's) when Message is empty, so a code is added to v2 without a new version.
const (
	FailNetwork      = "network"
	FailRateLimited  = "rate_limited"
	FailHost         = "host_unreachable"
	FailBadLink      = "bad_link"
	FailUnsupported  = "unsupported_site"
	FailOutdated     = "downloader_outdated"
	FailLogin        = "login_required"
	FailLoginExpired = "login_expired"
	FailCookies      = "login_unreadable"
	FailMembers      = "members_only"
	FailAge          = "age_restricted"
	FailRegion       = "region_blocked"
	FailPrivate      = "private"
	FailDRM          = "drm"
	FailHiddenVideo  = "hidden_video"
	FailNoSpeech     = "no_speech"
	FailDesktopSetup = "desktop_setup"
	FailTranslation  = "translation"
	// FailAIUnavailable is the desktop's AI service, or a translation source of its own, refusing the account rather than the request, as one out of credit or over its quota: nothing a retry or the phone mends, so it comes with RemedyDesktop and an empty Message. An addition to v2: a phone from before it tells the user FailOther's words.
	FailAIUnavailable = "ai_unavailable"
	FailOther         = "failed"
)

// The remedies of Failure.
const (
	// RemedyRetry is a failure that may go away by itself, as the network's: OpJobRetry.
	RemedyRetry = "retry"
	// RemedyUpdateYTDLP is a downloader too old for the site: OpUpdateYTDLP.
	RemedyUpdateYTDLP = "update_ytdlp"
	// RemedyDesktop is a failure the user fixes on the desktop, as a setting or a login.
	RemedyDesktop = "desktop"
)

// ToolUpdate is the answer to OpUpdateYTDLP.
type ToolUpdate struct {
	Started bool `json:"started"`
}

// EventsWait asks for the events after revision Since of epoch Epoch, waiting up to WaitMS milliseconds (at most MaxEventWait) for one.
type EventsWait struct {
	Epoch  string `json:"epoch"`
	Since  int64  `json:"since"`
	WaitMS int64  `json:"wait_ms"`
}

// MaxEventWait is the longest an OpEvents call waits.
const MaxEventWait = 25 * time.Second

// Events is the answer to EventsWait: the newest event of each thing that changed after Since, in the order of their revisions, and Rev, the revision to wait after next. Reset says Since was not one the desktop can count from: the phone lists the jobs again.
type Events struct {
	Epoch  string  `json:"epoch"`
	Rev    int64   `json:"rev"`
	Reset  bool    `json:"reset,omitempty"`
	Events []Event `json:"events"`
}

// Event is a change: of a job (EventJob, with the job as it is now), a job no longer followed (EventJobGone, with its ID), of a library's learning state (EventState, with the library's ID), or of what a library's films are listed as (EventFilms, with the library's ID). A phone ignores a kind it does not know.
type Event struct {
	Rev  int64  `json:"rev"`
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Job  *Job   `json:"job,omitempty"`
}

// The kinds of Event.
const (
	EventJob     = "job"
	EventJobGone = "job.gone"
	// EventFilms is that library ID's films are listed otherwise for a change no job made, as the user showing a work's films a card each on the desktop: the phone lists the desktop's films again. An addition to v2, which a phone from before it ignores.
	EventFilms = "films"
)

// The codes of ErrorBody that v2 adds to v1's.
const (
	// CodeUnsupported is an operation, a version or a job option the desktop does not speak (status 501).
	CodeUnsupported = "unsupported"
	// CodeLinksOff is a link sent while the desktop does not take links from phones (ReceiveOff, status 403).
	CodeLinksOff = "links_off"
	// CodeNotReady is a job the desktop cannot start now, as without a library open, with the open library locked or without a language to translate into; Message says why (status 409).
	CodeNotReady = "not_ready"
	// CodeConflict is an action a job's state does not allow, as retrying one still running (status 409).
	CodeConflict = "conflict"
	// CodeReadOnly is an operation that changes something, called by a device the desktop's user made read-only (Hello.ReadOnly, status 403).
	CodeReadOnly = "read_only"
)

// StatusOf is the HTTP status of an answer with error code code.
func StatusOf(code string) int {
	switch code {
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeNoCode, CodeWrongCode, CodeCodeUsed, CodeCodeExpired, CodeLinksOff, CodeReadOnly, CodeWrongPassword:
		return http.StatusForbidden
	case CodeTooMany:
		return http.StatusTooManyRequests
	case CodeNotFound:
		return http.StatusNotFound
	case CodeInvalid:
		return http.StatusBadRequest
	case CodeUnsupported:
		return http.StatusNotImplemented
	case CodeNotReady, CodeConflict, CodeNoAI:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
