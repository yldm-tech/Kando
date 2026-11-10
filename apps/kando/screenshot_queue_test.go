package main

import (
	"slices"
	"testing"
	"time"

	"kando/packages/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestScreenshotsQueue renders the queue (screenshot_helpers_test.go): three links added at once while the pipeline holds the first in its download, so the queue shows one card downloading and the others waiting; and the confirmation ⌘Q opens over it while that job runs. Besides that it renders what the message bar says when the link added in the top bar is one the library already has waiting, one it has already processed, with 打开, and a channel's.
func TestScreenshotsQueue(t *testing.T) {
	f := newScreenshotFixture(t)
	// add types link into the top bar and clicks 开始处理. The fixture's tools have no speech model, which precheck asks for before a job is added, so the scene's pipeline is one precheck does not look into.
	add := func(a *app, tt *ui.Tester, link string) {
		t.Helper()
		a.pipeline = &fakePipeline{block: true}
		screenshotClick(t, tt, "视频链接")
		tt.Type(link)
		screenshotClick(t, tt, "开始处理")
		tt.Frame()
		if len(a.library.Jobs) != len(f.jobs) {
			t.Fatalf("%s was queued: %+v", link, a.library.Jobs)
		}
	}
	f.shoot(t, "queue-already-queued", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://youtu.be/"+protoFilmByID("j6").VideoID)
	})
	f.shoot(t, "queue-already-in-library", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://youtu.be/"+protoFilmByID("j1").VideoID)
	})
	f.shoot(t, "queue-channel-link", func(a *app, tt *ui.Tester, _ chan func()) {
		add(a, tt, "https://www.youtube.com/@veritasium")
	})
	f.shoot(t, "queue", func(a *app, _ *ui.Tester, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
	})
	f.shoot(t, "queue-quit", func(a *app, _ *ui.Tester, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
		a.areasBeforeQuit(&mygo.QuitEvent{})
	})
}

// screenshotQueueDownloading adds three links at once while the pipeline holds the first in its download, and returns once that one is downloading.
func screenshotQueueDownloading(t *testing.T, a *app, updates chan func()) {
	t.Helper()
	a.pipeline = &fakePipeline{block: true}
	t.Cleanup(a.stop)
	_ = a.Enqueue("https://www.youtube.com/watch?v=aqz-KE-bpKQ", "https://www.bilibili.com/video/BV1GJ411x7h7", "https://vimeo.com/76979871")
	deadline := time.After(5 * time.Second)
	for !slices.ContainsFunc(a.library.Jobs, func(j core.Job) bool { return j.Stage == core.StageDownloading }) {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("no job started downloading: %+v", a.library.Jobs)
		}
	}
}
