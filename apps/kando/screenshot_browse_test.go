package main

import (
	"testing"

	"kando/apps/kando/locales/testlocale"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsBrowse renders the built-in browser (screenshot_helpers_test.go). The scenes show what the app draws around the page, which a test's web view leaves blank:
//   - browse: 浏览 chosen in the sidebar, right after 全部影片, and the page's start before a web page opens
//   - browse-typing: the address field with the keyboard and an address typed
//   - browse-clear: the confirmation of 清除登录信息… over the start, as the browser draws it where the pages' sign-ins can be cleared, which NSGo's web view cannot yet (browseHooks)
//   - browse-not-ready: 下载视频 on a page while the speech model is missing, the message bar saying so with 打开设置
//
// The page's own buttons (browse/buttons.go) are drawn by the browser's script inside the page, which a test's web view does not run, so no scene shows them.
func TestScreenshotsBrowse(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "browse", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		a.Select(pageBrowse)
	})
	f.shoot(t, "browse-typing", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		a.Select(pageBrowse)
		a.browseFocusAddress()
		refresh(tt, a)
		typeFocused(t, tt, "www.bilibili.com/video/BV1xx411c7mD")
	})
	f.shoot(t, "browse-clear", func(a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		a.browse.clear = func() error { return nil }
		a.Select(pageBrowse)
		refresh(tt, a)
		screenshotClick(t, tt, whole(testlocale.Messages().BrowseClearData()))
	})
	f.shoot(t, "browse-not-ready", func(a *app, tt *nsgotest.App, _ chan func()) {
		a.Select(pageBrowse)
		const page = "https://www.bilibili.com/video/BV1xx411c7mD"
		a.browseLoad(page)
		refresh(tt, a)
		browseReport(tt, ns.WebNavigation{URL: page})
		screenshotClick(t, tt, "id:browse-download")
		tt.AnswerScript(browsePage, `"`+page+`"`, "")
		tt.Settle()
	})
}
