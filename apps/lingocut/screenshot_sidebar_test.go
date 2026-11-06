package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSidebar renders the library at the size of the prototype's window (prototype/01-library.png), to compare the sidebar's card with it side by side, with the status line's first check done (screenshot_helpers_test.go).
func TestScreenshotsSidebar(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "library-prototype-size", func(a *app, tt *ui.Tester, _ chan func()) {
		a.checkReadiness(true)
		tt.SetSize(1304, 782)
	})
}
