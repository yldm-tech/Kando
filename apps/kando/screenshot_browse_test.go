package main

import (
	"testing"

	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsBrowse renders the built-in browser (screenshot_helpers_test.go). The scenes show what the app draws around the page, which a test's web view leaves blank:
//   - browse: 浏览 chosen in the sidebar, right after 全部影片, and the page's start before a web page opens, with 清除登录信息… at its foot
//   - browse-typing: the address field with the keyboard and an address typed
//   - browse-clear: the confirmation of 清除登录信息… over the start
//   - browse-not-ready: 下载视频 on a page while the speech model is missing, the message bar saying so with 打开设置
//
// The page's own buttons (browse/buttons.go) are drawn by the browser's script inside the page, which a test's web view does not run, so no scene shows them.
func TestScreenshotsBrowse(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "browse", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		screenshotBrowse(a)
	})
	// The start's line once Chrome's sign-in is the one the pages and downloads use, with 刷新登录 and 更改…, after the pages took it as the browser first opened.
	f.shoot(t, "browse-signin", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		a.signin.browser = "chrome"
		a.browse.signin.opened, a.browse.signin.signedIn = true, "chrome"
		screenshotBrowse(a)
	})
	f.shoot(t, "browse-typing", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		screenshotBrowse(a)
		a.browseFocusAddress()
		refresh(tt, a)
		typeFocused(t, tt, "www.bilibili.com/video/BV1xx411c7mD")
	})
	f.shoot(t, "browse-clear", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		screenshotBrowse(a)
		refresh(tt, a)
		screenshotClick(t, tt, whole(testlocale.Messages().BrowseClearData()))
	})
	f.shoot(t, "browse-not-ready", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotBrowse(a)
		const page = "https://www.bilibili.com/video/BV1xx411c7mD"
		browseOpen(t, a, tt, page)
		screenshotClick(t, tt, "id:browse-download")
		tt.AnswerScript(browsePage, `"`+page+`"`, "")
		tt.Settle()
	})
}

// screenshotBrowse shows the browser as the app does once its window is ready, which the scenes' app never runs: its start offers 清除登录信息…, which clears the store the pages share (browseHooks).
func screenshotBrowse(a *app) {
	a.browse.clear = a.cx.ClearWebsiteData
	a.Select(pageBrowse)
}
