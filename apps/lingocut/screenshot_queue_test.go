package main

import (
	"slices"
	"testing"
	"time"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsQueue renders the queue (screenshot_helpers_test.go): three links added at once while the pipeline holds the first in its download, so the queue shows one card downloading and the others waiting.
func TestScreenshotsQueue(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "queue", func(a *app, _ *ui.Tester, updates chan func()) {
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
	})
}
