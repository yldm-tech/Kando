package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsLogs renders the command palette finding 显示日志文件, the 帮助 menu's row that reveals the log file, which is how the row is reached where there is no menu bar of the app's own. The log file is opened in a temporary folder first, as main opens it once the window is ready; the row is there only while there is one.
func TestScreenshotsLogs(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "palette-log", func(a *app, tt *nsgotest.App, updates chan func()) {
		logsAt(t, a)
		tt.Key("mod+k")
		screenshotWait(t, tt, a, updates, "subtitles", func() bool { return !a.palette.reading })
		typeFocused(t, tt, "日志")
	})
}
