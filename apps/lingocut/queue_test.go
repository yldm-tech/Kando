package main

import (
	"context"
	"errors"
	"testing"

	"lingocut/packages/core"
)

func TestEnqueueRunsTheJob(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/v" || a.library.Jobs[0].Language != "简体中文" || a.library.Jobs[0].LibraryDir != "/videos" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if a.library.Jobs[0].Stage != core.StageComplete {
		t.Fatalf("stage %q", a.library.Jobs[0].Stage)
	}
}

func TestEnqueueRefusesWhileAJobRuns(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{block: true})
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); !errors.Is(err, errBusy) {
		t.Fatalf("err %v, want errBusy", err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/a" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if err := a.Enqueue("https://example.com/c"); !errors.Is(err, errBusy) {
		t.Fatalf("err %v, want errBusy", err)
	}
	a.Cancel(a.library.Jobs[0].ID)
	waitUntil(t, updates, func() bool { return !a.running() })
	if job := a.library.Jobs[0]; job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("job after Cancel %+v", job)
	}
}

func TestEnqueueSaysWhyASourceIsRefused(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	if err := a.Enqueue(); !errors.Is(err, core.ErrEmptySource) {
		t.Fatalf("err %v", err)
	}
	if err := a.Enqueue("not a link"); err == nil {
		t.Fatal("a bad link was added")
	}
	a.prefs.language = " "
	if err := a.Enqueue("https://example.com/v"); err == nil || err.Error() != "请填写目标语言" {
		t.Fatalf("err %v", err)
	}
	a.prefs.language = "简体中文"
	a.edit(func(s *session) { s.LibraryDir = "" })
	if err := a.Enqueue("https://example.com/v"); err == nil || err.Error() != "先创建或导入一个资料库" {
		t.Fatalf("err %v", err)
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// TestCancelLeavesOtherJobsAlone checks that Cancel of a finished or unknown job does not stop the one running.
func TestCancelLeavesOtherJobsAlone(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{block: true})
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Stage: core.StageComplete}}
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	a.Cancel("old")
	a.Cancel("missing")
	if !a.running() {
		t.Fatal("the running job stopped")
	}
	a.Cancel(a.queue.selectedID)
	waitUntil(t, updates, func() bool { return !a.running() })
}
