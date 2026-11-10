// Package browse draws the built-in browser (浏览): a toolbar with back, forward, reload, home, the address field and 下载视频, the message bar under it, and the web page, or the page's start before a web page is open. The web view, its page and what they do belong to the app (state_browse.go); this package only draws them and hands the clicks back.
package browse

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// State is what the toolbar and the page show of the browser.
type State struct {
	// Page is set while a web page is open; without one the page's start shows.
	Page bool
	// Loading is set while the page loads; reload then stops it.
	Loading      bool
	CanGoBack    bool
	CanGoForward bool
	// Clearing is set while the pages' sign-ins are being cleared.
	Clearing bool
}

// Notice is the message bar under the toolbar: Kind is "warning" or "success", Section the settings section that fixes what it says ("" when none does), and Open whether it shows.
type Notice struct {
	Kind    string
	Text    string
	Section string
	Open    bool
}

// App is the browser's state the page reads, and where its clicks go.
type App interface {
	BrowseState() State
	// BrowseWebView is the web view showing the page, nil until it is made.
	BrowseWebView() *mygo.WebView
	// BrowseAddress is the address field's text.
	BrowseAddress() *string
	// BrowseAddressFocus asks for the keyboard in the address field; the page gives it and clears the request.
	BrowseAddressFocus() *bool
	// BrowseAddressFocused reports, each frame, whether the address field has the keyboard.
	BrowseAddressFocused(focused bool)
	// BrowseGo opens what the address field holds: a web address, or a search for it.
	BrowseGo()
	BrowseBack()
	BrowseForward()
	BrowseReload()
	BrowseStop()
	// BrowseHome closes the web page and shows the page's start.
	BrowseHome()
	// BrowseDownload adds the page shown to the library's queue.
	BrowseDownload()
	BrowseNotice() *Notice
	// BrowseClearOpen is the 清除登录信息 confirmation's open flag; BrowseAskClear opens it and BrowseClear clears once it is confirmed.
	BrowseClearOpen() *bool
	BrowseAskClear()
	BrowseClear()
	// OpenSettings shows the settings at section, as the message bar's 打开设置 asks.
	OpenSettings(section string)
}

// Show draws the browser across the content column: the toolbar along its top, the message bar, then the web page, or the start before a page is open, and the confirmation of 清除登录信息 over them.
func Show(c *ui.Context, t *ui.Theme, a App) {
	s := a.BrowseState()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		toolbar(c, t, a, s)
		notice(c, a)
		switch view := a.BrowseWebView(); {
		case !s.Page:
			start(c, a, s)
		case view != nil:
			ui.WebView(c, view).Grow(1).MinHeight(0)
		default:
			// The web view is made after this frame (state_browse.go); the page's room stays blank until then.
			ui.Column(c).Grow(1).MinHeight(0)
		}
	})
	clearDialog(c, a)
}
