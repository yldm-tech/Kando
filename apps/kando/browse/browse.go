// Package browse draws the built-in browser (浏览): a toolbar with back, forward, reload, home, the address field and 下载视频, the message bar under it, and the web page, or the page's start before a web page is open. The page's address, its state and what the buttons do belong to the app (state_browse.go); this package draws them, shows the page in NSGo's web view, puts the browser's own buttons into the pages of the sites it knows (buttons.go) and hands the clicks and the page's reports back.
package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/sites"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// State is what the toolbar and the page show of the browser.
type State struct {
	// Page is set while a web page is open; without one the page's start shows.
	Page bool
	// NotVideo is set while the page lists videos without being one, as a site's home page, its search results or a channel do: 下载视频 is off on it, and its tooltip says why.
	NotVideo bool
	// List is set while the page is a playlist or a channel: 下载视频 then lists its videos and asks before it adds them.
	List bool
	// Title names the page's video, "" while it is not known: the library's name for it once the library has it, and else the page's own title.
	Title string
	// Have is what the library holds of the page's video: "" nothing yet, HaveQueued a job waiting or being processed, HaveDone a film. 下载视频 then says so and shows it rather than adding it again.
	Have string
	// Loading is set while the page loads; reload then stops it.
	Loading      bool
	CanGoBack    bool
	CanGoForward bool
	// Clearable is set when the pages' sign-ins can be cleared at all (清除登录信息…), and Clearing while they are being cleared.
	Clearable bool
	Clearing  bool
	// SignIn names the browser whose sign-in the pages and downloads borrow (Settings › 通用 › 浏览器登录), "" while none is chosen; the start says it. SigningIn is set while its sign-in is being read or put into the pages, when 刷新登录 waits.
	SignIn    string
	SigningIn bool
}

// What State.Have says the library holds of the page's video.
const (
	HaveQueued = "queued"
	HaveDone   = "done"
)

// Notice is the message bar under the toolbar: Kind is "warning" or "success", Section the settings section that fixes what it says ("" when none does), Film the job its 在资料库中显示 shows (BrowseShowFilm, "" for none), List the playlist its Add Whole Playlist adds, and Open whether it shows.
type Notice struct {
	Kind    string
	Text    string
	Section string
	Film    string
	// List is the whole playlist the bar's Add Whole Playlist adds (BrowseAddList), as after one of its videos was added; "" for none.
	List string
	Open bool
}

// App is the browser's state the page reads, and where its clicks and the web page's reports go. The page reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	BrowseState() State
	// BrowseSource is the address the web view loads, "" until a web page is first asked for: the web view is made then, and loads its page again only when the source changes.
	BrowseSource() string
	// BrowseWeb is the web view's Ref, which the app drives the page through: back, forward, reload, stop and the scripts it runs in the page.
	BrowseWeb() *core.Ref
	// BrowseScript runs in each page before the page's own scripts; when it changes, the web view loads its source again with it, in place of the page it shows, keeping its history (core.WebView.InjectedJavaScriptBeforeContentFunc).
	BrowseScript() string
	// BrowseMuted is whether the web view keeps its pages quiet, as while another page of the window shows: Windows and Linux silence the whole view, frames and Web Audio included, and macOS suspends its media, both for what starts later too (core.WebView.Muted).
	BrowseMuted() bool
	// BrowseNavigated hears the page's address, title and history as it starts loading, finishes and navigates, within the document too, BrowseOpenWindow a window the page asks to open, which the platform never opens itself, BrowseFailed a page that failed to load or a navigation the web view refused, and BrowseMessage what a page posted to window.nsgo, with the page's origin.
	BrowseNavigated(nav core.WebNavigation)
	BrowseOpenWindow(req core.WebWindowRequest)
	BrowseFailed(err error)
	BrowseMessage(msg core.WebMessage)
	// BrowseAddress is the address field's text.
	BrowseAddress() *string
	// BrowseAddressFocus asks for the keyboard in the address field; the page gives it and clears the request.
	BrowseAddressFocus() *bool
	// BrowseAddressFocused hears whether the address field has the keyboard, as it takes and loses it.
	BrowseAddressFocused(focused bool)
	// BrowseGo opens what the address field holds: a web address, or a search for it.
	BrowseGo()
	BrowseBack()
	BrowseForward()
	BrowseReload()
	BrowseStop()
	// BrowseHome closes the web page and shows the page's start.
	BrowseHome()
	// BrowseDownload adds the page shown to the library's queue, or shows its video where the library has it already (State.Have).
	BrowseDownload()
	// BrowseShowFilm shows job id in the library: the film, once it is done, and else its card in 处理中.
	BrowseShowFilm(id string)
	BrowseNotice() *Notice
	// BrowseAddList lists the videos of the playlist at source and asks before it adds them (Notice.List).
	BrowseAddList(source string)
	// BrowseClearOpen reports whether the 清除登录信息 confirmation is open; BrowseAskClear opens it, BrowseClear clears once it is confirmed and BrowseCancelClear closes it without clearing.
	BrowseClearOpen() bool
	BrowseAskClear()
	BrowseClear()
	BrowseCancelClear()
	// BrowseRefreshSignIn reads the chosen browser's sign-in again and puts it into the pages (刷新登录), for a site the user signed in to there since.
	BrowseRefreshSignIn()
	// OpenSettings shows the settings at section, as the message bar's 打开设置 asks.
	OpenSettings(section string)
	// The start's sites: their logos, OpenSite, which opens one in the browser, and OpenSites, which opens the sheet that lists them all (支持的网站…).
	sites.Logos
	OpenSite(s catalog.Site)
	OpenSites()
}

// View is the browser across the content column: the toolbar along its top, the message bar, then the web page, or the start before a page is open, and the confirmation of 清除登录信息 over them.
type View struct {
	App App
}

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	a := v.App
	colors := ui.ColorsOf(cx)
	state := live.Memo(cx, a, a.BrowseState)
	// The web view is made once a page is first asked for and stays while the browser does, hidden behind the start after home, so back finds the pages before it, its script changing included (page).
	body := core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		Children: []core.Node{
			core.Show{When: live.Func(a, func() bool { return a.BrowseSource() != "" }), Then: func(*core.Scope) core.Node { return page{a} }},
			core.Show{When: func() bool { return !state().Page }, Then: func(*core.Scope) core.Node { return start{a} }},
		},
	}
	return core.View{
		TestID:    "browse",
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		StyleFunc: ui.Filled(colors.Background),
		Children:  []core.Node{toolbar{a}, notice{a}, body, clearDialog{a}},
	}
}
