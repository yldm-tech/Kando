package session

import (
	"slices"
	"testing"

	"kando/packages/sync"
)

// A job keeps its place while its state's rank stays, moves last among the running and waiting ones or first among the others when it changes, and is told finished once, as it turns done.
func TestJobsKeepTheOrderAListGivesThem(t *testing.T) {
	job := func(id, state string) sync.Job { return sync.Job{ID: id, State: state} }
	ids := func(list []sync.Job) []string {
		var out []string
		for _, j := range list {
			out = append(out, j.ID)
		}
		return out
	}
	list := []sync.Job{job("run", sync.JobRunning), job("w1", sync.JobWaiting), job("w2", sync.JobWaiting), job("old", sync.JobDone)}
	steps := []struct {
		job      sync.Job
		want     []string
		finished int
	}{
		{job("w1", sync.JobPaused), []string{"run", "w1", "w2", "old"}, 0},
		{job("new", sync.JobWaiting), []string{"run", "w1", "w2", "new", "old"}, 0},
		{job("run", sync.JobDone), []string{"w1", "w2", "new", "run", "old"}, 1},
		{job("w2", sync.JobRunning), []string{"w2", "w1", "new", "run", "old"}, 0},
		{job("run", sync.JobDone), []string{"w2", "w1", "new", "run", "old"}, 0},
		{job("w1", sync.JobFailed), []string{"w2", "new", "w1", "run", "old"}, 0},
	}
	for _, step := range steps {
		var finished []sync.Job
		list, finished = upsertJob(list, step.job, nil)
		if got := ids(list); !slices.Equal(got, step.want) || len(finished) != step.finished {
			t.Fatalf("after %s %s: %v, %d finished; want %v, %d", step.job.ID, step.job.State, got, len(finished), step.want, step.finished)
		}
	}
}

// A computer that made the phone read-only answers no call that changes something, and the phone offers a job option only to a computer that takes it while it queues links at once.
func TestWhatAComputerLetsThePhoneDo(t *testing.T) {
	q := Queue{Reach: Online, Ops: []string{sync.OpJobs, sync.OpJobCreate, sync.OpJobCancel}, Options: []string{sync.OptionQuality}, Receive: sync.ReceiveQueue}
	if !q.Can(sync.OpJobCreate) || !q.Can(sync.OpJobCancel) || q.Can(sync.OpJobPause) {
		t.Fatal("Can does not follow the operations the computer answers")
	}
	if !q.Offers(sync.OptionQuality) || q.Offers(sync.OptionSubtitles) {
		t.Fatal("Offers does not follow the options the computer takes")
	}
	all := sync.JobOptions{Quality: sync.Quality720, Subtitles: sync.SubtitlesRecognize}
	if got := sendOptions(q, all); got != (sync.JobOptions{Quality: sync.Quality720}) {
		t.Fatalf("a link goes with %+v to a computer that takes the quality alone", got)
	}
	asks := q
	asks.Receive = sync.ReceiveAsk
	if asks.Offers(sync.OptionQuality) || sendOptions(asks, all) != (sync.JobOptions{}) {
		t.Fatal("a computer that asks its user first is offered options it would not keep")
	}
	away := q
	away.Reach = Offline
	if away.Offers(sync.OptionQuality) || sendOptions(away, all) != all {
		t.Fatal("a link waiting for a computer out of reach does not keep the options it was chosen with")
	}
	ro := q
	ro.ReadOnly = true
	if ro.Can(sync.OpJobCreate) || ro.Can(sync.OpJobCancel) || !ro.Can(sync.OpJobs) || ro.Offers(sync.OptionQuality) {
		t.Fatal("a read-only phone is let change something, or not read")
	}
}
