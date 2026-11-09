package main

import (
	"testing"

	"kando/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSites renders the 支持的网站 sheet over the library as 帮助 › 支持的网站… opens it, the sheet searched by a link and by a site off the list, and the card of an unsupported site that offers it (screenshot_helpers_test.go).
func TestScreenshotsSites(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "sites", func(a *app, tt *ui.Tester, updates chan func()) {
		a.OpenSites()
		tt.Frame()
	})
	f.shoot(t, "sites-search", func(a *app, tt *ui.Tester, updates chan func()) {
		a.OpenSites()
		tt.Frame()
		a.sites.query = "nico"
		tt.Frame()
	})
	f.shoot(t, "sites-link", func(a *app, tt *ui.Tester, updates chan func()) {
		a.OpenSites()
		tt.Frame()
		a.sites.query = "https://www.bilibili.com/video/BV1xx411c7mD"
		tt.Frame()
	})
	f.shoot(t, "sites-not-found", func(a *app, tt *ui.Tester, updates chan func()) {
		a.OpenSites()
		tt.Frame()
		a.sites.query = "https://unknown-video-site.example/watch/1"
		tt.Frame()
	})
	f.shoot(t, "sites-unsupported-card", func(a *app, tt *ui.Tester, updates chan func()) {
		failedJob(t, a, "https://unknown-video-site.example/watch/1", core.FailUnsupported, "")
		tt.Frame()
	})
}
