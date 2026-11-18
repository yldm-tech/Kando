package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/saved"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsSaved renders the Saved Lines sheet over the library with three lines of How Rivers Shape Landscapes saved, the second with the learner's note under it, and the sheet with the note field of the first line open, a note typed in it.
func TestScreenshotsSaved(t *testing.T) {
	f := newScreenshotFixture(t)
	// sheet saves lines 1, 3 and 9 of j1, newest last, in a library that has none, as the scenes share its folder, writes a note on line 3 and opens the sheet over the library.
	sheet := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		onEngine(a, func() {
			if err := os.Remove(filepath.Join(f.lib, saved.FileName)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			a.saved.book = saved.Book{}
			for _, i := range []int{1, 3, 9} {
				a.ToggleSaveLine(i)
			}
		})
		tt.Key("escape")
		onEngine(a, func() {
			a.saved.book.SetNote(a.saved.book.Lines[1].ID, "cuts deeper：越切越深，切的是河道本身")
			a.OpenSavedSheet()
		})
		refresh(tt, a)
	}
	f.shoot(t, "saved", sheet)
	f.shoot(t, "saved-note", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		sheet(t, a, tt, updates)
		id := a.saved.book.Lines[2].ID
		tt.Press("id:saved-note-edit-" + id)
		tt.Type("id:saved-note-field-"+id, "picks up 不是“捡起”，是一路裹挟")
	})
}
