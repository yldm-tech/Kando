package synctest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"kando/packages/sync"
)

// A phone sends a link, follows its job through the events as the desktop moves it along, and pauses, resumes and cancels it.
func TestTheQueueOverV2(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Test Mac")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	defer c.Close()
	if _, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid}); err != nil {
		t.Fatal(err)
	}
	h, err := c.Hello(ctx)
	if err != nil || h.Desktop != "Test Mac" || h.Receive != sync.ReceiveQueue {
		t.Fatalf("hello %+v, %v", h, err)
	}
	list, err := c.Jobs(ctx)
	if err != nil || len(list.Jobs) != 0 {
		t.Fatalf("jobs %+v, %v", list, err)
	}
	created, err := c.CreateJob(ctx, "https://youtu.be/abc", sync.JobOptions{})
	if err != nil || created.Job == nil || created.Job.Device != "Pixel" {
		t.Fatalf("created %+v, %v", created, err)
	}
	job := *created.Job
	ev, err := c.Events(ctx, list.Epoch, list.Rev, time.Second)
	if err != nil || len(ev.Events) != 1 || ev.Events[0].ID != job.ID {
		t.Fatalf("events %+v, %v", ev, err)
	}
	job.State, job.Steps[0] = sync.JobRunning, sync.Step{Name: sync.StepDownload, State: sync.StepRunning, Progress: 0.3}
	d.SetJob(job)
	ev, err = c.Events(ctx, ev.Epoch, ev.Rev, time.Second)
	if err != nil || len(ev.Events) != 1 || ev.Events[0].Job.Steps[0].Progress != 0.3 {
		t.Fatalf("events %+v, %v", ev, err)
	}
	for _, step := range []struct{ op, want string }{{sync.OpJobPause, sync.JobPaused}, {sync.OpJobResume, sync.JobWaiting}, {sync.OpJobCancel, sync.JobCanceled}, {sync.OpJobRetry, sync.JobWaiting}} {
		got, err := c.ActOnJob(ctx, step.op, job.ID)
		if err != nil || got.State != step.want {
			t.Fatalf("%s: %+v, %v", step.op, got, err)
		}
	}
	_, err = c.ActOnJob(ctx, sync.OpJobResume, job.ID)
	var e *sync.Error
	if !errors.As(err, &e) || e.Code != sync.CodeConflict {
		t.Fatalf("resuming a waiting job: %v", err)
	}
	if _, err := c.UpdateYTDLP(ctx); err != nil || d.YTDLPUpdates() != 1 {
		t.Fatalf("update: %v, %d", err, d.YTDLPUpdates())
	}
	d.SetReceive(sync.ReceiveAsk)
	if created, err := c.CreateJob(ctx, "https://youtu.be/def", sync.JobOptions{}); err != nil || !created.Pending || len(d.Pending()) != 1 {
		t.Fatalf("asked: %+v, %v, pending %q", created, err, d.Pending())
	}
}

// A phone takes a failed or canceled job off the queue, which tells the phones following it the job is gone; one still going is a conflict, and a desktop from before the operation neither lists it nor does it.
func TestRemovingAStoppedJob(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Test Mac")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	defer c.Close()
	if _, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid}); err != nil {
		t.Fatal(err)
	}
	if h, err := c.Hello(ctx); err != nil || !slices.Contains(h.Ops, sync.OpJobRemove) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	d.SetJob(sync.Job{ID: "failed", Link: "https://youtu.be/a", State: sync.JobFailed, Failure: &sync.Failure{Code: sync.FailNetwork}})
	d.SetJob(sync.Job{ID: "canceled", Link: "https://youtu.be/b", State: sync.JobCanceled})
	d.SetJob(sync.Job{ID: "running", Link: "https://youtu.be/c", State: sync.JobRunning})
	list, err := c.Jobs(ctx)
	if err != nil || len(list.Jobs) != 3 {
		t.Fatalf("jobs %+v, %v", list, err)
	}
	got, err := c.ActOnJob(ctx, sync.OpJobRemove, "failed")
	if err != nil || got.ID != "failed" || got.State != sync.JobFailed {
		t.Fatalf("removing the failed job: %+v, %v", got, err)
	}
	ev, err := c.Events(ctx, list.Epoch, list.Rev, time.Second)
	if err != nil || len(ev.Events) != 1 || ev.Events[0].Kind != sync.EventJobGone || ev.Events[0].ID != "failed" {
		t.Fatalf("events %+v, %v", ev, err)
	}
	if got, err := c.ActOnJob(ctx, sync.OpJobRemove, "canceled"); err != nil || got.State != sync.JobCanceled {
		t.Fatalf("removing the canceled job: %+v, %v", got, err)
	}
	var e *sync.Error
	if _, err := c.ActOnJob(ctx, sync.OpJobRemove, "running"); !errors.As(err, &e) || e.Code != sync.CodeConflict {
		t.Fatalf("removing a running job: %v", err)
	}
	if _, err := c.ActOnJob(ctx, sync.OpJobRemove, "failed"); !errors.As(err, &e) || e.Code != sync.CodeNotFound {
		t.Fatalf("removing it again: %v", err)
	}
	if jobs := d.Jobs(); len(jobs) != 1 || jobs[0].ID != "running" {
		t.Fatalf("left %+v", jobs)
	}

	d.SetNoJobRemove(true)
	d.SetJob(sync.Job{ID: "old", Link: "https://youtu.be/d", State: sync.JobFailed})
	if h, err := c.Hello(ctx); err != nil || slices.Contains(h.Ops, sync.OpJobRemove) {
		t.Fatalf("hello of an older desktop %+v, %v", h, err)
	}
	if _, err := c.ActOnJob(ctx, sync.OpJobRemove, "old"); !errors.As(err, &e) || e.Code != sync.CodeUnsupported {
		t.Fatalf("removing on an older desktop: %v", err)
	}
	if len(d.Jobs()) != 2 {
		t.Fatalf("an older desktop removed a job: %+v", d.Jobs())
	}
}
