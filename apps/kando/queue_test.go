package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/track"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// gatePipeline holds every job in Download until the test lets it through, so a test sees which job runs and in what order. started receives each source as its Download begins. A link downloads its sound first and its video beside its making (media.Make): only the first is held, and the one right after it goes through (passed).
type gatePipeline struct {
	testFolder
	hearsHello
	started chan string
	release chan struct{}
	mu      sync.Mutex
	passed  map[string]bool
}

func newGatePipeline() *gatePipeline {
	return &gatePipeline{started: make(chan string, 16), release: make(chan struct{})}
}

func (g *gatePipeline) Check() error { return nil }

func (g *gatePipeline) Download(ctx context.Context, source string) (string, error) {
	if g.next(source) {
		return g.video(filepath.Base(source) + ".mp4")
	}
	g.started <- source
	select {
	case <-g.release:
		g.mu.Lock()
		g.passed[source] = true
		g.mu.Unlock()
		return g.video(filepath.Base(source) + ".mp4")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// next reports whether a download of source is the one right after its held download went through, the video of a link made from its sound, which is not held.
func (g *gatePipeline) next(source string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.passed == nil {
		g.passed = map[string]bool{}
	}
	if g.passed[source] {
		delete(g.passed, source)
		return true
	}
	return false
}

func (g *gatePipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
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
	a, tt, updates := newTestApp(t, &fakePipeline{})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/v" || a.library.Jobs[0].Language != "简体中文" || a.library.Jobs[0].LibraryDir != testVideos {
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
	a, tt, updates := newTestApp(t, g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("first started %s", got)
	}
	noneStarted(t, g, updates)
	refresh(tt, a)
	if len(a.library.Jobs) != 3 || textCount(tt, "排队中") != 2 || textCount(tt, "下载中") != 1 {
		t.Fatalf("jobs %d texts %q", len(a.library.Jobs), tt.Texts())
	}
	ids := jobIDs(a)
	for _, name := range []string{"a", "b", "c"} {
		if !shows(tt, "example.com/"+name) {
			t.Fatalf("no card for %s: %q", name, tt.Texts())
		}
	}

	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("second started %s", got)
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageComplete || job.SubtitlePath != filepath.Join(testVideos, "a.bilingual.srt") {
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
		if job.Stage != core.StageComplete || job.SubtitlePath != filepath.Join(testVideos, name+".bilingual.srt") || job.CompletedAt.IsZero() {
			t.Fatalf("%s ended %+v", name, job)
		}
		// Two jobs can finish at the same time.Now() on Windows, whose clock is coarser than the time between them.
		if job.CompletedAt.Before(finished) {
			t.Fatalf("%s finished at %v, before the job before it at %v", name, job.CompletedAt, finished)
		}
		finished = job.CompletedAt
	}
}

// TestTheComputerStaysAwakeWhileJobsRun: while jobs run one after another, one hold keeps the computer, not the display, from sleeping, and it is given back once the last has ended. A job cancelled while running gives it back too, and a hold still taken as the app quits is given back then.
func TestTheComputerStaysAwakeWhileJobsRun(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	awake := &fakeAwake{}
	a.queue.keepAwake = awake.keepAwake
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	if len(awake.holds) != 1 || awake.held != 1 {
		t.Fatalf("while a runs: holds %+v, %d held", awake.holds, awake.held)
	}
	if hold := awake.holds[0]; hold.reason != "正在处理视频" || hold.display {
		t.Fatalf("the hold is %+v, want the reason 正在处理视频 and the display free to sleep", hold)
	}
	g.release <- struct{}{}
	nextStarted(t, g, updates)
	if len(awake.holds) != 1 || awake.held != 1 {
		t.Fatalf("as b follows a: holds %+v, %d held, want the first hold kept", awake.holds, awake.held)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	if awake.held != 0 {
		t.Fatalf("after the last job: holds %+v, %d held", awake.holds, awake.held)
	}

	if err := a.Enqueue("https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	if len(awake.holds) != 2 || awake.held != 1 {
		t.Fatalf("while c runs: holds %+v, %d held", awake.holds, awake.held)
	}
	a.Cancel(a.RunningJob())
	settle(t, a, tt, updates)
	if awake.held != 0 {
		t.Fatalf("after cancelling c: holds %+v, %d held", awake.holds, awake.held)
	}

	if err := a.Enqueue("https://example.com/d"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	if awake.held != 1 {
		t.Fatalf("while d runs: holds %+v, %d held", awake.holds, awake.held)
	}
	done := a.queue.done
	a.areasQuitting()
	if awake.held != 0 {
		t.Fatalf("after quitting: holds %+v, %d held", awake.holds, awake.held)
	}
	// Quitting stops d, which then reports nothing, so it is never marked done and settle would wait for it in vain: its pipeline returning is the end.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("quitting left d's pipeline running")
	}
}

// TestTypingALinkWhileAJobRunsQueuesIt checks that 开始处理 adds to the queue instead of being refused while another job runs.
func TestTypingALinkWhileAJobRunsQueuesIt(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
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
	a, tt, updates := newTestApp(t, g)
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
	refresh(tt, a)
	if !shows(tt, "已取消") {
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
	a, tt, updates := newTestApp(t, g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.Cancel(ids["https://example.com/a"])
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after cancelling a, %s started", got)
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || statusText(testlocale.Messages(), job) != "已取消" {
		t.Fatalf("a after Cancel %+v", job)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestCancelLeavesOtherJobsAlone checks that Cancel of a finished or unknown job does not stop the one running.
func TestCancelLeavesOtherJobsAlone(t *testing.T) {
	a, _, updates := newTestApp(t, &fakePipeline{block: true})
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
	queueUntilState(t, updates, func() bool { return !a.running() })
}

func TestRemovingTheRunningJobStartsTheNext(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
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
	a, tt, updates := newTestApp(t, g)
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: testVideos}}
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

// TestRetryGoesToTheLibrary checks that 重试 in the window, unlike the control socket's jobs.retry, shows the library with the retried job's card followed, and that a job without a language when the settings name none says so on the line under the link field and stays where it is.
func TestRetryGoesToTheLibrary(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	a.library.Jobs = []core.Job{
		{ID: "old", Source: "https://example.com/old", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: testVideos},
		{ID: "bare", Source: "https://example.com/bare", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: testVideos},
	}
	a.page = pageYouTube
	a.Retry("old")
	if a.page != pageFilms || a.queue.selectedID != "old" || a.queue.job == nil || a.queue.job.ID != "old" {
		t.Fatalf("Retry showed page %q, card %q", a.page, a.queue.selectedID)
	}
	if job := stageOf(t, a, "old"); job.Stage != core.StageQueued || job.Language != "简体中文" {
		t.Fatalf("old after Retry %+v", job)
	}
	nextStarted(t, g, updates)

	a.page = pageYouTube
	a.prefs.language = ""
	a.retry(stageOf(t, a, "bare"))
	if a.page != pageYouTube || a.queue.status != testlocale.Messages().QueueNoLanguage() {
		t.Fatalf("retry without a language: page %q, status %q", a.page, a.queue.status)
	}
	if job := stageOf(t, a, "bare"); job.Stage != core.StageFailed {
		t.Fatalf("bare after a refused retry %+v", job)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestRetrySaysWhatHoldsItBack checks that 重试 that cannot queue the job says the real reason in the message bar, even while another job runs: with no target language set, that one is missing and where to set it; for a job already waiting, nothing, since its card says so.
func TestRetrySaysWhatHoldsItBack(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	a.library.Jobs = []core.Job{
		{ID: "bare", Source: "https://example.com/bare", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: testVideos},
	}
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	a.prefs.language = ""
	a.Retry("bare")
	if job := stageOf(t, a, "bare"); job.Stage != core.StageFailed {
		t.Fatalf("bare after a refused retry %+v", job)
	}
	if a.lib.notice.Text != testlocale.Messages().QueueNoLanguage() || a.lib.notice.Section != settings.SectionTranslate {
		t.Fatalf("notice %+v", a.lib.notice)
	}

	a.lib.notice = films.Notice{}
	a.Retry(jobIDs(a)["https://example.com/b"])
	if a.lib.notice.Open {
		t.Fatalf("retrying a waiting job says %+v", a.lib.notice)
	}
	g.release <- struct{}{}
	nextStarted(t, g, updates)
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestSwitchLibraryKeepsTheRunningJob opens another library while a job runs: the job goes on, its card stays in 处理中 saying which library it saves into, and once it is done the message bar says where it went. The jobs queued in the first library run after it, before the one queued in the second, and each job stays in the library it was queued in.
func TestSwitchLibraryKeepsTheRunningJob(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
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
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("started %s", got)
	}
	ids := jobIDs(a)

	a.switchLibrary(second)
	noneStarted(t, g, updates)
	if a.RunningJob() != ids["https://example.com/a"] {
		t.Fatalf("running %q after the switch, want a", a.RunningJob())
	}
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageDownloading || job.Err != nil || job.LibraryDir != first {
		t.Fatalf("a after the switch %+v", job)
	}
	if err := a.Enqueue("https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	if !shows(tt, "处理中") || textCount(tt, "存入「first」") != 2 {
		t.Fatalf("the second library's queue does not show the first library's jobs: %q", tt.Texts())
	}

	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after a, %s started; want b, queued before c", got)
	}
	done := stageOf(t, a, ids["https://example.com/a"])
	if done.Stage != core.StageComplete || done.LibraryDir != first {
		t.Fatalf("a %+v", done)
	}
	if want := testlocale.Messages().QueueDoneElsewhere(jobName(done), "first"); a.lib.notice.Text != want || a.lib.notice.Kind != "success" {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/c" {
		t.Fatalf("after b, %s started", got)
	}
	g.release <- struct{}{}
	queueUntilState(t, updates, func() bool { return !a.running() })
	refresh(tt, a)
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageComplete || job.LibraryDir != first {
		t.Fatalf("b %+v", job)
	}
	if job := stageOf(t, a, jobIDs(a)["https://example.com/c"]); job.Stage != core.StageComplete || job.LibraryDir != second {
		t.Fatalf("c %+v", job)
	}
	if shows(tt, "存入「first」") {
		t.Fatalf("a finished job of the first library stayed in the second's queue: %q", tt.Texts())
	}
}

// TestCancelAJobOfAnotherLibrary cancels, from the open library's 处理中, the job still running for the library left: it stops as 已取消, its card leaves the open library's queue, and the message bar says nothing of a failure.
func TestCancelAJobOfAnotherLibrary(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{block: true})
	t.Cleanup(func() {
		if a.libraryWatch != nil {
			close(a.libraryWatch)
			a.libraryWatch = nil
		}
	})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	first := filepath.Join(t.TempDir(), "first.kando")
	a.switchLibrary(first)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	id := a.RunningJob()
	a.switchLibrary(filepath.Join(t.TempDir(), "second.kando"))
	refresh(tt, a)
	tt.Press("取消处理")
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || job.LibraryDir != first {
		t.Fatalf("a %+v", job)
	}
	if a.lib.notice.Open || shows(tt, "存入「first」") {
		t.Fatalf("notice %+v texts %q", a.lib.notice, tt.Texts())
	}
}

// TestForgettingTheRunningJobsLibraryLetsItFinish forgets the library a job is running for: the job goes on and completes into that folder, the message bar says where it was saved, and the job queued behind it in the forgotten library does not start.
func TestForgettingTheRunningJobsLibraryLetsItFinish(t *testing.T) {
	g := newGatePipeline()
	a, _, updates := newTestApp(t, g)
	t.Cleanup(func() {
		if a.libraryWatch != nil {
			close(a.libraryWatch)
			a.libraryWatch = nil
		}
	})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	first := filepath.Join(t.TempDir(), "first.kando")
	second := filepath.Join(t.TempDir(), "second.kando")
	a.switchLibrary(second)
	a.switchLibrary(first)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("started %s", got)
	}
	ids := jobIDs(a)

	a.forgetLibrary(first)
	if dir := a.snap().LibraryDir; filepath.Clean(dir) != filepath.Clean(second) {
		t.Fatalf("open library %q after forgetting the first, want the second", dir)
	}
	if a.RunningJob() != ids["https://example.com/a"] {
		t.Fatalf("running %q after forgetting its library, want a", a.RunningJob())
	}
	g.release <- struct{}{}
	queueUntilState(t, updates, func() bool { return !a.running() })
	noneStarted(t, g, updates)
	done := stageOf(t, a, ids["https://example.com/a"])
	if done.Stage != core.StageComplete || done.LibraryDir != first {
		t.Fatalf("a %+v", done)
	}
	if want := testlocale.Messages().QueueDoneElsewhere(jobName(done), "first"); a.lib.notice.Text != want {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageQueued {
		t.Fatalf("b %+v", job)
	}
}

// TestJobsOfAForgottenLibraryWait queues jobs in three libraries: the open one, another the app knows, and one it has forgotten. The queue runs the first two, the other library's after the open one's as they were queued, and shows the other library's in 处理中; the forgotten library's job waits, out of sight, until that library is opened again.
func TestJobsOfAForgottenLibraryWait(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	a.edit(func(s *session) { s.Libraries = []string{testVideos, "/other.kando"} })
	a.library.Jobs = []core.Job{
		{ID: "gone", Source: "https://example.com/gone", Language: "简体中文", Stage: core.StageQueued, LibraryDir: "/gone.kando"},
		{ID: "other", Source: "https://example.com/other", Language: "简体中文", Stage: core.StageQueued, LibraryDir: "/other.kando"},
		{ID: "open", Source: "https://example.com/open", Language: "简体中文", Stage: core.StageQueued, LibraryDir: testVideos},
	}
	queued := func() []string {
		var keys []string
		for _, item := range a.QueueFilms() {
			keys = append(keys, item.JobID+"@"+item.Library)
		}
		return keys
	}
	if got := strings.Join(queued(), ","); got != "other@other,open@" {
		t.Fatalf("处理中 %s", got)
	}
	a.pump()
	if got := nextStarted(t, g, updates); got != "https://example.com/open" {
		t.Fatalf("started %s", got)
	}
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/other" {
		t.Fatalf("started %s", got)
	}
	refresh(tt, a)
	if !shows(tt, "存入「other」") {
		t.Fatalf("texts %q", tt.Texts())
	}
	g.release <- struct{}{}
	queueUntilState(t, updates, func() bool { return !a.running() })
	noneStarted(t, g, updates)
	if job := stageOf(t, a, "gone"); job.Stage != core.StageQueued {
		t.Fatalf("gone %+v", job)
	}
	if got := strings.Join(queued(), ","); got != "" {
		t.Fatalf("处理中 %s", got)
	}
}

// TestAJobSavesIntoItsOwnLibrary checks the pipeline a job runs with: it saves into the job's library, not the one open, and borrows the login the job names, while the app's pipeline stays as it was. A job saved before jobs named their library saves into the open one.
func TestAJobSavesIntoItsOwnLibrary(t *testing.T) {
	testVideosFor(t)
	a, _, _ := newTestApp(t, media.Tools{OutputDir: testVideos})
	got, ok := a.queuePipeline(core.Job{LibraryDir: "/other.kando", Cookies: "chrome"}).(media.Tools)
	if !ok || got.OutputDir != "/other.kando" || got.Cookies != "chrome" {
		t.Fatalf("pipeline %+v", got)
	}
	if tools := a.pipeline.(media.Tools); tools.OutputDir != testVideos || tools.Cookies != "" {
		t.Fatalf("the app's pipeline changed: %+v", tools)
	}
	if got := a.queuePipeline(core.Job{}).(media.Tools); got.OutputDir != testVideos {
		t.Fatalf("pipeline of an old job %+v", got)
	}
}

// TestRestartResumesInterruptedJobs saves a shelf the way a quit in the middle of a job leaves it and opens it again: the interrupted job and those queued behind it run again, oldest first, and finished jobs stay finished.
func TestRestartResumesInterruptedJobs(t *testing.T) {
	testVideosFor(t)
	path := filepath.Join(t.TempDir(), "library.json")
	before, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	before.Jobs = []core.Job{
		{ID: "c", Source: "https://example.com/c", Language: "简体中文", Stage: core.StageQueued, LibraryDir: testVideos},
		{ID: "b", Source: "https://example.com/b", Language: "简体中文", Stage: core.StageQueued, LibraryDir: testVideos},
		{ID: "a", Source: "https://example.com/a", Language: "简体中文", Stage: core.StageTranscribing, LibraryDir: testVideos, VideoPath: testVideos + "/a.mp4"},
		{ID: "done", Source: "https://example.com/done", Language: "简体中文", Stage: core.StageComplete, LibraryDir: testVideos, VideoPath: testVideos + "/done.mp4"},
	}
	if err := before.Save(); err != nil {
		t.Fatal(err)
	}

	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
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
	a, tt, updates := newTestApp(t, p)
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Stage: core.StageQueued, LibraryDir: testVideos}}

	a.pump()
	settle(t, a, tt, updates)

	job := stageOf(t, a, "old")
	if job.Language != "简体中文" || p.language != "简体中文" {
		t.Fatalf("language = %q, pipeline language = %q", job.Language, p.language)
	}
}

func TestEnqueueSaysWhyASourceIsRefused(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{block: true})
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
	if err := a.Enqueue("https://example.com/v"); err == nil || err.Error() != testlocale.Messages().QueueNoLibrary() {
		t.Fatalf("err %v", err)
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// TestALinkTypedWithoutTheSchemeIsQueued checks that an address typed or pasted without https://, as vimeo.com/76979871, is queued with https:// before it, as a browser would open it, rather than refused as not a link.
func TestALinkTypedWithoutTheSchemeIsQueued(t *testing.T) {
	for _, typed := range []string{"vimeo.com/76979871", "www.youtube.com/watch?v=Rv3Hn7Qm2Ts"} {
		t.Run(typed, func(t *testing.T) {
			a, tt, updates := newTestApp(t, &fakePipeline{})
			startJob(t, tt, typed)
			if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://"+typed || !shows(tt, "已加入，开始处理。") {
				t.Fatalf("jobs %+v, texts %q", a.library.Jobs, tt.Texts())
			}
			settle(t, a, tt, updates)
		})
	}
}

func TestViewRunsJobToCompletion(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(t, p)
	startJob(t, tt, "https://example.com/video")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://example.com/video" || !shows(tt, "已加入，开始处理。") {
		t.Fatalf("texts %q after starting", tt.Texts())
	}

	settle(t, a, tt, updates)
	if !shows(tt, "最近完成") || !shows(tt, "talk") || shows(tt, "处理中") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
	if !shows(tt, "开始处理") {
		t.Fatal("start button did not come back")
	}
}

func TestViewCancelsRunningJob(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{block: true})
	startJob(t, tt, "https://example.com/video")
	refresh(tt, a)
	tt.Press("取消处理")

	settle(t, a, tt, updates)
	if !shows(tt, "已取消") || !shows(tt, "开始处理") {
		t.Fatalf("texts %q after cancelling", tt.Texts())
	}
}

// TestFilmCancelStopsItsOwnJob checks that 取消 on an open film cancels that film's job through Cancel(id), and that RunningJob names the job while it runs.
func TestFilmCancelStopsItsOwnJob(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{block: true})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	id := a.library.Jobs[0].ID
	if got := a.RunningJob(); got != id {
		t.Fatalf("RunningJob = %q, want %q", got, id)
	}
	a.openKey = id
	refresh(tt, a)
	tt.Press("取消")
	queueUntilState(t, updates, func() bool { return !a.running() })
	job, ok := a.library.Find(id)
	if !ok || job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || a.RunningJob() != "" {
		t.Fatalf("job %+v, RunningJob %q", job, a.RunningJob())
	}
}

// refusedTranslation is fakePipeline with a translation the AI service refuses as a whole, as one whose key it rejects.
type refusedTranslation struct{ fakePipeline }

func (*refusedTranslation) Translate(context.Context, []core.Segment, string) ([]core.Segment, error) {
	return nil, errors.New("translation API returned 401 Unauthorized")
}

// A job whose translation fails as a whole is still a film: it goes to the done grid with its original lines, says it has no translation and why, and 翻译 translates its missing lines once the service works, without downloading or transcribing it again.
func TestAFailedTranslationStillMakesTheFilm(t *testing.T) {
	a, tt, updates := newTestApp(t, &refusedTranslation{})
	startJob(t, tt, "https://example.com/video")
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	if job.Stage != core.StageComplete || job.SubtitlePath == "" || job.TranslationErr == "" || job.Untranslated != 1 {
		t.Fatalf("job %+v", job)
	}
	msgs := testlocale.Messages()
	if shows(tt, "处理中") || !shows(tt, msgs.FilmsNoTranslation()) || !shows(tt, msgs.PrecheckTranslateAuth()) {
		t.Fatalf("the film does not say it has no translation: %q", tt.Texts())
	}
	tt.Press("film-translate")
	if job := a.library.Jobs[0]; job.Redo != core.RedoMissing || job.Stage != core.StageQueued || job.Language != "简体中文" {
		t.Fatalf("翻译 queued %+v", job)
	}
}

func TestRetryKeepsTheSameCard(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{{
		ID: "same", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文",
		Stage: core.StageFailed, VideoPath: testVideos + "/Colibri [TRWgBxVAb1M].mp4",
		Err: media.ReadStageError("translate: translation API returned 401"),
	}}
	refresh(tt, a)
	tt.Press("重试")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage == core.StageFailed {
		t.Fatalf("right after retry: %#v", a.library.Jobs)
	}

	settle(t, a, tt, updates)
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage != core.StageComplete {
		t.Fatalf("after retry: %#v", a.library.Jobs)
	}
	if shows(tt, "失败") {
		t.Fatalf("failed card stayed: %q", tt.Texts())
	}
}

// TestSwitchLibraryRemembersTheDirectoryAndKeepsTheJob switches to a new library while a job runs: the library is made and saved in settings.json, and the job goes on.
func TestSwitchLibraryRemembersTheDirectoryAndKeepsTheJob(t *testing.T) {
	p := &fakePipeline{block: true}
	a, tt, updates := newTestApp(t, p)
	t.Cleanup(func() {
		if a.libraryWatch != nil {
			close(a.libraryWatch)
			a.libraryWatch = nil
		}
	})
	startJob(t, tt, "https://example.com/video")
	if !a.running() {
		t.Fatal("job did not start")
	}
	id := a.RunningJob()
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
	drain := time.After(150 * time.Millisecond)
	for waiting := true; waiting; {
		select {
		case fn := <-updates:
			fn()
		case <-drain:
			waiting = false
		}
	}
	if a.RunningJob() != id {
		t.Fatalf("running %q after the switch, want %q", a.RunningJob(), id)
	}
	a.Cancel(id)
	settle(t, a, tt, updates)
}

// TestFilmPageCancelsTheFilmItShows opens a film still waiting in the queue, which the job running gives way to (queuePrefer), and presses its 取消: that film leaves the queue as 已取消, and the job that gave way to it goes on.
func TestFilmPageCancelsTheFilmItShows(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.openKey = ids["https://example.com/b"]
	refresh(tt, a)
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("the film opened did not go first: %s started", got)
	}
	tt.Press("取消")
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("after b's 取消, %s started", got)
	}
	if job := stageOf(t, a, ids["https://example.com/b"]); job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("b after its 取消 %+v", job)
	}
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

// TestTheFilmOpenedThatCannotRunTakesNoTurn opens a film waiting in the queue that cannot run now, as one whose library is not open on this computer: the job running goes on rather than give way to it.
func TestTheFilmOpenedThatCannotRunTakesNoTurn(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	apply(tt, a, func() {
		for i := range a.library.Jobs {
			if a.library.Jobs[i].ID == ids["https://example.com/b"] {
				a.library.Jobs[i].LibraryDir = filepath.Join(t.TempDir(), "elsewhere")
			}
		}
		a.openKey = ids["https://example.com/b"]
	})
	refresh(tt, a)
	noneStarted(t, g, updates)
	if a.queue.active != ids["https://example.com/a"] {
		t.Fatalf("running %q, want a", a.queue.active)
	}
	a.Cancel(ids["https://example.com/a"])
	settle(t, a, tt, updates)
}

// ytdlpPipeline downloads with the real media.Tools.Download, run on a yt-dlp script that prints what the real yt-dlp prints for Download's arguments, and stubs the stages after it.
type ytdlpPipeline struct {
	hearsHello
	tools media.Tools
}

func (p ytdlpPipeline) Check() error { return nil }

func (p ytdlpPipeline) Download(ctx context.Context, source string) (string, error) {
	return p.tools.Download(ctx, source)
}

func (ytdlpPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

// gatedYTDLP writes a yt-dlp that names the video, says a quarter of the download is done (half of the first of two formats) and waits until gate exists before it finishes, the video then in the test's library (testVideos).
func gatedYTDLP(t *testing.T, thumbnail, gate string) string {
	t.Helper()
	testVideosFor(t)
	video := filepath.Join(testVideos, "zoo.mp4")
	return writeFakeScript(t, t.TempDir(), "yt-dlp", `echo 'kando-info {"id": "jNQXAC9IVRw", "title": "Me at the zoo", "thumbnail": "`+thumbnail+`", "duration": 19}'
echo 'kando-formats ["395", "251"]'
echo 'kando-progress downloading 111890 223780 NA 1048576 3 NA NA 395'
i=0
while [ ! -e '`+gate+`' ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done
echo 'kando-progress finished 252182 252182 NA NA NA NA NA 251'
echo v > '`+video+`'
echo '`+video+`'`)
}

func TestTheCardFollowsTheDownload(t *testing.T) {
	png := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(png) }))
	t.Cleanup(srv.Close)
	gate := filepath.Join(t.TempDir(), "gate")
	a, tt, updates := newTestApp(t, ytdlpPipeline{tools: media.Tools{YTDLP: gatedYTDLP(t, srv.URL+"/hq.jpg", gate), OutputDir: testVideos}})
	path := filepath.Join(t.TempDir(), "library.json")
	a.openLibrary(path)
	a.lib.posterDir = t.TempDir()
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	id := a.library.Jobs[0].ID
	queueUntilState(t, updates, func() bool { return stageOf(t, a, id).Progress.Known })
	job := stageOf(t, a, id)
	if job.Title != "Me at the zoo" || job.Progress.Fraction != 0.25 {
		t.Fatalf("job %+v", job)
	}
	refresh(tt, a)
	for _, want := range []string{"Me at the zoo", "下载中 · 25% · 还剩 0:03"} {
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
	queueUntilState(t, updates, func() bool { return libraryHasImage(a.Thumbnail(film)) })

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
	job.Err, job.FailKind, job.Cookies, job.Title = media.ReadStageError("download: yt-dlp: exit status 1"), kind, cookies, "Coastal Erosion Documentary"
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
	a, tt, _ := newTestApp(t, &fakePipeline{})
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
		{"https://example.com/", core.FailUnsupported, "", "example.com 暂不支持", "没在页面里找到视频。换个链接，或先下载到本机再拖进来。", []string{"换个链接", "选择本地文件", "查看支持的网站"}},
		{"https://www.instagram.com/p/C0000000000/", core.FailLogin, "", "要登录才能看", "instagram.com 的这个视频需要账号。借用浏览器里已登录的账号即可，不保存密码。", []string{"借用 Chrome 登录并重试"}},
		{yt, core.FailNetwork, "", "连不上网络", "检查网络连接后再重试。", []string{"重试"}},
		{"https://nonexistent-host-kando-review.invalid/video/1", core.FailHost, "", "找不到 nonexistent-host-kando-review.invalid", "网络正常的话，多半是链接里的网址写错了，或者这个网站已经关闭。换个链接，或检查网络后再重试。", []string{"换个链接", "重试"}},
		{yt, core.FailCookies, "safari", "读不到 Safari 的登录", "macOS 只让有「完全磁盘访问权限」的 App 读 Safari 的登录。在「系统设置 › 隐私与安全性 › 完全磁盘访问权限」里允许看懂，回来再重试，或改用别的浏览器。", []string{"打开系统设置", "已允许，重试", "改用 Chrome", "选择浏览器…"}},
		{yt, core.FailCookies, "chrome", "读不到 Chrome 的登录", "这台电脑上找不到 Chrome 的登录数据。换一个已登录的浏览器再试。", []string{"改用 Safari", "选择浏览器…"}},
		{"https://www.youtube.com/watch?v=dQw4w9WgX", core.FailBadLink, "", "链接不完整", testlocale.Messages().QueueRemedyBadLinkWhy(), []string{"换个链接"}},
	} {
		film := failedJob(t, a, tc.source, tc.kind, tc.cookies)
		if tc.kind == core.FailCookies && tc.cookies == "safari" {
			// The real error of yt-dlp reading Safari's cookies without Full Disk Access (packages/media/testdata/ytdlp/errors/cookies-safari-denied.stderr).
			job, _ := a.library.Find(film.JobID)
			job.Err = media.ReadStageError("download: yt-dlp: exit status 1: ERROR: [Errno 1] Operation not permitted: '/Users/someone/Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies'")
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
	refresh(tt, a)
	for _, want := range []string{"你所在的地区不能看", "上传者限制了地区。别处有这个视频的话，换个链接。", "换个链接", "移除"} {
		if textCount(tt, want) == 0 {
			t.Fatalf("the card does not say %q", want)
		}
	}
}

// recordingYTDLP writes a yt-dlp that saves its arguments, one a line, to args and fails as one too old for the site does, printing the real stderr of yt-dlp 2024.03.10 on YouTube. Its -U, also written to args, updates it to updatesTo, or leaves it at 2024.03.10 as the newest there is when updatesTo is "".
func recordingYTDLP(t *testing.T, args, updatesTo string) string {
	t.Helper()
	stderr, err := filepath.Abs(filepath.Join("..", "..", "packages", "media", "testdata", "ytdlp", "errors", "outdated-youtube.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	update := "echo \"Latest version: stable@$v\"; echo \"yt-dlp is up to date (stable@$v)\""
	if updatesTo != "" {
		update = "echo " + updatesTo + " > \"$0.version\"; echo 'Updated yt-dlp to stable@" + updatesTo + "'"
	}
	return writeFakeScript(t, t.TempDir(), "yt-dlp", "v=$(cat \"$0.version\" 2>/dev/null || echo 2024.03.10)\n"+
		"case \"$1\" in --version) echo \"$v\"; exit 0;; -U) echo -U >> '"+args+"'; "+update+"; exit 0;; esac\n"+
		"for a in \"$@\"; do echo \"$a\"; done >> '"+args+"'\ncat '"+stderr+"' >&2\nexit 1")
}

// toolsForRemedies is media.Tools that passes Check with tools that are never reached, as a job fails at its download.
func toolsForRemedies(t *testing.T, ytdlp string) media.Tools {
	model := filepath.Join(t.TempDir(), "ggml-tiny.bin")
	if err := os.WriteFile(model, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	return media.Tools{OutputDir: testVideos, YTDLP: ytdlp, FFmpeg: "/bin/false", Whisper: "/bin/false", WhisperModel: model, Translator: media.ChatTranslator{BaseURL: "http://127.0.0.1:9/v1", Model: "m"}}
}

func TestUpdateAndRetryUpdatesYTDLPAndRunsTheFailedDownloadsAgain(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	tool := recordingYTDLP(t, args, "2026.11.03")
	a, tt, updates := newTestApp(t, toolsForRemedies(t, tool))
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailOutdated {
		t.Fatalf("job %+v", job)
	}
	a.queueReadYTDLP()
	queueUntilState(t, updates, func() bool { return a.queue.ytdlp.version != "" })
	film, _ := a.JobFilm(id)
	r, _ := a.FailureRemedy(film)
	if r.Why != "本机的 yt-dlp 是 2024.03.10 版，更新几秒就好。" || r.Actions[0].ID != remedyUpdate {
		t.Fatalf("remedy %+v", r)
	}
	a.RunRemedy(film, remedyUpdate)
	if r, _ := a.FailureRemedy(film); r.Headline != "正在更新下载组件…" || !r.Actions[0].Disabled {
		t.Fatalf("while updating %+v", r)
	}
	queueUntilState(t, updates, func() bool { return !a.queue.ytdlp.updating })
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
	a, tt, updates := newTestApp(t, toolsForRemedies(t, recordingYTDLP(t, args, "2026.11.03")))
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

// hiddenXPost is a post on X whose video the fake yt-dlp finds only through Chrome's sign-in (fakeYTDLPHiddenPost), as X shows a sensitive post's video only to an account that allows it.
const hiddenXPost = "https://x.com/someone/status/2122000096271503767"

// newHiddenXApp starts the app for a test of X's hidden videos with the fake yt-dlp, saving into a library of its own, and tools after it that a download that fails never reaches.
func newHiddenXApp(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	tools := toolsForRemedies(t, fakeTool(t, filepath.Join(t.TempDir(), "bin"), "yt-dlp"))
	tools.CookieDir = filepath.Join(t.TempDir(), "cookies")
	a, tt, updates := newTestApp(t, tools)
	lib := t.TempDir()
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	// The downloaded video's poster is read, and its cover.jpg written into the film's folder, off the engine's goroutine once the job is done; it finishes before the library folder is removed, which it would otherwise make again partway (a TempDir cleanup that fails with "directory not empty").
	t.Cleanup(func() {
		timeout := time.After(5 * time.Second)
		for len(a.lib.posterBusy) > 0 || len(a.lib.thumbBusy) > 0 {
			select {
			case fn := <-updates:
				onEngine(a, fn)
			case <-timeout:
				t.Error("the poster was still being read when the test ended")
				return
			}
		}
	})
	return a, tt, updates
}

// hiddenXDownloaded fails the test unless the job id got past its download, which only the post's video lets it do.
func hiddenXDownloaded(t *testing.T, a *app, id string) {
	t.Helper()
	job := stageOf(t, a, id)
	if job.FailKind != "" || job.VideoPath == "" || (job.Err != nil && strings.HasPrefix(job.Err.Error(), "download:")) {
		t.Fatalf("the post's video was not downloaded: %+v", job)
	}
}

// A post on X that yt-dlp found no video in when it borrowed no sign-in says that X shows such videos only to signed-in accounts, rather than yt-dlp's words: 选择浏览器… opens Settings › 通用, where the sign-in is chosen, and 重试 then downloads the video with it.
func TestHiddenXVideoSignedOutLeadsToTheSignIn(t *testing.T) {
	a, tt, updates := newHiddenXApp(t)
	withBrowsers(t, a, "chrome", "firefox")
	if err := a.Enqueue(hiddenXPost); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailHiddenVideo || job.Cookies != "" {
		t.Fatalf("job %+v", job)
	}
	msgs := testlocale.Messages()
	film, _ := a.JobFilm(id)
	r, ok := a.FailureRemedy(film)
	if !ok || r.Headline != "X 只给已登录的账号看这个视频" || r.Why != "敏感帖子和受保护账号的视频，X 只给已登录的账号看。选一个已登录 X 的浏览器，再重试。" ||
		strings.Join(remedyLabels(r), ",") != strings.Join([]string{msgs.QueueRemedyChooseBrowser(), msgs.QueueRemedyRetry()}, ",") || !r.Actions[0].Primary || r.Actions[1].Primary {
		t.Fatalf("remedy %+v", r)
	}
	refresh(tt, a)
	if textCount(tt, r.Headline) == 0 {
		t.Fatalf("the card does not say %q; texts %q", r.Headline, tt.Texts())
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "No video could be found") {
			t.Fatalf("the card shows yt-dlp's words: %q", text)
		}
	}
	a.RunRemedy(film, remedyChooseBrowser)
	refresh(tt, a)
	wantSettings(t, a, settings.SectionGeneral)
	a.SetSignInBrowser("chrome")
	settle(t, a, tt, updates)
	a.RunRemedy(film, remedyRetry)
	settle(t, a, tt, updates)
	hiddenXDownloaded(t, a, id)
	if job := stageOf(t, a, id); job.Cookies != "chrome" {
		t.Fatalf("the retry borrowed %q", job.Cookies)
	}
}

// With no browser on the computer whose sign-in yt-dlp can borrow, the card of X's hidden video says so, and offers to look for one again in place of 选择浏览器….
func TestHiddenXVideoSignedOutWithoutABrowser(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	withBrowsers(t, a)
	msgs := testlocale.Messages()
	film := failedJob(t, a, hiddenXPost, core.FailHiddenVideo, "")
	r, ok := a.FailureRemedy(film)
	if !ok || r.Why != msgs.QueueRemedySentences(msgs.QueueRemedyXSignedOutNeed(), msgs.QueueRemedyNoBrowser()) ||
		strings.Join(remedyLabels(r), ",") != strings.Join([]string{msgs.QueueRemedyFindBrowsers(), msgs.QueueRemedyRetry()}, ",") {
		t.Fatalf("remedy %+v", r)
	}
}

// A post on X that yt-dlp found no video in through a browser's sign-in is one X does not show that account the video of: the card names X's setting for sensitive media rather than offering the sign-in again or saying it ran out, and 重试 and 改用 … run again with that browser or the other one.
func TestHiddenXVideoSignedInNamesXsSetting(t *testing.T) {
	a, tt, updates := newHiddenXApp(t)
	withBrowsers(t, a, "chrome", "firefox")
	a.signin.browser = "firefox"
	if err := a.Enqueue(hiddenXPost); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailHiddenVideo || job.Cookies != "firefox" {
		t.Fatalf("job %+v", job)
	}
	msgs := testlocale.Messages()
	film, _ := a.JobFilm(id)
	r, ok := a.FailureRemedy(film)
	want := "确认 Firefox 里已登录 X。敏感帖子的视频要在「设置和隐私 › 隐私和安全 › 你看到的内容」里打开「显示可能含有敏感内容的媒体」才看得到，受保护账号的视频只给关注者看。"
	if !ok || r.Headline != "X 没给你的账号显示视频" || r.Why != want ||
		strings.Join(remedyLabels(r), ",") != strings.Join([]string{msgs.QueueRemedyRetry(), msgs.QueueRemedySwitchBrowser("Chrome")}, ",") {
		t.Fatalf("remedy %+v", r)
	}
	// 重试 keeps the job's browser, whose account still sees no video.
	a.RunRemedy(film, r.Actions[0].ID)
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.FailKind != core.FailHiddenVideo || job.Cookies != "firefox" {
		t.Fatalf("after 重试 %+v", job)
	}
	a.RunRemedy(film, r.Actions[1].ID)
	settle(t, a, tt, updates)
	hiddenXDownloaded(t, a, id)
	if job := stageOf(t, a, id); job.Cookies != "chrome" {
		t.Fatalf("the switch borrowed %q", job.Cookies)
	}
}

// 换个链接 does what the prototype's swapLink does: the card goes, the link field above is emptied and takes the focus, and the message bar says to paste the link from elsewhere there. A link pasted then starts a job of its own, without the old card's borrowed login.
func TestSwapLinkRemovesTheCardAndFocusesTheLinkField(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", core.FailRegion, "chrome")
	a.queue.source = "half typed"
	refresh(tt, a)
	tt.Press("换个链接")
	refresh(tt, a)
	if _, ok := a.library.Find(film.JobID); ok {
		t.Fatal("the card stayed")
	}
	if a.queue.source != "" || a.lib.notice.Text != "已移除「Coastal Erosion Documentary」。如果别处有这个视频，把链接粘贴到右上角。" {
		t.Fatalf("source %q, notice %q", a.queue.source, a.lib.notice.Text)
	}
	refresh(tt, a)
	if !tt.Find("视频链接").Focused {
		t.Fatal("the link field does not have the focus")
	}
	typeFocused(t, tt, "https://vimeo.com/76979871")
	if a.queue.source != "https://vimeo.com/76979871" {
		t.Fatalf("typed into %q", a.queue.source)
	}
	tt.Press("开始处理")
	confirmAddSheet(tt)
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
	a, tt, updates := newTestApp(t, &fakePipeline{})
	withBrowsers(t, a, "firefox")
	a.queue.ytdlp.browsers = nil
	a.queueReadYTDLP()
	queueUntilState(t, updates, func() bool { return len(a.queue.ytdlp.browsers) == 1 })
	installedBrowsers = func() []media.Browser {
		t.Error("a frame looked for the browsers")
		return nil
	}
	film := failedJob(t, a, "https://www.instagram.com/p/C0000000000/", core.FailLogin, "")
	refresh(tt, a)
	if textCount(tt, "借用 Firefox 登录并重试") == 0 {
		t.Fatal("the card does not offer Firefox")
	}
	if r, _ := a.FailureRemedy(film); r.Actions[0].ID != remedyBorrow+"firefox" {
		t.Fatalf("remedy %+v", r)
	}
}

// With no browser whose login yt-dlp reads, as on a Windows or Linux machine without one, a card that wants a login says so and offers to look again instead of a button that borrows from a browser that is not there. Safari is never offered in its place.
func TestNoBrowserToBorrowFrom(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
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
	applyNext(t, updates)
	if a.lib.notice.Text != "还是没找到能借用登录的浏览器。" {
		t.Fatalf("notice %q", a.lib.notice.Text)
	}
	withBrowsers(t, a, "chromium")
	a.queue.ytdlp.browsers = nil
	a.RunRemedy(film, remedyFindBrowsers)
	applyNext(t, updates)
	refresh(tt, a)
	if textCount(tt, "借用 Chromium 登录并重试") == 0 {
		t.Fatal("the card does not offer Chromium")
	}
}

// 更新并重试 on a yt-dlp that is already the newest runs nothing again: the site's fix is not out yet, and the card says so with 重试 and another look for an update.
func TestUpdateThatFindsTheNewestRunsNothingAgain(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	a, tt, updates := newTestApp(t, toolsForRemedies(t, recordingYTDLP(t, args, "")))
	if err := a.Enqueue("https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	a.queueReadYTDLP()
	queueUntilState(t, updates, func() bool { return a.queue.ytdlp.version != "" })
	film, _ := a.JobFilm(id)
	a.RunRemedy(film, remedyUpdate)
	queueUntilState(t, updates, func() bool { return !a.queue.ytdlp.updating })
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

// A read of the yt-dlp the pipeline used to download with can land after the pipeline moved on to another, as when Kando's own yt-dlp came in and was updated while it ran: it is stale, and what the queue knows of the newer yt-dlp stays.
func TestAStaleYTDLPReadKeepsTheNewerOne(t *testing.T) {
	old := recordingYTDLP(t, filepath.Join(t.TempDir(), "old-args"), "")
	a, _, updates := newTestApp(t, toolsForRemedies(t, old))
	a.queueReadYTDLP()
	var read func()
	select {
	case read = <-updates:
	case <-time.After(5 * time.Second):
		t.Fatal("the read never finished")
	}
	newer := filepath.Join(t.TempDir(), "yt-dlp")
	tools := a.pipeline.(media.Tools)
	tools.YTDLP = newer
	a.pipeline = tools
	a.queue.ytdlp.path, a.queue.ytdlp.version = newer, "2026.11.03"
	read()
	if y := a.queue.ytdlp; y.path != newer || y.version != "2026.11.03" {
		t.Fatalf("the stale read of %s overwrote the newer yt-dlp: %+v", old, y)
	}
}

// systemYTDLP makes the yt-dlp at path, version 2024.03.10, the one the queue knows, installed as media.FindYTDLPInstall finds the one a Linux distribution's packages put in /usr/bin: a Python script Kando may not write, which it cannot update.
func systemYTDLP(a *app, path string) {
	a.queue.ytdlp.path, a.queue.ytdlp.version = path, "2024.03.10"
	a.queue.ytdlp.install = media.YTDLPInstall{Kind: "system", Dir: filepath.Dir(path)}
}

// A yt-dlp the system's packages installed is not upgraded with pip: its card says to update it with the package manager, and where Kando can download a yt-dlp of its own it offers that first, says while it downloads, and says why when the download failed, offering it again.
func TestOutdatedWithASystemYTDLPOffersKandosOwn(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	systemYTDLP(a, "/usr/bin/yt-dlp")
	// The card names the folder as the system writes it: \usr\bin on Windows.
	dir := filepath.Dir("/usr/bin/yt-dlp")
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", core.FailOutdated, "")
	r, _ := a.FailureRemedy(film)
	if r.Why != "本机的 yt-dlp 是 2024.03.10 版，看懂没法替它更新：yt-dlp 在 "+dir+"，看懂没有权限改动它；用安装它的方式更新，例如系统的包管理器或 sudo pip。更新后再重试。" || strings.Join(remedyLabels(r), ",") != "已更新，重试" {
		t.Fatalf("without a yt-dlp to download: %+v", r)
	}

	release := make(chan struct{})
	fakeKits(t, a, release, "yt-dlp")
	r, _ = a.FailureRemedy(film)
	if r.Why != "本机的 yt-dlp 是 2024.03.10 版，看懂没法替它更新：yt-dlp 在 "+dir+"，看懂没有权限改动它；用安装它的方式更新，例如系统的包管理器或 sudo pip。也可以让看懂下载自己的一份。" ||
		strings.Join(remedyLabels(r), ",") != "下载 yt-dlp 并重试,已更新，重试" || r.Actions[0].ID != remedyDownloadYTDLP || !r.Actions[0].Primary || r.Actions[1].ID != remedyRetry {
		t.Fatalf("with a yt-dlp to download: %+v", r)
	}
	refresh(tt, a)
	tt.Press("下载 yt-dlp 并重试")
	if got := a.kitsDownloading(); len(got) != 1 || got[0] != "yt-dlp" {
		t.Fatalf("downloading %v", got)
	}
	r, _ = a.FailureRemedy(film)
	if r.Headline != "正在下载 yt-dlp…" || r.Why != "看懂自己的 yt-dlp 下载好后，会自动重新下载。" || !r.Working || len(r.Actions) != 1 || !r.Actions[0].Disabled {
		t.Fatalf("while downloading: %+v", r)
	}

	close(release)
	queueUntilState(t, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	r, _ = a.FailureRemedy(film)
	if r.Why != "没能下载 yt-dlp：服务器返回 HTTP 503。再试一次，或更新本机的 yt-dlp 后重试。" || strings.Join(remedyLabels(r), ",") != "下载 yt-dlp 并重试,已更新，重试" {
		t.Fatalf("after the download failed: %+v", r)
	}
	if job := stageOf(t, a, film.JobID); job.Stage != core.StageFailed || job.FailKind != core.FailOutdated {
		t.Fatalf("the download ran again without a yt-dlp: %+v", job)
	}
}

// Once the yt-dlp 下载 yt-dlp 并重试 asked for is in, the tools are looked up again and find it. The kit is the release Kando was built with, here as old as the yt-dlp that failed, so it is updated with -U before every download yt-dlp was too old for runs again with it.
func TestDownloadingKandosYTDLPRunsTheOutdatedDownloadsAgain(t *testing.T) {
	kit, ok := media.KitFor("yt-dlp")
	if !ok {
		t.Skip("Kando downloads no yt-dlp on this platform")
	}
	a, tt, updates := newTestApp(t, toolsForRemedies(t, recordingYTDLP(t, filepath.Join(t.TempDir(), "system-args"), "")))
	// The tools are looked up again with ffmpeg and whisper-cli first on PATH; Kando's own yt-dlp comes before any on PATH. They are scripts of their own rather than links to /bin/false, which macOS does not have: a dangling link is no program, so the lookup would fall through to whatever the machine has, or find none and fail the job's check before its download. They answer their version, as the check of the tools asks it (toolsCheck), which would otherwise hold the jobs back as programs that do not start, and fail at anything else.
	bin := t.TempDir()
	for _, name := range []string{"ffmpeg", "whisper-cli"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\ncase \"$1\" in -version|--version) echo 1.0; exit 0;; esac\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	own := t.TempDir()
	saved := media.OwnToolsDir
	media.OwnToolsDir = own
	t.Cleanup(func() { media.OwnToolsDir = saved })
	// An update of Kando's own yt-dlp runs on a goroutine of its own and writes its version beside it (recordingYTDLP); one still running as the test ends, as the update when the tools changed can be on a loaded runner, would write into own while it is removed, so the test waits it out first. Registered after own, this runs before own is removed.
	t.Cleanup(func() { queueUntilState(t, updates, func() bool { return !a.queue.ytdlp.updating }) })
	args := filepath.Join(t.TempDir(), "args")
	downloaded := recordingYTDLP(t, args, "2026.11.03")
	a.kits.install = func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error) {
		data, err := os.ReadFile(downloaded)
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, kit.ID, kit.Program)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, os.WriteFile(path, data, 0o755)
	}

	source := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	if err := a.Enqueue(source); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	id := a.library.Jobs[0].ID
	if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailOutdated {
		t.Fatalf("job %+v", job)
	}
	systemYTDLP(a, a.pipeline.(media.Tools).YTDLP)
	film, _ := a.JobFilm(id)
	a.RunRemedy(film, remedyDownloadYTDLP)
	queueUntilState(t, updates, func() bool { return !a.queue.ytdlp.kitRetry && !a.queue.ytdlp.updating })
	want := filepath.Join(own, kit.ID, "yt-dlp")
	if a.pipeline.(media.Tools).YTDLP != want {
		t.Fatalf("the pipeline downloads with %q, want %q", a.pipeline.(media.Tools).YTDLP, want)
	}
	if y := a.queue.ytdlp; y.path != want || y.version != "2026.11.03" || y.install.Kind != "standalone" || y.install.Update == nil {
		t.Fatalf("the queue knows yt-dlp as %+v", y)
	}
	if want := "下载组件已更新到 2026.11.03，已重新开始下载 1 个视频。"; a.lib.notice.Text != want {
		t.Fatalf("notice %q, want %q", a.lib.notice.Text, want)
	}
	settle(t, a, tt, updates)
	data, _ := os.ReadFile(args)
	if got := strings.Split(strings.TrimSpace(string(data)), "\n"); len(got) < 2 || got[0] != "-U" || strings.Count(string(data), source) != 1 || strings.Count(string(data), "-U") != 1 {
		t.Fatalf("Kando's yt-dlp ran with\n%s", data)
	}
}

func TestOpenSystemSettingsForSafarisLogin(t *testing.T) {
	var opened string
	old := openExternal
	openExternal = func(_ *ns.Scope, url string, done func(bool)) {
		opened = url
		done(true)
	}
	t.Cleanup(func() { openExternal = old })
	a, _, _ := newTestApp(t, &fakePipeline{})
	film := failedJob(t, a, "https://www.youtube.com/watch?v=Tq92D6wQ1mg", core.FailCookies, "safari")
	a.RunRemedy(film, remedyDiskAccess)
	if opened != fullDiskAccess {
		t.Fatalf("opened %q", opened)
	}
}

// failedWith is a failed card of the open library whose job's error is err, as media.Run wraps it.
func failedWith(t *testing.T, a *app, err string) films.Film {
	t.Helper()
	job := core.Job{ID: "f", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, Err: media.ReadStageError(err), LibraryDir: a.snap().LibraryDir}
	a.library.Jobs = []core.Job{job}
	film, ok := a.JobFilm(job.ID)
	if !ok {
		t.Fatal("no card")
	}
	return film
}

// A job a translation source of its own failed (media.Tools.Engine) says which source failed, with what it said, and opens no AI service setting, which is not where that source is set up, even for a refused key.
func TestAFailedCardNamesTheTranslationSource(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	for _, said := range []string{"456 Quota Exceeded", "translation API returned 401 Unauthorized: invalid key"} {
		job := core.Job{ID: "f", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, LibraryDir: a.snap().LibraryDir,
			Err: &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: errors.New(said)}}
		a.library.Jobs = []core.Job{job}
		film, ok := a.JobFilm(job.ID)
		if !ok {
			t.Fatal("no card")
		}
		if reason, section := a.FailureHint(film); reason != "翻译来源「DeepL」失败："+said || section != "" {
			t.Errorf("%q: %q %q", said, reason, section)
		}
	}
	refresh(tt, a)
	wantText(t, tt, "翻译来源「DeepL」失败：translation API returned 401 Unauthorized: invalid key")
	if shows(tt, "打开设置") {
		t.Fatal("the card opens the AI service's settings for a source of its own")
	}
}

// A done film a translation source of its own left untranslated says which source failed, and opens no AI service setting; one the AI service left untranslated says why as before.
func TestAFilmNotTranslatedNamesTheTranslationSource(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	job := core.Job{ID: "d", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageComplete, LibraryDir: a.snap().LibraryDir, Untranslated: 3,
		TranslationErr: "translation API returned 401 Unauthorized: invalid key", TranslationSource: "DeepL"}
	a.library.Jobs = []core.Job{job}
	film, ok := a.JobFilm(job.ID)
	if !ok {
		t.Fatal("no card")
	}
	if reason, section := a.TranslationHint(film); reason != "翻译来源「DeepL」失败：translation API returned 401 Unauthorized: invalid key" || section != "" {
		t.Fatalf("%q %q", reason, section)
	}
	job.TranslationSource = ""
	a.library.Jobs = []core.Job{job}
	film, _ = a.JobFilm(job.ID)
	if reason, section := a.TranslationHint(film); reason != "AI 服务不接受这个 Key，去「AI 服务」里检查一下。" || section != settings.SectionAI {
		t.Fatalf("the AI service's: %q %q", reason, section)
	}
}

// A failed card says why in plain Chinese, never the pipeline's English: what went wrong with the translation, and 打开设置 to 「AI 服务」 when a setting there fixes it; and for a failure no reason is known for, the stage that failed with what the tool said, without the tool's own prefixes.
func TestAFailedCardSaysWhyInChinese(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
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
		{"translate: " + media.ErrNoTranscript.Error(), "这部影片的原文字幕不见了，没法单独重新翻译。请重新处理整部影片。", ""},
		{"transcribe: " + track.ErrNewer.Error(), "这部影片的字幕是新版看懂生成的。请更新看懂后再处理。", ""},
		{"transcribe: " + media.ErrNoLength.Error(), "看懂读不出这段视频有多长，没法生成字幕。文件可能已损坏，或不是视频。", ""},
		{"translate: " + media.ErrSubtitlesChanged.Error(), "这部影片的字幕在看懂生成后被改过，找不到哪些句子没译。重新翻译可以重译每一句。", ""},
		{"boom", "处理失败：boom", ""},
	} {
		film := failedWith(t, a, tc.err)
		reason, section := a.FailureHint(film)
		if reason != tc.reason || section != tc.section {
			t.Errorf("%q: %q %q, want %q %q", tc.err, reason, section, tc.reason, tc.section)
		}
	}
	failedWith(t, a, "translate: translation API returned 401 Unauthorized: invalid key")
	refresh(tt, a)
	for _, want := range []string{"AI 服务不接受这个 Key，去「AI 服务」里检查一下。", "打开设置", "重试"} {
		if !shows(tt, want) {
			t.Fatalf("the card does not say %q: %q", want, tt.Texts())
		}
	}
	tt.Press("打开设置")
	wantSettings(t, a, settings.SectionAI)
}

// A film whose translation left some lines untranslated is on the shelf as done, and its card and the hero say how many lines are missing.
func TestADoneFilmSaysWhichLinesAreUntranslated(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	video := filepath.Join(a.snap().LibraryDir, "Talk", "video.mp4")
	a.library.Jobs = []core.Job{{ID: "d", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: strings.TrimSuffix(video, ".mp4") + ".bilingual.srt", Untranslated: 12, LibraryDir: a.snap().LibraryDir}}
	refresh(tt, a)
	// The poster's line, and the hero's, as the film is the one done last.
	if textCount(tt, "YouTube · 12 句翻译失败，其余已完成") != 2 {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// A job stopped because another library was opened did not fail: its card says it was stopped, as statusText words it, not 处理失败 in red over that.
func TestAStoppedCardSaysItWasStopped(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
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

// In English, a failed download's card, its remedies, a failed job's reason and the sidebar's status line read English, with each sentence whole: the site, the browser and the version in their places, and the reason read back from a status line said in English.
func TestFailedCardsInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	yt, bili := "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd", "https://www.bilibili.com/video/BV1Hx411c7Pq"
	a.queue.ytdlp.path, a.queue.ytdlp.version = "/usr/local/bin/yt-dlp", "2024.03.10"
	a.queue.ytdlp.install = media.YTDLPInstall{Kind: "standalone", Dir: "/usr/local/bin"}
	withBrowsers(t, a, "safari")
	for _, tc := range []struct {
		source, cookies string
		kind            core.FailKind
		headline, why   string
		actions         []string
	}{
		{yt, "", core.FailOutdated, "YouTube has changed, and the download component needs an update", "This computer's yt-dlp is version 2024.03.10, and Kando can't update it: yt-dlp is in /usr/local/bin, where Kando isn't allowed to replace it; update it the way you installed it, or run sudo yt-dlp -U. Retry once it's updated.", []string{"Updated, Retry"}},
		{bili, "safari", core.FailLoginExpired, "", "Sign in to Bilibili again in Safari, then come back and retry.", []string{"Signed In Again, Retry"}},
		{yt, "", core.FailAge, "Age verification required", "Use the YouTube account you're signed in to in your browser.", []string{"Use Safari Sign-In and Retry"}},
		{"https://example.com/", "", core.FailUnsupported, "example.com isn't supported", "No video was found on the page. Use another link, or download it to this computer first and drag it in.", []string{"Use Another Link", "Choose Local File", "See Supported Websites"}},
	} {
		film := failedJob(t, a, tc.source, tc.kind, tc.cookies)
		r, ok := a.FailureRemedy(film)
		if !ok || (tc.headline != "" && r.Headline != tc.headline) || r.Why != tc.why || strings.Join(remedyLabels(r), ",") != strings.Join(tc.actions, ",") {
			t.Errorf("%s: %+v", tc.kind, r)
		}
		a.remove(film.JobID)
	}
	withBrowsers(t, a)
	film := failedJob(t, a, bili, core.FailMembers, "")
	if r, _ := a.FailureRemedy(film); r.Headline != "Video for Bilibili Premium members only" || r.Why != "It needs Bilibili Premium. Couldn't find a browser on this computer whose sign-in Kando can use. Sign in with Chrome, Firefox, Edge, Brave or Chromium, then look again." || strings.Join(remedyLabels(r), ",") != "Find Browsers Again,Use Another Link" {
		t.Errorf("members: %+v", r)
	}
	a.remove(film.JobID)

	failedJob(t, a, yt, core.FailRegion, "")
	refresh(tt, a)
	for _, want := range []string{"Not available in your region", "The uploader restricted where it can be watched. If the video is available elsewhere, use another link.", "Use Another Link"} {
		wantText(t, tt, want)
	}
	if shows(tt, "你所在的地区不能看") || shows(tt, "换个链接") {
		t.Fatal("the card still reads Chinese")
	}

	for err, want := range map[string]string{
		"download: yt-dlp: exit status 1: ERROR: Something else":               "Download failed: Something else",
		"translate: translation API returned 401 Unauthorized: invalid key":    "The AI service doesn't accept this key. Check it in “AI Service”.",
		"transcribe: whisper-cli: signal: killed: whisper_init: out of memory": "Generating subtitles failed: whisper_init: out of memory",
	} {
		if reason, _ := a.FailureHint(failedWith(t, a, err)); reason != want {
			t.Errorf("%q: %q, want %q", err, reason, want)
		}
	}

	// The sidebar's status line, found while the app spoke Chinese, says it in the language shown when it is drawn.
	a.SetInterfaceLanguage("zh-Hans")
	a.checkReadiness(false)
	refresh(tt, a)
	if r := a.readiness(); r.Text(a.cx) != "已就绪，可以开始处理" {
		t.Fatalf("status line %q", r.Text(a.cx))
	}
	a.SetInterfaceLanguage("en")
	refresh(tt, a)
	if r := a.readiness(); r.Text(a.cx) != "Ready to process" {
		t.Fatalf("status line %q", r.Text(a.cx))
	}
}

// TestAddingAVideoTwiceQueuesItOnce adds a video already waiting in the queue again, by another link to it: no second card is made, which would download into the same folder, and the message bar says it is already queued. The same video twice among the links added at once makes one card too.
func TestAddingAVideoTwiceQueuesItOnce(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{block: true})
	t.Cleanup(func() {
		for _, job := range a.library.Jobs {
			if job.ID != a.queue.active {
				a.Cancel(job.ID)
			}
		}
		a.stop()
		queueUntilState(t, updates, func() bool { return !a.running() })
	})
	if err := a.Enqueue("https://www.youtube.com/watch?v=TRWgBxVAb1M", "https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	ids := jobIDs(a)
	for _, again := range []string{"https://youtu.be/TRWgBxVAb1M?t=9", "https://www.youtube.com/watch?v=TRWgBxVAb1M&list=PLx0sYbCqOb8", " https://example.com/v "} {
		err := a.Enqueue(again)
		var dup *queueDuplicate
		if !errors.As(err, &dup) || err.Error() != "这个视频已在处理队列中。" || len(a.library.Jobs) != 2 {
			t.Fatalf("adding %q again: err %v, jobs %+v", again, err, a.library.Jobs)
		}
		if want := ids[strings.TrimSpace(again)]; want != "" && dup.ID != want {
			t.Fatalf("adding %q names job %s, want %s", again, dup.ID, want)
		}
	}

	startJob(t, tt, "https://www.youtube.com/shorts/TRWgBxVAb1M")
	refresh(tt, a)
	if len(a.library.Jobs) != 2 || !shows(tt, "这个视频已在处理队列中。") || a.queue.source == "" {
		t.Fatalf("jobs %d, source %q, texts %q", len(a.library.Jobs), a.queue.source, tt.Texts())
	}

	if err := a.Enqueue("https://www.bilibili.com/video/BV1GJ411x7h7", "https://www.bilibili.com/video/BV1GJ411x7h7?p=1", "https://b23.tv/xYz12Ab", "https://b23.tv/xYz12Ab"); err != nil {
		t.Fatalf("the same video twice among links added at once: %v", err)
	}
	if len(a.library.Jobs) != 4 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	// Another part of a video in several parts is another video.
	if err := a.Enqueue("https://www.bilibili.com/video/BV1GJ411x7h7?p=2"); err != nil || len(a.library.Jobs) != 5 {
		t.Fatalf("part 2: err %v, jobs %d", err, len(a.library.Jobs))
	}
}

// TestAddingAVideoInTheLibraryOffersToOpenIt adds a link to a video the library has already processed: nothing is queued, and the message bar says the video is there with 打开, which opens its film.
func TestAddingAVideoInTheLibraryOffersToOpenIt(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{{
		ID: "done", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Title: "Colibri", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: testVideos + "/Colibri [TRWgBxVAb1M]/video.mp4", SubtitlePath: testVideos + "/Colibri [TRWgBxVAb1M]/video.bilingual.srt", LibraryDir: testVideos, CompletedAt: time.Now(),
	}}
	startJob(t, tt, "https://youtu.be/TRWgBxVAb1M")
	refresh(tt, a)
	if len(a.library.Jobs) != 1 || a.lib.notice.Film != "done" || !shows(tt, "这个视频已在资料库中。") {
		t.Fatalf("jobs %+v, notice %+v, texts %q", a.library.Jobs, a.lib.notice, tt.Texts())
	}
	tt.Press("打开")
	if a.openKey != "done" || a.lib.notice.Open {
		t.Fatalf("open %q, notice %+v", a.openKey, a.lib.notice)
	}

	// Once the film it names is removed, the bar still showing has nothing to open, and no 打开.
	a.closeFilm()
	a.libraryEnqueue("https://youtu.be/TRWgBxVAb1M")
	refresh(tt, a)
	if !slices.Contains(tt.Texts(), "打开") {
		t.Fatalf("no 打开 before the film is removed; texts %q", tt.Texts())
	}
	a.remove("done")
	refresh(tt, a)
	if !a.lib.notice.Open || slices.Contains(tt.Texts(), "打开") {
		t.Fatalf("notice %+v, texts %q", a.lib.notice, tt.Texts())
	}

	// A message about something else has no 打开.
	a.warn("别的消息", "")
	refresh(tt, a)
	if slices.Contains(tt.Texts(), "打开") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestAddingAFailedVideoAgainRetriesIt adds a link whose earlier job failed, or was cancelled: that job runs again on its card, rather than a second card being made beside it. A job of the same video in another library is not this library's, and the video is added as a new job.
func TestAddingAFailedVideoAgainRetriesIt(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "failed", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文", Stage: core.StageFailed, Err: media.ReadStageError("download: boom"), LibraryDir: testVideos},
		{ID: "cancelled", Source: "file:///Users/me/Movies/clip.mp4", Language: "简体中文", Stage: core.StageFailed, Err: context.Canceled, LibraryDir: testVideos},
		{ID: "elsewhere", Source: "https://vimeo.com/76979871", Language: "简体中文", Stage: core.StageFailed, Err: media.ReadStageError("download: boom"), LibraryDir: "/other"},
	}
	startJob(t, tt, "https://youtu.be/TRWgBxVAb1M")
	refresh(tt, a)
	if len(a.library.Jobs) != 3 || !shows(tt, "已加入，开始处理。") {
		t.Fatalf("jobs %+v, texts %q", a.library.Jobs, tt.Texts())
	}
	if job := stageOf(t, a, "failed"); job.Stage == core.StageFailed || job.Err != nil {
		t.Fatalf("failed job after adding it again %+v", job)
	}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, "failed"); job.Stage != core.StageComplete {
		t.Fatalf("failed job %+v", job)
	}

	// Queued again from another page, such as the YouTube filter whose top bar adds links too, the window stays on that page.
	a.page = pageYouTube
	if err := a.Enqueue("file:///Users/me/Movies/../Movies/clip.mp4", "https://vimeo.com/76979871"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageYouTube {
		t.Fatalf("page %q after adding a cancelled video again, want the YouTube page", a.page)
	}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, "cancelled"); job.Stage != core.StageComplete || len(a.library.Jobs) != 4 {
		t.Fatalf("cancelled job %+v, jobs %+v", job, a.library.Jobs)
	}
	if job := stageOf(t, a, "elsewhere"); job.Stage != core.StageFailed {
		t.Fatalf("the other library's job ran: %+v", job)
	}
}

// TestQuitStopsTheRunningJob checks that quitting stops the running job and waits for its pipeline to return, without marking it 已取消 or starting the next one, so the next launch runs it again from library.json.
func TestQuitStopsTheRunningJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	g := newGatePipeline()
	a, _, updates := newTestApp(t, g)
	a.openLibrary(path)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	id := jobIDs(a)["https://example.com/a"]
	queueUntilState(t, updates, func() bool { return stageOf(t, a, id).Stage == core.StageDownloading })
	done := a.queue.done

	start := time.Now()
	a.areasQuitting()
	if waited := time.Since(start); waited >= queueQuitWait {
		t.Fatalf("quitting waited %v for a job that stops at once", waited)
	}
	select {
	case <-done:
	default:
		t.Fatal("quitting returned before the job's pipeline did")
	}
	noneStarted(t, g, updates)
	if len(updates) > 0 {
		t.Fatalf("the stopped job still reported %d updates", len(updates))
	}
	if job := stageOf(t, a, id); job.Stage != core.StageDownloading || job.Err != nil {
		t.Fatalf("job stopped by quitting ended %+v", job)
	}
	a.pump()
	noneStarted(t, g, updates)

	reloaded, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range reloaded.Jobs {
		if job.Stage != core.StageQueued || job.Err != nil {
			t.Fatalf("next launch reads %+v, want it queued again", job)
		}
	}
}

// TestQuitWaitsOnlySoLongForTheJob checks that a job whose pipeline does not return when asked to stop holds the quit back no longer than queueQuitWait.
func TestQuitWaitsOnlySoLongForTheJob(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	stopped := false
	a.queue.active, a.queue.done = "stuck", stuck
	a.queue.cancel = func(cause error) { stopped = errors.Is(cause, errQuitting) }
	start := time.Now()
	a.areasQuitting()
	if waited := time.Since(start); !stopped || waited < queueQuitWait || waited > queueQuitWait+time.Second {
		t.Fatalf("stopped %v, waited %v, want %v", stopped, waited, queueQuitWait)
	}
}

// TestQuitWhileAJobRunsAsksFirst checks that ⌘Q goes ahead at once while no job runs, and while one runs is held back by a confirmation, which keeps the app running on 取消 or Escape and quits on 退出 without asking again.
func TestQuitWhileAJobRunsAsksFirst(t *testing.T) {
	idle, _, _ := newTestApp(t, &fakePipeline{})
	e := &quitEvent{}
	idle.areasBeforeQuit(e)
	if e.DefaultPrevented() || idle.queue.quitAsk || !idle.window.quitting {
		t.Fatalf("idle quit prevented %v, asked %v, going ahead %v", e.DefaultPrevented(), idle.queue.quitAsk, idle.window.quitting)
	}

	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	a.window.hidden = true
	refresh(tt, a)
	title := testlocale.Messages().QueueQuitWhileRunningTitle()
	message := testlocale.Messages().QueueQuitWhileRunningMessage(jobName(stageOf(t, a, a.RunningJob())))

	ask := func() {
		t.Helper()
		e := &quitEvent{}
		a.areasBeforeQuit(e)
		refresh(tt, a)
		if !e.DefaultPrevented() || a.window.quitting {
			t.Fatalf("quit during a job: prevented %v, going ahead %v", e.DefaultPrevented(), a.window.quitting)
		}
		if !shows(tt, title) || !shows(tt, message) || !a.modalOpen() || a.window.hidden {
			t.Fatalf("confirmation shows %v, modal %v, window hidden %v; texts %q", shows(tt, title), a.modalOpen(), a.window.hidden, tt.Texts())
		}
	}
	ask()
	// A second ⌘Q while the confirmation is up neither stacks another nor quits.
	ask()
	if a.overlay.modal != 1 || tt.Quits != 0 {
		t.Fatalf("modal %d, quits %d after asking twice", a.overlay.modal, tt.Quits)
	}

	tt.Key("escape")
	refresh(tt, a)
	if shows(tt, title) || a.modalOpen() || tt.Quits != 0 || !a.running() {
		t.Fatalf("after Escape: shown %v, modal %v, quits %d, running %v", shows(tt, title), a.modalOpen(), tt.Quits, a.running())
	}

	ask()
	tt.Press(testlocale.Messages().QueueQuitWhileRunningQuit())
	refresh(tt, a)
	if tt.Quits != 1 || a.modalOpen() {
		t.Fatalf("after 退出: quits %d, modal %v", tt.Quits, a.modalOpen())
	}
	e = &quitEvent{}
	a.areasBeforeQuit(e)
	if e.DefaultPrevented() || !a.window.quitting {
		t.Fatalf("the confirmed quit was held back: prevented %v, going ahead %v", e.DefaultPrevented(), a.window.quitting)
	}
}

// TestQuitConfirmationFollowsTheRunningJob checks that the confirmation names the job running now, the next one once the job it opened for has finished, and closes by itself once no job runs any more, the quit still held back.
func TestQuitConfirmationFollowsTheRunningJob(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	message := func(source string) string {
		return testlocale.Messages().QueueQuitWhileRunningMessage(jobName(stageOf(t, a, ids[source])))
	}
	e := &quitEvent{}
	a.areasBeforeQuit(e)
	refresh(tt, a)
	if !e.DefaultPrevented() || !shows(tt, message("https://example.com/a")) {
		t.Fatalf("quit prevented %v; texts %q", e.DefaultPrevented(), tt.Texts())
	}

	g.release <- struct{}{}
	nextStarted(t, g, updates)
	queueUntilState(t, updates, func() bool { return a.RunningJob() == ids["https://example.com/b"] })
	refresh(tt, a)
	if !shows(tt, message("https://example.com/b")) || shows(tt, message("https://example.com/a")) {
		t.Fatalf("confirmation after the first job finished; texts %q", tt.Texts())
	}

	g.release <- struct{}{}
	queueUntilState(t, updates, func() bool { return !a.running() })
	refresh(tt, a)
	refresh(tt, a)
	if shows(tt, testlocale.Messages().QueueQuitWhileRunningTitle()) || a.modalOpen() || tt.Quits != 0 {
		t.Fatalf("with no job left: shown %v, modal %v, quits %d", shows(tt, testlocale.Messages().QueueQuitWhileRunningTitle()), a.modalOpen(), tt.Quits)
	}
}

// TestQuitOnASignalDoesNotAsk checks that a quit a signal asked for goes ahead while a job runs, whether a quit the app heard first, as the platform's from the same signal, opened the confirmation or not.
func TestQuitOnASignalDoesNotAsk(t *testing.T) {
	for _, heardFirst := range []bool{false, true} {
		g := newGatePipeline()
		a, tt, updates := newTestApp(t, g)
		t.Cleanup(a.stop)
		if err := a.Enqueue("https://example.com/a"); err != nil {
			t.Fatal(err)
		}
		nextStarted(t, g, updates)
		if heardFirst {
			e := &quitEvent{}
			a.areasBeforeQuit(e)
			refresh(tt, a)
			if !e.DefaultPrevented() {
				t.Fatal("the quit the app heard from the signal went ahead before the signal was seen")
			}
		}
		a.queueQuitOnSignal()
		// nsgotest counts a quit once the engine has sent it to the platform
		tt.Settle()
		if tt.Quits != 1 {
			t.Fatalf("heard first %v: the signal asked for %d quits, want 1", heardFirst, tt.Quits)
		}
		e := &quitEvent{}
		a.areasBeforeQuit(e)
		if e.DefaultPrevented() || !a.window.quitting {
			t.Fatalf("heard first %v: the signal's quit was held back: prevented %v, going ahead %v", heardFirst, e.DefaultPrevented(), a.window.quitting)
		}
	}
}

// TestSessionEndDoesNotAsk checks that a logout, restart or shutdown on macOS and Windows (quitEvent.SessionEnding, quitHeard) goes ahead while a job runs without showing the main window or opening the confirmation, lets the window close without the close button's own handling, and stops the job as the app quits; also when a ⌘Q had already opened the confirmation.
func TestSessionEndDoesNotAsk(t *testing.T) {
	for _, hides := range []bool{true, false} {
		for _, asked := range []bool{false, true} {
			setWindowHidesOnClose(t, hides)
			g := newGatePipeline()
			a, tt, updates := newTestApp(t, g)
			t.Cleanup(a.stop)
			if err := a.Enqueue("https://example.com/a"); err != nil {
				t.Fatal(err)
			}
			nextStarted(t, g, updates)
			if asked {
				a.areasBeforeQuit(&quitEvent{})
				refresh(tt, a)
				if !a.queue.quitAsk {
					t.Fatalf("hides %v: ⌘Q during a job did not ask", hides)
				}
			}
			a.window.hidden = !asked
			modal := a.overlay.modal

			e := &quitEvent{SessionEnding: true}
			a.areasBeforeQuit(e)
			refresh(tt, a)
			if e.DefaultPrevented() || !a.window.quitting {
				t.Fatalf("hides %v, asked %v: the session's end was held back: prevented %v, going ahead %v", hides, asked, e.DefaultPrevented(), a.window.quitting)
			}
			if !asked && (a.queue.quitAsk || a.modalOpen() || !a.window.hidden || shows(tt, testlocale.Messages().QueueQuitWhileRunningTitle())) {
				t.Fatalf("hides %v: the session's end asked %v, modal %v, window hidden %v", hides, a.queue.quitAsk, a.modalOpen(), a.window.hidden)
			}
			if a.overlay.modal != modal {
				t.Fatalf("hides %v, asked %v: modal %d after the session's end, %d before", hides, asked, a.overlay.modal, modal)
			}

			ce := &ns.WindowClosing{}
			a.windowClosing(ce)
			if windowClosingCanceled(ce) || tt.Quits != 0 || a.window.hidden != !asked {
				t.Fatalf("hides %v, asked %v: closing at the session's end prevented %v, quits %d, window hidden %v", hides, asked, windowClosingCanceled(ce), tt.Quits, a.window.hidden)
			}

			done := a.queue.done
			a.areasQuitting()
			select {
			case <-done:
			default:
				t.Fatalf("hides %v, asked %v: the session's end left the job running", hides, asked)
			}
		}
	}
}

// resumePipeline downloads into its own folder, counting each step it runs; its first translation waits until the job is stopped, telling translating when it starts.
type resumePipeline struct {
	dir                                string
	downloads, transcribes, translates atomic.Int32
	translating                        chan struct{}
}

func (p *resumePipeline) Check() error { return nil }

func (p *resumePipeline) Download(context.Context, string) (string, error) {
	p.downloads.Add(1)
	video := filepath.Join(p.dir, "Colibri [TRWgBxVAb1M]", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		return "", err
	}
	return video, os.WriteFile(video, []byte("video"), 0o644)
}

func (p *resumePipeline) Length(context.Context, string) time.Duration { return 30 * time.Second }

func (p *resumePipeline) Hear(_ context.Context, _ string, from, _ time.Duration, heard func(core.Segment)) (string, error) {
	p.transcribes.Add(1)
	if from == 0 {
		heard(core.Segment{End: time.Second, Text: "hello"})
	}
	return "en", nil
}

func (p *resumePipeline) Translate(ctx context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	if p.translates.Add(1) == 1 {
		close(p.translating)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return segments, nil
}

// settleStages applies background updates until no job runs and returns each stage job id showed on the shelf, starting with the one it is in now.
func settleStages(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id string) []core.Stage {
	t.Helper()
	stages := []core.Stage{stageOf(t, a, id).Stage}
	timeout := time.After(5 * time.Second)
	for a.running() {
		select {
		case fn := <-updates:
			fn()
			if stage := stageOf(t, a, id).Stage; stage != stages[len(stages)-1] {
				stages = append(stages, stage)
			}
		case <-timeout:
			t.Fatalf("job still running, stages %v", stages)
		}
	}
	refresh(tt, a)
	return stages
}

// A job stopped once it heard its film is retried from its translation: the video it downloaded and the speech it heard are used again. Processing the done film again hears it anew but does not download it again.
func TestRetryResumesAfterTheStagesItFinished(t *testing.T) {
	p := &resumePipeline{dir: t.TempDir(), translating: make(chan struct{})}
	a, tt, updates := newTestApp(t, p)
	if err := a.Enqueue("https://www.youtube.com/watch?v=TRWgBxVAb1M"); err != nil {
		t.Fatal(err)
	}
	id := a.library.Jobs[0].ID
	for waiting := true; waiting; {
		select {
		case <-p.translating:
			waiting = false
		case fn := <-updates:
			apply(tt, a, fn)
		case <-time.After(5 * time.Second):
			t.Fatal("the job never translated")
		}
	}
	a.Cancel(id)
	settleStages(t, a, tt, updates, id)
	failed := stageOf(t, a, id)
	if failed.Stage != core.StageFailed || failed.VideoPath == "" {
		t.Fatalf("first run %+v", failed)
	}

	a.Retry(id)
	stages := settleStages(t, a, tt, updates, id)
	if want := []core.Stage{core.StageQueued, core.StageTranslating, core.StageComplete}; !slices.Equal(stages, want) {
		t.Fatalf("retry stages %v, want %v", stages, want)
	}
	// A link downloads its sound, then its video beside its making (media.Make): twice, the first run only.
	if p.downloads.Load() != 2 || p.transcribes.Load() != 1 {
		t.Fatalf("downloaded %d times, transcribed %d times", p.downloads.Load(), p.transcribes.Load())
	}

	done := stageOf(t, a, id)
	a.Reprocess(films.Film{Key: id, JobID: id, VideoPath: done.VideoPath, SubtitlePath: done.SubtitlePath})
	stages = settleStages(t, a, tt, updates, id)
	if want := []core.Stage{core.StageQueued, core.StageTranscribing, core.StageTranslating, core.StageComplete}; !slices.Equal(stages, want) {
		t.Fatalf("reprocess stages %v, want %v", stages, want)
	}
	if p.downloads.Load() != 2 || p.transcribes.Load() != 2 {
		t.Fatalf("downloaded %d times, transcribed %d times", p.downloads.Load(), p.transcribes.Load())
	}
}

// fakeDesktop stands for the system outside the window that the queue tells about its jobs (queueDesktop): it records the notifications, the badges and the progress bars, in the order the queue gave them, and is in front while front is set.
type fakeDesktop struct {
	front   bool
	notes   []fakeNote
	cleared int
	badges  []int
	bars    []queueBar
}

type fakeNote struct{ id, title, body string }

// useFakeDesktop has the queue of a tell f what it would tell the system.
func useFakeDesktop(a *app) *fakeDesktop {
	f := &fakeDesktop{}
	a.queue.sys.desktop = queueDesktop{
		active:   func() bool { return f.front },
		notify:   func(id, title, body string) { f.notes = append(f.notes, fakeNote{id, title, body}) },
		clear:    func() { f.cleared++ },
		badge:    func(n int) { f.badges = append(f.badges, n) },
		progress: func(p queueBar) { f.bars = append(f.bars, p) },
	}
	return f
}

func (f *fakeDesktop) badge() int {
	if len(f.badges) == 0 {
		return 0
	}
	return f.badges[len(f.badges)-1]
}

func (f *fakeDesktop) bar() queueBar {
	if len(f.bars) == 0 {
		return queueBar{}
	}
	return f.bars[len(f.bars)-1]
}

// failTranscribe fails every job as it transcribes, as whisper-cli does with audio it cannot read.
type failTranscribe struct{ fakePipeline }

func (failTranscribe) Hear(context.Context, string, time.Duration, time.Duration, func(core.Segment)) (string, error) {
	return "", errors.New("whisper-cli: exit status 3: failed to read audio")
}

// TestAJobThatEndsInTheBackgroundNotifiesOnce checks that a job done while another app is in front posts one notification, named after the video, which says its subtitles are ready, and that the job being shown again does not post another.
func TestAJobThatEndsInTheBackgroundNotifiesOnce(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	f := useFakeDesktop(a)
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	want := []fakeNote{{job.ID, jobName(job), "双语字幕做好了。"}}
	if !slices.Equal(f.notes, want) {
		t.Fatalf("notifications %+v, want %+v", f.notes, want)
	}
	a.show(job)
	if len(f.notes) != 1 {
		t.Fatalf("showing the done job again notified again: %+v", f.notes)
	}
}

// TestAFailedJobNotifiesWhy checks that a job that failed in the background posts a notification saying what its card says went wrong.
func TestAFailedJobNotifiesWhy(t *testing.T) {
	a, tt, updates := newTestApp(t, &failTranscribe{})
	f := useFakeDesktop(a)
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	if job.Stage != core.StageFailed {
		t.Fatalf("job %+v", job)
	}
	want := []fakeNote{{job.ID, jobName(job), "字幕没能做好。生成字幕失败：failed to read audio"}}
	if !slices.Equal(f.notes, want) {
		t.Fatalf("notifications %+v, want %+v", f.notes, want)
	}
	// The reason is said in the app's interface language (a.messages()), which the running app switches to English as Settings does
	testlocale.Use(t, "en")
	a.SetInterfaceLanguage("en")
	if got := testlocale.Messages().QueueNotifyFailed(a.queueFailReason(job)); got != "Couldn't make the subtitles. Generating subtitles failed: failed to read audio" {
		t.Fatalf("in English: %q", got)
	}
}

// TestNoNotificationWhenItIsNoNews checks the jobs that end without a notification: while Kando is in front, where the window shows it; with notifications turned off in Settings › 通用; for a job the user stopped; and where the system shows none of the app's notifications.
func TestNoNotificationWhenItIsNoNews(t *testing.T) {
	for name, setup := range map[string]func(a *app, f *fakeDesktop){
		"in front":    func(_ *app, f *fakeDesktop) { f.front = true },
		"turned off":  func(a *app, _ *fakeDesktop) { a.SetJobNotifications(false) },
		"unsupported": func(a *app, _ *fakeDesktop) { a.queue.sys.desktop.notify = nil },
	} {
		t.Run(name, func(t *testing.T) {
			a, tt, updates := newTestApp(t, &fakePipeline{})
			f := useFakeDesktop(a)
			setup(a, f)
			if err := a.Enqueue("https://example.com/v"); err != nil {
				t.Fatal(err)
			}
			settle(t, a, tt, updates)
			if a.library.Jobs[0].Stage != core.StageComplete || len(f.notes) != 0 {
				t.Fatalf("job %+v, notifications %+v", a.library.Jobs[0], f.notes)
			}
		})
	}
	t.Run("stopped", func(t *testing.T) {
		g := newGatePipeline()
		a, tt, updates := newTestApp(t, g)
		f := useFakeDesktop(a)
		if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
			t.Fatal(err)
		}
		nextStarted(t, g, updates)
		ids := jobIDs(a)
		a.Cancel(ids["https://example.com/b"])
		a.Cancel(ids["https://example.com/a"])
		settle(t, a, tt, updates)
		if len(f.notes) != 0 {
			t.Fatalf("cancelled jobs notified: %+v", f.notes)
		}
	})
}

// TestTheBadgeAndProgressFollowTheQueue checks that the app's icon counts the jobs still to do, the running one and those waiting, and shows how far the running one has gone in its stage, indeterminate before its tool says, and that both go once the queue is empty.
func TestTheBadgeAndProgressFollowTheQueue(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	f := useFakeDesktop(a)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b", "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	if f.badge() != 3 || f.bar() != (queueBar{Shown: true, Indeterminate: true}) {
		t.Fatalf("while a runs: badge %d, bar %+v", f.badge(), f.bar())
	}
	running := stageOf(t, a, a.RunningJob())
	running.Progress = core.Fraction(0.426)
	a.showProgress(running)
	if want := (queueBar{Shown: true, Value: 0.43}); f.bar() != want {
		t.Fatalf("bar %+v, want %+v", f.bar(), want)
	}
	told := len(f.bars)
	running.Progress = core.Fraction(0.429)
	a.showProgress(running)
	if len(f.bars) != told {
		t.Fatalf("a change of less than a percent redrew the bar: %+v", f.bars)
	}

	a.remove(jobIDs(a)["https://example.com/c"])
	if f.badge() != 2 {
		t.Fatalf("after removing a waiting job: badge %d", f.badge())
	}
	g.release <- struct{}{}
	nextStarted(t, g, updates)
	if f.badge() != 1 {
		t.Fatalf("while b runs: badge %d", f.badge())
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	if f.badge() != 0 || f.bar() != (queueBar{}) {
		t.Fatalf("after the last job: badge %d, bar %+v", f.badge(), f.bar())
	}
	for i := 1; i < len(f.badges); i++ {
		if f.badges[i] == f.badges[i-1] {
			t.Fatalf("the badge was told the same count twice in a row: %v", f.badges)
		}
	}
}

// TestTheIconCountsTheOpenLibraryAndReachesALateDesktop checks that a job waiting in a library the app does not know is left out of the badge, as pump runs it only once that library is opened again (queueCanRun), and that what the queue worked out before it could reach the system, as before the window is ready, is told as soon as it can.
func TestTheIconCountsTheOpenLibraryAndReachesALateDesktop(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	f := useFakeDesktop(a)
	d := a.queue.sys.desktop
	a.queue.sys.desktop = queueDesktop{}
	a.library.Upsert(core.Job{ID: "elsewhere", Source: "https://example.com/elsewhere", Stage: core.StageQueued, LibraryDir: "/libraries/other"})
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	if len(f.badges) != 0 || len(f.bars) != 0 {
		t.Fatalf("told the system before it could reach it: badges %v, bars %+v", f.badges, f.bars)
	}

	a.queueUseDesktop(d)
	if len(f.badges) != 1 || f.badge() != 2 {
		t.Fatalf("badges %v, want [2]: the running job and the one waiting in the open library", f.badges)
	}
	if len(f.bars) != 1 || f.bar() != (queueBar{Shown: true, Indeterminate: true}) {
		t.Fatalf("bars %+v, want one indeterminate bar", f.bars)
	}

	g.release <- struct{}{}
	nextStarted(t, g, updates)
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	if f.badge() != 0 || f.bar() != (queueBar{}) {
		t.Fatalf("with only a forgotten library's job waiting: badge %d, bar %+v", f.badge(), f.bar())
	}
}

// TestComingToTheFrontClearsTheNotifications checks that the queue removes its notifications as the app becomes the active one, and that an app whose queue has no way to the system, as before the window is ready, does nothing then.
func TestComingToTheFrontClearsTheNotifications(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.areasBecameActive()
	f := useFakeDesktop(a)
	a.areasBecameActive()
	if f.cleared != 1 {
		t.Fatalf("cleared %d times", f.cleared)
	}
}

// TestClickingANotificationShowsTheJob checks that a click on the notification of a done job opens its film, one of a failed job shows the library where its card is, and one of a job no longer on the shelf leaves the page as it is.
func TestClickingANotificationShowsTheJob(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	doneJob := a.library.Jobs[0]
	done := doneJob.ID
	failed := failedWith(t, a, "boom")
	a.library.Upsert(doneJob)

	a.page = pageYouTube
	a.queueShowJob(done)
	if a.page != pageFilms || a.openKey != done {
		t.Fatalf("a done job shows page %q, film %q", a.page, a.openKey)
	}
	a.queueShowJob(failed.JobID)
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("a failed job shows page %q, film %q", a.page, a.openKey)
	}
	a.page = pageYouTube
	a.queueShowJob("gone")
	if a.page != pageYouTube {
		t.Fatalf("a job no longer on the shelf shows page %q", a.page)
	}
}

// TestTheNotificationSwitchIsSaved checks the switch in Settings › 通用: it turns the notifications off, settings.json keeps that, and they are on again for an app that never turned them off.
func TestTheNotificationSwitchIsSaved(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if !a.JobNotifications() {
		t.Fatal("notifications are off by default")
	}
	st := settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"影片处理完时通知", "看懂在后台时，影片的字幕做好了或没能做好，都会发一条通知。"} {
		if !shows(st, text) {
			t.Fatalf("no %q: %q", text, st.Texts())
		}
	}
	st.Press("影片处理完时通知")
	if a.JobNotifications() {
		t.Fatal("the switch left the notifications on")
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.Notify == nil || *saved.Notify {
		t.Fatalf("saved %+v", saved.savedQueue)
	}
	b, _, _ := newTestApp(t, &fakePipeline{})
	b.restoreSettings(saved)
	if b.JobNotifications() {
		t.Fatal("restored notifications on")
	}
	c, _, _ := newTestApp(t, &fakePipeline{})
	c.restoreSettings(savedSettings{})
	if !c.JobNotifications() {
		t.Fatal("an app that never turned them off has notifications off")
	}
	a.SetJobNotifications(true)
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	if saved := loadSavedSettings(a.settingsPath); saved.Notify != nil {
		t.Fatalf("notifications on are saved as %v", *saved.Notify)
	}
}

// retranslatePipeline is the pipeline of a done film as the actions that translate it again see it: it counts the downloads and transcriptions, remembers each line Translate is sent and the language, translates a line as the language and the line, and hears one line, "Hello.". With hold, Translate waits until the job is stopped.
type retranslatePipeline struct {
	testFolder
	downloads, transcriptions int
	sent                      []string
	language                  string
	translateErr              error
	hold                      bool
	// video is the film's video, which Download gives for a link; silent hears no speech in it.
	video  string
	silent bool
}

func (p *retranslatePipeline) Check() error { return nil }

func (p *retranslatePipeline) Download(_ context.Context, source string) (string, error) {
	p.downloads++
	if u, err := url.Parse(source); err == nil {
		if path, ok := core.FilePath(u); ok {
			return path, nil
		}
	}
	if p.video == "" {
		return p.testFolder.video("talk.mp4")
	}
	return p.video, nil
}

func (p *retranslatePipeline) Length(context.Context, string) time.Duration { return 30 * time.Second }

func (p *retranslatePipeline) Hear(_ context.Context, _ string, from, _ time.Duration, heard func(core.Segment)) (string, error) {
	p.transcriptions++
	if from == 0 && !p.silent {
		heard(core.Segment{End: time.Second, Text: "Hello."})
	}
	return "", nil
}

func (p *retranslatePipeline) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	p.language = language
	if p.hold {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if p.translateErr != nil {
		return nil, p.translateErr
	}
	out := make([]core.Segment, len(segments))
	for i, s := range segments {
		p.sent = append(p.sent, s.Text)
		out[i] = s
		out[i].Text = language + ":" + s.Text
	}
	return out, nil
}

// retranslateLibrary is a library holding one done film, Talk, whose job translated it into 简体中文 and exported its three lines, the second of which it left untranslated; the settings now name 日本語 as the target language.
func retranslateLibrary(t *testing.T, p *retranslatePipeline) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	return retranslateLibraryAt(t, p, lib, filepath.Join(lib, "Talk", "video.mp4"))
}

// retranslateLibraryAt is retranslateLibrary with the library at lib and Talk's video at video, which may be outside it, as a local video processed where it is.
func retranslateLibraryAt(t *testing.T, p *retranslatePipeline, lib, video string) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	p.video = video
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(video, ".mp4")
	for path, body := range map[string]string{
		video:                   "v",
		base + ".srt":           "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\nBye.\n\n",
		base + ".bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\n再见。\nBye.\n\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.prefs.language = "日本語"
	a.library.Jobs = []core.Job{{
		ID: "talk", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: base + ".bilingual.srt", LibraryDir: lib, Untranslated: 1, CompletedAt: time.Now(),
	}}
	a.refreshLibrary()
	refresh(tt, a)
	return a, tt, updates
}

// chooseFilmMenu chooses label in the ⋯ menu of the film on screen.
func chooseFilmMenu(t *testing.T, tt *nsgotest.App, path ...string) {
	t.Helper()
	tt.Press("id:film-more")
	tt.ChoosePopup(path...)
}

// translateAgainInto chooses 重新翻译为 ▸ language in the ⋯ of the film shown, and confirms replacing the film's translation when it asks.
func translateAgainInto(t *testing.T, a *app, tt *nsgotest.App, language string) {
	t.Helper()
	m := testlocale.Messages()
	tt.Press("id:film-more")
	tt.ChoosePopup(m.FilmsTranslateAgain(), subtitle.TargetLanguageNameIn(m, language))
	if a.lib.retranslateOpen {
		tt.Press("id:retranslate-confirm-run")
	}
}

// The card of a film whose translation failed for some lines offers to translate them: only those lines go to the translator, in the language the film's other lines are in rather than the one set now, and the film is neither downloaded nor transcribed again.
func TestTranslateMissingLinesSendsOnlyThoseLines(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates := retranslateLibrary(t, p)
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "1 句翻译失败，其余已完成") {
		t.Fatalf("the card does not say a line failed: %q", tt.Texts())
	}
	tt.Press("补译未译的句子")
	settle(t, a, tt, updates)
	if !slices.Equal(p.sent, []string{"How are you?"}) || p.language != "简体中文" {
		t.Fatalf("sent %q in %q", p.sent, p.language)
	}
	if p.downloads != 0 || p.transcriptions != 0 {
		t.Fatalf("%d downloads, %d transcriptions", p.downloads, p.transcriptions)
	}
	job := stageOf(t, a, "talk")
	if job.Stage != core.StageComplete || job.Untranslated != 0 || job.Redo != "" || job.Language != "简体中文" {
		t.Fatalf("job %+v", job)
	}
	if shows(tt, "补译未译的句子") {
		t.Fatalf("the button stayed once every line was translated: %q", tt.Texts())
	}
}

// A film whose every line was translated has no button to translate missing lines, nor the menu item.
func TestNoMissingLinesNoButton(t *testing.T) {
	a, tt, _ := retranslateLibrary(t, &retranslatePipeline{})
	job := stageOf(t, a, "talk")
	job.Untranslated = 0
	a.library.Upsert(job)
	refresh(tt, a)
	if shows(tt, "补译未译的句子") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Press("id:film-more")
	if menu := popupLabels(tt); slices.Contains(menu, "补译未译的句子") || !slices.Contains(menu, testlocale.Messages().FilmsTranslateAgain()) {
		t.Fatalf("menu %q", menu)
	}
}

// 重新翻译 translates every line of a done film again into the target language set now, without downloading or transcribing it.
func TestTranslateAgainUsesTheLanguageSetNow(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates := retranslateLibrary(t, p)
	translateAgainInto(t, a, tt, "日本語")
	settle(t, a, tt, updates)
	if !slices.Equal(p.sent, []string{"Hello.", "How are you?", "Bye."}) || p.language != "日本語" {
		t.Fatalf("sent %q in %q", p.sent, p.language)
	}
	if p.downloads != 0 || p.transcriptions != 0 {
		t.Fatalf("%d downloads, %d transcriptions", p.downloads, p.transcriptions)
	}
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Language != "日本語" || job.Redo != "" {
		t.Fatalf("job %+v", job)
	}
}

// 重新处理 processes the entire film again but for its download, which is still beside the job (media.Run), into the target language set now rather than the one it was translated into, once confirmed.
// 重新处理 transcribes a done film again and translates it into the language it was translated into, not the one the settings name now, so transcribing it in the language the user said it is spoken in does not change its translation; the control socket's jobs.retry answers that language. A film that translated none of its lines is translated into the language the settings name.
func TestProcessAgainKeepsTheFilmsLanguage(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates := retranslateLibrary(t, p)
	chooseFilmMenu(t, tt, "重新处理")
	tt.Press("id:reprocess-confirm-run")
	if job := stageOf(t, a, "talk"); job.RedoLanguage != "简体中文" || a.remoteJobOf(job).Language != "简体中文" {
		t.Fatalf("job %+v, answered %+v", job, a.remoteJobOf(job))
	}
	settle(t, a, tt, updates)
	if p.downloads != 0 || p.transcriptions != 1 || p.language != "简体中文" {
		t.Fatalf("%d downloads, %d transcriptions, language %q", p.downloads, p.transcriptions, p.language)
	}
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Language != "简体中文" || job.Redo != "" || job.RedoLanguage != "" {
		t.Fatalf("job %+v", job)
	}

	untranslated := stageOf(t, a, "talk")
	untranslated.TranslationErr = "no translator"
	a.library.Upsert(untranslated)
	film, _ := a.JobFilm("talk")
	a.AskReprocess(film)
	if _, items, language := a.Reprocessing(); len(items) != 1 || items[0].JobID != "talk" || language != "日本語" {
		t.Fatalf("asks about %+v in %q", items, language)
	}
	a.CloseReprocess()
	a.Retry("talk")
	if job := stageOf(t, a, "talk"); job.RedoLanguage != "日本語" || a.remoteJobOf(job).Language != "日本語" {
		t.Fatalf("job %+v", job)
	}
	settle(t, a, tt, updates)
	if p.language != "日本語" || stageOf(t, a, "talk").Language != "日本語" {
		t.Fatalf("translated into %q, job %+v", p.language, stageOf(t, a, "talk"))
	}
}

// 重新处理 asks first, from the library and from the film page, naming the film and the language it would be translated into, since it transcribes the film and pays for its translation again: 取消 queues nothing, and the confirmation holds the window's modal flag only while it is open.
func TestProcessAgainAsksFirst(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates := retranslateLibrary(t, p)
	msgs := testlocale.Messages()
	chooseFilmMenu(t, tt, "重新处理")
	if !hasText(tt, msgs.FilmsReprocessConfirm("Talk")) || !hasText(tt, msgs.FilmsReprocessConfirmMessage(msgs.SettingsLanguageSimplifiedChinese())) || !a.modalOpen() {
		t.Fatalf("no confirmation naming the film and the language: %q", tt.Texts())
	}
	if role := tt.Find("id:reprocess-confirm").Props.Role; role != ns.RoleAlertDialog {
		t.Fatalf("the confirmation's role is %v", role)
	}
	tt.Press("id:reprocess-cancel")
	refresh(tt, a)
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Redo != "" || a.running() || a.modalOpen() || tt.Exists("id:reprocess-confirm") {
		t.Fatalf("取消 left job %+v, running %v, modal %v", job, a.running(), a.modalOpen())
	}

	film, _ := a.JobFilm("talk")
	a.Open(film)
	refresh(tt, a)
	tt.Press("id:film-more")
	tt.ChoosePopup("重新处理整部影片")
	if !tt.Exists("id:reprocess-confirm") || a.running() {
		t.Fatalf("the film page did not ask first: %q", tt.Texts())
	}
	tt.Press("id:reprocess-confirm-run")
	settle(t, a, tt, updates)
	if a.openKey != "talk" || p.transcriptions != 1 || p.language != "简体中文" || a.modalOpen() {
		t.Fatalf("open %q, %d transcriptions in %q, modal %v", a.openKey, p.transcriptions, p.language, a.modalOpen())
	}
}

// reprocessKeptTheFilm checks that Talk of retranslateLibrary, which was being processed again, is the done film it was: in the language it was translated into, on the done grid, with the transcript and bilingual subtitles it had on disk, which the film opens with.
func reprocessKeptTheFilm(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), before core.Job, srt, bilingual string) {
	t.Helper()
	job := stageOf(t, a, "talk")
	if job.Stage != core.StageComplete || job.Err != nil || job.Redo != "" || job.RedoLanguage != "" || job.Language != "简体中文" || job.Untranslated != 1 || !job.CompletedAt.Equal(before.CompletedAt) || job.SubtitlePath != before.SubtitlePath {
		t.Fatalf("job %+v", job)
	}
	for path, want := range map[string]string{strings.TrimSuffix(before.VideoPath, ".mp4") + ".srt": srt, before.SubtitlePath: bilingual} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("%s is now %q, %v", path, data, err)
		}
	}
	if !shows(tt, "补译未译的句子") {
		t.Fatalf("the film is no longer a done card: %q", tt.Texts())
	}
	film, _ := a.JobFilm("talk")
	a.Open(film)
	queueUntilState(t, updates, func() bool { return a.player.subs.ready })
	if cues := a.player.subs.cues; len(cues) != 3 || cues[0].Text != "你好。\nHello." {
		t.Fatalf("cues %q", cues)
	}
}

// readFile is the file at path, failing t when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// 重新处理 that fails leaves the film done and watchable with the subtitles it had, rather than a failed card, and the message bar says why.
func TestAFailedReprocessKeepsTheFilm(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key")}
	a, tt, updates := retranslateLibrary(t, p)
	before := stageOf(t, a, "talk")
	srt, bilingual := readFile(t, strings.TrimSuffix(before.VideoPath, ".mp4")+".srt"), readFile(t, before.SubtitlePath)
	film, _ := a.JobFilm("talk")
	a.Reprocess(film)
	settle(t, a, tt, updates)
	if p.transcriptions != 1 {
		t.Fatalf("%d transcriptions", p.transcriptions)
	}
	if want := testlocale.Messages().QueueReprocessFailed("Talk", testlocale.Messages().PrecheckTranslateAuth()); a.lib.notice.Text != want || a.lib.notice.Section != settings.SectionAI {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
	reprocessKeptTheFilm(t, a, tt, updates, before, srt, bilingual)
}

// Stopping 重新处理 while it runs leaves the film done and watchable with the subtitles it had, with nothing in the message bar.
func TestCancellingAReprocessKeepsTheFilm(t *testing.T) {
	p := &retranslatePipeline{hold: true}
	a, tt, updates := retranslateLibrary(t, p)
	before := stageOf(t, a, "talk")
	srt, bilingual := readFile(t, strings.TrimSuffix(before.VideoPath, ".mp4")+".srt"), readFile(t, before.SubtitlePath)
	film, _ := a.JobFilm("talk")
	a.Reprocess(film)
	if !a.running() {
		t.Fatal("重新处理 did not start")
	}
	queueUntilState(t, updates, func() bool { return stageOf(t, a, "talk").Stage == core.StageTranslating })
	a.Cancel("talk")
	settle(t, a, tt, updates)
	if a.lib.notice.Open {
		t.Fatalf("notice %+v", a.lib.notice)
	}
	reprocessKeptTheFilm(t, a, tt, updates, before, srt, bilingual)
}

// With no target language set, 重新翻译 queues nothing and the message bar says why. The film page draws no message bar, so from there the settings open where the language is chosen.
// 重新翻译为 ▸ lists the target languages, the one set now first and the film's own checked. Translating into the film's own language runs at once; into another, which replaces its translation, it asks first, and 取消 queues nothing. With no language set, the film's own comes first.
func TestTranslateAgainIntoAsksBeforeChangingTheLanguage(t *testing.T) {
	p := &retranslatePipeline{hold: true}
	a, tt, _ := retranslateLibrary(t, p)
	m := testlocale.Messages()
	tt.Press("id:film-more")
	var into []string
	for _, it := range tt.PopupMenu {
		if it.Label == m.FilmsTranslateAgain() {
			for _, sub := range it.Items {
				into = append(into, fmt.Sprintf("%s%v", sub.Label, sub.Checked))
			}
		}
	}
	if len(into) < 3 || into[0] != m.SettingsLanguageJapanese()+"false" || into[1] != m.SettingsLanguageSimplifiedChinese()+"true" {
		t.Fatalf("submenu %q", into)
	}
	tt.ChoosePopup(m.FilmsTranslateAgain(), m.SettingsLanguageJapanese())
	if !a.lib.retranslateOpen || a.running() || !tt.Exists("id:retranslate-confirm") {
		t.Fatalf("another language did not ask: open %v running %v", a.lib.retranslateOpen, a.running())
	}
	tt.Press("id:retranslate-cancel")
	if a.lib.retranslateOpen || a.lib.retranslateRelease != nil || a.running() {
		t.Fatal("取消 queued the translation or kept the modal flag")
	}
	chooseFilmMenu(t, tt, m.FilmsTranslateAgain(), m.SettingsLanguageSimplifiedChinese())
	if a.lib.retranslateOpen || !a.running() {
		t.Fatal("the film's own language asked first")
	}
	if job := stageOf(t, a, "talk"); job.RedoLanguage != "简体中文" {
		t.Fatalf("queued %+v", job)
	}

	a.prefs.language = ""
	film, _ := a.JobFilm("talk")
	film.Language = "日本語"
	cx := a.cx
	if items := subtitle.TranslateIntoMenu(cx, a.TargetLanguage(), film.Language, func(string) {}); items[0].Label != m.SettingsLanguageJapanese() || !items[0].Checked {
		t.Fatalf("with no language set the menu starts with %+v", items[0])
	}
}

// A translation of the missing lines that fails leaves the film done with the subtitles it had, rather than a failed card: the message bar says why, the card still offers to translate the missing lines, and doing so again translates only those, still without downloading or transcribing the film.
func TestAFailedRetranslationKeepsTheFilmDone(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key")}
	a, tt, updates := retranslateLibrary(t, p)
	before := stageOf(t, a, "talk")
	tt.Press("补译未译的句子")
	settle(t, a, tt, updates)
	job := stageOf(t, a, "talk")
	if job.Stage != core.StageComplete || job.Err != nil || job.Redo != "" || job.Language != "简体中文" || job.Untranslated != 1 || !job.CompletedAt.Equal(before.CompletedAt) {
		t.Fatalf("job %+v", job)
	}
	if want := testlocale.Messages().QueueRedoFailed("Talk", testlocale.Messages().PrecheckTranslateAuth()); a.lib.notice.Text != want || a.lib.notice.Section != settings.SectionAI {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
	if !shows(tt, "补译未译的句子") || !shows(tt, "1 句翻译失败，其余已完成") {
		t.Fatalf("the film is no longer a done card: %q", tt.Texts())
	}
	p.translateErr = nil
	film, _ := a.JobFilm("talk")
	a.TranslateMissing(film)
	settle(t, a, tt, updates)
	if !slices.Equal(p.sent, []string{"How are you?"}) || p.downloads != 0 || p.transcriptions != 0 {
		t.Fatalf("sent %q, %d downloads, %d transcriptions", p.sent, p.downloads, p.transcriptions)
	}
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Untranslated != 0 {
		t.Fatalf("job %+v", job)
	}
}

// Stopping 重新翻译, while it runs or while it waits behind another job, puts the film back as it was, in the language it was translated into, with nothing in the message bar: its subtitles were never touched.
func TestCancellingATranslationAgainKeepsTheFilmDone(t *testing.T) {
	p := &retranslatePipeline{hold: true}
	a, tt, updates := retranslateLibrary(t, p)
	translateAgainInto(t, a, tt, "日本語")
	if !a.running() {
		t.Fatal("重新翻译 did not start")
	}
	a.Cancel("talk")
	settle(t, a, tt, updates)
	check := func(when string) {
		t.Helper()
		job := stageOf(t, a, "talk")
		if job.Stage != core.StageComplete || job.Err != nil || job.Redo != "" || job.RedoLanguage != "" || job.Language != "简体中文" || job.Untranslated != 1 {
			t.Fatalf("%s: job %+v", when, job)
		}
		if a.lib.notice.Open {
			t.Fatalf("%s: notice %+v", when, a.lib.notice)
		}
		if !shows(tt, "补译未译的句子") {
			t.Fatalf("%s: the film is no longer a done card: %q", when, tt.Texts())
		}
	}
	check("stopped while running")

	// Another job holds the queue, so 重新翻译 waits behind it and is cancelled from its queued card.
	a.queue.active = "other"
	translateAgainInto(t, a, tt, "日本語")
	if job := stageOf(t, a, "talk"); job.Stage != core.StageQueued || job.RedoLanguage != "日本語" || job.Language != "简体中文" {
		t.Fatalf("queued job %+v", job)
	}
	a.Cancel("talk")
	a.queue.active = ""
	refresh(tt, a)
	check("cancelled while queued")
}

// A film whose translation was redone while it is open shows the new subtitles once the job is done, though the file keeps its path: here a local video processed where it is, outside the library, which the library's watcher does not look at.
func TestAnOpenFilmShowsItsNewTranslation(t *testing.T) {
	p := &retranslatePipeline{}
	root := t.TempDir()
	a, tt, updates := retranslateLibraryAt(t, p, filepath.Join(root, "我的影片.kando"), filepath.Join(root, "Downloads", "Talk", "video.mp4"))
	film, _ := a.JobFilm("talk")
	a.Open(film)
	queueUntilState(t, updates, func() bool { return a.player.subs.ready })
	if cues := a.player.subs.cues; len(cues) != 3 || cues[0].Text != "你好。\nHello." {
		t.Fatalf("cues %q", cues)
	}
	a.TranslateAgain(film, "日本語")
	a.Retranslate()
	settle(t, a, tt, updates)
	queueUntilState(t, updates, func() bool {
		cues := a.player.subs.cues
		return a.player.subs.ready && len(cues) == 3 && cues[0].Text == "日本語:Hello.\nHello."
	})
}

// The film page's ⋯ translates the open film again and the film stays open, following its job.
func TestFilmPageTranslatesAgain(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates := retranslateLibrary(t, p)
	film, _ := a.JobFilm("talk")
	a.Open(film)
	refresh(tt, a)
	tt.Press("id:film-more")
	if menu := popupLabels(tt); !slices.Contains(menu, testlocale.Messages().FilmsTranslateAgain()) || !slices.Contains(menu, "补译未译的句子") {
		t.Fatalf("menu %q", menu)
	}
	tt.ChoosePopup(testlocale.Messages().FilmsTranslateAgain(), testlocale.Messages().SettingsLanguageJapanese())
	tt.Press("id:retranslate-confirm-run")
	settle(t, a, tt, updates)
	if a.openKey != "talk" || p.language != "日本語" || len(p.sent) != 3 || p.downloads != 0 {
		t.Fatalf("open %q, sent %q in %q, %d downloads", a.openKey, p.sent, p.language, p.downloads)
	}
}

// queueUntilState runs the background updates, as the engine would, until done reports true, for a test that reads the app's state rather than the window.
func queueUntilState(t *testing.T, updates <-chan func(), done func() bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatal("timed out waiting for a background update")
		}
	}
}

// An account out of credit is said as such, with 「AI 服务」 to go to, rather than as a busy service or a wrong key.
func TestAFailedRetranslationSaysTheAccountIsOutOfCredit(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New(`translation API returned 402 Payment Required: {"error":{"message":"Insufficient Balance"}}`)}
	a, tt, updates := retranslateLibrary(t, p)
	tt.Press("补译未译的句子")
	settle(t, a, tt, updates)
	if want := testlocale.Messages().QueueRedoFailed("Talk", testlocale.Messages().PrecheckTranslateQuota()); a.lib.notice.Text != want || a.lib.notice.Section != settings.SectionAI {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
}

// Retry All queues again the failures that need no choice of the user's, oldest first, and leaves a login to borrow, a cancelled job and the others to their own cards; Clear Failed takes every failed card off and leaves a cancelled one.
func TestRetryAllAndRemoveFailed(t *testing.T) {
	g := newGatePipeline()
	a, tt, _ := newTestApp(t, g)
	failed := func(id string, kind core.FailKind) core.Job {
		return core.Job{ID: id, Source: "https://example.com/" + id, Language: "简体中文", Stage: core.StageFailed, Err: media.ReadStageError("download: boom"), FailKind: kind, LibraryDir: testVideos}
	}
	stopped := failed("stopped", "")
	stopped.Err = context.Canceled
	// The shelf keeps its jobs newest first.
	a.library.Jobs = []core.Job{failed("net", core.FailNetwork), failed("login", core.FailLogin), stopped, failed("plain", "")}
	refresh(tt, a)
	if r, f := a.Failures(sidebar.SourceAll); r != 2 || f != 3 {
		t.Fatalf("failures %d retryable, %d failed", r, f)
	}
	tt.Press("queue-retry-all")
	if a.RunningJob() != "plain" || stageOf(t, a, "net").Stage != core.StageQueued || stageOf(t, a, "login").Stage != core.StageFailed || stageOf(t, a, "stopped").Stage != core.StageFailed {
		t.Fatalf("after Retry All: running %q, jobs %+v", a.RunningJob(), a.library.Jobs)
	}
	if r, f := a.Failures(sidebar.SourceAll); r != 0 || f != 1 {
		t.Fatalf("failures after Retry All: %d retryable, %d failed", r, f)
	}

	a.library.Jobs = append(a.library.Jobs, failed("other", core.FailRegion))
	refresh(tt, a)
	tt.Press("queue-remove-failed")
	if _, ok := a.library.Find("login"); ok {
		t.Fatal("Clear Failed kept a failed card")
	}
	if _, ok := a.library.Find("other"); ok {
		t.Fatal("Clear Failed kept a failed card")
	}
	if _, ok := a.library.Find("stopped"); !ok {
		t.Fatal("Clear Failed took a cancelled card")
	}
	<-g.started
}

// With a source chosen in the sidebar, Retry All and Clear Failed act only on the failed cards 处理中 shows, those from that source, and leave the others' failures as they are.
func TestRetryAllAndRemoveFailedKeepToTheSourceShown(t *testing.T) {
	g := newGatePipeline()
	a, tt, _ := newTestApp(t, g)
	failed := func(id, source string) core.Job {
		return core.Job{ID: id, Source: source, Language: "简体中文", Stage: core.StageFailed, Err: media.ReadStageError("download: boom"), LibraryDir: testVideos}
	}
	a.library.Jobs = []core.Job{
		failed("yt1", "https://youtu.be/TRWgBxVAb1M"), failed("yt2", "https://youtu.be/dQw4w9WgXcQ"),
		failed("web1", "https://example.com/a"), failed("web2", "https://example.com/b"),
	}
	a.SetSourceFilter(sidebar.SourceYouTube)
	refresh(tt, a)
	if r, f := a.Failures(sidebar.SourceYouTube); r != 2 || f != 2 {
		t.Fatalf("YouTube's failures %d retryable, %d failed, want 2 and 2", r, f)
	}
	tt.Press("queue-retry-all")
	for _, id := range []string{"web1", "web2"} {
		if stageOf(t, a, id).Stage != core.StageFailed {
			t.Fatalf("Retry All on YouTube's page queued %s again: %+v", id, a.library.Jobs)
		}
	}
	if stageOf(t, a, "yt2").Stage != core.StageQueued && a.RunningJob() != "yt2" {
		t.Fatalf("Retry All left YouTube's yt2 failed: %+v", a.library.Jobs)
	}
	<-g.started

	a.library.Jobs = append(a.library.Jobs, failed("yt3", "https://youtu.be/9bZkp7q19f0"), failed("yt4", "https://youtu.be/kJQP7kiw5Fk"))
	refresh(tt, a)
	tt.Press("queue-remove-failed")
	for _, id := range []string{"yt3", "yt4"} {
		if _, ok := a.library.Find(id); ok {
			t.Fatalf("Clear Failed on YouTube's page kept %s", id)
		}
	}
	for _, id := range []string{"web1", "web2"} {
		if _, ok := a.library.Find(id); !ok {
			t.Fatalf("Clear Failed on YouTube's page took %s from another source", id)
		}
	}
}

// The × of a waiting card takes a job that has done nothing yet off the shelf, and cancels one that has, as a film waiting to be processed again, which keeps its card.
func TestWithdrawRemovesOnlyAFreshWaitingJob(t *testing.T) {
	g := newGatePipeline()
	a, _, _ := newTestApp(t, g)
	for _, source := range []string{"https://example.com/run", "https://example.com/fresh"} {
		if err := a.Enqueue(source); err != nil {
			t.Fatal(err)
		}
	}
	<-g.started
	ids := jobIDs(a)
	again := core.Job{ID: "again", Source: "https://example.com/again", Language: "简体中文", Stage: core.StageComplete, VideoPath: testVideos + "/again.mp4", SubtitlePath: testVideos + "/again.bilingual.srt", CompletedAt: time.Now(), LibraryDir: testVideos}
	a.library.Upsert(again)
	a.Retry("again")
	a.Withdraw(ids["https://example.com/fresh"])
	if _, ok := a.library.Find(ids["https://example.com/fresh"]); ok {
		t.Fatal("× kept a waiting job that had done nothing")
	}
	a.Withdraw("again")
	if job := stageOf(t, a, "again"); job.Stage != core.StageComplete {
		t.Fatalf("× on a film waiting to be processed again left %+v", job)
	}
	a.Withdraw(ids["https://example.com/run"])
	if _, ok := a.library.Find(ids["https://example.com/run"]); !ok {
		t.Fatal("× removed the running job")
	}
}

// A waiting card says how many jobs run before it: the running one and those queued before it.
func TestWaitingCardsCountTheJobsAhead(t *testing.T) {
	g := newGatePipeline()
	a, _, _ := newTestApp(t, g)
	for _, source := range []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"} {
		if err := a.Enqueue(source); err != nil {
			t.Fatal(err)
		}
	}
	<-g.started
	ids := jobIDs(a)
	ahead := map[string]int{}
	for _, item := range a.QueueFilms() {
		ahead[item.JobID] = item.Ahead
	}
	if ahead[ids["https://example.com/a"]] != 0 || ahead[ids["https://example.com/b"]] != 1 || ahead[ids["https://example.com/c"]] != 2 {
		t.Fatalf("ahead %v for %v", ahead, ids)
	}
}

// noSpeechPipeline is retranslatePipeline but for its speech recognition, which hears no speech, so a job that hears the film fails (media.ErrNoSpeech).
func noSpeechPipeline(p *retranslatePipeline) *retranslatePipeline {
	p.silent = true
	return p
}

// folderLibrary is a library holding one video found in its folder that no job holds, Lecture, with bilingual subtitles beside it when subtitled; the settings name 日本語 as the target language. It returns the film the library shows for it.
func folderLibrary(t *testing.T, p media.Pipeline, subtitled bool) (*app, *nsgotest.App, chan func(), films.Film) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "Films.kando")
	video := filepath.Join(lib, "Lecture", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{video: "v"}
	if subtitled {
		files[filepath.Join(lib, "Lecture", "video.bilingual.srt")] = "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n"
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.prefs.language = "日本語"
	a.library.Jobs = nil
	a.refreshLibrary()
	refresh(tt, a)
	item, ok := films.Find(a.films(), video)
	if !ok || item.JobID != "" || (item.SubtitlePath != "") != subtitled {
		t.Fatalf("films %+v", a.films())
	}
	return a, tt, updates, item
}

// folderVideoKept checks that item, a video found in the library's folder, is again the film it was, with no job holding it and the subtitles it had, and that its card is back on the done grid.
func folderVideoKept(t *testing.T, a *app, tt *nsgotest.App, item films.Film) {
	t.Helper()
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs left on the shelf %+v", a.library.Jobs)
	}
	refresh(tt, a)
	now, ok := films.Find(a.films(), item.Key)
	if !ok || now.JobID != "" || now.SubtitlePath != item.SubtitlePath || films.InQueue(now) {
		t.Fatalf("film %+v, was %+v", now, item)
	}
	if a.openKey == "" && !tt.Exists("id:film-"+item.Key) {
		t.Fatalf("the film is not on the done grid: %q", tt.Texts())
	}
}

// 生成字幕 under a video found in the library's folder makes its subtitles on a card of its own, which becomes the film once it is done.
func TestMakeSubtitlesOfAFolderVideo(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates, item := folderLibrary(t, p, false)
	tt.Press("id:film-make-subtitles")
	settle(t, a, tt, updates)
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	job := a.library.Jobs[0]
	if job.Stage != core.StageComplete || job.FromFolder || job.VideoPath != item.VideoPath || job.SubtitlePath != strings.TrimSuffix(item.VideoPath, ".mp4")+".bilingual.srt" || p.language != "日本語" || p.transcriptions != 1 {
		t.Fatalf("job %+v, %d transcriptions in %q", job, p.transcriptions, p.language)
	}
	if done, ok := films.Find(a.films(), job.ID); !ok || done.VideoPath != item.VideoPath || len(a.films()) != 1 {
		t.Fatalf("films %+v", a.films())
	}
}

// 生成字幕 that fails puts the video back as it was, without a failed card, and the message bar says why; one stopped while it runs says nothing. A film open on it stays open, back on the video.
func TestAFailedMakeSubtitlesKeepsTheFolderVideo(t *testing.T) {
	t.Run("fails", func(t *testing.T) {
		p := noSpeechPipeline(&retranslatePipeline{})
		a, tt, updates, item := folderLibrary(t, p, false)
		a.Reprocess(item)
		if job := a.library.Jobs[0]; !job.FromFolder || !a.running() {
			t.Fatalf("job %+v running %v", job, a.running())
		}
		settle(t, a, tt, updates)
		if p.transcriptions != 1 {
			t.Fatalf("%d transcriptions", p.transcriptions)
		}
		if want := testlocale.Messages().QueueMakeSubtitlesFailed(item.Name, ""); !a.lib.notice.Open || !strings.HasPrefix(a.lib.notice.Text, want) || a.lib.notice.Text == want {
			t.Fatalf("notice %+v, want it to start with %q and say why", a.lib.notice, want)
		}
		folderVideoKept(t, a, tt, item)
	})
	t.Run("stopped from the open film", func(t *testing.T) {
		p := &retranslatePipeline{hold: true}
		a, tt, updates, item := folderLibrary(t, p, false)
		a.Open(item)
		refresh(tt, a)
		tt.Press("id:film-start")
		job := a.library.Jobs[0]
		if a.openKey != job.ID || !a.running() {
			t.Fatalf("open %q, job %+v, running %v", a.openKey, job, a.running())
		}
		queueUntilState(t, updates, func() bool { return stageOf(t, a, job.ID).Stage == core.StageTranslating })
		a.Cancel(job.ID)
		settle(t, a, tt, updates)
		if a.lib.notice.Open || a.openKey != item.Key || !hasText(tt, testlocale.Messages().FilmMakeSubtitles()) {
			t.Fatalf("notice %+v, open %q, texts %q", a.lib.notice, a.openKey, tt.Texts())
		}
		folderVideoKept(t, a, tt, item)
		a.closeFilm()
		folderVideoKept(t, a, tt, item)
	})
}

// 重新处理 of a video found in the library's folder with subtitles beside it asks first, and if its translation fails the video keeps its subtitles, as a job's 重新处理 keeps the film.
func TestAFailedReprocessKeepsTheFolderVideo(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key")}
	a, tt, updates, item := folderLibrary(t, p, true)
	bilingual := readFile(t, item.SubtitlePath)
	chooseFilmMenu(t, tt, testlocale.Messages().FilmsReprocess())
	tt.Press("id:reprocess-confirm-run")
	if job := a.library.Jobs[0]; !job.FromFolder || job.Redo != core.RedoAll || job.RedoLanguage != "日本語" || job.SubtitlePath != item.SubtitlePath {
		t.Fatalf("job %+v", job)
	}
	settle(t, a, tt, updates)
	if want := testlocale.Messages().QueueReprocessFailed(item.Name, testlocale.Messages().PrecheckTranslateAuth()); a.lib.notice.Text != want || a.lib.notice.Section != settings.SectionAI {
		t.Fatalf("notice %+v, want %q", a.lib.notice, want)
	}
	if now := readFile(t, item.SubtitlePath); now != bilingual {
		t.Fatalf("subtitles are now %q", now)
	}
	folderVideoKept(t, a, tt, item)
}

// The film page of a video found in the library's folder with subtitles beside it offers 重新处理整部影片, which asks first, as it would replace them; once confirmed the film stays open on its job.
func TestAFolderVideoWithSubtitlesAsksBeforeProcessingFromTheFilm(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, updates, item := folderLibrary(t, p, true)
	a.Open(item)
	refresh(tt, a)
	if !hasText(tt, testlocale.Messages().FilmReprocess()) {
		t.Fatalf("no 重新处理整部影片: %q", tt.Texts())
	}
	tt.Press("id:film-start")
	if !tt.Exists("id:reprocess-confirm") || len(a.library.Jobs) != 0 {
		t.Fatalf("it did not ask first: jobs %+v", a.library.Jobs)
	}
	tt.Press("id:reprocess-confirm-run")
	if len(a.library.Jobs) != 1 || a.openKey != a.library.Jobs[0].ID {
		t.Fatalf("open %q, jobs %+v", a.openKey, a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if job := a.library.Jobs[0]; job.Stage != core.StageComplete || job.FromFolder || job.Redo != "" || job.Language != "日本語" || job.SubtitlePath != item.SubtitlePath {
		t.Fatalf("job %+v", job)
	}
}

// A video of the library's folder dropped on the window again, as the library's own folder dropped back onto Kando, is the film it is. One with subtitles beside it is in the library already: nothing is queued, its files stay as they are, and the message bar offers to open it, as for a link already in the library. One without any is made as 生成字幕 makes it, on a card that puts the video back as it was if it fails.
func TestDroppingALibraryFolderAgainQueuesNothingItHolds(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, _, item := folderLibrary(t, p, true)
	folder := filepath.Dir(item.VideoPath)
	bilingual := readFile(t, item.SubtitlePath)
	a.dropFiles([]string{folder})
	refresh(tt, a)
	if len(a.library.Jobs) != 0 || a.running() || p.transcriptions != 0 {
		t.Fatalf("jobs %+v, running %v, %d transcriptions", a.library.Jobs, a.running(), p.transcriptions)
	}
	if a.lib.notice.Text != testlocale.Messages().QueueAlreadyInLibrary() || a.lib.notice.Film != item.Key || !slices.Contains(tt.Texts(), "打开") {
		t.Fatalf("notice %+v, texts %q", a.lib.notice, tt.Texts())
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || readFile(t, item.SubtitlePath) != bilingual {
		t.Fatalf("the folder now holds %v", entries)
	}
	tt.Press("打开")
	if a.openKey != item.Key {
		t.Fatalf("open %q, want %q", a.openKey, item.Key)
	}

	p = &retranslatePipeline{}
	a, tt, updates, item := folderLibrary(t, p, false)
	a.dropFiles([]string{filepath.Dir(item.VideoPath)})
	if len(a.library.Jobs) != 1 || !a.library.Jobs[0].FromFolder || a.library.Jobs[0].VideoPath != item.VideoPath {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if job := a.library.Jobs[0]; job.Stage != core.StageComplete || p.transcriptions != 1 || len(a.films()) != 1 {
		t.Fatalf("job %+v, %d transcriptions, films %+v", job, p.transcriptions, a.films())
	}
}

// The video of a film downloaded from a link, dropped on the window or added with kando add, is that film: no second job is queued to write its subtitles again, and the message bar offers to open it.
func TestDroppingADownloadedFilmsVideoQueuesNothing(t *testing.T) {
	p := &retranslatePipeline{}
	a, tt, _ := retranslateLibrary(t, p)
	job := stageOf(t, a, "talk")
	srt, bilingual := readFile(t, strings.TrimSuffix(job.VideoPath, ".mp4")+".srt"), readFile(t, job.SubtitlePath)
	for _, dropped := range []string{job.VideoPath, filepath.Dir(job.VideoPath)} {
		a.lib.notice = films.Notice{}
		a.dropFiles([]string{dropped})
		refresh(tt, a)
		if len(a.library.Jobs) != 1 || a.running() || a.lib.notice.Film != "talk" || a.lib.notice.Text != testlocale.Messages().QueueAlreadyInLibrary() {
			t.Fatalf("dropping %s: jobs %+v, notice %+v", dropped, a.library.Jobs, a.lib.notice)
		}
	}
	err := a.Enqueue(core.FileURL(job.VideoPath))
	var dup *queueDuplicate
	if !errors.As(err, &dup) || dup.ID != "talk" || dup.Stage != core.StageComplete || len(a.library.Jobs) != 1 {
		t.Fatalf("err %v, jobs %+v", err, a.library.Jobs)
	}
	if p.transcriptions != 0 || readFile(t, strings.TrimSuffix(job.VideoPath, ".mp4")+".srt") != srt || readFile(t, job.SubtitlePath) != bilingual {
		t.Fatalf("%d transcriptions; the film's subtitles changed", p.transcriptions)
	}
}

// 重新处理 of a film of another library than the open one that fails, or is stopped, is not said to be done: the film is done as it was, and the message bar keeps saying why it failed, or says nothing for one stopped.
func TestAFailedReprocessInAnotherLibraryIsNotDone(t *testing.T) {
	for _, stop := range []bool{false, true} {
		p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key"), hold: stop}
		a, tt, updates := retranslateLibrary(t, p)
		film, _ := a.JobFilm("talk")
		a.Reprocess(film)
		other := filepath.Join(t.TempDir(), "other.kando")
		a.edit(func(s *session) { s.LibraryDir = other })
		if stop {
			queueUntilState(t, updates, func() bool { return stageOf(t, a, "talk").Stage == core.StageTranslating })
			a.Cancel("talk")
		}
		settle(t, a, tt, updates)
		done := testlocale.Messages().QueueDoneElsewhere(jobName(stageOf(t, a, "talk")), "我的影片")
		switch {
		case a.lib.notice.Text == done:
			t.Fatalf("stopped %v: the bar says the film is done: %+v", stop, a.lib.notice)
		case stop && a.lib.notice.Open:
			t.Fatalf("a stopped reprocess says %+v", a.lib.notice)
		case !stop && (a.lib.notice.Kind != "warning" || a.lib.notice.Text != testlocale.Messages().QueueReprocessFailed("Talk", testlocale.Messages().PrecheckTranslateAuth())):
			t.Fatalf("notice %+v", a.lib.notice)
		}
		if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Language != "简体中文" {
			t.Fatalf("job %+v", job)
		}
	}
}

// The film page draws no message bar, so 重新处理 of the film open that fails says why in its title bar, with 打开设置 when a setting fixes it, while the film plays on with the subtitles it had; one stopped says nothing there, and neither does the film once it is processed again.
func TestAFailedReprocessSaysWhyOnTheFilmPage(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key")}
	a, tt, updates := retranslateLibrary(t, p)
	film, _ := a.JobFilm("talk")
	a.Open(film)
	a.Reprocess(film)
	settle(t, a, tt, updates)
	m := testlocale.Messages()
	if a.openKey != "talk" || !hasText(tt, m.FilmReprocessFailed(m.PrecheckTranslateAuth())) {
		t.Fatalf("open %q, texts %q", a.openKey, tt.Texts())
	}
	tt.Press("id:film-redo-settings")
	wantSettings(t, a, settings.SectionAI)

	a.prefs.open = false
	a.Open(film)
	p.translateErr = nil
	a.Reprocess(film)
	refresh(tt, a)
	if tt.Exists("id:film-redo-failed") {
		t.Fatalf("the failure still shows while the film is processed again: %q", tt.Texts())
	}
	settle(t, a, tt, updates)
	if tt.Exists("id:film-redo-failed") || stageOf(t, a, "talk").Language != "简体中文" {
		t.Fatalf("texts %q, job %+v", tt.Texts(), stageOf(t, a, "talk"))
	}

	p.hold = true
	a.Reprocess(film)
	queueUntilState(t, updates, func() bool { return stageOf(t, a, "talk").Stage == core.StageTranslating })
	a.Cancel("talk")
	settle(t, a, tt, updates)
	if tt.Exists("id:film-redo-failed") {
		t.Fatalf("a stopped reprocess says %q", tt.Texts())
	}
}

// Retrying 重新处理 that failed after its transcription, once the AI service is fixed, reads back the speech it heard rather than transcribing the whole film again.
func TestRetryingAFailedReprocessDoesNotTranscribeAgain(t *testing.T) {
	p := &retranslatePipeline{translateErr: errors.New("translation API returned 401 Unauthorized: invalid key")}
	a, tt, updates := retranslateLibrary(t, p)
	film, _ := a.JobFilm("talk")
	a.Reprocess(film)
	settle(t, a, tt, updates)
	if p.transcriptions != 1 {
		t.Fatalf("%d transcriptions", p.transcriptions)
	}
	p.translateErr = nil
	a.Retry("talk")
	settle(t, a, tt, updates)
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || p.transcriptions != 1 || job.Untranslated != 0 {
		t.Fatalf("job %+v, %d transcriptions", job, p.transcriptions)
	}
}

// A film that was not translated at all is in no language yet: 重新翻译为 ▸ on the film page checks none, and translating it into another language than the one its job was asked for replaces no translation, so it does not ask first.
func TestTranslateAgainOfAFilmWithoutATranslationAsksNothing(t *testing.T) {
	p := &retranslatePipeline{hold: true}
	a, tt, _ := retranslateLibrary(t, p)
	job := stageOf(t, a, "talk")
	job.TranslationErr, job.Untranslated = "no translator", 3
	a.library.Upsert(job)
	film, _ := a.JobFilm("talk")
	a.Open(film)
	refresh(tt, a)
	m := testlocale.Messages()
	tt.Press("id:film-more")
	for _, it := range tt.PopupMenu {
		for _, sub := range it.Items {
			if it.Label == m.FilmTranslateAgain() && sub.Checked {
				t.Fatalf("the film page's submenu checks %q", sub.Label)
			}
		}
	}
	tt.ChoosePopup(m.FilmTranslateAgain(), m.SettingsLanguageJapanese())
	if a.lib.retranslateOpen || !a.running() {
		t.Fatalf("asked %v, running %v", a.lib.retranslateOpen, a.running())
	}
	if job := stageOf(t, a, "talk"); job.Redo != core.RedoTranslation || job.RedoLanguage != "日本語" {
		t.Fatalf("queued %+v", job)
	}
}

// A job that made its film without translating it, as when the AI service refused the key, does not notify that the bilingual subtitles are ready: the notification says the subtitles are in the original language only, and why.
func TestAFilmWithoutATranslationNotifiesWhy(t *testing.T) {
	a, tt, updates := newTestApp(t, &refusedTranslation{})
	f := useFakeDesktop(a)
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	m := testlocale.Messages()
	want := []fakeNote{{job.ID, jobName(job), m.QueueNotifyUntranslated(m.PrecheckTranslateAuth())}}
	if job.TranslationErr == "" || !slices.Equal(f.notes, want) {
		t.Fatalf("job %+v, notifications %+v, want %+v", job, f.notes, want)
	}
}

// A job of another library than the open one that made its film without translating it says so in the message bar, and why, rather than that it is done.
func TestAFilmWithoutATranslationInAnotherLibrarySaysSo(t *testing.T) {
	a, tt, updates := newTestApp(t, &refusedTranslation{})
	if err := a.Enqueue("https://example.com/v"); err != nil {
		t.Fatal(err)
	}
	library := a.snap().LibraryDir
	a.edit(func(s *session) { s.LibraryDir = filepath.Join(t.TempDir(), "other.kando") })
	settle(t, a, tt, updates)
	job := a.library.Jobs[0]
	m := testlocale.Messages()
	want := m.QueueDoneElsewhereUntranslated(jobName(job), films.LibraryName(library), m.PrecheckTranslateAuth())
	if job.TranslationErr == "" || a.lib.notice.Text != want || a.lib.notice.Kind != "warning" || a.lib.notice.Section != settings.SectionAI {
		t.Fatalf("job %+v, notice %+v, want %q", job, a.lib.notice, want)
	}
}

// A download that borrowed Chrome's sign-in on a Mac that keeps Chrome's data folder from Kando fails with a card that says, as Settings › 通用 › 浏览器登录 does, that Kando needs Full Disk Access, with 打开系统设置 and 已允许，重试, which borrows Chrome again. Without Chrome's folder the card still says there is no sign-in data, and offers another browser.
func TestAFailedDownloadSaysWhenMacOSKeepsTheBrowserFolder(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		name           string
		chrome, closed bool
		why            string
		actions        []string
	}{
		{"Chrome's folder closed", true, true, msgs.QueueRemedyDiskAccessWhy("Chrome"), []string{msgs.QueueRemedyOpenSystemSettings(), msgs.QueueRemedyAllowed(), msgs.QueueRemedySwitchBrowser("Safari"), msgs.QueueRemedyChooseBrowser()}},
		{"no Chrome", false, false, msgs.QueueRemedyCookiesWhy("Chrome"), []string{msgs.QueueRemedySwitchBrowser("Safari"), msgs.QueueRemedyChooseBrowser()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chromeHome(t, tc.chrome, tc.closed)
			tools := toolsForRemedies(t, chromeYTDLP(t))
			tools.CookieDir = filepath.Join(t.TempDir(), "cookies")
			a, tt, updates := newTestApp(t, tools)
			lib := t.TempDir()
			a.edit(func(s *session) { s.LibraryDir = lib })
			a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
			withBrowsers(t, a, "chrome", "safari")
			a.signin.browser = "chrome"
			if err := a.Enqueue("https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd"); err != nil {
				t.Fatal(err)
			}
			settle(t, a, tt, updates)
			id := a.library.Jobs[0].ID
			if job := stageOf(t, a, id); job.Stage != core.StageFailed || job.FailKind != core.FailCookies || job.Cookies != "chrome" {
				t.Fatalf("job %+v", job)
			}
			film, _ := a.JobFilm(id)
			r, ok := a.FailureRemedy(film)
			if !ok || r.Headline != msgs.QueueRemedyCookies("Chrome") || r.Why != tc.why || strings.Join(remedyLabels(r), ",") != strings.Join(tc.actions, ",") || !r.Actions[0].Primary {
				t.Fatalf("remedy %+v", r)
			}
			if tc.closed && (r.Actions[0].ID != remedyDiskAccess || r.Actions[1].ID != remedyBorrow+"chrome") {
				t.Fatalf("actions %+v", r.Actions)
			}
			refresh(tt, a)
			if textCount(tt, r.Why) == 0 {
				t.Fatalf("the card does not say %q; texts %q", r.Why, tt.Texts())
			}
		})
	}
}

// makerPipeline makes a film a part at a time (media.Maker): its speech recognition hears a line every ten seconds, "line N", and stops at a minute in until released is closed, the first cell heard by then; its translation puts "译:" before each line.
type makerPipeline struct {
	length   time.Duration
	released chan struct{}
}

func (p *makerPipeline) Check() error { return nil }

func (p *makerPipeline) Download(_ context.Context, source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	if path, ok := core.FilePath(parsed); ok {
		return path, nil
	}
	return "", errors.New("not a link")
}

func (p *makerPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	out := make([]core.Segment, len(segments))
	for i, s := range segments {
		s.Text = "译:" + s.Text
		out[i] = s
	}
	return out, nil
}

func (p *makerPipeline) Length(context.Context, string) time.Duration { return p.length }

func (p *makerPipeline) Hear(ctx context.Context, _ string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	for at := time.Duration(0); at < p.length; at += 10 * time.Second {
		if at < from || at >= to {
			continue
		}
		if at >= 60*time.Second {
			select {
			case <-p.released:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		heard(core.Segment{Start: at, End: at + 2*time.Second, Text: "line " + at.String()})
	}
	return "ja", nil
}

// TestAFilmIsWatchedWhileItIsMade adds a video whose lines are made a part at a time: the film shows the lines made so far, translated, while the rest are still being heard, and every line once it is done.
func TestAFilmIsWatchedWhileItIsMade(t *testing.T) {
	p := &makerPipeline{length: 90 * time.Second, released: make(chan struct{})}
	a, tt, updates := newTestApp(t, p)
	video := filepath.Join(t.TempDir(), "talk", "talk.mp4")
	writeFakeClip(t, video, 90, "202020", "101010")
	if err := a.Enqueue(core.FileURL(video)); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	id := a.library.Jobs[0].ID
	queueUntilState(t, updates, func() bool { return stageOf(t, a, id).SubtitlePath != "" })
	job := stageOf(t, a, id)
	if job.Stage.Done() || !strings.HasSuffix(job.SubtitlePath, "talk.bilingual.srt") {
		t.Fatalf("job %s shows %q, want its bilingual subtitles while it is made", job.Stage, job.SubtitlePath)
	}
	item, ok := films.Find(a.films(), id)
	if !ok {
		t.Fatalf("no film for %s in %+v", video, a.films())
	}
	playerOpenFilm(t, a, tt, updates, item)
	if got := len(a.player.subs.machine); got == 0 || got >= 9 || a.player.subs.machine[0].Text != "译:line 0s\nline 0s" {
		t.Fatalf("while the film is made it shows %+v", a.player.subs.machine)
	}
	// Its lines are drafts while it is made, none proofread as a whole yet: the line under the title says so, and each row of the subtitle list has its dot.
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.FilmStatusDraft(0))
	if !tt.Exists("id:film-cue-draft") {
		t.Fatal("no row of the subtitle list is marked a draft")
	}
	close(p.released)
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageComplete || job.Untranslated != 0 {
		t.Fatalf("job %s, %d untranslated: %v", job.Stage, job.Untranslated, job.Err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && len(a.player.subs.machine) == 9 })
	if last := a.player.subs.machine[8].Text; last != "译:line 1m20s\nline 1m20s" {
		t.Fatalf("last line %q", last)
	}
	// Done without a final proofread, which this pipeline cannot make, the film is one done, as before.
	refresh(tt, a)
	wantText(t, tt, msgs.FilmStatusTranslated(100))
	if tt.Exists("id:film-cue-draft") {
		t.Fatal("a row is still marked a draft once the film is done")
	}
}

// TestAFilmMadeAgainShowsItsLinesDone processes a done film again entirely: while the redo runs, the film shows the lines it had, done, none marked a draft, and the job is done again once the redo is.
func TestAFilmMadeAgainShowsItsLinesDone(t *testing.T) {
	p := &makerPipeline{length: 90 * time.Second, released: make(chan struct{})}
	close(p.released)
	a, tt, updates := newTestApp(t, p)
	a.prefs.review = true
	video := filepath.Join(t.TempDir(), "talk", "talk.mp4")
	writeFakeClip(t, video, 90, "202020", "101010")
	if err := a.Enqueue(core.FileURL(video)); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	id := a.library.Jobs[0].ID
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageComplete {
		t.Fatalf("job %s: %v", job.Stage, job.Err)
	}
	p.released = make(chan struct{})
	a.Retry(id)
	refresh(tt, a)
	queueUntilState(t, updates, func() bool { return stageOf(t, a, id).Stage == core.StageTranscribing })
	item, ok := films.Find(a.films(), id)
	if !ok {
		t.Fatalf("no film for %s", id)
	}
	playerOpenFilm(t, a, tt, updates, item)
	if got := len(a.player.subs.machine); got != 9 {
		t.Fatalf("while it is made again the film shows %d lines, want the 9 it had", got)
	}
	if tt.Exists("id:film-cue-draft") || len(a.Subs().Drafts) != 0 {
		t.Fatal("a film made again marks the lines it had drafts")
	}
	close(p.released)
	settle(t, a, tt, updates)
	if job := stageOf(t, a, id); job.Stage != core.StageComplete || job.Redo != "" {
		t.Fatalf("job %s redo %q: %v", job.Stage, job.Redo, job.Err)
	}
}

// preferPipeline makes films a part at a time as makerPipeline does, its speech recognition stopping a minute into "first" until released is closed, and records which film each run heard.
type preferPipeline struct {
	makerPipeline
	mu    sync.Mutex
	heard []string
}

func (p *preferPipeline) Hear(ctx context.Context, video string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	name := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	p.mu.Lock()
	p.heard = append(p.heard, name)
	p.mu.Unlock()
	if name != "first" {
		inner := makerPipeline{length: p.length, released: closedGate}
		return inner.Hear(ctx, video, from, to, heard)
	}
	return p.makerPipeline.Hear(ctx, video, from, to, heard)
}

func (p *preferPipeline) runs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.heard)
}

var closedGate = func() chan struct{} { c := make(chan struct{}); close(c); return c }()

// TestTheFilmOpenedIsMadeFirst opens a film waiting behind one being made: the one being made gives way, the opened one is made, and the other goes on where it was.
func TestTheFilmOpenedIsMadeFirst(t *testing.T) {
	p := &preferPipeline{makerPipeline: makerPipeline{length: 90 * time.Second, released: make(chan struct{})}}
	a, tt, updates := newTestApp(t, p)
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first", "first.mp4"), filepath.Join(dir, "second", "second.mp4")
	writeFakeClip(t, first, 90, "202020", "101010")
	writeFakeClip(t, second, 90, "202020", "101010")
	if err := a.Enqueue(core.FileURL(first)); err != nil {
		t.Fatal(err)
	}
	if err := a.Enqueue(core.FileURL(second)); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	var firstID, secondID string
	for _, j := range a.library.Jobs {
		if strings.HasSuffix(j.Source, "first.mp4") {
			firstID = j.ID
		} else {
			secondID = j.ID
		}
	}
	// The first is heard to its first minute, where it waits; the second waits behind it.
	queueUntilState(t, updates, func() bool { return stageOf(t, a, firstID).SubtitlePath != "" })
	if a.queue.active != firstID || stageOf(t, a, secondID).Stage != core.StageQueued {
		t.Fatalf("running %q, second %s", a.queue.active, stageOf(t, a, secondID).Stage)
	}
	item, ok := films.Find(a.films(), secondID)
	if !ok {
		t.Fatal("no film for the second job")
	}
	apply(tt, a, func() { a.Open(item) })
	queueUntilState(t, updates, func() bool { return stageOf(t, a, secondID).Stage == core.StageComplete })
	if job := stageOf(t, a, firstID); job.Stage.Done() {
		t.Fatalf("the first ended %s while the second was made", job.Stage)
	}
	close(p.released)
	settle(t, a, tt, updates)
	if job := stageOf(t, a, firstID); job.Stage != core.StageComplete {
		t.Fatalf("the first ended %s: %v", job.Stage, job.Err)
	}
	// The first gave way, the second was made, then the first went on: it heard again only what it had not.
	runs := p.runs()
	at := slices.Index(runs, "second")
	if at < 0 || !slices.Contains(runs[:at], "first") || !slices.Contains(runs[at:], "first") {
		t.Fatalf("runs %v, want the first, then the second, then the first again", runs)
	}
	doc, err := track.Read(first)
	if err != nil || len(doc.View()) != 9 {
		t.Fatalf("the first film shows %d lines (%v), want each once", len(doc.View()), err)
	}
}
