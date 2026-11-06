package main

import (
	"testing"

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
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.Key(ui.Cmd, ui.KeySlash)
		tt.Frame()
	})
}
