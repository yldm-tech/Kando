package main

import (
	"testing"
	"time"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsCheatsheet renders the shortcut cheat sheet over the library and over the open film (screenshot_helpers_test.go).
func TestScreenshotsCheatsheet(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "cheatsheet", func(_ *app, tt *ui.Tester, _ chan func()) {
		tt.Key(ui.Cmd, ui.KeySlash)
		tt.Frame()
	})
	f.shoot(t, "cheatsheet-film", func(a *app, tt *ui.Tester, updates chan func()) {
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
		tt.Key(ui.Cmd, ui.KeySlash)
		tt.Frame()
	})
}
