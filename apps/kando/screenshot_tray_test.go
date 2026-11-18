package main

import (
	"testing"

	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsTray renders what running in the background adds to the window (screenshot_helpers_test.go): the queue of TestScreenshotsQueue once it was paused, its first job back among the waiting ones, which say the queue is paused; and Settings › 通用 with its 后台运行 card on macOS, where the system opens Kando at login. The status icon itself is the system's to draw, so it is in no scene.
func TestScreenshotsTray(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "queue-paused", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotQueueDownloading(t, a, updates)
		onEngine(a, func() { a.queueSetPaused(true) })
		screenshotWait(t, tt, a, updates, "the pause", func() bool { return !a.running() })
	})
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionGeneral, "settings-general-background", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		a.tray.sys, a.tray.login = (&fakeTraySystem{}).system(true), true
	})
}
