package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsLogs renders the command palette finding 显示日志文件, the 帮助 menu's row that reveals the log file, which is how the row is reached where there is no menu bar of the app's own. The log file is opened in a temporary folder first, as main opens it once the window is ready; the row is there only while there is one.
func TestScreenshotsLogs(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "palette-log", func(a *app, tt *ui.Tester, updates chan func()) {
		logsAt(t, a)
		tt.Key(ui.Cmd, ui.KeyK)
		tt.Frame()
		screenshotWait(t, tt, updates, "subtitles", func() bool { return !a.palette.reading })
		tt.Type("日志")
		tt.Frame()
	})
}
