package main

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"

	"kando/apps/kando/browse"
	"kando/apps/kando/locales"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// browseUI is the state of the built-in browser (browse/): the web view's source and Ref, what the toolbar shows of its page, the address field, the message bar and the 清除登录信息 confirmation.
type browseUI struct {
	// source is the address the web view loads (browse.App.BrowseSource): "" until the first web page opens, which makes the web view, and the web view loads its page again only when it changes. view is the web view's Ref, through which the browser drives its page; the web view lives as long as the browser's column, hidden behind the start after home, so back finds the pages before it.
	source string
	view   *core.Ref
	// clear deletes what the pages stored; nil where it cannot be done, which leaves 清除登录信息… out. The tests set a fake.
	clear func() error
	// url is the page's address, "" or about:blank on the start; loading, canBack and canForward are the page's as the web view last reported them (browseNavigated).
	url                 string
	loading             bool
	canBack, canForward bool
	// address is the address field's text. shown is the address the field last showed of the page: the field follows the page while it does not have the keyboard or still shows that, and keeps what the user is typing otherwise. focus asks for the keyboard in the field; focused is whether it has it.
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
	// agent keeps the pages' user agent for the downloads, which read it from the pipeline's goroutine.
	agent *browseAgent
	// muted is whether the page's sound is off, as it is while another page of the window shows (browseAfterChange).
	muted bool
}

func newBrowseUI() browseUI {
	agent := &browseAgent{}
	agent.set(browseUserAgent())
	return browseUI{view: &core.Ref{}, agent: agent}
}

// browseAgent is the user agent of the browser's pages, which a download sends with their cookies so the site sees the same browser that signed in. It is "" where the pages keep the platform's own, which the app does not know.
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

// browseHistoryMessage starts what browseScript has a page post when it changes its address without loading another page: "push", "replace" or "pop" follows it.
const browseHistoryMessage = "kando-history:"

// browseHooks: once the window exists the browser removes the cookies folders of downloads that never finished, as none runs yet; its ready runs before the queue's (areas), which starts the first job left waiting. After each change the confirmation closes when the window went to another page, and the page's sound is off while another page shows.
func (a *app) browseHooks() areaHooks {
	return areaHooks{
		ready: func() {
			if tools, ok := a.pipeline.(media.Tools); ok {
				media.SweepCookieDirs(tools.CookieDir)
				// TODO(nsgo#112): give every download the cookies the browser's pages hold for its site, tools.SiteCookies = browse.CookieSource over the web view's cookie store with a.browse.agent.get, as MyGo's Cookies.List gave them; until then a site the user signed in to here downloads signed out, and a download that needs the sign-in fails with the queue's remedy of borrowing a browser's (media.Tools.Cookies) https://github.com/nsgo-dev/nsgo/issues/112
			}
			// TODO(nsgo#114): clear the pages' cookies and storage (a.browse.clear), as MyGo's App.ClearBrowsingData did; until then 清除登录信息… is left out of the start and the File menu https://github.com/nsgo-dev/nsgo/issues/114
		},
		afterChange: a.browseAfterChange,
	}
}

// browseKeys are the browser's rows of the shortcut table, which work while its page shows: 打开位置… (⌘L) in the File menu with 清除登录信息…, which has no key, where the sign-ins can be cleared, and 重新载入 (⌘R), 后退 (⌘[) and 前进 (⌘]) in the View menu.
func (a *app) browseKeys(cx *core.Scope) []shortcut {
	shown := func() bool { return a.page == pageBrowse }
	keys := []shortcut{
		{Group: groupFile, Label: locales.BrowseOpenLocation(cx), Key: "mod+l", When: shown, Run: a.browseFocusAddress},
	}
	if a.browse.clear != nil {
		keys = append(keys, shortcut{Group: groupFile, Label: locales.BrowseClearData(cx), When: func() bool { return shown() && !a.browse.clearing }, Run: a.BrowseAskClear})
	}
	return append(keys,
		shortcut{Group: groupView, Label: locales.BrowseReload(cx), Key: "mod+r", When: func() bool { return shown() && a.browseShowing() }, Run: a.BrowseReload},
		shortcut{Group: groupView, Label: locales.BrowseBack(cx), Key: "mod+[", When: func() bool { return shown() && a.browse.canBack }, Run: a.BrowseBack},
		shortcut{Group: groupView, Label: locales.BrowseForward(cx), Key: "mod+]", When: func() bool { return shown() && a.browse.canForward }, Run: a.BrowseForward},
	)
}

// browseShowing reports whether a web page is open, rather than the start.
func (a *app) browseShowing() bool { return a.browse.url != "" && a.browse.url != browseBlank }

func (a *app) browseFocusAddress() { a.browse.focus = true }

// browseLoad opens rawURL, an http or https address, in the web view, which the first address makes.
func (a *app) browseLoad(rawURL string) {
	a.browse.url, a.browse.loading = rawURL, true
	a.browse.address, a.browse.shown = rawURL, rawURL
	a.browseDropLoadFailure()
	a.browseNavigate(rawURL)
}

// browseNavigate has the web view load rawURL. The web view loads its source as it changes; an address the source holds already, which the page has left since, the page itself goes to.
func (a *app) browseNavigate(rawURL string) {
	if rawURL != a.browse.source {
		a.browse.source = rawURL
		return
	}
	// TODO(nsgo): give core.Ref a LoadURL for web views, so the app loads an address again without the page's help; the web view loads only when its source changes, so the address its source holds already goes through a script of the page's
	quoted, _ := json.Marshal(rawURL)
	a.browse.view.InjectJavaScript("location.assign(" + string(quoted) + ")")
}

// browseAfterChange closes the confirmation, giving back the window's modal flag, when the window went to another page, which no longer draws it, and turns the page's sound off while another page shows and on again once the browser's shows, as the hidden web view keeps its page and a video in it plays on. It reports whether it closed the confirmation.
func (a *app) browseAfterChange() bool {
	changed := false
	if a.page != pageBrowse && a.browse.clearOpen {
		a.browseCancelClear()
		changed = true
	}
	if muted := a.page != pageBrowse; a.browse.muted != muted {
		a.browse.muted = muted
		if a.browse.source != "" {
			// TODO(nsgo#114): turn the page's sound off and on with the web view's own switch, as MyGo's Page.SetAudioMuted did, which also silences what plays in the page's frames and what starts while it is muted https://github.com/nsgo-dev/nsgo/issues/114
			a.browse.view.InjectJavaScript(browseMuteScript(muted))
		}
	}
	return changed
}

// browseMuteScript mutes the page's videos and sounds, marking the ones it muted, or unmutes those it marked, so what the page muted itself stays muted.
func browseMuteScript(muted bool) string {
	if muted {
		return `for (const m of document.querySelectorAll("video, audio")) { if (!m.muted) { m.muted = true; m.dataset.kandoMuted = "1"; } }`
	}
	return `for (const m of document.querySelectorAll("[data-kando-muted]")) { m.muted = false; delete m.dataset.kandoMuted; }`
}

// browseColumnGone runs when the browser's column goes with its web view: the web view made when it comes back loads the page that was shown, or the blank page under the start, though without the history before it.
func (a *app) browseColumnGone() {
	switch {
	case a.browseShowing():
		a.browse.source = a.browse.url
	case a.browse.source != "":
		a.browse.source = browseBlank
	}
	a.browse.loading, a.browse.canBack, a.browse.canForward = false, false, false
	a.browse.muted = false
}

// browseUserAgent is the pages' user agent: on macOS WKWebView's own, which names no browser, with Safari's version after it, as sites serve such a user agent older pages and refuse it their sign-in. WebKitGTK and WebView2 already name their browsers and keep their own ("").
func browseUserAgent() string {
	if runtime.GOOS == "darwin" {
		// TODO(nsgo#114): append Safari's version to the platform's own user agent, as MyGo's PageOptions.UserAgentSuffix did, rather than replacing it with WKWebView's as macOS writes it today https://github.com/nsgo-dev/nsgo/issues/114
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15"
	}
	return ""
}

// browseScript runs in each page before its own scripts. A link or a script that opens a window loads its address in the same view when the user clicked it, and does nothing otherwise, as a pop-up a script opens by itself, so the page never opens a window of the app; the web view's Access keeps it to http and https addresses. A page that changes its address without loading another, as a site that is one page does, says so (browseHistoryMessage), and the app reads the address from the page (browseHeard).
func browseScript() string {
	// TODO(nsgo#113): hear a page open a window, with whether the user asked for it, and change its address and title within the document from the web view itself, as MyGo's SetWindowOpenHandler and OnPageTitleUpdated did, then drop this script https://github.com/nsgo-dev/nsgo/issues/113
	return `(() => {
  const go = (raw) => {
    try {
      const u = new URL(raw, location.href);
      if (u.protocol === "http:" || u.protocol === "https:") location.assign(u.href);
    } catch (e) {}
  };
  document.addEventListener("click", (e) => {
    if (e.defaultPrevented || e.button !== 0 || !(e.target instanceof Element)) return;
    const a = e.target.closest("a[href]");
    if (!a) return;
    const target = (a.getAttribute("target") || "").toLowerCase();
    const opens = (target !== "" && target !== "_self" && target !== "_top" && target !== "_parent") || e.metaKey || e.ctrlKey || e.shiftKey;
    if (!opens) return;
    e.preventDefault();
    go(a.href);
  });
  window.open = function (url) {
    if (url && navigator.userActivation && navigator.userActivation.isActive) go(String(url));
    return null;
  };
  const post = (kind) => { try { window.nsgo.postMessage("` + browseHistoryMessage + `" + kind); } catch (e) {} };
  for (const [name, kind] of [["pushState", "push"], ["replaceState", "replace"]]) {
    const original = history[name];
    history[name] = function (...args) {
      const result = original.apply(this, args);
      post(kind);
      return result;
    };
  }
  addEventListener("popstate", () => post("pop"));
})();`
}

// browseNavigated follows the page's address and history as the web view reports them: as a page starts loading, finishes and navigates. The address field follows the page unless the user is typing in it.
func (a *app) browseNavigated(nav core.WebNavigation) {
	if nav.URL != "" {
		a.browse.url = nav.URL
	}
	a.browse.loading, a.browse.canBack, a.browse.canForward = nav.Loading, nav.CanGoBack, nav.CanGoForward
	a.browseFollow()
}

// browseFollow takes away a failure to load once a page shows, and has the address field follow the page's address unless the user is typing in it.
func (a *app) browseFollow() {
	shown := ""
	if a.browseShowing() {
		a.browseDropLoadFailure()
		shown = a.browse.url
	}
	if !a.browse.focused || a.browse.address == a.browse.shown {
		a.browse.address = shown
	}
	a.browse.shown = shown
}

// browseFailed says in the message bar that a page did not load, and why, as the web view reports it. A navigation the web view refused, to a file, the app's own schemes or another app's, says nothing, as it never started.
func (a *app) browseFailed(err error) {
	var failed *core.WebError
	if errors.As(err, &failed) && failed.URL != "" && failed.URL != browseBlank && !browse.Allowed(failed.URL) {
		return
	}
	a.browse.loading = false
	// TODO(nsgo): give core.WebError the platform's error code, so a load the user cut short by going elsewhere (NSURLErrorCancelled, -999) or one that turned into a file (WebKit's 102, frame load interrupted) says nothing, as MyGo dropped them; until then the bar says so, until the next page starts loading
	a.browseLoadFailed(err.Error())
}

// browseHeard is what a page posted: that it changed its address without loading another page (browseScript), as the web view reports nothing then. The address is read from the page itself rather than taken from what it posted, which any page could fill with another site's address. A page that pushed an address has the one it left to go back to, and none to go forward to.
func (a *app) browseHeard(data string) {
	kind, ok := strings.CutPrefix(data, browseHistoryMessage)
	if !ok {
		return
	}
	a.browse.view.Evaluate("location.href", func(raw string, err error) {
		var u string
		if err != nil || json.Unmarshal([]byte(raw), &u) != nil || !browse.Allowed(u) {
			return
		}
		a.browse.url = u
		if kind == "push" {
			a.browse.canBack, a.browse.canForward = true, false
		}
		a.browseFollow()
		a.Changed()
	})
}

// browseGo opens what the address field holds, an address or a search for it in the interface's language.
func (a *app) browseGo() {
	text := strings.TrimSpace(a.browse.address)
	if text == "" {
		return
	}
	a.browseLoad(browse.Target(text, a.prefs.interfaceLanguage))
}

// browseDownload adds the page shown to the library's queue, read from the page itself, as a site that changes its page without loading another may not have said so; the address the web view last reported stands in when the page cannot answer.
func (a *app) browseDownload() {
	if !a.browseShowing() {
		return
	}
	a.browse.view.Evaluate("location.href", func(raw string, err error) {
		u := a.browse.url
		var now string
		if err == nil && json.Unmarshal([]byte(raw), &now) == nil && browse.Allowed(now) {
			u = now
		}
		a.browseEnqueue(u)
		a.Changed()
	})
}

// browseEnqueue adds u to the library's queue, as the library's link field does: what precheck finds wrong shows in the message bar with the setting that fixes it, and the pipeline's own checks, with the remedies of a failed job, take care of a page without a video it can download.
func (a *app) browseEnqueue(u string) {
	if !browse.Allowed(u) {
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
	a.browseTell(a.messages().BrowseAdded())
}

// browseAskClear opens the confirmation of 清除登录信息…, which holds the window's modal flag while it is open.
func (a *app) browseAskClear() {
	if a.browse.clearOpen || a.browse.clearing || a.browse.clear == nil {
		return
	}
	a.browse.clearOpen = true
	a.browse.release = a.holdModal()
}

// browseCancelClear closes the confirmation and gives back the window's modal flag.
func (a *app) browseCancelClear() {
	a.browse.clearOpen = false
	if a.browse.release != nil {
		a.browse.release()
		a.browse.release = nil
	}
}

// browseClear closes the confirmation and deletes the cookies and storage of the browser's pages off the engine's goroutine, as it waits for the web engine, then says how it went and loads the page shown again, which keeps what it held until it does.
func (a *app) browseClear() {
	a.browseCancelClear()
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
		a.browseWarn(a.messages().BrowseClearFailed(err.Error()), "")
		return
	}
	a.browseTell(a.messages().BrowseCleared())
	if a.browseShowing() {
		a.browse.view.Reload()
		a.browse.loading = true
	}
}

func (a *app) browseBack() {
	if a.browse.source != "" && a.browse.canBack {
		a.browse.view.GoBack()
		a.browse.loading = true
	}
}

func (a *app) browseForward() {
	if a.browse.source != "" && a.browse.canForward {
		a.browse.view.GoForward()
		a.browse.loading = true
	}
}

func (a *app) browseReload() {
	if a.browseShowing() {
		a.browse.view.Reload()
		a.browse.loading = true
	}
}

func (a *app) browseStop() {
	a.browse.view.StopLoading()
	a.browse.loading = false
}

// browseHome shows the start: the web view loads the blank page, which stops the page shown, sound and all, and keeps it in the history for back. Before there is a web view it only clears the address.
func (a *app) browseHome() {
	a.browse.address, a.browse.shown = "", ""
	a.browse.loading = false
	a.browseDropLoadFailure()
	if a.browse.source == "" {
		a.browse.url = ""
		return
	}
	a.browse.url = browseBlank
	a.browse.view.StopLoading()
	a.browseNavigate(browseBlank)
}

// browseWarn and browseTell show the browser's message bar: something that went wrong, with the settings section that fixes it ("" when none does), or something that worked. The text is said already, in the interface's language.
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
	a.browseWarn(a.messages().BrowseLoadFailed(reason), "")
	a.browse.loadFailed = true
}

// browseDropLoadFailure closes the message bar when it says a page did not load, as another page is on its way.
func (a *app) browseDropLoadFailure() {
	if a.browse.loadFailed {
		a.browse.notice.Open = false
		a.browse.loadFailed = false
	}
}
