package main

import (
	"testing"

	"lingocut/apps/lingocut/settings"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSettings renders settings › 翻译 and the account page (screenshot_helpers_test.go).
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "settings", func(a *app, tt *ui.Tester, _ chan func()) { settingsOpenAt(a, tt, settings.SectionTranslate) })
	f.shoot(t, "account", func(a *app, tt *ui.Tester, _ chan func()) { settingsOpenAt(a, tt, settings.SectionAccount) })
}
