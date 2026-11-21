package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsSubexport renders the prototype's film, How Rivers Shape Landscapes (j1), with the 导出字幕 dialog open over it (⌘E), as it opens: bilingual SubRip, the translation on top.
func TestScreenshotsSubexport(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "subexport-dialog", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		// TODO(nsgo#1057): show the film's own picture, its frames decoded by the fake ffmpeg as MyGo's player showed them, once Kando plays what ffmpeg decodes (playerRemux in playback.go), which waits on an NSGo audio output fed decoded sound; the system's player plays no fake clip headless, so the stage shows the film's poster behind the report of a load https://github.com/nsgo-dev/nsgo/issues/1057
		screenshotLoad(tt, a)
		a.OpenSubexport()
		refresh(tt, a)
		if !tt.Exists("id:export-dialog") {
			t.Fatalf("the dialog did not open: %q", tt.Texts())
		}
	})
}
