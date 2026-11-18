package main

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"kando/apps/kando/browse"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"
	kcore "kando/packages/core"
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
	// title is the title of the page at titleURL, as the web view reported it once the page loaded or the page said it changed (browse.TitleMessage); it names the page's video while titleURL is the page shown.
	title, titleURL string
	// address is the address field's text. shown is the address the field last showed of the page: the field follows the page while it does not have the keyboard or still shows that, and keeps what the user is typing otherwise. focus asks for the keyboard in the field; focused is whether it has it.
	address string
	shown   string
	focus   bool
	focused bool
	notice  browse.Notice
	// noticeURL is the page the message bar speaks of, as 下载视频's 已加入资料库, and the bar closes once another page shows; "" for a message about no page. noticeReady marks a warning of precheckReady, which the bar follows until it is fixed (precheckFollow). noticeTimer closes a message that something worked after noticeFor (noticeLater).
	noticeURL   string
	noticeReady bool
	noticeTimer *film.Timer
	// loadFailed marks the message bar as a page that failed to load, which the next load that starts takes away. cutShort is set once the app cut a load short, as Stop, home or another address does, so that load's failure says nothing; the next web page that starts loading clears it.
	loadFailed bool
	cutShort   bool
	// clearOpen is the confirmation's open flag, and release gives back the window's modal flag it holds (holdModal); clearing is set while clear runs.
	clearOpen bool
	release   func()
	clearing  bool
	// agent keeps the pages' user agent for the downloads, which read it from the pipeline's goroutine.
	agent *browseAgent
	// quiet is whether the page's videos and sounds are paused and muted, as they are while another page of the window shows (browseAfterChange).
	quiet bool
	// buttons is what the page's own buttons were told (state_browse_buttons.go).
	buttons browseButtons
	// signin is what the browser knows of its pages' sign-in (state_browse_signin.go).
	signin browseSignInState
}

func newBrowseUI() browseUI {
	agent := &browseAgent{}
	agent.set(browse.UserAgent())
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

// browseHooks: once the window exists the browser removes the cookies folders of downloads that never finished, as none runs yet; its ready runs before the queue's (areas), which starts the first job left waiting. It reads nothing of the chosen browser's sign-in then: that waits for the browser's page to show (browseSignInOnOpen). After each change the confirmation closes when the window went to another page, the page's videos are paused and its sound off while another page shows, and the first time the browser's page shows in a launch its pages take the chosen browser's sign-in.
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

// browseKeys are the browser's rows of the shortcut table, which work while its page shows: 打开位置… (⌘L) and 下载视频 (⌘D), on a page of one video the library does not have yet, in the File menu with 清除登录信息…, which has no key, where the sign-ins can be cleared, and 重新载入 (⌘R), 后退 (⌘[) and 前进 (⌘]) in the View menu.
func (a *app) browseKeys(cx *core.Scope) []shortcut {
	shown := func() bool { return a.page == pageBrowse }
	keys := []shortcut{
		{Group: groupFile, Label: locales.BrowseOpenLocation(cx), Key: "mod+l", When: shown, Run: a.browseFocusAddress},
		{Group: groupFile, Label: locales.BrowseDownload(cx), Key: "mod+d", When: func() bool { return shown() && a.browseCanDownload() }, Run: a.BrowseDownload},
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

// browseCanDownload reports whether 下载视频 adds the page shown: a page of one video, which the library has no job for yet that is waiting, running or done.
func (a *app) browseCanDownload() bool {
	if !a.browseShowing() || a.browseNotVideo(a.browse.url) {
		return false
	}
	_, have := a.browseJob(a.browse.url)
	return !have
}

// browseJob is the open library's job for the video at u that is waiting, being processed or done (queueFindSource); a failed or cancelled one does not count, as 下载视频 queues it again on its card.
func (a *app) browseJob(u string) (kcore.Job, bool) {
	job, ok := a.queueFindSource(u)
	return job, ok && job.Stage != kcore.StageFailed
}

// browseVideo is what the toolbar knows of the page's video (browse.State's Title and Have): its name, the library's once the library has it and the page's title otherwise, and whether the library has it queued or done.
func (a *app) browseVideo() (title, have string) {
	if a.browse.titleURL == a.browse.url {
		title = a.browse.title
	}
	job, ok := a.browseJob(a.browse.url)
	if !ok {
		return title, ""
	}
	if strings.TrimSpace(job.Title) != "" || job.VideoPath != "" {
		title = a.libraryJobName(job)
	}
	if job.Stage == kcore.StageComplete {
		return title, browse.HaveDone
	}
	return title, browse.HaveQueued
}

// browseShowFilm goes to the library and shows job id there, as the message bar's 在资料库中显示 and 下载视频 on a video the library has do: the film opens once it is done, and otherwise 处理中 shows every card, the job's among them. A job no longer in the open library only shows the library.
func (a *app) browseShowFilm(id string) {
	a.Select(pageFilms)
	item, ok := films.Find(a.films(), id)
	switch {
	case !ok:
	case films.InQueue(item) || item.VideoPath == "":
		a.lib.queueOpen = true
	default:
		a.Open(item)
	}
}

// browseLoad opens rawURL, an http or https address, in the web view, which the first address makes.
func (a *app) browseLoad(rawURL string) {
	if a.browseOffline() {
		return
	}
	a.browseCutShort()
	a.browse.url, a.browse.loading = rawURL, true
	a.browse.address, a.browse.shown = rawURL, rawURL
	a.browseDropLoadFailure()
	a.browseNavigate(rawURL)
}

// browseNavigate has the web view load rawURL. The web view loads its source as it changes; an address the source holds already, which the page has left since, the web view loads again itself (core.Ref.Load).
func (a *app) browseNavigate(rawURL string) {
	if rawURL != a.browse.source {
		a.browse.source = rawURL
		return
	}
	a.browse.view.Load(core.WebSrc(rawURL))
}

// browseAfterChange closes the confirmation, giving back the window's modal flag, when the window went to another page, which no longer draws it, pauses the page's videos and turns its sound off while another page shows, and turns the sound on again without playing them once the browser's shows, as the hidden web view keeps its page and a video in it plays on, keeps the message bar about what is still so (browseFollowNotice), tells the page's own buttons what changed for them (browsePagePush) and reads the chosen browser's sign-in into the pages the first time the browser's page shows (browseSignInOnOpen). It reports whether it closed the confirmation, changed the bar or started reading the sign-in.
func (a *app) browseAfterChange() bool {
	changed := a.browseFollowNotice()
	if a.browseSignInOnOpen() {
		changed = true
	}
	if a.page != pageBrowse && a.browse.clearOpen {
		a.browseCancelClear()
		changed = true
	}
	if quiet := a.page != pageBrowse; a.browse.quiet != quiet {
		a.browse.quiet = quiet
		if a.browse.source != "" {
			// TODO(nsgo#114): turn the page's sound off and on with the web view's own switch, as MyGo's Page.SetAudioMuted did, which also silences what plays in the page's frames and what starts while it is muted https://github.com/nsgo-dev/nsgo/issues/114
			a.browse.view.InjectJavaScript(browse.QuietScript(quiet))
		}
	}
	a.browsePagePush()
	return changed
}

// browseFollowNotice closes the message bar once what it says is no longer so: a message about a page once another page shows, or the start, and a warning of precheckReady once it is fixed, which until then says what is missing now. It reports whether it changed the bar.
func (a *app) browseFollowNotice() bool {
	n := &a.browse.notice
	switch {
	case !n.Open:
		return false
	case a.browse.noticeURL != "":
		if a.browse.url == a.browse.noticeURL {
			return false
		}
		n.Open = false
		return true
	case a.browse.noticeReady:
		return a.precheckFollow(&n.Text, &n.Section, &n.Open)
	}
	return false
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
	a.browse.focused, a.browse.quiet, a.browse.cutShort = false, false, false
	a.browse.buttons = browseButtons{}
}

// browseNavigated follows the page's address and history as the web view reports them: as a page starts loading, finishes and navigates. The address field follows the page unless the user is typing in it. A page of the page's own making (data:, blob:, about: other than the start's blank page), where the web view lets it go (browse/page.go), keeps the last web address, which 下载视频 then adds. A load that starts takes away the message that a page did not load.
func (a *app) browseNavigated(nav core.WebNavigation) {
	// A page the web view went to by itself while offline mode is on, as one a swipe through the history or a script opened, is left for the start at once (browseOffline).
	if a.privacy.offline && browse.Allowed(nav.URL) && a.browse.source != "" {
		a.browse.canBack, a.browse.canForward = nav.CanGoBack, nav.CanGoForward
		a.browseHome()
		return
	}
	if nav.URL == browseBlank || browse.Allowed(nav.URL) {
		a.browse.url = nav.URL
	}
	// The page's title is its own once it has loaded; as it starts loading the web view may still report the title of the page before it.
	if !nav.Loading && browse.Allowed(nav.URL) {
		a.browse.title, a.browse.titleURL = strings.TrimSpace(nav.Title), nav.URL
	}
	a.browse.loading, a.browse.canBack, a.browse.canForward = nav.Loading, nav.CanGoBack, nav.CanGoForward
	if nav.Loading {
		a.browseDropLoadFailure()
		if a.browseShowing() {
			a.browse.cutShort = false
		}
	}
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

// browseFailed says in the message bar that a page did not load, and why, as the web view reports it; the web view reports no load the page itself cut short by starting another, nor one that turned into a file. A navigation the web view refused, to a file, the app's own schemes or another app's, says nothing, as it never started; nor does a load the app cut short (cutShort), whose failure the platform reports as cancelled, or any failure while the start shows.
func (a *app) browseFailed(err error) {
	var failed *core.WebError
	if errors.As(err, &failed) && failed.URL != "" && failed.URL != browseBlank && !browse.Allowed(failed.URL) {
		return
	}
	if a.browse.cutShort || !a.browseShowing() {
		return
	}
	a.browse.loading = false
	a.browseLoadFailed(err.Error())
}

// browseCutShort marks the load under way, if one is, as one the app cut short by starting another.
func (a *app) browseCutShort() {
	if a.browse.loading {
		a.browse.cutShort = true
	}
}

// browseHeard is what a page posted: that it changed its address without loading another page, or its title (browse.Script), as the web view reports nothing then, or what its own buttons ask and add (browsePageAsk, browsePageAdd). The address and the title are read from the page itself rather than taken from what it posted, which any page could fill with another site's address. A page that pushed an address has the one it left to go back to, and none to go forward to.
func (a *app) browseHeard(msg core.WebMessage) {
	data := msg.Data
	if links, ok := browse.ParseAsk(data); ok {
		a.browsePageAsk(msg.Origin, links)
		return
	}
	if link, title, ok := browse.ParseAdd(data); ok {
		a.browsePageAdd(msg.Origin, link, title)
		return
	}
	if data == browse.TitleMessage {
		a.browse.view.Evaluate("[location.href, document.title]", func(raw string, err error) {
			var page []string
			if err != nil || json.Unmarshal([]byte(raw), &page) != nil || len(page) != 2 || !browse.Allowed(page[0]) {
				return
			}
			a.browse.title, a.browse.titleURL = strings.TrimSpace(page[1]), page[0]
			a.Changed()
		})
		return
	}
	kind, ok := strings.CutPrefix(data, browse.HistoryMessage)
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
	a.browseLoad(kcore.AddressTarget(text, a.prefs.interfaceLanguage))
}

// browseDownload adds the page shown to the library's queue, read from the page itself, as a site that changes its page without loading another may not have said so; the address the web view last reported stands in when the page cannot answer. A video the library has already, as 下载视频 then says, it shows there instead (browseShowHad).
func (a *app) browseDownload() {
	if !a.browseShowing() || a.browseShowHad(a.browse.url) {
		return
	}
	a.browse.view.Evaluate("location.href", func(raw string, err error) {
		u := a.browse.url
		var now string
		if err == nil && json.Unmarshal([]byte(raw), &now) == nil && browse.Allowed(now) {
			u = now
		}
		a.browseEnqueue(u, "")
		a.Changed()
	})
}

// browseAdd is 下载视频 for the video at u, as the page's own buttons ask for it (browsePageAdd): the video the library has already it shows there (browseShowHad), and any other it adds as the toolbar's does (browseEnqueue), named by title when the page gave one.
func (a *app) browseAdd(u, title string) {
	if !a.browseShowHad(u) {
		a.browseEnqueue(u, title)
	}
}

// browseShowHad shows the video at u in the library when the library has a job for it that is waiting, running or done (browseJob), as 下载视频 does on such a video, and reports whether it did.
func (a *app) browseShowHad(u string) bool {
	job, ok := a.browseJob(u)
	if ok {
		a.browseShowFilm(job.ID)
	}
	return ok
}

// browseEnqueue adds u to the library's queue, as the library's link field does, and the message bar says so, naming the video by title, or by the page's title when title is "" and the page shown is u's, with 在资料库中显示 for its card, and Add Whole Playlist for a video of a playlist (core.Link.WholeList): a playlist or a channel opens the dialog that lists its videos (browseAskList), a page that lists videos without being one, which 下载视频 is off on, is not added, what precheck finds wrong shows in the message bar with the setting that fixes it, and the pipeline's own checks, with the remedies of a failed job, take care of a page without a video it can download. What the bar says of the page closes once another page shows; what it says of the app, once that is fixed.
func (a *app) browseEnqueue(u, title string) {
	if !browse.Allowed(u) {
		return
	}
	if a.browseList(u) {
		a.browseAskList(u)
		return
	}
	if a.browseNotVideo(u) {
		a.browseWarn(a.messages().BrowseDownloadNotVideo(), "")
		a.browse.noticeURL = a.browse.url
		return
	}
	if msg, section := a.precheck([]string{u}); msg != "" {
		a.browseWarn(msg, section)
		if a.precheckAboutApp(msg) {
			a.browse.noticeReady = true
		} else {
			a.browse.noticeURL = a.browse.url
		}
		return
	}
	if err := a.Enqueue(u); err != nil {
		a.browseWarn(sourceError(a.messages(), err), "")
		var dup *queueDuplicate
		if errors.As(err, &dup) {
			a.browse.notice.Film = dup.ID
		}
		a.browse.noticeURL = a.browse.url
		return
	}
	if title == "" && a.browse.titleURL == u {
		title = a.browse.title
	}
	text := a.messages().BrowseAdded()
	if title != "" {
		text = a.messages().BrowseAddedTitled(title)
	}
	job, _ := a.browseJob(u)
	a.browseTell(text, job.ID)
	a.browse.notice.List = a.playlistWhole(u)
	a.browse.noticeURL = a.browse.url
}

// browseList reports whether u is a playlist or a channel whose videos 下载视频 lists (core.Link.IsList), which the pipeline can do (playlistCan).
func (a *app) browseList(u string) bool {
	link, ok := kcore.ParseLink(u)
	return ok && link.IsList() && a.playlistCan()
}

// browseAskList opens the dialog that lists the videos of the playlist or the channel at u, once precheckReady finds nothing missing; what it finds missing shows in the message bar until it is fixed. The videos the dialog adds are said in the browser's message bar (browseEnqueueAll).
func (a *app) browseAskList(u string) {
	if msg, section := a.precheckReady(); msg != "" {
		a.browseWarn(msg, section)
		a.browse.noticeReady = true
		return
	}
	a.playlistAsk([]string{u}, playlistFromBrowse)
}

// browseEnqueueAll adds sources, the videos the playlist dialog opened from the browser read, to the library's queue, and the browser's message bar says how many were added, or why they could not be.
func (a *app) browseEnqueueAll(sources []string) {
	m := a.messages()
	if msg, section := a.precheck(sources); msg != "" {
		a.browseWarn(msg, section)
		a.browse.noticeReady = a.precheckAboutApp(msg)
		return
	}
	added, err := a.queueEnqueue(sources)
	switch {
	case err != nil && added > 0:
		a.browseWarn(m.LibraryAddedSome(added, sourceError(m, err)), "")
	case err != nil:
		a.browseWarn(sourceError(m, err), "")
	default:
		a.browseTell(m.LibraryAddedStarting(added), "")
	}
}

// browseNotVideo reports whether u is a link to a page that lists videos without being one, which 下载视频 does not add: a site's home page, its search results, a feed or a hashtag, and a playlist or a channel when the pipeline cannot list their videos (browseList).
func (a *app) browseNotVideo(u string) bool {
	link, ok := kcore.ParseLink(u)
	return ok && !link.OneVideo() && !a.browseList(u)
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
	a.browseTell(a.messages().BrowseCleared(), "")
	if a.browseShowing() {
		a.browseCutShort()
		a.browse.view.Reload()
		a.browse.loading = true
	}
}

// browseOffline reports whether offline mode keeps the web view from loading a page, and says so in the message bar with the section that turns it off (state_privacy.go). Every way to a page asks it: an address, 后退, 前进 and 重新载入; a page the web view goes to by itself goes back to the start (browseNavigated).
func (a *app) browseOffline() bool {
	if !a.privacy.offline {
		return false
	}
	a.browseWarn(a.messages().PrivacyNoBrowse(), sectionid.Privacy)
	return true
}

func (a *app) browseBack() {
	if a.browse.source != "" && a.browse.canBack && !a.browseOffline() {
		a.browseCutShort()
		a.browse.view.GoBack()
		a.browse.loading = true
	}
}

func (a *app) browseForward() {
	if a.browse.source != "" && a.browse.canForward && !a.browseOffline() {
		a.browseCutShort()
		a.browse.view.GoForward()
		a.browse.loading = true
	}
}

func (a *app) browseReload() {
	if a.browseShowing() && !a.browseOffline() {
		a.browseCutShort()
		a.browse.view.Reload()
		a.browse.loading = true
	}
}

// browseStop stops the page loading, whose failure, which the platform reports as cancelled, says nothing (cutShort).
func (a *app) browseStop() {
	a.browse.view.StopLoading()
	a.browse.loading, a.browse.cutShort = false, true
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
	a.browse.url, a.browse.cutShort = browseBlank, true
	a.browse.view.StopLoading()
	a.browseNavigate(browseBlank)
}

// browseWarn and browseTell show the browser's message bar: something that went wrong, with the settings section that fixes it ("" when none does), which stays until it is closed or is no longer so (browseFollowNotice), or something that worked, with the job its 在资料库中显示 shows ("" for none), which closes by itself after noticeFor. The text is said already, in the interface's language.
func (a *app) browseWarn(text, section string) {
	a.browse.notice = browse.Notice{Kind: "warning", Text: text, Section: section, Open: true}
	a.browse.loadFailed, a.browse.noticeURL, a.browse.noticeReady = false, "", false
}

func (a *app) browseTell(text, film string) {
	a.browse.notice = browse.Notice{Kind: "success", Text: text, Film: film, Open: true}
	a.browse.loadFailed, a.browse.noticeURL, a.browse.noticeReady = false, "", false
	shown := a.browse.notice
	a.noticeLater(&a.browse.noticeTimer, func() {
		if a.browse.notice == shown {
			a.browse.notice.Open = false
		}
	})
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
