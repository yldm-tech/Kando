package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsPlayer renders the prototype's film, How Rivers Shape Landscapes (j1), open on its first line, once its subtitles and its picture have loaded (screenshot_helpers_test.go).
func TestScreenshotsPlayer(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "film", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
	})
}
