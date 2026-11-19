package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/films"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/apps/kando/toolhealth"
	"kando/packages/core"
	"kando/packages/media"
)

// The copies of the programs the tests run: the ones the pipeline names, and a copy of ffmpeg that works elsewhere on the computer.
const (
	testYTDLP   = "/usr/local/bin/yt-dlp"
	testFFmpeg  = "/opt/homebrew/bin/ffmpeg"
	testWhisper = "/usr/local/bin/whisper-cli"
	otherFFmpeg = "/usr/local/bin/ffmpeg"
)

// brokenFFmpeg is how a Homebrew ffmpeg answers whose libbluray Homebrew upgraded under it.
var brokenFFmpeg = media.Health{State: media.HealthBroken, Problem: media.ProblemLibrary, Library: "libbluray"}

// fakeCopies makes a's checks of the tools find, for each program, the copies found names, and run them with the healths health names; a copy not in it works.
func fakeCopies(a *app, found map[string][]string, health map[string]media.Health) {
	a.tools.candidates = func(program string) []string { return found[program] }
	a.tools.probe = func(_ context.Context, _, path string) media.Health {
		if h, ok := health[path]; ok {
			return h
		}
		return media.Health{State: media.HealthReady, Version: "1.0"}
	}
}

// checkTools runs a check of the tools and waits for its answer.
func checkTools(t *testing.T, a *app, wait func(func() bool)) {
	t.Helper()
	gen := a.tools.gen
	a.toolsCheck()
	if a.tools.gen == gen {
		t.Fatal("no check started")
	}
	wait(func() bool { return len(a.tools.health) > 0 })
}

// TestToolsRunTheFirstCopyThatWorks checks that a copy that does not start gives way to another copy on the computer that does, so jobs run and nothing says a program is broken.
func TestToolsRunTheFirstCopyThatWorks(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: testYTDLP, FFmpeg: testFFmpeg, Whisper: testWhisper})
	fakeCopies(a, map[string][]string{"ffmpeg": {testFFmpeg, otherFFmpeg}}, map[string]media.Health{testFFmpeg: brokenFFmpeg})
	checkTools(t, a, func(done func() bool) { waitUntil(t, tt, a, updates, done) })
	if got := a.pipeline.(media.Tools).FFmpeg; got != otherFFmpeg {
		t.Fatalf("ffmpeg = %q, want the copy that works, %q", got, otherFFmpeg)
	}
	if broken := a.toolsBroken(); len(broken) != 0 {
		t.Fatalf("broken %v", broken)
	}
}

// TestToolsSayACopyThatDoesNotStart checks that a copy of ffmpeg that cannot load a library is said not to run everywhere the app speaks of the programs: the sidebar's status line, the message bar of a link added, Settings › 通用, with Kando's own offered in its place, the first-run guide and the feedback; and that a job is held back rather than failing at it.
func TestToolsSayACopyThatDoesNotStart(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: testYTDLP, FFmpeg: testFFmpeg, Whisper: testWhisper, WhisperModel: "/models/ggml-base.bin"})
	fakeKits(t, a, make(chan struct{}))
	fakeCopies(a, map[string][]string{"ffmpeg": {testFFmpeg}}, map[string]media.Health{testFFmpeg: brokenFFmpeg})
	checkTools(t, a, func(done func() bool) { waitUntil(t, tt, a, updates, done) })
	a.kitsCancel()
	waitUntil(t, tt, a, updates, func() bool { return len(a.kits.running) == 0 })
	delete(a.kits.quietFailed, "ffmpeg")

	if msg := a.readiness().Text(a.cx); msg != "组件无法运行：ffmpeg" {
		t.Fatalf("status line %q", msg)
	}
	if msg, section := a.precheck([]string{"https://example.com/v"}); msg != "处理视频的程序无法运行：ffmpeg，到「设置 › 通用」里修复。" || section != settings.SectionGeneral {
		t.Fatalf("precheck = %q, %q", msg, section)
	}
	st := settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"无法运行", "启动时缺少它需要的 libbluray", "改用看懂自带的"} {
		wantText(t, st, text)
	}
	st.Press("id:settings-tool-fix-ffmpeg")
	if job := a.kits.running["ffmpeg"]; job == nil || job.quiet {
		t.Fatalf("改用看懂自带的 started %+v, want a download that shows", job)
	}
	a.kitsCancel()

	rows := a.onboardingTools(a.cx)
	if rows[1].Name != "ffmpeg" || rows[1].State == onboarding.ToolReady || rows[1].Broken != "启动时缺少它需要的 libbluray" {
		t.Fatalf("the guide's ffmpeg is %+v", rows[1])
	}
	facts := a.feedbackFacts()
	i := slices.IndexFunc(facts.Tools, func(tool feedback.Tool) bool { return tool.Name == "ffmpeg" })
	if i < 0 || facts.Tools[i].String() != "ffmpeg 无法运行（启动时缺少它需要的 libbluray）" {
		t.Fatalf("the feedback's tools %v", facts.Tools)
	}
}

// TestToolsCheckAgainAfterAFailure checks that a job failing at a stage a program runs has the copies run again, once for that job, as the program may have broken while Kando was open; and a job that had failed before the window was ready does not.
func TestToolsCheckAgainAfterAFailure(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{YTDLP: testYTDLP, FFmpeg: testFFmpeg, Whisper: testWhisper})
	a.library.Jobs = []core.Job{{ID: "old", Stage: core.StageFailed, Err: media.ReadStageError("transcribe: ffmpeg: exit status 1")}}
	a.toolsAfterFailure()
	if a.tools.gen != 0 {
		t.Fatal("a job that failed before the window was ready started a check")
	}
	a.tools.ready = true
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "new", Stage: core.StageFailed, Err: media.ReadStageError("transcribe: ffmpeg: signal: abort trap")},
		core.Job{ID: "translation", Stage: core.StageFailed, Err: media.ReadStageError("translate: the AI service is busy")})
	a.toolsAfterFailure()
	if a.tools.gen != 1 {
		t.Fatalf("%d checks after a transcription failed, want 1", a.tools.gen)
	}
	a.toolsAfterFailure()
	if a.tools.gen != 1 {
		t.Fatal("the same failure started another check")
	}
}

// TestToolRemedyDownloadsKandosOwnAndRetries checks the card of a job a copy of ffmpeg that does not start stopped: it says so and why, offers 下载 ffmpeg 并重试, which downloads Kando's own ffmpeg, and once it works runs again every job ffmpeg stopped, but not one that failed for another reason.
func TestToolRemedyDownloadsKandosOwnAndRetries(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: testYTDLP, FFmpeg: testFFmpeg, Whisper: testWhisper, WhisperModel: "/models/ggml-base.bin"})
	release := make(chan struct{})
	dir := fakeKits(t, a, release)
	fakeCopies(a, map[string][]string{"ffmpeg": {testFFmpeg}}, map[string]media.Health{testFFmpeg: brokenFFmpeg})
	// The queue is paused, so the jobs run again wait in it rather than running.
	a.queue.paused = true
	a.library.Jobs = []core.Job{
		{ID: "stopped", Source: "https://example.com/a", Stage: core.StageFailed, LibraryDir: testVideos, Err: media.ReadStageError("transcribe: ffmpeg: signal: abort trap: dyld[1]: Library not loaded")},
		{ID: "also", Source: "https://example.com/b", Stage: core.StageFailed, LibraryDir: testVideos, Err: media.ReadStageError("download: ffmpeg: exit status 134")},
		{ID: "translation", Source: "https://example.com/c", Stage: core.StageFailed, LibraryDir: testVideos, Err: media.ReadStageError("translate: the AI service is busy")},
	}
	checkTools(t, a, func(done func() bool) { waitUntil(t, tt, a, updates, done) })
	a.kitsCancel()
	waitUntil(t, tt, a, updates, func() bool { return len(a.kits.running) == 0 })
	delete(a.kits.quietFailed, "ffmpeg")

	item, ok := films.Find(a.films(), "stopped")
	if !ok {
		t.Fatalf("no card of the job in %v", a.films())
	}
	r, ok := a.FailureRemedy(item)
	if !ok || r.Headline != "ffmpeg 无法启动" || r.Why != "启动时缺少它需要的 libbluray。改用看懂自带的 ffmpeg 就能继续。" || len(r.Actions) != 1 || r.Actions[0].Label != "下载 ffmpeg 并重试" {
		t.Fatalf("remedy %+v, %v", r, ok)
	}
	if other, _ := films.Find(a.films(), "translation"); func() bool { _, has := a.FailureRemedy(other); return has }() {
		t.Fatal("a failed translation offers ffmpeg")
	}
	a.RunRemedy(item, r.Actions[0].ID)
	if got := a.tools.retry["ffmpeg"]; !slices.Equal(got, []string{"stopped", "also"}) && !slices.Equal(got, []string{"also", "stopped"}) {
		t.Fatalf("jobs to retry %v", got)
	}
	if r, _ := a.FailureRemedy(item); !r.Working || !strings.Contains(r.Headline, "正在下载 ffmpeg") {
		t.Fatalf("while the download runs, the card says %+v", r)
	}
	close(release)
	kit, _ := media.KitFor("ffmpeg")
	own := filepath.Join(dir, kit.ID, "ffmpeg")
	waitUntil(t, tt, a, updates, func() bool { return len(a.tools.retry) == 0 })
	if got := a.pipeline.(media.Tools).FFmpeg; got != own {
		t.Fatalf("ffmpeg = %q, want Kando's own %q", got, own)
	}
	for id, want := range map[string]core.Stage{"stopped": core.StageQueued, "also": core.StageQueued, "translation": core.StageFailed} {
		if job, _ := a.library.Find(id); job.Stage != want {
			t.Errorf("%s is %v, want %v", id, job.Stage, want)
		}
	}
}

// TestToolsChooseTheComputersCopy checks that while the computer has a copy that works besides Kando's own, the user can run it instead, which settings.json keeps and the next start applies, and go back to Kando's own.
func TestToolsChooseTheComputersCopy(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: testYTDLP, FFmpeg: testFFmpeg, Whisper: testWhisper})
	dir := fakeKits(t, a, make(chan struct{}))
	kit, _ := media.KitFor("ffmpeg")
	own := filepath.Join(dir, kit.ID, "ffmpeg")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCopies(a, map[string][]string{"ffmpeg": {own, testFFmpeg}}, nil)
	checkTools(t, a, func(done func() bool) { waitUntil(t, tt, a, updates, done) })
	a.kitsCancel()
	if got, other := a.pipeline.(media.Tools).FFmpeg, a.tools.others["ffmpeg"]; got != own || other != testFFmpeg {
		t.Fatalf("ffmpeg = %q, the other copy %q", got, other)
	}
	st := settingsOpenAt(a, tt, settings.SectionGeneral)
	if !st.Exists("id:settings-tool-copy-ffmpeg") {
		t.Fatalf("no choice of ffmpeg's copy in %q", st.Texts())
	}

	a.UseComputerTool("ffmpeg", true)
	if got := a.pipeline.(media.Tools).FFmpeg; got != testFFmpeg {
		t.Fatalf("ffmpeg = %q once the computer's was chosen", got)
	}
	var saved savedSettings
	a.toolsHooks().save(&saved)
	if len(saved.ToolCopies) != 1 || saved.ToolCopies["ffmpeg"] != toolhealth.Computer {
		t.Fatalf("saved %v", saved.ToolCopies)
	}
	b, _, _ := newTestApp(t, media.Tools{FFmpeg: own})
	fakeCopies(b, map[string][]string{"ffmpeg": {own, testFFmpeg}}, nil)
	b.toolsHooks().restore(saved)
	if got := b.pipeline.(media.Tools).FFmpeg; got != testFFmpeg {
		t.Fatalf("the next start runs %q", got)
	}

	a.UseComputerTool("ffmpeg", false)
	if got := a.pipeline.(media.Tools).FFmpeg; got != own || a.tools.choice["ffmpeg"] != toolhealth.Own {
		t.Fatalf("ffmpeg = %q, chosen %v, once Kando's own was chosen again", got, a.tools.choice)
	}
}

// TestQueueFreshensKandosOwnYTDLPOnceADay checks that Kando updates its own yt-dlp in the background, as the release it pins grows too old for the sites, once a day and not again the same day, nor a yt-dlp the computer installed.
func TestQueueFreshensKandosOwnYTDLPOnceADay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Kando downloads no yt-dlp on Windows")
	}
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: testYTDLP})
	dir := fakeKits(t, a, make(chan struct{}))
	a.queueFreshenYTDLP()
	if a.queue.ytdlp.updating {
		t.Fatal("updated a yt-dlp the computer installed")
	}
	kit, _ := media.KitFor("yt-dlp")
	own := filepath.Join(dir, kit.ID, "yt-dlp")
	runs := filepath.Join(t.TempDir(), "runs")
	script := "#!/bin/sh\ncase \"$1\" in\n-U) echo update >> " + runs + " ;;\n--version) echo 2026.11.15 ;;\nesac\n"
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := a.pipeline.(media.Tools)
	tools.YTDLP = own
	a.pipeline = tools
	a.queue.ytdlp.path, a.queue.ytdlp.version = own, "2026.10.26"

	a.queueFreshenYTDLP()
	if !a.queue.ytdlp.updating {
		t.Fatal("Kando's own yt-dlp was not updated")
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.queue.ytdlp.updating })
	if a.queue.ytdlp.freshened.IsZero() || a.queue.ytdlp.version != "2026.11.15" {
		t.Fatalf("freshened %v, version %q", a.queue.ytdlp.freshened, a.queue.ytdlp.version)
	}
	a.queue.ytdlp.freshenTried = time.Time{}
	a.queueFreshenYTDLP()
	if a.queue.ytdlp.updating {
		t.Fatal("updated again the same day")
	}
	if data, _ := os.ReadFile(runs); strings.Count(string(data), "update") != 1 {
		t.Fatalf("yt-dlp -U ran %q", data)
	}
	var saved savedSettings
	a.saveQueue(&saved)
	if !saved.YTDLPUpdated.Equal(a.queue.ytdlp.freshened) {
		t.Fatalf("saved %v, want %v", saved.YTDLPUpdated, a.queue.ytdlp.freshened)
	}
}
