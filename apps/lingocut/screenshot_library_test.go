package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsLibrary renders the library page (screenshot_helpers_test.go).
func TestScreenshotsLibrary(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "library", func(*app, *ui.Tester, chan func()) {})
}
