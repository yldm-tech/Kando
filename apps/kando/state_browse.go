package main

import (
	"cmp"
	"context"
	"net"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"kando/apps/kando/browse"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"golang.org/x/net/publicsuffix"
)

// browsePage is what the browser drives of its web view's page. *mygo.Page is one; the tests give a fake, as they have no window to make a web view in.
type browsePage interface {
	LoadURL(rawURL string) error
	GoBack()
	GoForward()
	Reload()
	Stop()
	URL() string
	CanGoBack() bool
	CanGoForward() bool
	IsLoading() bool
	UserAgent() string
	SetAudioMuted(muted bool)
}

// browseUI is the state of the built-in browser (browse/): its web view, once made, what the toolbar shows of its page, the address field, the message bar and the 清除登录信息 confirmation.
type browseUI struct {
	// page and view are the web view and its page, nil until the first web page opens; the web view lives as long as the window, hidden while another page shows, so coming back finds the page where it was.
	page browsePage
	view *mygo.WebView
	// open makes the web view, loading rawURL, and clear deletes what the pages stored; browseHooks' ready sets both, and the tests set fakes. Without open no web view is made.
	open  func(rawURL string) (browsePage, *mygo.WebView, error)
	clear func() error
	// pending is the address to load once the web view is made, after the frame that asked for it (browseAfterFrame).
	pending string
	// url is the page's address, "" or about:blank on the start; loading, canBack and canForward are the page's as last read (browseRefresh).
	url                 string
	loading             bool
	canBack, canForward bool
	// address is the address field's text. shown is the address the field last showed of the page: the field follows the page while it does not have the keyboard or still shows that, and keeps what the user is typing otherwise. focus asks for the keyboard in the field; focused is whether it had it in the last frame.
	address string
	shown   string
	focus   bool
	focused bool
	notice  browse.Notice
	// loadFailed marks the message bar as a page that failed to load, which the next page that opens takes away.
	loadFailed bool
	// clearOpen is the confirmation's open flag, and release gives back the window's modal flag it holds (holdModal); clearing is set while clear runs.
	clearOpen bool
	release   func()
	clearing  bool
	// agent keeps the web view's user agent for the downloads, which read it from the pipeline's goroutine.
	agent *browseAgent
	// muted is whether the page's sound is off, as it is while another page of the window shows (browseAfterFrame).
	muted bool
}

func newBrowseUI() browseUI { return browseUI{agent: &browseAgent{}} }

// browseAgent is the user agent of the browser's pages, which a download sends with their cookies so the site sees the same browser that signed in. It is "" until a page has loaded.
type browseAgent struct {
	mu sync.Mutex
	ua string
}

func (b *browseAgent) set(ua string) {
	b.mu.Lock()
	b.ua = ua
	b.mu.Unlock()
}

func (b *browseAgent) get() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ua
}

// browseBlank is the page home loads, which shows the start; the page's history keeps the pages before it, so back returns to them.
const browseBlank = "about:blank"

// browseHooks: once the window exists the browser can make its web view and clear its data, and every download asks the pages' cookies for its site (media.Tools.SiteCookies). Its ready runs before the queue's (areas), which starts the first job left waiting, so that job gets the cookies too; before that it removes the cookies folders of downloads that never finished, as none runs yet. After each frame the confirmation gives back the modal flag once it has closed, a web view asked for is made, and the page's sound is off while another page shows.
func (a *app) browseHooks() areaHooks {
	return areaHooks{
		ready: func() {
			a.browse.open = a.browseOpenWebView
			a.browse.clear = mygo.App.ClearBrowsingData
			if tools, ok := a.pipeline.(media.Tools); ok {
				media.SweepCookieDirs(tools.CookieDir)
				tools.SiteCookies = browseCookieSource(mygo.Cookies.List, a.browse.agent)
				a.pipeline = tools
			}
		},
		afterFrame: a.browseAfterFrame,
	}
}

// browseKeys are the browser's rows of the shortcut table, which work while its page shows: 打开位置… (⌘L) in the File menu with 清除登录信息…, which has no key, and 重新载入 (⌘R), 后退 (⌘[) and 前进 (⌘]) in the View menu.
func (a *app) browseKeys() []shortcut {
	shown := func() bool { return a.page == pageBrowse }
	return []shortcut{
		{Group: groupFile, Label: locales.BrowseOpenLocation(), Mods: ui.Cmd, Key: ui.KeyL, When: shown, Run: a.browseFocusAddress},
		{Group: groupFile, Label: locales.BrowseClearData(), When: func() bool { return shown() && !a.browse.clearing }, Run: a.BrowseAskClear},
		{Group: groupView, Label: locales.BrowseReload(), Mods: ui.Cmd, Key: ui.KeyR, When: func() bool { return shown() && a.browseShowing() }, Run: a.BrowseReload},
		{Group: groupView, Label: locales.BrowseBack(), Mods: ui.Cmd, Key: ui.KeyBracketLeft, When: func() bool { return shown() && a.browse.canBack }, Run: a.BrowseBack},
		{Group: groupView, Label: locales.BrowseForward(), Mods: ui.Cmd, Key: ui.KeyBracketRight, When: func() bool { return shown() && a.browse.canForward }, Run: a.BrowseForward},
	}
}

// browseShowing reports whether a web page is open, rather than the start.
func (a *app) browseShowing() bool { return a.browse.url != "" && a.browse.url != browseBlank }

func (a *app) browseFocusAddress() { a.browse.focus = true }

// browseLoad opens rawURL, an http or https address: in the web view, or, before there is one, in the web view browseAfterFrame makes after this frame.
func (a *app) browseLoad(rawURL string) {
	a.browse.url, a.browse.loading = rawURL, true
	a.browse.address, a.browse.shown = rawURL, rawURL
	a.browseDropLoadFailure()
	if a.browse.page == nil {
		a.browse.pending = rawURL
		return
	}
	if err := a.browse.page.LoadURL(rawURL); err != nil {
		a.browseLoadFailed(err.Error())
	}
}

// browseAfterFrame gives back the window's modal flag once the confirmation has closed, closing it first when the window went to another page, which no longer draws it, turns the page's sound off while another page shows and on again once the browser's shows, as the hidden web view keeps its page and a video in it plays on, and makes the web view a load asked for.
func (a *app) browseAfterFrame() {
	if a.page != pageBrowse {
		a.browse.clearOpen = false
	}
	if !a.browse.clearOpen && a.browse.release != nil {
		a.browse.release()
		a.browse.release = nil
	}
	if muted := a.page != pageBrowse; a.browse.page != nil && a.browse.muted != muted {
		a.browse.page.SetAudioMuted(muted)
		a.browse.muted = muted
	}
	if a.browse.pending == "" || a.browse.page != nil || a.browse.open == nil {
		return
	}
	rawURL := a.browse.pending
	a.browse.pending = ""
	page, view, err := a.browse.open(rawURL)
	if err != nil {
		a.browse.url, a.browse.loading = "", false
		a.browseLoadFailed(err.Error())
		return
	}
	a.browse.page, a.browse.view = page, view
}

// browseOpenWebView makes the browser's web view in the main window and loads rawURL in it. Its pages are someone else's (PageOptions.Untrusted): they call no Go and hear no event of the app. Such a page may only leave its own site with a listener of OnWillNavigate, which lets it go to http and https addresses and nowhere else, as a file dropped on it; a link that opens a window opens in the same view when the user clicked it, and never in a window of the app. The page uses the app's default session, whose cookies stay after the app quits and which the downloads read.
func (a *app) browseOpenWebView(rawURL string) (browsePage, *mygo.WebView, error) {
	view, err := a.win.NewWebView(mygo.WebViewOptions{
		BackgroundColor: "#FFFFFF",
		Page: mygo.PageOptions{
			Untrusted:       true,
			UserAgentSuffix: browseUserAgentSuffix(),
			SwipeNavigation: true,
			PinchZoom:       true,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	page := view.Page()
	page.OnWillNavigate(a.browseWillNavigate)
	page.OnDidNavigate(func(string) { a.update(a.browseRefresh) })
	page.OnPageTitleUpdated(func(*mygo.TitleEvent) { a.update(a.browseRefresh) })
	page.OnDidFinishLoad(func() { a.update(a.browseRefresh) })
	page.OnDidFailLoad(func(e *mygo.LoadError) {
		reason := cmp.Or(e.Description, strconv.Itoa(e.Code))
		a.update(func() {
			a.browseRefresh()
			a.browseLoadFailed(reason)
		})
	})
	page.SetWindowOpenHandler(a.browseWindowOpen)
	if err := page.LoadURL(rawURL); err != nil {
		view.Destroy()
		return nil, nil, err
	}
	a.win.Invalidate()
	return page, view, nil
}

// browseWillNavigate is the page's OnWillNavigate listener: it lets the page go to http and https addresses and stops it going anywhere else, as a file, the app's own schemes or another app's.
func (a *app) browseWillNavigate(e *mygo.NavigateEvent) {
	if !browseAllowed(e.URL) {
		e.PreventDefault()
		return
	}
	a.update(func() { a.browse.loading = true })
}

// browseWindowOpen is the page's window open handler: a link or script that opens a window loads its http or https address in the same view when the user clicked it, and does nothing otherwise, as a pop-up a script opens by itself. It always answers nil, which refuses the window, so the page never opens a window of the app.
func (a *app) browseWindowOpen(req mygo.WindowOpenRequest) *mygo.WindowOptions {
	if req.UserInitiated && browseAllowed(req.URL) {
		a.update(func() { a.browseLoad(req.URL) })
	}
	return nil
}

// browseUserAgentSuffix is what the pages' user agent ends with: on macOS Safari's version, as sites serve a WKWebView, whose user agent names no browser, older pages and refuse it their sign-in. WebKitGTK and WebView2 already name their browsers.
func browseUserAgentSuffix() string {
	if runtime.GOOS == "darwin" {
		return "Version/26.0 Safari/605.1.15"
	}
	return ""
}

// browseRefresh reads the page's address and state after it navigated, finished or failed to load, or changed its title, as a site that changes its page without loading another does, and keeps the user agent its downloads send. The address field follows the page unless the user is typing in it.
func (a *app) browseRefresh() {
	p := a.browse.page
	if p == nil {
		return
	}
	if u := p.URL(); u != "" {
		a.browse.url = u
	}
	a.browse.loading, a.browse.canBack, a.browse.canForward = p.IsLoading(), p.CanGoBack(), p.CanGoForward()
	if a.browseShowing() {
		a.browseDropLoadFailure()
	}
	shown := ""
	if a.browseShowing() {
		shown = a.browse.url
	}
	if !a.browse.focused || a.browse.address == a.browse.shown {
		a.browse.address = shown
	}
	a.browse.shown = shown
	if ua := p.UserAgent(); ua != "" {
		a.browse.agent.set(ua)
	}
}

// browseGo opens what the address field holds, an address or a search for it.
func (a *app) browseGo() {
	text := strings.TrimSpace(a.browse.address)
	if text == "" {
		return
	}
	a.browseLoad(browseTarget(text, i18n.Locale()))
}

// browseTarget is the address the address field's text opens in the interface's language: the text itself when it is an http or https address, the text after https:// when it reads as a host, as example.com/watch or localhost:8080, and otherwise a search for it, on Bing in simplified Chinese, where Google does not answer, and on Google elsewhere.
func browseTarget(text, language string) string {
	if u, err := url.Parse(text); err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != "" {
		return text
	}
	if core.LooksLikeHost(text) {
		return "https://" + text
	}
	search := "https://www.google.com/search?q="
	if language == "zh-Hans" {
		search = "https://www.bing.com/search?q="
	}
	return search + url.QueryEscape(text)
}

// browseAllowed reports whether a page may go to rawURL: only http and https addresses, not files, the app's own schemes or other apps'.
func browseAllowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != ""
}

// browseCurrentURL is the address of the page shown, read from the page itself, as a site that changes its page without loading another tells nothing; "" on the start.
func (a *app) browseCurrentURL() string {
	if !a.browseShowing() {
		return ""
	}
	if p := a.browse.page; p != nil {
		if u := p.URL(); u != "" && u != browseBlank {
			return u
		}
	}
	return a.browse.url
}

// browseDownload adds the page shown to the library's queue, as the library's link field does: what precheck finds wrong shows in the message bar with the setting that fixes it, and the pipeline's own checks, with the remedies of a failed job, take care of a page without a video it can download.
func (a *app) browseDownload() {
	u := a.browseCurrentURL()
	if !browseAllowed(u) {
		return
	}
	if msg, section := a.precheck([]string{u}); msg != "" {
		a.browseWarn(msg, section)
		return
	}
	if err := a.Enqueue(u); err != nil {
		a.browseWarn(sourceError(err), "")
		return
	}
	a.browseTell(locales.BrowseAdded())
}

// browseAskClear opens the confirmation of 清除登录信息…, which holds the window's modal flag while it is open.
func (a *app) browseAskClear() {
	if a.browse.clearOpen || a.browse.clearing {
		return
	}
	a.browse.clearOpen = true
	a.browse.release = a.holdModal()
}

// browseClear deletes the cookies and storage of the browser's pages off the UI thread, as it waits for the web engine, then says how it went and loads the page shown again, which keeps what it held until it does.
func (a *app) browseClear() {
	if a.browse.clearing || a.browse.clear == nil {
		return
	}
	a.browse.clearing = true
	clear, update := a.browse.clear, a.update
	go func() {
		err := clear()
		update(func() { a.browseCleared(err) })
	}()
}

func (a *app) browseCleared(err error) {
	a.browse.clearing = false
	if err != nil {
		a.browseWarn(locales.BrowseClearFailed(err.Error()), "")
		return
	}
	a.browseTell(locales.BrowseCleared())
	if p := a.browse.page; p != nil && a.browseShowing() {
		p.Reload()
		a.browse.loading = true
	}
}

func (a *app) browseBack() {
	if p := a.browse.page; p != nil && a.browse.canBack {
		p.GoBack()
		a.browse.loading = true
	}
}

func (a *app) browseForward() {
	if p := a.browse.page; p != nil && a.browse.canForward {
		p.GoForward()
		a.browse.loading = true
	}
}

func (a *app) browseReload() {
	if p := a.browse.page; p != nil && a.browseShowing() {
		p.Reload()
		a.browse.loading = true
	}
}

func (a *app) browseStop() {
	if p := a.browse.page; p != nil {
		p.Stop()
	}
	a.browse.loading = false
}

// browseHome shows the start: the web view loads the blank page, which stops the page shown, sound and all, and keeps it in the history for back. Before the web view is made it only forgets the load it waited for.
func (a *app) browseHome() {
	a.browse.pending = ""
	a.browse.address, a.browse.shown = "", ""
	a.browse.loading = false
	a.browseDropLoadFailure()
	if a.browse.page == nil {
		a.browse.url = ""
		return
	}
	a.browse.url = browseBlank
	a.browse.page.Stop()
	if err := a.browse.page.LoadURL(browseBlank); err != nil {
		a.browseLoadFailed(err.Error())
	}
}

// browseWarn and browseTell show the browser's message bar: something that went wrong, with the settings section that fixes it ("" when none does), or something that worked.
func (a *app) browseWarn(text, section string) {
	a.browse.notice = browse.Notice{Kind: "warning", Text: text, Section: section, Open: true}
	a.browse.loadFailed = false
}

func (a *app) browseTell(text string) {
	a.browse.notice = browse.Notice{Kind: "success", Text: text, Open: true}
	a.browse.loadFailed = false
}

// browseLoadFailed says in the message bar that a page did not load, and why.
func (a *app) browseLoadFailed(reason string) {
	a.browseWarn(locales.BrowseLoadFailed(reason), "")
	a.browse.loadFailed = true
}

// browseDropLoadFailure closes the message bar when it says a page did not load, as another page is on its way.
func (a *app) browseDropLoadFailure() {
	if a.browse.loadFailed {
		a.browse.notice.Open = false
		a.browse.loadFailed = false
	}
}

// browseCookieSource is media.Tools.SiteCookies over list, which lists the cookies of the browser's pages (mygo.Cookies.List): for a source on a site, the cookies of its registrable domain and every host under it, so a video on www.example.com gets the sign-in example.com's accounts page left; for an IP address or a host without a public suffix, as localhost, those a request to it would send. It runs on the pipeline's goroutine; agent gives the pages' user agent, "" before any page loaded. A source that is not an http or https address gets none.
func browseCookieSource(list func(mygo.CookieFilter) ([]mygo.Cookie, error), agent *browseAgent) func(context.Context, string) ([]media.Cookie, string, error) {
	return func(_ context.Context, source string) ([]media.Cookie, string, error) {
		u, err := url.Parse(source)
		if err != nil || !browseAllowed(source) || u.Hostname() == "" {
			return nil, "", nil
		}
		filter := mygo.CookieFilter{URL: source}
		if host := strings.ToLower(u.Hostname()); net.ParseIP(host) == nil {
			if site, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
				filter = mygo.CookieFilter{Domain: site}
			}
		}
		cookies, err := list(filter)
		if err != nil {
			return nil, "", err
		}
		out := make([]media.Cookie, 0, len(cookies))
		for _, c := range cookies {
			out = append(out, media.Cookie{Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HTTPOnly, Expires: c.Expires, Name: c.Name, Value: c.Value})
		}
		return out, agent.get(), nil
	}
}
