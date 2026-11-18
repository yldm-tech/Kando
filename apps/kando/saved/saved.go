// Package saved is the app's side of the subtitle lines a learner saved while watching: the lines themselves, with their notes and review schedule, are the library's study record (kando/packages/study), which this package names for the app; the export (export.go) writes them out for Anki.
package saved

import (
	"fmt"
	"time"

	"kando/packages/study"
)

// Line is a saved line, and Book the saved lines of one library (study.Line, study.Book).
type (
	Line = study.Line
	Book = study.Book
)

// FileName is the file a library keeps its saved lines in (study.FileName).
const FileName = study.FileName

// ErrUnread is what Save returns for a book whose file could not be read (study.ErrUnread).
var ErrUnread = study.ErrUnread

// Load reads the saved lines of the library folder dir (study.Load), which takes them from saved-lines.json the first time.
func Load(dir string) (Book, error) { return study.Load(dir) }

// SameStart reports whether a and b are one line's start (study.SameStart).
func SameStart(a, b time.Duration) bool { return study.SameStart(a, b) }

// Clock is where a line starts, as the player's subtitle list writes it: "1:02.4", or "1:02:03.4" past an hour.
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h, m, s, tenth := ms/3_600_000, ms/60_000%60, ms/1000%60, ms/100%10
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%d", h, m, s, tenth)
	}
	return fmt.Sprintf("%d:%02d.%d", m, s, tenth)
}
