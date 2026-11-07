package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
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
	if len(a.library.Jobs) != 3 || a.queue.active != "" || len(a.queue.order) != 0 {
		t.Fatalf("after the last job: jobs %+v active %q waiting %q", a.library.Jobs, a.queue.active, a.queue.order)
	}
	// Each job is complete with its own subtitles, and they finished in the order they were added.
	var finished time.Time
	for _, name := range []string{"a", "b", "c"} {
		job := stageOf(t, a, ids["https://example.com/"+name])
		if job.Stage != core.StageComplete || job.SubtitlePath != "/videos/"+name+".bilingual.srt" || job.CompletedAt.IsZero() {
			t.Fatalf("%s ended %+v", name, job)
		}
		if !job.CompletedAt.After(finished) {
			t.Fatalf("%s finished at %v, not after the job before it at %v", name, job.CompletedAt, finished)
		}
		finished = job.CompletedAt
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
	first := filepath.Join(t.TempDir(), "first.kando")
	second := filepath.Join(t.TempDir(), "second.kando")
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

// A queued job from before jobs stored their target language still uses the
// current translation setting when it resumes after restart.
func TestRestartFillsMissingTranslationLanguage(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(p)
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Stage: core.StageQueued, LibraryDir: "/videos"}}

	a.pump()
	settle(t, a, tt, updates)

	job := stageOf(t, a, "old")
	if job.Language != "简体中文" || p.language != "简体中文" {
		t.Fatalf("language = %q, pipeline language = %q", job.Language, p.language)
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
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/video" || !tt.HasText("已加入，开始处理。") {
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

// TestFilmPageCancelsTheFilmItShows opens a film still waiting in the queue and presses its 取消: that film leaves the queue as 已取消, and the job running meanwhile goes on.
func TestFilmPageCancelsTheFilmItShows(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.openKey = ids["https://example.com/b"]
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("b after its 取消 %+v", job)
	}
	noneStarted(t, g, updates)
	if a.queue.active != ids["https://example.com/a"] {
		t.Fatalf("the film page's 取消 stopped the running job; active %q", a.queue.active)
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage == core.StageFailed {
		t.Fatalf("a after b's 取消 %+v", job)
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

// ytdlpPipeline downloads with the real media.Tools.Download, run on a yt-dlp script that prints what the real yt-dlp prints for Download's arguments, and stubs the stages after it.
type ytdlpPipeline struct{ tools media.Tools }

func (p ytdlpPipeline) Check() error { return nil }

func (p ytdlpPipeline) Download(ctx context.Context, source string) (string, error) {
	return p.tools.Download(ctx, source)
}

func (ytdlpPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (ytdlpPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (ytdlpPipeline) Export(_ context.Context, video string, _, _ []core.Segment) (string, error) {
	return strings.TrimSuffix(video, ".mp4") + ".bilingual.srt", nil
}

// gatedYTDLP writes a yt-dlp that names the video, says a quarter of the download is done (half of the first of two formats) and waits until gate exists before it finishes.
func gatedYTDLP(t *testing.T, thumbnail, gate string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yt-dlp")
	script := `#!/bin/sh
echo 'kando-info {"id": "jNQXAC9IVRw", "title": "Me at the zoo", "thumbnail": "` + thumbnail + `", "duration": 19}'
echo 'kando-formats ["395", "251"]'
echo 'kando-progress downloading 111890 223780 NA 1048576 3 NA NA 395'
i=0
while [ ! -e '` + gate + `' ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done
echo 'kando-progress finished 252182 252182 NA NA NA NA NA 251'
echo '/videos/zoo.mp4'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheCardFollowsTheDownload(t *testing.T) {
	png := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(png) }))
	t.Cleanup(srv.Close)
	gate := filepath.Join(t.TempDir(), "gate")
	a, tt, updates := newTestApp(ytdlpPipeline{tools: media.Tools{YTDLP: gatedYTDLP(t, srv.URL+"/hq.jpg", gate), OutputDir: "/videos"}})
	path := filepath.Join(t.TempDir(), "library.json")
	a.openLibrary(path)
	a.lib.posterDir = t.TempDir()
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	id := a.library.Jobs[0].ID
	waitUntil(t, updates, func() bool { return stageOf(t, a, id).Progress.Known })
	job := stageOf(t, a, id)
	if job.Title != "Me at the zoo" || job.Progress.Fraction != 0.25 {
		t.Fatalf("job %+v", job)
	}
	tt.Frame()
	for _, want := range []string{"Me at the zoo", "下载中 · 25% · 1.0 MB/s · 还剩 0:03"} {
		if textCount(tt, want) == 0 {
			t.Fatalf("the card does not say %q", want)
		}
	}
	// The name is saved as soon as it is known, so a download that fails or a quit keeps it.
	saved, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := saved.Find(id); s.Title != "Me at the zoo" || s.Thumbnail != srv.URL+"/hq.jpg" {
		t.Fatalf("saved %+v", s)
	}
	film, _ := a.JobFilm(id)
	waitUntil(t, updates, func() bool { return a.Thumbnail(film) != nil })

	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageComplete || job.Progress != (core.Progress{}) {
		t.Fatalf("finished %+v", job)
	}
}

// pngBytes is a small picture, as a site's thumbnail.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 9))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// failedJob puts a job whose download failed for kind on the shelf of the open library and returns its card.
func failedJob(t *testing.T, a *app, source string, kind core.FailKind, cookies string) films.Film {
	t.Helper()
	job, err := core.NewJob(source)
	if err != nil {
		t.Fatal(err)
	}
	job.Language, job.Stage, job.LibraryDir = "简体中文", core.StageFailed, a.snap().LibraryDir
	job.Err, job.FailKind, job.Cookies, job.Title = errors.New("download: yt-dlp: exit status 1"), kind, cookies, "Coastal Erosion Documentary"
	a.library.Upsert(job)
	film, _ := a.JobFilm(job.ID)
	return film
}

// withBrowsers makes the browsers ids, in media.Browsers' order, the ones this machine has: what a look for them finds, and what the queue already knows, as queueReadYTDLP leaves it.
func withBrowsers(t *testing.T, a *app, ids ...string) {
	t.Helper()
	old := installedBrowsers
	installedBrowsers = func() []media.Browser {
		var out []media.Browser
		for _, b := range media.Browsers {
			if slices.Contains(ids, b.ID) {
				out = append(out, b)
			}
		}
		return out
	}
	t.Cleanup(func() { installedBrowsers = old })
	a.queue.ytdlp.browsers = installedBrowsers()
}

func remedyLabels(r films.Remedy) []string {
	var labels []string
	for _, action := range r.Actions {
		labels = append(labels, action.Label)
	}
	return labels
}

func TestAFailedDownloadOffersWhatFixesIt(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	withBrowsers(t, a, "chrome", "safari")
	yt, bili := "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", "https://www.bilibili.com/video/BV1Hx411c7Pq"
	a.queue.ytdlp.path, a.queue.ytdlp.version = "/usr/local/bin/yt-dlp", "2026.09.14"
	a.queue.ytdlp.install = media.YTDLPInstall{Kind: "standalone", Update: []string{"/usr/local/bin/yt-dlp", "-U"}}
	for _, tc := range []struct {
		source        string
		kind          core.FailKind
		cookies       string
		headline, why string
		actions       []string
	}{
		{yt, core.FailOutdated, "", "YouTube 改版了，下载组件要更新", "本机的 yt-dlp 是 2026.09.14 版，更新几秒就好。", []string{"更新并重试"}},
		{bili, core.FailLoginExpired, "safari", "哔哩哔哩登录已过期", "先在 Safari 里重新登录 B 站，再回来重试。", []string{"已重新登录，重试", "改用 Chrome"}},
		{yt, core.FailRegion, "", "你所在的地区不能看", "上传者限制了地区。别处有这个视频的话，换个链接。", []string{"换个链接"}},
		{yt, core.FailPrivate, "", "视频已设为私密或被删除", "没法下载。别处有的话，换个链接。", []string{"换个链接"}},
		{bili, core.FailMembers, "", "大会员专享视频", "需要大会员。借用已开通会员的浏览器登录即可。", []string{"借用 Chrome 登录并重试", "换个链接"}},
		{yt, core.FailAge, "", "需要年龄验证", "借用浏览器里已登录的 YouTube 账号即可。", []string{"借用 Chrome 登录并重试"}},
		{"https://www.netflix.com/watch/1", core.FailDRM, "", "Netflix 受 DRM 保护", "视频是加密的，没法下载。有本地文件的话可以直接拖进来。", []string{"选择本地文件"}},
		{"https://example.com/", core.FailUnsupported, "", "example.com 暂不支持", "没在页面里找到视频。换个链接，或先下载到本机再拖进来。", []string{"换个链接", "选择本地文件"}},
		{"https://www.instagram.com/p/C0000000000/", core.FailLogin, "", "要登录才能看", "instagram.com 的这个视频需要账号。借用浏览器里已登录的账号即可，不保存密码。", []string{"借用 Chrome 登录并重试"}},
		{yt, core.FailNetwork, "", "连不上网络", "检查网络连接后再重试。", []string{"重试"}},
		{"https://nonexistent-host-kando-review.invalid/video/1", core.FailHost, "", "找不到 nonexistent-host-kando-review.invalid", "网络正常的话，多半是链接里的网址写错了，或者这个网站已经关闭。换个链接，或检查网络后再重试。", []string{"换个链接", "重试"}},
		{yt, core.FailCookies, "safari", "读不到 Safari 的登录", "macOS 只让有「完全磁盘访问权限」的 App 读 Safari 的登录。在系统设置里允许看懂，回来再重试，或改用别的浏览器。", []string{"打开系统设置", "已允许，重试", "改用 Chrome"}},
		{yt, core.FailCookies, "chrome", "读不到 Chrome 的登录", "这台电脑上找不到 Chrome 的登录数据。换一个已登录的浏览器再试。", []string{"改用 Safari"}},
		{"https://www.youtube.com/watch?v=dQw4w9WgX", core.FailBadLink, "", "链接不完整", "链接好像少了一截，重试也下载不了。复制完整的链接，换个链接再试。", []string{"换个链接"}},
	} {
		film := failedJob(t, a, tc.source, tc.kind, tc.cookies)
		if tc.kind == core.FailCookies && tc.cookies == "safari" {
			// The real error of yt-dlp reading Safari's cookies without Full Disk Access (packages/media/testdata/ytdlp/errors/cookies-safari-denied.stderr).
			job, _ := a.library.Find(film.JobID)
			job.Err = errors.New("download: yt-dlp: exit status 1: ERROR: [Errno 1] Operation not permitted: '/Users/someone/Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies'")
			a.library.Upsert(job)
		}
		r, ok := a.FailureRemedy(film)
		if !ok || r.Headline != tc.headline || (tc.why != "" && r.Why != tc.why) || strings.Join(remedyLabels(r), ",") != strings.Join(tc.actions, ",") {
			t.Errorf("%s: %+v", tc.kind, r)
		}
		a.remove(film.JobID)
	}
	// A failure yt-dlp did not explain keeps the plain reason and 重试.
	plain := failedJob(t, a, yt, "", "")
	if _, ok := a.FailureRemedy(plain); ok {
		t.Fatal("an unexplained failure has a remedy")
	}
	a.remove(plain.JobID)

	failedJob(t, a, yt, core.FailRegion, "")
	tt.Frame()
	for _, want := range []string{"你所在的地区不能看", "上传者限制了地区。别处有这个视频的话，换个链接。", "换个链接", "移除"} {
		if textCount(tt, want) == 0 {
			t.Fatalf("the card does not say %q", want)
		}
	}
}

// recordingYTDLP writes a yt-dlp that saves its arguments, one a line, to args and fails as one too old for the site does, printing the real stderr of yt-dlp 2024.03.10 on YouTube. Its -U updates it to updatesTo, or leaves it at 2024.03.10 as the newest there is when updatesTo is "".
func recordingYTDLP(t *testing.T, args, updatesTo string) string {
	t.Helper()
	stderr, err := filepath.Abs(filepath.Join("..", "..", "packages", "media", "testdata", "ytdlp", "errors", "outdated-youtube.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "yt-dlp")
	update := "echo \"Latest version: stable@$v\"; echo \"yt-dlp is up to date (stable@$v)\""
	if updatesTo != "" {
		update = "echo " + updatesTo + " > \"$0.version\"; echo 'Updated yt-dlp to stable@" + updatesTo + "'"
	}
	script := "#!/bin/sh\nv=$(cat \"$0.version\" 2>/dev/null || echo 2024.03.10)\n" +
		"case \"$1\" in --version) echo \"$v\"; exit 0;; -U) " + update + "; exit 0;; esac\n" +
		"for a in \"$@\"; do echo \"$a\"; done >> '" + args + "'\ncat '" + stderr + "' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// toolsForRemedies is media.Tools that passes Check with tools that are never reached, as a job fails at its download.
func toolsForRemedies(t *testing.T, ytdlp string) media.Tools {
	model := filepath.Join(t.TempDir(), "ggml-tiny.bin")
	if err := os.WriteFile(model, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	return media.Tools{OutputDir: "/videos", YTDLP: ytdlp, FFmpeg: "/bin/false", Whisper: "/bin/false", WhisperModel: model, Translator: media.ChatTranslator{BaseURL: "http://127.0.0.1:9/v1", Model: "m"}}
}

func TestUpdateAndRetryUpdatesYTDLPAndRunsTheFailedDownloadsAgain(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	tool := recordingYTDLP(t, args, "2026.11.03")
	a, tt, updates := newTestApp(toolsForRemedies(t, tool))
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailOutdated {
		t.Fatalf("job %+v", job)
	}
	a.queueReadYTDLP()
	waitUntil(t, updates, func() bool { return a.queue.ytdlp.version != "" })
	film, _ := a.JobFilm(id)
	r, _ := a.FailureRemedy(film)
	if r.Why != "本机的 yt-dlp 是 2024.03.10 版，更新几秒就好。" || r.Actions[0].ID != remedyUpdate {
		t.Fatalf("remedy %+v", r)
	}
	a.RunRemedy(film, remedyUpdate)
	if r, _ := a.FailureRemedy(film); r.Headline != "正在更新下载组件…" || !r.Actions[0].Disabled {
		t.Fatalf("while updating %+v", r)
	}
	waitUntil(t, updates, func() bool { return !a.queue.ytdlp.updating })
	if a.queue.ytdlp.version != "2026.11.03" || a.lib.notice.Text != "下载组件已更新到 2026.11.03，已重新开始下载 1 个视频。" {
		t.Fatalf("version %q, notice %q", a.queue.ytdlp.version, a.lib.notice.Text)
	}
	settle(t, a, tt, updates)
	data, _ := os.ReadFile(args)
	if strings.Count(string(data), "https://www.youtube.com/watch?v=jNQXAC9IVRw") != 2 {
		t.Fatalf("yt-dlp ran with\n%s", data)
	}
}

func TestBorrowingALoginRunsTheDownloadWithTheBrowsersCookies(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	a, tt, updates := newTestApp(toolsForRemedies(t, recordingYTDLP(t, args, "2026.11.03")))
	withBrowsers(t, a, "chrome", "safari")
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Tq92D6wQ1mg", core.FailAge, "")
	a.RunRemedy(film, remedyBorrow+"chrome")
	settle(t, a, tt, updates)
	data, _ := os.ReadFile(args)
	if !strings.Contains(string(data), "--cookies-from-browser\nchrome\n") {
		t.Fatalf("yt-dlp ran with\n%s", data)
	}
	if job := stageOf(t, a, film.JobID); job.Cookies != "chrome" {
		t.Fatalf("job %+v", job)
	}
	// The job keeps its browser; switching to another runs with that one.
	a.RunRemedy(film, remedyBorrow+"safari")
	settle(t, a, tt, updates)
	data, _ = os.ReadFile(args)
	if !strings.Contains(string(data), "--cookies-from-browser\nsafari\n") {
		t.Fatalf("yt-dlp ran with\n%s", data)
	}
}

// 换个链接 does what the prototype's swapLink does: the card goes, the link field above is emptied and takes the focus, and the message bar says to paste the link from elsewhere there. A link pasted then starts a job of its own, without the old card's borrowed login.
func TestSwapLinkRemovesTheCardAndFocusesTheLinkField(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(g)
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", core.FailRegion, "chrome")
	a.queue.source = "half typed"
	tt.Frame()
	if err := tt.Click("换个链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := a.library.Find(film.JobID); ok {
		t.Fatal("the card stayed")
	}
	if a.queue.source != "" || a.lib.notice.Text != "已移除「Coastal Erosion Documentary」。如果别处有这个视频，把链接粘贴到右上角。" {
		t.Fatalf("source %q, notice %q", a.queue.source, a.lib.notice.Text)
	}
	tt.Frame()
	if !tt.Focused("视频链接") {
		t.Fatal("the link field does not have the focus")
	}
	tt.Type("https://vimeo.com/76979871")
	if a.queue.source != "https://vimeo.com/76979871" {
		t.Fatalf("typed into %q", a.queue.source)
	}
	if err := tt.Click("开始处理"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://vimeo.com/76979871" {
		t.Fatalf("started %s", got)
	}
	if job := a.library.Jobs[len(a.library.Jobs)-1]; job.Source != "https://vimeo.com/76979871" || job.Cookies != "" {
		t.Fatalf("job %+v", job)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// The browsers are looked for off the UI thread, when the window is ready and when the tools change; drawing a failed card only reads what was found.
func TestFailedCardsDoNotLookForBrowsers(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	withBrowsers(t, a, "firefox")
	a.queue.ytdlp.browsers = nil
	a.queueReadYTDLP()
	waitUntil(t, updates, func() bool { return len(a.queue.ytdlp.browsers) == 1 })
	installedBrowsers = func() []media.Browser {
		t.Error("a frame looked for the browsers")
		return nil
	}
	film := failedJob(t, a, "https://www.instagram.com/p/C0000000000/", core.FailLogin, "")
	tt.Frame()
	if textCount(tt, "借用 Firefox 登录并重试") == 0 {
		t.Fatal("the card does not offer Firefox")
	}
	if r, _ := a.FailureRemedy(film); r.Actions[0].ID != remedyBorrow+"firefox" {
		t.Fatalf("remedy %+v", r)
	}
}

// With no browser whose login yt-dlp reads, as on a Windows or Linux machine without one, a card that wants a login says so and offers to look again instead of a button that borrows from a browser that is not there. Safari is never offered in its place.
func TestNoBrowserToBorrowFrom(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	withBrowsers(t, a)
	yt, bili := "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", "https://www.bilibili.com/video/BV1Hx411c7Pq"
	none := "这台电脑上没找到能借用登录的浏览器。在 Chrome、Firefox、Edge、Brave 或 Chromium 里登录后，重新查找。"
	for _, tc := range []struct {
		source  string
		kind    core.FailKind
		why     string
		actions []string
	}{
		{bili, core.FailMembers, "需要大会员。" + none, []string{"重新查找浏览器", "换个链接"}},
		{yt, core.FailAge, "要用已登录的 YouTube 账号。" + none, []string{"重新查找浏览器"}},
		{"https://www.instagram.com/p/C0000000000/", core.FailLogin, "instagram.com 的这个视频需要账号。" + none, []string{"重新查找浏览器"}},
		{yt, core.FailRateLimit, "请求太频繁。等几分钟再重试。", []string{"重试"}},
		{yt, core.FailCookies, "这台电脑上找不到 Chrome 的登录数据。换一个已登录的浏览器再试。", nil},
	} {
		film := failedJob(t, a, tc.source, tc.kind, map[core.FailKind]string{core.FailCookies: "chrome"}[tc.kind])
		r, ok := a.FailureRemedy(film)
		if !ok || r.Why != tc.why || strings.Join(remedyLabels(r), ",") != strings.Join(tc.actions, ",") {
			t.Errorf("%s: %+v", tc.kind, r)
		}
		for _, action := range r.Actions {
			if strings.HasPrefix(action.ID, remedyBorrow) {
				t.Errorf("%s offers %s", tc.kind, action.ID)
			}
		}
		a.remove(film.JobID)
	}

	// Looking again finds nothing yet, and says so; once a browser has a login folder, the card offers it.
	film := failedJob(t, a, yt, core.FailAge, "")
	a.RunRemedy(film, remedyFindBrowsers)
	waitUpdate(t, updates)
	if a.lib.notice.Text != "还是没找到能借用登录的浏览器。" {
		t.Fatalf("notice %q", a.lib.notice.Text)
	}
	withBrowsers(t, a, "chromium")
	a.queue.ytdlp.browsers = nil
	a.RunRemedy(film, remedyFindBrowsers)
	waitUpdate(t, updates)
	tt.Frame()
	if textCount(tt, "借用 Chromium 登录并重试") == 0 {
		t.Fatal("the card does not offer Chromium")
	}
}

// 更新并重试 on a yt-dlp that is already the newest runs nothing again: the site's fix is not out yet, and the card says so with 重试 and another look for an update.
func TestUpdateThatFindsTheNewestRunsNothingAgain(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	a, tt, updates := newTestApp(toolsForRemedies(t, recordingYTDLP(t, args, "")))
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	a.queueReadYTDLP()
	waitUntil(t, updates, func() bool { return a.queue.ytdlp.version != "" })
	film, _ := a.JobFilm(id)
	a.RunRemedy(film, remedyUpdate)
	waitUntil(t, updates, func() bool { return !a.queue.ytdlp.updating })
	if a.lib.notice.Text != "下载组件 yt-dlp 已是最新的 2024.03.10 版，网站的修复还没发布，过一两天再试。" {
		t.Fatalf("notice %q", a.lib.notice.Text)
	}
	if a.running() || stageOf(t, a, id).Stage != core.StageFailed {
		t.Fatal("the download ran again")
	}
	r, _ := a.FailureRemedy(film)
	if r.Headline != "YouTube 改版了，下载组件还没跟上" || r.Why != "本机的 yt-dlp 已是最新的 2024.03.10 版，网站的修复还没发布，过一两天再重试。" || strings.Join(remedyLabels(r), ",") != "重试,检查更新" {
		t.Fatalf("remedy %+v", r)
	}
	data, _ := os.ReadFile(args)
	if strings.Count(string(data), "https://www.youtube.com/watch?v=jNQXAC9IVRw") != 1 {
		t.Fatalf("yt-dlp ran with\n%s", data)
	}
}

func TestOpenSystemSettingsForSafarisLogin(t *testing.T) {
	var opened string
	old := openExternal
	openExternal = func(url string) error { opened = url; return nil }
	t.Cleanup(func() { openExternal = old })
	a, _, _ := newTestApp(&fakePipeline{})
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Tq92D6wQ1mg", core.FailCookies, "safari")
	a.RunRemedy(film, remedyDiskAccess)
	if opened != fullDiskAccess {
		t.Fatalf("opened %q", opened)
	}
}

// failedWith is a failed card of the open library whose job's error is err, as media.Run wraps it.
func failedWith(t *testing.T, a *app, err string) films.Film {
	t.Helper()
	job := core.Job{ID: "f", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, Err: errors.New(err), LibraryDir: a.snap().LibraryDir}
	a.library.Jobs = []core.Job{job}
	film, ok := a.JobFilm(job.ID)
	if !ok {
		t.Fatal("no card")
	}
	return film
}

// A failed card says why in plain Chinese, never the pipeline's English: what went wrong with the translation, and 打开设置 to 「AI 服务」 when a setting there fixes it; and for a failure no reason is known for, the stage that failed with what the tool said, without the tool's own prefixes.
func TestAFailedCardSaysWhyInChinese(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	for _, tc := range []struct {
		err, reason, section string
	}{
		{"translate: decode translation response: context deadline exceeded (Client.Timeout or context cancellation while reading body)", "AI 服务一直没有回应，已自动重试，还是不行。稍后再重试。", ""},
		{`translate: Post "https://api.example.com/v1/chat/completions": dial tcp: lookup api.example.com: no such host`, "连不上 AI 服务，检查网络后再重试。", ""},
		{"translate: translation is not a JSON array of strings: invalid character '好' looking for beginning of value", "AI 返回的译文格式不对，已重发几次还是不行。重试，或在「AI 服务」里换个模型。", settings.SectionAI},
		{"translate: translation returned 3 lines for 40", "AI 返回的译文格式不对，已重发几次还是不行。重试，或在「AI 服务」里换个模型。", settings.SectionAI},
		{"translate: translation API returned 401 Unauthorized: invalid key", "AI 服务不接受这个 Key，去「AI 服务」里检查一下。", settings.SectionAI},
		{"translate: translation API returned 403 Forbidden: no access", "AI 服务不接受这个 Key，去「AI 服务」里检查一下。", settings.SectionAI},
		{"translate: translation API returned 404 Not Found: model not found", "AI 服务拒绝了请求，检查「AI 服务」里的地址和模型。", settings.SectionAI},
		{"translate: translation API returned 429 Too Many Requests: slow down", "AI 服务一直很忙，已自动重试，还是不行。过几分钟再重试。", ""},
		{"translate: something nobody has seen", "翻译失败：something nobody has seen", ""},
		{"download: yt-dlp: exit status 1: ERROR: [generic] abc: Something nobody has seen", "下载失败：Something nobody has seen", ""},
		{"download: yt-dlp: exit status 1: ERROR: Something else", "下载失败：Something else", ""},
		{"transcribe: whisper-cli: exit status 3: failed to read audio", "生成字幕失败：failed to read audio", ""},
		{"transcribe: whisper-cli: signal: killed: whisper_init: out of memory", "生成字幕失败：whisper_init: out of memory", ""},
		{"export: open /v/a.srt: permission denied", "导出字幕失败：open /v/a.srt: permission denied", ""},
		{"boom", "处理失败：boom", ""},
	} {
		film := failedWith(t, a, tc.err)
		reason, section := a.FailureHint(film)
		if reason != tc.reason || section != tc.section {
			t.Errorf("%q: %q %q, want %q %q", tc.err, reason, section, tc.reason, tc.section)
		}
	}
	failedWith(t, a, "translate: translation API returned 401 Unauthorized: invalid key")
	tt.Frame()
	for _, want := range []string{"AI 服务不接受这个 Key，去「AI 服务」里检查一下。", "打开设置", "重试"} {
		if !tt.HasText(want) {
			t.Fatalf("the card does not say %q: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	wantSettings(t, a, settings.SectionAI)
}

// A film whose translation left some lines untranslated is on the shelf as done, and its card and the hero say how many lines are missing.
func TestADoneFilmSaysWhichLinesAreUntranslated(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	video := filepath.Join(a.snap().LibraryDir, "Talk", "video.mp4")
	a.library.Jobs = []core.Job{{ID: "d", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: strings.TrimSuffix(video, ".mp4") + ".bilingual.srt", Untranslated: 12, LibraryDir: a.snap().LibraryDir}}
	tt.Frame()
	// The poster's line, and the hero's, as the film is the one done last.
	if textCount(tt, "YouTube · 12 句翻译失败，其余已完成") == 0 || textCount(tt, "YouTube · 已导出 video.bilingual.srt · 12 句翻译失败，其余已完成") == 0 {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// A job stopped because another library was opened did not fail: its card says it was stopped, as statusText words it, not 处理失败 in red over that.
func TestAStoppedCardSaysItWasStopped(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	job := core.Job{ID: "s", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, Err: core.ErrLibrarySwitched, LibraryDir: a.snap().LibraryDir}
	a.library.Jobs = []core.Job{job}
	film, ok := a.JobFilm(job.ID)
	if !ok {
		t.Fatal("no card")
	}
	if reason, section := a.FailureHint(film); reason != "已停止：切换了资料库" || section != "" {
		t.Fatalf("%q %q", reason, section)
	}
}
