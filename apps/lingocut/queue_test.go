package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestViewRunsJobToCompletion(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if a.queue.job == nil || a.queue.status != "任务已加入队列" {
		t.Fatalf("texts %q after starting", tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("最近完成") || !tt.HasText("talk") || tt.HasText("处理中") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
	if !tt.HasText("开始处理") {
		t.Fatal("start button did not come back")
	}
}

func TestViewCancelsRunningJob(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	startJob(t, tt, "https://example.com/video")
	tt.Frame()
	if err := tt.Click("取消处理"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("已取消") || !tt.HasText("开始处理") {
		t.Fatalf("texts %q after cancelling", tt.Texts())
	}
}

// TestFilmCancelStopsItsOwnJob checks that 取消 on an open film cancels that film's job through Cancel(id), and that RunningJob names the job while it runs.
func TestFilmCancelStopsItsOwnJob(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	id := a.queue.selectedID
	if got := a.RunningJob(); got != id {
		t.Fatalf("RunningJob = %q, want %q", got, id)
	}
	a.openKey = id
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return !a.running() })
	job, ok := a.library.Find(id)
	if !ok || job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || a.RunningJob() != "" {
		t.Fatalf("job %+v, RunningJob %q", job, a.RunningJob())
	}
}

func TestRetryKeepsTheSameCard(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{{
		ID: "same", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文",
		Stage: core.StageFailed, VideoPath: "/videos/Colibri [TRWgBxVAb1M].mp4",
		Err: errors.New("translate: translation API returned 401"),
	}}
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage == core.StageFailed {
		t.Fatalf("right after retry: %#v", a.library.Jobs)
	}

	settle(t, a, tt, updates)
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage != core.StageComplete {
		t.Fatalf("after retry: %#v", a.library.Jobs)
	}
	if tt.HasText("失败") {
		t.Fatalf("failed card stayed: %q", tt.Texts())
	}
}

func TestSwitchLibraryStopsTheJobAndRemembersTheDirectory(t *testing.T) {
	p := &fakePipeline{block: true}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if !a.running() {
		t.Fatal("job did not start")
	}
	dir := filepath.Join(t.TempDir(), "library")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.switchLibrary(dir)
	if a.snap().LibraryDir != dir {
		t.Fatalf("library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != dir || len(saved.Libraries) != 1 || saved.Libraries[0] != dir {
		t.Fatalf("saved %#v", saved)
	}
	settle(t, a, tt, updates)
	if a.running() {
		t.Fatal("switching the library left the job running")
	}
}
