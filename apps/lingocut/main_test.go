package main

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lingocut/packages/core"
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
	return a, ui.NewTester(a.view, 820, 560), updates
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

func TestViewRejectsInvalidSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	startJob(t, tt, "not-a-url")
	tt.Frame()
	if a.job != nil || !tt.HasText("video source must be a valid URL") {
		t.Fatalf("texts %q after an invalid source", tt.Texts())
	}
}
