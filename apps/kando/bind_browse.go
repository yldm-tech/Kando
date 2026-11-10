package main

import (
	"runtime"

	"kando/apps/kando/browse"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The browser's reads and clicks (browse.App).

func (a *app) BrowseState() browse.State {
	return browse.State{
		Page:         a.browseShowing(),
		Loading:      a.browse.loading,
		CanGoBack:    a.browse.canBack,
		CanGoForward: a.browse.canForward,
		Clearing:     a.browse.clearing,
	}
}

func (a *app) BrowseWebView() *mygo.WebView { return a.browse.view }

func (a *app) BrowseAddress() *string { return &a.browse.address }

func (a *app) BrowseAddressFocus() *bool { return &a.browse.focus }

func (a *app) BrowseAddressFocused(focused bool) { a.browse.focused = focused }

func (a *app) BrowseGo() { a.browseGo() }

func (a *app) BrowseBack() { a.browseBack() }

func (a *app) BrowseForward() { a.browseForward() }

func (a *app) BrowseReload() { a.browseReload() }

func (a *app) BrowseStop() { a.browseStop() }

func (a *app) BrowseHome() { a.browseHome() }

func (a *app) BrowseDownload() { a.browseDownload() }

func (a *app) BrowseNotice() *browse.Notice { return &a.browse.notice }

func (a *app) BrowseClearOpen() *bool { return &a.browse.clearOpen }

func (a *app) BrowseAskClear() { a.browseAskClear() }

func (a *app) BrowseClear() { a.browseClear() }

// browseColumn is the browser's page in the content column (mainColumn), from the column's top to its edges.
func (a *app) browseColumn(c *ui.Context, t *ui.Theme) {
	browse.Show(c, t, a)
}

// browseOwnsTitleBar reports whether the browser draws its own bar at the top of the content column, keeping clear of the window controls and dragging the window, so the content has no drag band over it (contentBand). It does on its page on macOS, where its toolbar lies beside the sidebar's card that holds the traffic lights; elsewhere the window controls take the top of the content, and the drag band keeps the toolbar under them.
func (a *app) browseOwnsTitleBar() bool { return a.page == pageBrowse && runtime.GOOS == "darwin" }
