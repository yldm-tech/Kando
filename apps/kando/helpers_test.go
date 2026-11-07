package main

import (
	"context"
	"image"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	"kando/packages/core"
	"kando/packages/media"

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
func newTestApp(p media.Pipeline) (*app, *ui.Tester, chan func()) {
	updates := make(chan func(), 16)
	a := newApp(p, func(fn func()) { updates <- fn })
	a.prefs.listModels = offlineModels
	a.edit(func(s *session) { s.LibraryDir = "/videos" })
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
			t.Fatalf("job still running, status %q", a.queue.status)
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

func filmRows(t *testing.T, tt *ui.Tester) int {
	t.Helper()
	tops := []float32{filmTop(t, tt, "one"), filmTop(t, tt, "two"), filmTop(t, tt, "three")}
	slices.Sort(tops)
	rows := 1
	for i := 1; i < len(tops); i++ {
		if tops[i]-tops[i-1] > 4 {
			rows++
		}
	}
	return rows
}

func filmTop(t *testing.T, tt *ui.Tester, title string) float32 {
	t.Helper()
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("missing %q in %q", title, tt.Texts())
	}
	return r.Y
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func textCount(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, s) {
			n++
		}
	}
	return n
}

func sampleStage(img *image.RGBA) [3]uint8 {
	// The film picture fills the page while it is open, so the middle of the window is on it.
	b := img.Bounds()
	x, y := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	r, g, b0, _ := img.At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b0 >> 8)}
}

func takeUpload(t *testing.T, uploadedFiles <-chan []cloud.File) []cloud.File {
	t.Helper()
	select {
	case files := <-uploadedFiles:
		return files
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not run")
	}
	return nil
}

func keysOf(files []cloud.File) []string {
	out := make([]string, len(files))
	for i, file := range files {
		out[i] = file.Key
	}
	return out
}

func waitUntil(t *testing.T, updates <-chan func(), done func() bool) {
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

func waitUpdate(t *testing.T, updates <-chan func()) {
	t.Helper()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a background update")
	}
}

// wantSettings fails t unless the settings are open at section, one of the settings package's Section values. Tests outside the settings area check the settings' own state this way, never a.page, because the settings become a window of their own.
func wantSettings(t *testing.T, a *app, section string) {
	t.Helper()
	if !a.prefs.open || a.prefs.section != section {
		t.Fatalf("settings open %v at %q, want open at %q", a.prefs.open, a.prefs.section, section)
	}
}

// titleBarShifts draws the window twice, without a title bar and under bar, after setup has run on each app, and returns how far down each text sits under bar, and the window drawn under it.
func titleBarShifts(t *testing.T, bar ui.TitleBar, setup func(*app), texts ...string) ([]float32, *ui.Tester) {
	t.Helper()
	plain, plainTT, _ := newTestApp(&fakePipeline{})
	barred, barredTT, _ := newTestApp(&fakePipeline{})
	barredTT.SetTitleBar(bar)
	if setup != nil {
		setup(plain)
		setup(barred)
	}
	plainTT.Frame()
	barredTT.Frame()
	shifts := make([]float32, len(texts))
	for i, text := range texts {
		before, ok := plainTT.Find(text)
		if !ok {
			t.Fatalf("missing %q in %q", text, plainTT.Texts())
		}
		after, ok := barredTT.Find(text)
		if !ok {
			t.Fatalf("missing %q in %q", text, barredTT.Texts())
		}
		shifts[i] = after.Y - before.Y
	}
	return shifts, barredTT
}

// offlineModels stands in for an AI service's model list in tests, which reach no network: the service offers the model already chosen, or none.
func offlineModels(_ context.Context, t media.ChatTranslator) ([]string, error) {
	if t.Model == "" {
		return nil, nil
	}
	return []string{t.Model}, nil
}
