package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsCorrect renders the prototype's film, How Rivers Shape Landscapes (j1), held on its first line with that line selected for correcting (Return), its bar over the picture, and with the line's fields showing (Return again).
func TestScreenshotsCorrect(t *testing.T) {
	f := newScreenshotFixture(t)
	selected := func(edit bool) func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		return func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
			// TODO(nsgo#107): show the film's own picture, its frames decoded by the fake ffmpeg as MyGo's player showed them; the system's player plays no fake clip headless, so the stage shows the film's poster behind the report of a load https://github.com/nsgo-dev/nsgo/issues/107
			screenshotLoad(tt, a)
			tt.Key("enter")
			if !tt.Exists("id:film-correct-bar") {
				t.Fatalf("Return selected no line: %q", tt.Texts())
			}
			if edit {
				tt.Key("enter")
				if !tt.Exists("id:film-correct-editor") {
					t.Fatalf("Return did not edit the line: %q", tt.Texts())
				}
			}
		}
	}
	f.shoot(t, "correct-selected", selected(false))
	f.shoot(t, "correct-editing", selected(true))
}
