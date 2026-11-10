package main

import (
	"testing"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsBrowse renders the built-in browser (screenshot_helpers_test.go). The tests have no window to make a web view in, so the scenes show what the app draws around the page:
//   - browse: 浏览 chosen in the sidebar, right after 全部影片, and the page's start before a web page opens, with 清除登录信息… at its foot
//   - browse-typing: the address field with the keyboard and an address typed
//   - browse-clear: the confirmation of 清除登录信息… over the start
//   - browse-not-ready: 下载视频 on a page while the speech model is missing, the message bar saying so with 打开设置
func TestScreenshotsBrowse(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "browse", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		a.Select(pageBrowse)
	})
	f.shoot(t, "browse-typing", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		a.Select(pageBrowse)
		a.browseFocusAddress()
		tt.Frame()
		tt.Type("www.bilibili.com/video/BV1xx411c7mD")
	})
	f.shoot(t, "browse-clear", func(a *app, tt *ui.Tester, _ chan func()) {
		screenshotReady(t, f, a)
		a.Select(pageBrowse)
		tt.Frame()
		if err := tt.Click(locales.BrowseClearData()); err != nil {
			t.Fatal(err)
		}
	})
	f.shoot(t, "browse-not-ready", func(a *app, tt *ui.Tester, _ chan func()) {
		a.Select(pageBrowse)
		fake := &fakeBrowsePage{}
		a.browse.open = fake.open
		a.browseLoad("https://www.bilibili.com/video/BV1xx411c7mD")
		tt.Frame()
		fake.loading = false
		a.browseRefresh()
		a.BrowseDownload()
	})
}
