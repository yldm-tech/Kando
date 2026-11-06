package main

import (
	"testing"
	"time"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsPlayer renders Colibri open, once its subtitles and, with ffmpeg, its picture have loaded (screenshot_helpers_test.go).
func TestScreenshotsPlayer(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "film", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotClick(t, tt, "Colibri")
		_, tools := f.pipeline.(media.Tools)
		deadline := time.After(8 * time.Second)
		for !tt.HasText("蜂鸟每秒振翅五十次。") || (tools && !tt.HasText("播放")) {
			select {
			case fn := <-updates:
				fn()
			case <-deadline:
				t.Fatalf("film did not load: %q err %q", tt.Texts(), a.player.playErr)
			}
			tt.Frame()
		}
	})
}
