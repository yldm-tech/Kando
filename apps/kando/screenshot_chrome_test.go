package main

import (
	"testing"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsChrome renders the main window's title bar of Windows and Linux (package chrome) over the library: Windows' with the app's mark, its name and the palette's button, and Linux's header bar with the button, the name in its middle and its own window buttons. A target runs on its own system, which the environment does not change, so the pictures show the bar only from `nsgo test -target windows` or `-target linux`; on macOS both scenes show the Mac's window.
func TestScreenshotsChrome(t *testing.T) {
	f := newScreenshotFixture(t)
	usePlatform(t, ns.OSWindows)
	f.shoot(t, "chrome-windows", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {})
	usePlatform(t, ns.OSLinux)
	f.shoot(t, "chrome-linux", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {})
}
