package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsCheatsheet renders the shortcut cheat sheet over the library and over the open film (screenshot_helpers_test.go).
func TestScreenshotsCheatsheet(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "cheatsheet", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		tt.Key("mod+/")
		if !a.overlay.cheatsheet {
			t.Fatal("⌘/ did not open the cheat sheet")
		}
	})
	f.shoot(t, "cheatsheet-film", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.Key("mod+/")
		if !a.overlay.cheatsheet {
			t.Fatal("⌘/ did not open the cheat sheet")
		}
	})
}
