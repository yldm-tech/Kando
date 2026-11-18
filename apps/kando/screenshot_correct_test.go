package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsCorrect renders the prototype's film, How Rivers Shape Landscapes (j1), held on its first line with that line selected for correcting (E), its bar over the picture, and with the line's fields showing (Return); then the AI service's other translations of a line (这句不对, ⇧E), and the bar asking whether to spread a fix to the film's other lines.
func TestScreenshotsCorrect(t *testing.T) {
	f := newScreenshotFixture(t)
	selected := func(edit bool) func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		return func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
			screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
			// TODO(nsgo#107): show the film's own picture, its frames decoded by the fake ffmpeg as MyGo's player showed them; the system's player plays no fake clip headless, so the stage shows the film's poster behind the report of a load https://github.com/nsgo-dev/nsgo/issues/107
			screenshotLoad(tt, a)
			tt.Key("e")
			if !tt.Exists("id:film-correct-bar") {
				t.Fatalf("E selected no line: %q", tt.Texts())
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
	// 这句不对 on the line that names the meanders, its translations come: they are set as the AI service's reply would set them, as no AI service answers headless.
	f.shoot(t, "correct-candidates", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[7].Start)
		screenshotLoad(tt, a)
		tt.Key("e")
		apply(tt, a, func() {
			a.correct.pickGen++
			a.correct.pick = &correctPick{gen: a.correct.pickGen, options: []string{"这些大幅度的弯道叫作曲流。", "这种大弯道就叫曲流。"}, at: 1}
		})
		if !tt.Exists("id:film-candidates") {
			t.Fatalf("no candidates: %q", tt.Texts())
		}
	})
	// The edit of 河曲 to 曲流 asks whether to change it in the two other lines that hold it.
	f.shoot(t, "correct-spread", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[7].Start)
		screenshotLoad(tt, a)
		tt.Key("e")
		tt.Key("enter")
		tt.Clear("id:film-correct-translation")
		tt.Type("id:film-correct-translation", "这些大幅度的弯道被称为曲流。")
		tt.Press("id:film-correct-submit")
		if !tt.Exists("id:film-spread") {
			t.Fatalf("no bar asks: %q", tt.Texts())
		}
		apply(tt, a, func() {
			if play := a.player.play; play != nil && play.Playing() {
				play.toggle()
			}
		})
	})
}
