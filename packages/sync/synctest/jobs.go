package synctest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"

	"kando/packages/sync"
)

// queue is the stand-in desktop's queue for protocol v2: what it does with a link a phone sends, the jobs, the links put before the user to start or not, and how many times the downloader was updated.
type queue struct {
	receive string
	jobs    []sync.Job
	made    int
	pending []string
	updates int
	// drops are how many links still to take and hang up on (DropCreates); older is a desktop speaking protocol v1 alone (SetOlder).
	drops int
	older bool
	// options are the job options the desktop takes (SetOptions), and asked those each link it took or put before the user came with (Asked); readOnly makes every paired device read-only (SetReadOnly).
	options  []string
	asked    []sync.JobOptions
	readOnly bool
}

// SetOptions has the desktop take the job options named (sync.OptionQuality, sync.OptionSubtitles), as Hello.Options lists them; it takes none by default, refusing a link that comes with any.
func (d *Desktop) SetOptions(options ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue.options = options
}

// Asked are the options each link the desktop took or put before the user came with, in the order they came.
func (d *Desktop) Asked() []sync.JobOptions {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.queue.asked)
}

// SetReadOnly makes every paired device read-only while on, as the desktop's user can (sync.Caller.ReadOnly): Hello says so, and every call that changes something is refused with sync.CodeReadOnly.
func (d *Desktop) SetReadOnly(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue.readOnly = on
}

// SetOlder makes the desktop speak protocol v1 alone while older, as an older Kando: it has no v2 call path, which it answers 404 with Go's plain-text body.
func (d *Desktop) SetOlder(older bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue.older = older
}

// DropCreates has the desktop take the next n links phones send and hang up before it answers each, as a call that dropped or timed out after the desktop did its work; sent again under the same call ID, a link is answered as it was taken.
func (d *Desktop) DropCreates(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue.drops = n
}

// SetReceive sets what the desktop does with a link a phone sends: sync.ReceiveQueue, the default, sync.ReceiveAsk or sync.ReceiveOff.
func (d *Desktop) SetReceive(mode string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue.receive = mode
}

// Pending are the links sent while the desktop asks first, in the order they came.
func (d *Desktop) Pending() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.queue.pending)
}

// Jobs are the desktop's jobs, newest last.
func (d *Desktop) Jobs() []sync.Job {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.queue.jobs)
}

// SetJob puts job in the queue in place of the one with its ID, or last, as the desktop's queue moving it along, and tells the phones waiting for events.
func (d *Desktop) SetJob(job sync.Job) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i := slices.IndexFunc(d.queue.jobs, func(j sync.Job) bool { return j.ID == job.ID }); i >= 0 {
		d.queue.jobs[i] = job
	} else {
		d.queue.jobs = append(d.queue.jobs, job)
	}
	d.desk.Journal.PublishJobs(d.queue.jobs)
}

// YTDLPUpdates is how many times a phone had the desktop update its downloader.
func (d *Desktop) YTDLPUpdates() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queue.updates
}

// call answers a v2 call, naming the caller by the name it paired with, unless the desktop is older (SetOlder) or the call is a link to drop (DropCreates).
func (d *Desktop) call(w http.ResponseWriter, r *http.Request) {
	token, _ := bytes.CutPrefix([]byte(r.Header.Get("Authorization")), []byte("Bearer "))
	d.mu.Lock()
	name := d.names[string(token)]
	older, readOnly := d.queue.older, d.queue.readOnly
	d.mu.Unlock()
	if older {
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	var call sync.Call
	_ = json.Unmarshal(data, &call)
	d.mu.Lock()
	drop := call.Op == sync.OpJobCreate && d.queue.drops > 0
	if drop {
		d.queue.drops--
	}
	d.mu.Unlock()
	caller := sync.Caller{DeviceID: "dev-" + string(token[:min(6, len(token))]), Name: name, ReadOnly: readOnly}
	if drop {
		sync.HandleCall(httptest.NewRecorder(), r, d.desk, caller)
		panic(http.ErrAbortHandler)
	}
	sync.HandleCall(w, r, d.desk, caller)
}

// jobService is the stand-in's queue as v2's calls act on it.
type jobService struct{ d *Desktop }

func (s jobService) Hello(context.Context, sync.Caller) (sync.Hello, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	return sync.Hello{Ops: []string{sync.OpHello, sync.OpJobCreate, sync.OpJobs, sync.OpJob, sync.OpJobCancel, sync.OpJobRetry, sync.OpJobPause, sync.OpJobResume, sync.OpUpdateYTDLP, sync.OpEvents, sync.OpStateGet, sync.OpStatePut, sync.OpStateResolve, sync.OpLookUp, sync.OpExplain, sync.OpSettingsGet, sync.OpSettingsPut},
		Options: slices.Clone(s.d.queue.options), State: []string{sync.KindPosition, sync.KindLine, sync.KindCorrection, sync.KindWord}, Receive: s.d.queue.receive}, nil
}

// CreateJob queues link as a waiting job of the caller's, puts it among the pending links, or refuses it, as the desktop's setting says.
func (s jobService) CreateJob(_ context.Context, caller sync.Caller, req sync.JobCreate) (sync.JobCreated, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case req.Options.Quality != "" && (!slices.Contains(d.queue.options, sync.OptionQuality) || (req.Options.Quality != sync.QualityBest && req.Options.Quality != sync.Quality720)),
		req.Options.Subtitles != "" && (!slices.Contains(d.queue.options, sync.OptionSubtitles) || (req.Options.Subtitles != sync.SubtitlesPlatform && req.Options.Subtitles != sync.SubtitlesRecognize)):
		return sync.JobCreated{}, sync.Errorf(sync.CodeUnsupported, "job options %+v are not taken", req.Options)
	case req.Link == "":
		return sync.JobCreated{}, sync.Errorf(sync.CodeInvalid, "no link")
	case d.queue.receive == sync.ReceiveOff:
		return sync.JobCreated{}, sync.Errorf(sync.CodeLinksOff, "links from phones are off")
	case d.queue.receive == sync.ReceiveAsk:
		d.queue.pending = append(d.queue.pending, req.Link)
		d.queue.asked = append(d.queue.asked, req.Options)
		return sync.JobCreated{Pending: true}, nil
	}
	for _, job := range d.queue.jobs {
		if job.Link == req.Link && job.State != sync.JobFailed && job.State != sync.JobCanceled {
			return sync.JobCreated{Job: &job, Existing: true}, nil
		}
	}
	d.queue.made++
	d.queue.asked = append(d.queue.asked, req.Options)
	job := sync.Job{ID: fmt.Sprintf("job%d", d.queue.made), Link: req.Link, Title: req.Link, Device: caller.Name, State: sync.JobWaiting, Steps: []sync.Step{
		{Name: sync.StepDownload, State: sync.StepWaiting}, {Name: sync.StepTranscribe, State: sync.StepWaiting}, {Name: sync.StepTranslate, State: sync.StepWaiting},
	}}
	d.queue.jobs = append(d.queue.jobs, job)
	d.desk.Journal.PublishJobs(d.queue.jobs)
	return sync.JobCreated{Job: &job}, nil
}

// ActOnJob cancels a waiting, running or paused job, retries a failed or canceled one, pauses a waiting or running one and resumes a paused one; anything else is a conflict.
func (s jobService) ActOnJob(_ context.Context, _ sync.Caller, op, id string) (sync.Job, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.queue.jobs, func(j sync.Job) bool { return j.ID == id })
	if i < 0 {
		return sync.Job{}, sync.Errorf(sync.CodeNotFound, "no job %q", id)
	}
	job := &d.queue.jobs[i]
	from := map[string][]string{
		sync.OpJobCancel: {sync.JobWaiting, sync.JobRunning, sync.JobPaused},
		sync.OpJobRetry:  {sync.JobFailed, sync.JobCanceled},
		sync.OpJobPause:  {sync.JobWaiting, sync.JobRunning},
		sync.OpJobResume: {sync.JobPaused},
	}
	to := map[string]string{sync.OpJobCancel: sync.JobCanceled, sync.OpJobRetry: sync.JobWaiting, sync.OpJobPause: sync.JobPaused, sync.OpJobResume: sync.JobWaiting}
	if !slices.Contains(from[op], job.State) {
		return sync.Job{}, sync.Errorf(sync.CodeConflict, "job %s is %s", id, job.State)
	}
	job.State = to[op]
	if op == sync.OpJobRetry {
		job.Failure = nil
	}
	d.desk.Journal.PublishJobs(d.queue.jobs)
	return *job, nil
}

func (s jobService) UpdateYTDLP(context.Context, sync.Caller) (sync.ToolUpdate, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.d.queue.updates++
	return sync.ToolUpdate{Started: true}, nil
}
