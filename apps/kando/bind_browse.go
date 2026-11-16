package main

import (
	"runtime"

	"kando/apps/kando/browse"
	"kando/apps/kando/live"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// The browser's reads and clicks (browse.App). The browser reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// BrowseState names the page's video and says what the library has of it only while the browser's page shows, as the library's jobs are looked through for it each time the app changes.
func (a *app) BrowseState() browse.State {
	s := browse.State{
		Page:         a.browseShowing(),
		NotVideo:     a.browseShowing() && a.browseNotVideo(a.browse.url),
		List:         a.browseShowing() && a.browseList(a.browse.url),
		Loading:      a.browse.loading,
		CanGoBack:    a.browse.canBack,
		CanGoForward: a.browse.canForward,
		Clearable:    a.browse.clear != nil,
		Clearing:     a.browse.clearing,
	}
	if a.signin.browser != "" {
		s.SignIn, s.SigningIn = media.BrowserName(a.signin.browser), a.browseSigningIn()
	}
	if s.Page && !s.NotVideo && !s.List && a.page == pageBrowse {
		s.Title, s.Have = a.browseVideo()
	}
	return s
}

func (a *app) BrowseSource() string { return a.browse.source }

func (a *app) BrowseWeb() *core.Ref { return a.browse.view }

func (a *app) BrowseUserAgent() string { return a.browse.agent.get() }

func (a *app) BrowseScript() string { return browse.Script() }

func (a *app) BrowseNavigated(nav core.WebNavigation) { a.browseNavigated(nav) }

func (a *app) BrowseFailed(err error) { a.browseFailed(err) }

func (a *app) BrowseMessage(msg core.WebMessage) { a.browseHeard(msg) }

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

func (a *app) BrowseShowFilm(id string) { a.browseShowFilm(id) }

// BrowseAddList lists the videos of the playlist at source and asks before it adds them, as the message bar's Add Whole Playlist does after one of its videos was added.
func (a *app) BrowseAddList(source string) {
	a.browse.notice.Open = false
	a.browseEnqueue(source, "")
}

func (a *app) BrowseNotice() *browse.Notice { return &a.browse.notice }

func (a *app) BrowseClearOpen() bool { return a.browse.clearOpen }

func (a *app) BrowseAskClear() { a.browseAskClear() }

func (a *app) BrowseClear() { a.browseClear() }

func (a *app) BrowseCancelClear() { a.browseCancelClear() }

func (a *app) BrowseRefreshSignIn() { a.browseRefreshSignIn() }

// browseColumn is the browser's page in the content column (mainColumn), from the column's top to its edges, under cx, the scope view builds it in. It is hidden while another page shows, and should the column go, as when view builds it only while the browser shows, the web view goes with it (browseColumnGone).
func (a *app) browseColumn(cx *core.Scope) core.Node {
	cx.Cleanup(a.browseColumnGone)
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		StyleFunc: live.Func(a, func() core.Style {
			if a.page != pageBrowse {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}),
		Children: []core.Node{browse.View{App: a}},
	}
}

// browseOwnsTitleBar reports whether the browser draws its own bar at the top of the content column, keeping clear of the window controls and dragging the window, so the content has no drag band over it (contentBand). It does on its page on macOS, where its toolbar lies beside the sidebar's card that holds the traffic lights; elsewhere the window controls take the top of the content, and the drag band keeps the toolbar under them.
func (a *app) browseOwnsTitleBar() bool { return a.page == pageBrowse && runtime.GOOS == "darwin" }
