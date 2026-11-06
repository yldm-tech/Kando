package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSettings renders settings › 翻译 and the account page (screenshot_helpers_test.go).
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "settings", func(_ *app, tt *ui.Tester, _ chan func()) { screenshotClick(t, tt, "翻译") })
	f.shoot(t, "account", func(_ *app, tt *ui.Tester, _ chan func()) { screenshotClick(t, tt, "账号") })
}
