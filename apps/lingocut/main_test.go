package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
)

// fakePipeline finishes every stage at once unless block is set, in which case Download waits for cancellation.
type fakePipeline struct {
	block    bool
	language string
}

func (f *fakePipeline) Check() error { return nil }

func (f *fakePipeline) Download(ctx context.Context, _ string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "/videos/talk.mp4", nil
}

func (f *fakePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, nil
}

func (f *fakePipeline) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/talk.bilingual.srt", nil
}

// newTestApp queues background updates so the test applies them on its own goroutine, as the window's UI thread would.
func newTestApp(p *fakePipeline) (*app, *ui.Tester, chan func()) {
	updates := make(chan func(), 16)
	a := newApp(p, func(fn func()) { updates <- fn })
	return a, ui.NewTester(a.view, 1100, 720), updates
}

func settle(t *testing.T, a *app, tt *ui.Tester, updates chan func()) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for a.running() {
		select {
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatalf("job still running, status %q", a.status)
		}
	}
	tt.Frame()
}

func startJob(t *testing.T, tt *ui.Tester, source string) {
	t.Helper()
	if err := tt.Click("视频链接"); err != nil {
		t.Fatal(err)
	}
	tt.Type(source)
	if err := tt.Click("开始处理"); err != nil {
		t.Fatal(err)
	}
}

func TestViewRunsJobToCompletion(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if a.job == nil || a.status != "任务已加入队列" {
		t.Fatalf("texts %q after starting", tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("完成：/videos/talk.bilingual.srt") {
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

func TestLibraryShowsAndRestoresJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	startJob(t, tt, "https://example.com/video")
	settle(t, a, tt, updates)
	if !tt.HasText("talk") || !tt.HasText("已完成") || !tt.HasText("完成：/videos/talk.bilingual.srt") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if !tt.HasText("1 部影片") || !tt.HasText("1") {
		t.Fatalf("texts %q missing the count", tt.Texts())
	}

	reloaded, err := loadLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.jobs) != 1 || reloaded.jobs[0].SubtitlePath != "/videos/talk.bilingual.srt" || reloaded.jobs[0].Language != "简体中文" {
		t.Fatalf("saved %#v", reloaded.jobs)
	}
}

func TestViewShowsDownloadedVideosInLibrary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "interview.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.libraryDir = dir
	a.refreshLibrary()
	tt.Frame()
	if !tt.HasText("interview.mp4") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestDropImportsLocalVideo(t *testing.T) {
	src := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	r, ok := tt.Find("开始处理")
	if !ok {
		t.Fatal("missing start button")
	}
	if !tt.DropFiles(r.X+r.W/2, r.Y+r.H/2, src) {
		t.Fatal("the library did not take the dropped file")
	}
	tt.Frame()
	if a.source != "file://"+src || a.status != "已选择本地视频" {
		t.Fatalf("source %q status %q texts %q", a.source, a.status, tt.Texts())
	}
}

func TestImportIgnoresNonVideo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(&fakePipeline{})
	a.libraryDir = dir
	a.importLocal(src)
	if a.status != "没有可导入的视频" {
		t.Fatalf("status %q", a.status)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("non-video was copied")
	}
}

func TestViewRejectsInvalidSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	startJob(t, tt, "not-a-url")
	tt.Frame()
	if a.job != nil || !tt.HasText("video source must be a valid URL") {
		t.Fatalf("texts %q after an invalid source", tt.Texts())
	}
}

func TestImportLocalVideoSetsFileSource(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.importLocal("/Users/test/interview.mp4")
	if a.source != "file:///Users/test/interview.mp4" {
		t.Fatalf("source = %q", a.source)
	}
	if a.status != "已选择本地视频" {
		t.Fatalf("status = %q", a.status)
	}
}
