package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"slices"
	gosync "sync"
	"time"
)

// Journal is what the desktop tells phones of its jobs, as last published, and the events of their changes, numbered by revision in the journal's epoch. The desktop publishes the jobs whenever they may have changed (PublishJobs); the journal finds what changed. It is safe for concurrent use.
type Journal struct {
	epoch string
	keep  int

	mu     gosync.Mutex
	rev    int64
	jobs   []Job
	events []Event
	// learning is the revision of each library's last learning state event, by its ID.
	learning map[string]int64
	// wake is closed, and made anew, at each change, waking the calls waiting on it.
	wake chan struct{}
}

// JournalKeep is how many events a journal keeps: a phone that waits after an older revision is told to list the jobs again (Events.Reset).
const JournalKeep = 1024

// NewJournal is an empty journal in a new epoch.
func NewJournal() *Journal {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return &Journal{epoch: hex.EncodeToString(b), keep: JournalKeep, wake: make(chan struct{})}
}

// Epoch names the journal's run.
func (j *Journal) Epoch() string { return j.epoch }

// PublishJobs makes jobs, in the order a list gives them (JobList), what phones are told, and records an event for each job new or changed since the last jobs published, and for each no longer among them. Each job's Rev is set here. It reports whether anything changed.
func (j *Journal) PublishJobs(jobs []Job) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	before := make(map[string]Job, len(j.jobs))
	for _, job := range j.jobs {
		before[job.ID] = job
	}
	next := make([]Job, len(jobs))
	changed := false
	seen := make(map[string]bool, len(jobs))
	for i, job := range jobs {
		job = job.clone()
		seen[job.ID] = true
		old, ok := before[job.ID]
		job.Rev = old.Rev
		if !ok || !reflect.DeepEqual(old, job) {
			j.rev++
			job.Rev = j.rev
			kept := job
			j.add(Event{Rev: j.rev, Kind: EventJob, ID: job.ID, Job: &kept})
			changed = true
		}
		next[i] = job
	}
	for _, old := range j.jobs {
		if !seen[old.ID] {
			j.rev++
			j.add(Event{Rev: j.rev, Kind: EventJobGone, ID: old.ID})
			changed = true
		}
	}
	if !changed {
		// The same jobs in another order are a change of the list, which no event tells; a phone lists them again when it needs their order.
		j.jobs = next
		return false
	}
	j.jobs = next
	close(j.wake)
	j.wake = make(chan struct{})
	return true
}

// PublishState records that library's learning state changed (EventState), so the phones waiting hear it and ask for the changes.
func (j *Journal) PublishState(library string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.rev++
	j.add(Event{Rev: j.rev, Kind: EventState, ID: library})
	if j.learning == nil {
		j.learning = map[string]int64{}
	}
	j.learning[library] = j.rev
	close(j.wake)
	j.wake = make(chan struct{})
}

// add keeps e, forgetting the oldest event past keep. The caller holds mu.
func (j *Journal) add(e Event) {
	j.events = append(j.events, e)
	if len(j.events) > j.keep {
		j.events = append(j.events[:0:0], j.events[len(j.events)-j.keep:]...)
	}
}

// List is the jobs as last published, at the journal's revision.
func (j *Journal) List() JobList {
	j.mu.Lock()
	defer j.mu.Unlock()
	return JobList{Epoch: j.epoch, Rev: j.rev, Jobs: append([]Job{}, j.jobs...)}
}

// Learning is, by library ID, the revision at which each library's learning state last changed (PublishState) in the journal's epoch, which the storage relay's state.json tells phones (RelayState.Learning).
func (j *Journal) Learning() map[string]int64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make(map[string]int64, len(j.learning))
	for lib, rev := range j.learning {
		out[lib] = rev
	}
	return out
}

// Job is job id as last published.
func (j *Journal) Job(id string) (Job, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, job := range j.jobs {
		if job.ID == id {
			return job, true
		}
	}
	return Job{}, false
}

// Wait answers the events after revision since of epoch, waiting up to wait (at most MaxEventWait) for one, or until ctx ends. A since of another epoch (as "" before the phone has listed the jobs), past the journal's revision or older than the events it keeps answers Reset at once.
func (j *Journal) Wait(ctx context.Context, epoch string, since int64, wait time.Duration) Events {
	wait = min(max(wait, 0), MaxEventWait)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		j.mu.Lock()
		out, ok := j.since(epoch, since)
		wake := j.wake
		j.mu.Unlock()
		if ok || len(out.Events) > 0 {
			return out
		}
		select {
		case <-wake:
		case <-timer.C:
			return out
		case <-ctx.Done():
			return out
		}
	}
}

// since is the newest event of each job, and of each library's learning state, after revision since, and whether since was not one to count from (Reset). The caller holds mu.
func (j *Journal) since(epoch string, since int64) (Events, bool) {
	out := Events{Epoch: j.epoch, Rev: j.rev, Events: []Event{}}
	oldest := j.rev + 1
	if len(j.events) > 0 {
		oldest = j.events[0].Rev
	}
	if epoch != j.epoch || since < 0 || since > j.rev || since < oldest-1 {
		out.Reset = true
		return out, true
	}
	last := map[string]int{}
	for i, e := range j.events {
		if e.Rev > since {
			last[e.about()] = i
		}
	}
	for i, e := range j.events {
		if e.Rev > since && last[e.about()] == i {
			out.Events = append(out.Events, e)
		}
	}
	return out, false
}

// clone is job with its steps and failure of its own, so the caller's changing its own afterwards changes nothing published.
func (job Job) clone() Job {
	job.Steps = slices.Clone(job.Steps)
	if job.Failure != nil {
		f := *job.Failure
		job.Failure = &f
	}
	return job
}

// about is what e is a change of, which a newer event of takes its place: a job, or a library's learning state.
func (e Event) about() string {
	if e.Kind == EventState {
		return "state/" + e.ID
	}
	return "job/" + e.ID
}
