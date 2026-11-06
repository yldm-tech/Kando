package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/core"
)

// gatePipeline holds every job in Download until the test lets it through, so a test sees which job runs and in what order. started receives each source as its Download begins.
type gatePipeline struct {
	started chan string
	release chan struct{}
}

func newGatePipeline() *gatePipeline {
	return &gatePipeline{started: make(chan string, 16), release: make(chan struct{})}
}

func (g *gatePipeline) Check() error { return nil }

func (g *gatePipeline) Download(ctx context.Context, source string) (string, error) {
	g.started <- source
	select {
	case <-g.release:
		return "/videos/" + filepath.Base(source) + ".mp4", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (g *gatePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (g *gatePipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (g *gatePipeline) Export(_ context.Context, video string, _, _ []core.Segment) (string, error) {
	return strings.TrimSuffix(video, ".mp4") + ".bilingual.srt", nil
}

// nextStarted applies background updates until the next job reaches Download and returns its source.
func nextStarted(t *testing.T, g *gatePipeline, updates <-chan func()) string {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case source := <-g.started:
			return source
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatal("no job started")
			return ""
		}
	}
}

// noneStarted applies background updates for a short while and fails if a job reaches Download meanwhile.
func noneStarted(t *testing.T, g *gatePipeline, updates <-chan func()) {
	t.Helper()
	quiet := time.After(150 * time.Millisecond)
	for {
		select {
		case source := <-g.started:
			t.Fatalf("%s started while another job ran", source)
		case fn := <-updates:
			fn()
		case <-quiet:
			return
		}
	}
}

func stageOf(t *testing.T, a *app, id string) core.Job {
	t.Helper()
	job, ok := a.library.Find(id)
	if !ok {
		t.Fatalf("job %s is not on the shelf: %#v", id, a.library.Jobs)
	}
	return job
}

// jobIDs maps each source to the ID of its job.
func jobIDs(a *app) map[string]string {
	ids := map[string]string{}
	for _, job := range a.library.Jobs {
		ids[job.Source] = job.ID
	}
	return ids
}

func TestEnqueueRunsTheJob(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/v" || a.library.Jobs[0].Language != "简体中文" || a.library.Jobs[0].LibraryDir != "/videos" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if a.library.Jobs[0].Stage != core.StageComplete || a.library.Jobs[0].CompletedAt.IsZero() {
		t.Fatalf("job %+v", a.library.Jobs[0])
	}
}

// TestQueueRunsJobsOneAtATimeInOrder adds three links at once: the first runs, the other two wait as 排队 cards, and each starts only when the one before it is done.
func TestQueueRunsJobsOneAtATimeInOrder(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("first started %s", got)
	}
	noneStarted(t, g, updates)
	tt.Frame()
	if len(a.library.Jobs) != 3 || textCount(tt, "排队") != 2 || textCount(tt, "下载中") != 1 {
		t.Fatalf("jobs %d texts %q", len(a.library.Jobs), tt.Texts())
	}
	ids := jobIDs(a)
	for _, name := range []string{"a", "b", "c"} {
		if !tt.HasText("example.com/" + name) {
			t.Fatalf("no card for %s: %q", name, tt.Texts())
		}
	}

	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("second started %s", got)
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageComplete || job.SubtitlePath != "/videos/a.bilingual.srt" {
		t.Fatalf("a after its turn %+v", job)
	}
	if job := stageOf(t, a, ids["https://example.com/c"]); job.Stage != core.StageQueued {
		t.Fatalf("c while b runs %+v", job)
	}
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/c" {
		t.Fatalf("third started %s", got)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	for source, id := range ids {
		if job := stageOf(t, a, id); job.Stage != core.StageComplete {
			t.Fatalf("%s ended %+v", source, job)
		}
	}
	if textCount(tt, "已完成") != 3 {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestTypingALinkWhileAJobRunsQueuesIt checks that 开始处理 adds to the queue instead of being refused while another job runs.
func TestTypingALinkWhileAJobRunsQueuesIt(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	a.queue.source = "https://example.com/b"
	a.start()
	if a.queue.job == nil || a.queue.job.Source != "https://example.com/b" || a.queue.job.Stage != core.StageQueued || a.queue.status != "任务已加入队列" {
		t.Fatalf("job %+v status %q", a.queue.job, a.queue.status)
	}
	noneStarted(t, g, updates)
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("started %s", got)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

func TestCancelTakesAWaitingJobOutOfTheQueue(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.Cancel(ids["https://example.com/b"])
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("b after Cancel %+v", job)
	}
	if !a.running() || a.queue.active != ids["https://example.com/a"] {
		t.Fatal("cancelling a waiting job stopped the running one")
	}
	tt.Frame()
	if !tt.HasText("已取消") {
		t.Fatalf("texts %q", tt.Texts())
	}
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/c" {
		t.Fatalf("after a, %s started", got)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageFailed {
		t.Fatalf("cancelled b ran: %+v", job)
	}
}

func TestCancelTheRunningJobStartsTheNext(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.Cancel(ids["https://example.com/a"])
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after cancelling a, %s started", got)
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || statusText(job) != "已取消" {
		t.Fatalf("a after Cancel %+v", job)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
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
	a.Cancel("")
	if !a.running() {
		t.Fatal("the running job stopped")
	}
	if job := stageOf(t, a, "old"); job.Stage != core.StageComplete {
		t.Fatalf("old %+v", job)
	}
	a.Cancel(a.queue.active)
	waitUntil(t, updates, func() bool { return !a.running() })
}

func TestRemovingTheRunningJobStartsTheNext(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.remove(ids["https://example.com/a"])
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after removing a, %s started", got)
	}
	if _, ok := a.library.Find(ids["https://example.com/a"]); ok {
		t.Fatal("the removed job came back")
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestRetryWaitsBehindTheQueue checks that 重试 while a job runs queues the failed job behind those already waiting, under its own ID.
func TestRetryWaitsBehindTheQueue(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: "/videos"}}
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	a.Retry("old")
	if job := stageOf(t, a, "old"); job.Stage != core.StageQueued || job.Err != nil {
		t.Fatalf("old after Retry %+v", job)
	}
	for _, want := range []string{"https://example.com/b", "https://example.com/old"} {
		g.release <- struct{}{}
		if got := nextStarted(t, g, updates); got != want {
			t.Fatalf("started %s, want %s", got, want)
		}
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, "old"); job.Stage != core.StageComplete || len(a.library.Jobs) != 3 {
		t.Fatalf("old %+v jobs %d", job, len(a.library.Jobs))
	}
}

// TestSwitchLibraryStopsTheRunningJob checks that opening another library stops the running job as 已停止：切换了资料库. Jobs queued in the first library wait for it and run when it is open again.
func TestSwitchLibraryStopsTheRunningJob(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	t.Cleanup(func() {
		if a.libraryWatch != nil {
			close(a.libraryWatch)
			a.libraryWatch = nil
		}
	})
	first := filepath.Join(t.TempDir(), "first.lingocut")
	second := filepath.Join(t.TempDir(), "second.lingocut")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.switchLibrary(first)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)

	a.switchLibrary(second)
	waitUntil(t, updates, func() bool { return !a.running() })
	tt.Frame()
	stopped := stageOf(t, a, ids["https://example.com/a"])
	if stopped.Stage != core.StageFailed || !errors.Is(stopped.Err, core.ErrLibrarySwitched) || statusText(stopped) != "已停止：切换了资料库" {
		t.Fatalf("a after the switch %+v", stopped)
	}
	noneStarted(t, g, updates)
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageQueued {
		t.Fatalf("b in the first library %+v", job)
	}

	if err := a.Enqueue("https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/c" {
		t.Fatalf("in the second library %s started", got)
	}
	g.release <- struct{}{}
	waitUntil(t, updates, func() bool { return !a.running() })
	noneStarted(t, g, updates)

	a.switchLibrary(first)
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("back in the first library %s started", got)
	}
	g.release <- struct{}{}
	waitUntil(t, updates, func() bool { return !a.running() })
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageComplete || job.LibraryDir != first {
		t.Fatalf("b %+v", job)
	}
}

// TestRestartResumesInterruptedJobs saves a shelf the way a quit in the middle of a job leaves it and opens it again: the interrupted job and those queued behind it run again, oldest first, and finished jobs stay finished.
func TestRestartResumesInterruptedJobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	before, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	before.Jobs = []core.Job{
		{ID: "c", Source: "https://example.com/c", Language: "简体中文", Stage: core.StageQueued, LibraryDir: "/videos"},
		{ID: "b", Source: "https://example.com/b", Language: "简体中文", Stage: core.StageQueued, LibraryDir: "/videos"},
		{ID: "a", Source: "https://example.com/a", Language: "简体中文", Stage: core.StageTranscribing, LibraryDir: "/videos", VideoPath: "/videos/a.mp4"},
		{ID: "done", Source: "https://example.com/done", Language: "简体中文", Stage: core.StageComplete, LibraryDir: "/videos", VideoPath: "/videos/done.mp4"},
	}
	if err := before.Save(); err != nil {
		t.Fatal(err)
	}

	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	a.openLibrary(path)
	if a.running() {
		t.Fatal("a job ran before the window was ready")
	}
	if job := stageOf(t, a, "a"); job.Stage != core.StageQueued {
		t.Fatalf("interrupted job read back as %+v", job)
	}
	a.pump()
	for _, want := range []string{"a", "b", "c"} {
		if got := nextStarted(t, g, updates); got != "https://example.com/"+want {
			t.Fatalf("started %s, want %s", got, want)
		}
		g.release <- struct{}{}
	}
	settle(t, a, tt, updates)
	for _, id := range []string{"a", "b", "c", "done"} {
		if job := stageOf(t, a, id); job.Stage != core.StageComplete {
			t.Fatalf("%s ended %+v", id, job)
		}
	}
	reloaded, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range reloaded.Jobs {
		if job.Stage != core.StageComplete {
			t.Fatalf("saved %+v", job)
		}
	}
}

func TestEnqueueSaysWhyASourceIsRefused(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{block: true})
	if err := a.Enqueue(); !errors.Is(err, core.ErrEmptySource) {
		t.Fatalf("err %v", err)
	}
	if err := a.Enqueue("not a link"); err == nil {
		t.Fatal("a bad link was added")
	}
	if err := a.Enqueue("not a link", "https://example.com/ok"); err == nil || len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/ok" {
		t.Fatalf("err %v jobs %+v; the good link should be added and the bad one named", err, a.library.Jobs)
	}
	a.Cancel(a.queue.active)
	a.library.Jobs = nil
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

func TestViewRunsJobToCompletion(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if a.queue.job == nil || a.queue.status != "任务已加入队列" {
		t.Fatalf("texts %q after starting", tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("已完成") || !tt.HasText("talk") {
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
	if err := tt.Click("取消"); err != nil {
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
	id := a.library.Jobs[0].ID
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
