package browse

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sites/catalog"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakeApp is the browser's state the page reads, kept as plain fields as the app keeps them, with the calls the page made and what the web view reported.
type fakeApp struct {
	live.Rev
	state      State
	source     string
	web        core.Ref
	muted      bool
	script     string
	address    string
	focus      bool
	focused    bool
	notice     Notice
	clearOpen  bool
	navs       []core.WebNavigation
	windows    []core.WebWindowRequest
	failures   []error
	messages   []core.WebMessage
	calls      []string
	settingsAt string
}

func (f *fakeApp) call(name string) { f.calls = append(f.calls, name) }

func (f *fakeApp) BrowseState() State                     { return f.state }
func (f *fakeApp) BrowseSource() string                   { return f.source }
func (f *fakeApp) BrowseWeb() *core.Ref                   { return &f.web }
func (f *fakeApp) BrowseMuted() bool                      { return f.muted }
func (f *fakeApp) BrowseScript() string                   { return f.script }
func (f *fakeApp) BrowseNavigated(nav core.WebNavigation) { f.navs = append(f.navs, nav) }
func (f *fakeApp) BrowseOpenWindow(req core.WebWindowRequest) {
	f.windows = append(f.windows, req)
}
func (f *fakeApp) BrowseFailed(err error)            { f.failures = append(f.failures, err) }
func (f *fakeApp) BrowseMessage(msg core.WebMessage) { f.messages = append(f.messages, msg) }
func (f *fakeApp) BrowseAddress() *string            { return &f.address }
func (f *fakeApp) BrowseAddressFocus() *bool         { return &f.focus }
func (f *fakeApp) BrowseAddressFocused(focused bool) { f.focused = focused }
func (f *fakeApp) BrowseGo()                         { f.call("go " + f.address) }
func (f *fakeApp) BrowseBack()                       { f.call("back") }
func (f *fakeApp) BrowseForward()                    { f.call("forward") }
func (f *fakeApp) BrowseReload()                     { f.call("reload") }
func (f *fakeApp) BrowseStop()                       { f.call("stop") }
func (f *fakeApp) BrowseHome()                       { f.call("home") }
func (f *fakeApp) BrowseDownload()                   { f.call("download") }
func (f *fakeApp) BrowseShowFilm(id string)          { f.call("show " + id) }
func (f *fakeApp) BrowseNotice() *Notice             { return &f.notice }
func (f *fakeApp) BrowseAddList(source string)       { f.call("add list " + source) }
func (f *fakeApp) BrowseClearOpen() bool             { return f.clearOpen }
func (f *fakeApp) BrowseAskClear()                   { f.call("ask clear"); f.clearOpen = true }
func (f *fakeApp) BrowseClear()                      { f.call("clear"); f.clearOpen = false }
func (f *fakeApp) BrowseCancelClear()                { f.call("cancel clear"); f.clearOpen = false }
func (f *fakeApp) BrowseRefreshSignIn()              { f.call("refresh sign-in") }
func (f *fakeApp) OpenSettings(section string)       { f.settingsAt = section }
func (f *fakeApp) SiteLogo(s catalog.Site) []byte    { return catalog.Logo(s) }
func (f *fakeApp) OpenSite(s catalog.Site)           { f.call("open " + s.Domain) }
func (f *fakeApp) OpenSites()                        { f.call("sites") }

// show shows the browser of a across the window, on a Mac, in testlocale's language.
func show(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignStretch}, Children: []core.Node{View{App: a}}}
	}, nsgotest.Size(1000, 700), nsgotest.Environment(env))
}

// change runs fn on the app's state as a handler or a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

// onPage is the state of a web page that loaded and has somewhere to go back to.
func onPage(a *fakeApp, url string) {
	a.source = url
	a.state = State{Page: true, CanGoBack: true}
}

// Before a page opens the start shows, without a web view: 下载视频, back, forward, reload and home are off, and 清除登录信息… shows only where the sign-ins can be cleared.
func TestStartShowsBeforeAPage(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.BrowseEmptyTitle())) || !app.Exists(whole(msgs.BrowseEmptyDescription())) {
		t.Fatalf("no start: %q", app.Texts())
	}
	if app.Exists("id:browse-page") {
		t.Fatal("a web view before any page was asked for")
	}
	for _, id := range []string{"browse-download", "browse-back", "browse-forward", "browse-reload", "browse-home"} {
		if app.Enabled("id:" + id) {
			t.Errorf("%s is on before a page", id)
		}
	}
	if app.Exists("id:browse-clear") {
		t.Fatal("清除登录信息… shows where the sign-ins cannot be cleared")
	}
	change(app, a, func() { a.state.Clearable = true })
	app.Press("id:browse-clear")
	if !slices.Equal(a.calls, []string{"ask clear"}) {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { a.state.Clearing = true })
	if app.Enabled("id:browse-clear") {
		t.Fatal("清除登录信息… is on while the sign-ins are being cleared")
	}
}

// Once a page is asked for, the web view shows it, held to the web, with the platform's user agent and Safari's version after it where the pages add it, and the app's script; what it reports reaches the app, and it keeps its pages quiet while the app says so. After home the start shows over the web view, which keeps its page.
func TestPageShowsTheWebView(t *testing.T) {
	a := &fakeApp{script: "window.kando = 1"}
	app := show(t, a)
	change(app, a, func() { onPage(a, "https://example.com/") })
	web := app.Find("id:browse-page")
	if web.Web.Source.Src != "https://example.com/" || web.Web.UserAgent != "" || web.Web.UserAgentSuffix != UserAgentSuffix() || web.Web.Muted || web.Web.InjectedJavaScriptBeforeContent != "window.kando = 1" {
		t.Fatalf("web view %+v", web.Web)
	}
	if !web.Web.Restricted || web.Web.Hosts != "*" || web.Web.Navigable("file:///etc/passwd") || web.Web.Navigable("kando://open") || !web.Web.Navigable("https://www.bilibili.com/video/BV1") {
		t.Fatalf("the web view is not held to the web: restricted %v hosts %q", web.Web.Restricted, web.Web.Hosts)
	}
	if app.Exists(whole(testlocale.Messages().BrowseEmptyTitle())) {
		t.Fatal("the start shows over the page")
	}
	app.Navigate("id:browse-page", core.WebNavigation{URL: "https://example.com/a", CanGoBack: true})
	if len(a.navs) == 0 || a.navs[len(a.navs)-1].URL != "https://example.com/a" || a.navs[len(a.navs)-1].Loading {
		t.Fatalf("navigations %+v", a.navs)
	}
	app.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: "A server with the specified hostname could not be found.", Navigation: &core.WebNavigation{URL: "https://nowhere.example/"}})
	// nsgotest names no kind for a page's new-window request, which shares a video's EventEnd (the web view and the video use the same bit).
	app.Report(nsgotest.Event{Kind: nsgotest.EventEnd, ID: web.ID, Data: core.WebWindowRequest{URL: "https://example.com/popup", UserInitiated: true}})
	app.PostMessage("id:browse-page", "kando-hello")
	// A page the user went on to, of another site, reaches the app too, with its origin, and the app checks what it asks.
	app.PostMessageFrom("id:browse-page", "https://www.youtube.com/watch?v=jNQXAC9IVRw", true, "kando-ask:[]")
	app.Settle()
	var we *core.WebError
	want := []core.WebMessage{{Data: "kando-hello", Origin: "https://example.com"}, {Data: "kando-ask:[]", Origin: "https://www.youtube.com"}}
	if len(a.failures) != 1 || !errors.As(a.failures[0], &we) || we.URL != "https://nowhere.example/" || !slices.Equal(a.messages, want) {
		t.Fatalf("failures %v, messages %q", a.failures, a.messages)
	}
	if len(a.windows) != 1 || a.windows[0] != (core.WebWindowRequest{URL: "https://example.com/popup", UserInitiated: true}) {
		t.Fatalf("windows %+v", a.windows)
	}
	change(app, a, func() { a.muted = true })
	if !app.Find("id:browse-page").Web.Muted {
		t.Fatal("the web view is not muted")
	}
	change(app, a, func() { a.muted = false })
	if app.Find("id:browse-page").Web.Muted {
		t.Fatal("the web view stayed muted")
	}
	change(app, a, func() { a.state.Page = false })
	if !app.Exists(whole(testlocale.Messages().BrowseEmptyTitle())) || len(app.QueryMounted("id:browse-page")) != 1 {
		t.Fatal("home did not show the start over the web view, or dropped the web view")
	}
}

// The toolbar's buttons reach the app: back and forward while the page has somewhere to go, stop in place of reload while it loads, and home and 下载视频 on a page.
func TestToolbarDrivesTheApp(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	change(app, a, func() { onPage(a, "https://example.com/") })
	if app.Enabled("id:browse-forward") {
		t.Fatal("forward is on without a page to go forward to")
	}
	if pill := app.Find("id:browse-download"); pill.Frame.Size.Height != 34 || pill.Style.CornerRadius != 17 || pill.Props.Label != testlocale.Messages().BrowseDownload() {
		t.Fatalf("下载视频 is %v with corners of %v, named %q", pill.Frame.Size, pill.Style.CornerRadius, pill.Props.Label)
	}
	app.Press("id:browse-back")
	app.Press("id:browse-reload")
	app.Press("id:browse-home")
	app.Press("id:browse-download")
	change(app, a, func() { a.state.Loading, a.state.CanGoForward = true, true })
	if app.Exists("id:browse-reload") {
		t.Fatal("reload shows while the page loads")
	}
	app.Press(testlocale.Messages().BrowseStop())
	app.Press("id:browse-forward")
	if want := []string{"back", "reload", "home", "download", "stop", "forward"}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
}

// Where the library has the page's video, 下载视频 says so, 已在队列中 or 已在资料库中, and its tooltip names the video and what pressing it does; pressing it is still the app's 下载视频, which shows the video then.
func TestDownloadSaysWhatTheLibraryHas(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	tooltip := func(want string) {
		t.Helper()
		app.Hover("id:browse-download")
		app.Advance(time.Second)
		defer app.HoverOut()
		if !app.Exists(whole(want)) {
			t.Fatalf("the tooltip does not say %q: %q", want, app.Texts())
		}
	}
	change(app, a, func() { onPage(a, "https://www.youtube.com/watch?v=x"); a.state.Title = "Me at the zoo" })
	tooltip(msgs.BrowseDownloadTitled("Me at the zoo"))
	for _, c := range []struct{ have, label, tip, untitled string }{
		{HaveQueued, msgs.BrowseQueued(), msgs.BrowseQueuedTitled("Me at the zoo"), msgs.BrowseQueuedTooltip()},
		{HaveDone, msgs.BrowseInLibrary(), msgs.BrowseInLibraryTitled("Me at the zoo"), msgs.BrowseInLibraryTooltip()},
	} {
		change(app, a, func() { a.state.Have, a.state.Title = c.have, "Me at the zoo" })
		if pill := app.Find("id:browse-download"); pill.Props.Label != c.label || !app.Exists("id:browse-download >> "+whole(c.label)) || !app.Enabled("id:browse-download") {
			t.Fatalf("with %s, 下载视频 reads %q", c.have, pill.Props.Label)
		}
		tooltip(c.tip)
		change(app, a, func() { a.state.Title = "" })
		tooltip(c.untitled)
	}
	app.Press("id:browse-download")
	if !slices.Equal(a.calls, []string{"download"}) {
		t.Fatalf("calls %q", a.calls)
	}
}

// ⌘L's request puts the keyboard in the address field and the app hears it take the keyboard; Enter opens what it holds.
func TestAddressField(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	if app.Find("id:browse-address").Focused {
		t.Fatal("the field has the keyboard before it was asked for")
	}
	change(app, a, func() { a.focus = true })
	if a.focus || !app.Find("id:browse-address").Focused || !a.focused {
		t.Fatalf("request %v, focused %v, heard %v", a.focus, app.Find("id:browse-address").Focused, a.focused)
	}
	app.Type(testlocale.Messages().BrowseAddressLabel(), "example.com")
	app.Submit("id:browse-address")
	if a.address != "example.com" || !slices.Equal(a.calls, []string{"go example.com"}) {
		t.Fatalf("address %q, calls %q", a.address, a.calls)
	}
}

// The message bar says what went wrong with 打开设置 when a setting fixes it, or that something worked without it, and its close button closes it.
func TestNoticeShowsAndCloses(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	if app.Exists("id:browse-notice") {
		t.Fatal("a message bar with nothing to say")
	}
	change(app, a, func() { a.notice = Notice{Kind: "warning", Text: "缺少语音模型", Section: "speech", Open: true} })
	if !app.Exists(whole("缺少语音模型")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press(whole(testlocale.Messages().BrowseOpenSettings()))
	if a.settingsAt != "speech" {
		t.Fatalf("settings at %q", a.settingsAt)
	}
	change(app, a, func() { a.notice = Notice{Kind: "success", Text: "已加入资料库", Open: true} })
	if app.Exists("id:browse-notice-settings") || !app.Exists(whole("已加入资料库")) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press(testlocale.Messages().BrowseNoticeClose())
	if a.notice.Open || app.Exists("id:browse-notice") {
		t.Fatal("the close button left the bar open")
	}
	// A bar that names a video the library has shows it there, closing.
	change(app, a, func() { a.notice = Notice{Kind: "success", Text: "已加入资料库", Film: "job-1", Open: true} })
	app.Press(whole(testlocale.Messages().BrowseShowInLibrary()))
	if a.notice.Open || !slices.Equal(a.calls, []string{"show job-1"}) {
		t.Fatalf("open %v, calls %q", a.notice.Open, a.calls)
	}
}

// 清除登录信息… asks first: Enter, Escape and 取消 keep the sign-ins, and only 清除 clears.
func TestClearAsksFirst(t *testing.T) {
	a := &fakeApp{state: State{Clearable: true}}
	app := show(t, a)
	msgs := testlocale.Messages()
	for _, key := range []string{"enter", "escape", ""} {
		change(app, a, func() { a.clearOpen = true })
		if !app.Exists(whole(msgs.BrowseClearTitle())) {
			t.Fatalf("no confirmation: %q", app.Texts())
		}
		if key == "" {
			app.Press("id:browse-clear-cancel")
		} else {
			app.Key(key)
		}
		if a.clearOpen || app.Exists(whole(msgs.BrowseClearTitle())) {
			t.Fatalf("%q left the confirmation open", key)
		}
	}
	change(app, a, func() { a.clearOpen = true })
	app.Press("id:browse-clear-confirm-button")
	if want := []string{"cancel clear", "cancel clear", "cancel clear", "clear"}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
}

// What the page reads follows the app's whole revision; a change that leaves it the same builds nothing again, so the web view keeps its page and the address field its focus and what is typed in it.
func TestBrowserStaysWhileTheAppChanges(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	change(app, a, func() { onPage(a, "https://example.com/") })
	app.Type("id:browse-address", "https://exa")
	web, field := app.Find("id:browse-page").ID, app.Find("id:browse-address").ID
	change(app, a, func() {})
	if app.Find("id:browse-page").ID != web || app.Find("id:browse-address").ID != field {
		t.Fatal("the browser was built again though nothing it shows changed")
	}
	if v := app.Find("id:browse-address"); !v.Focused || a.address != "https://exa" {
		t.Fatalf("the field lost what was typed: focused %v, address %q", v.Focused, a.address)
	}
	if n := len(app.Find("id:browse-page").WebCommands); n != 0 {
		t.Fatalf("the web view was sent %d commands", n)
	}
}

// The browser follows the interface's language as it changes, the words set as its buttons are built among them.
func TestBrowserFollowsTheLanguage(t *testing.T) {
	a := &fakeApp{state: State{Clearable: true}}
	app := show(t, a)
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true})
	env.Locale = "en"
	app.SetEnvironment(env)
	for _, want := range []string{"Download Video", locales.In("en").BrowseEmptyTitle(), locales.In("en").BrowseClearData()} {
		if !app.Exists(whole(want)) {
			t.Errorf("no %q in %q", want, app.Texts())
		}
	}
	if label := app.Find("id:browse-download").Props.Label; label != "Download Video" {
		t.Errorf("下载视频 is named %q in English", label)
	}
}

// A page goes only to http and https addresses.
func TestAllowedOnlyTheWeb(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://example.com/":      true,
		"http://example.com/a?b=c":  true,
		"file:///Users/me/a.mp4":    false,
		"kando://open":              false,
		"intent://scan/#Intent;end": false,
		"javascript:alert(1)":       false,
		"about:blank":               false,
		"https://":                  false,
	} {
		if got := Allowed(raw); got != want {
			t.Errorf("Allowed(%q) = %v", raw, got)
		}
		if got := (webOnly{}).Allows(core.AccessRequest{Permission: core.AccessWebView, Resource: raw, Method: "GET"}); got != want {
			t.Errorf("webOnly allows %q: %v", raw, got)
		}
	}
	if !(webOnly{}).Allows(core.AccessRequest{Permission: core.AccessNetwork, Resource: "file:///x"}) {
		t.Error("webOnly holds back what is not a page of the web view")
	}
}

// The start offers the sites learners study from most, each opening in the browser, and 支持的网站…, which lists the rest.
func TestStartOpensSites(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	for _, s := range [][2]string{{"YouTube", "youtube.com"}, {"哔哩哔哩", "bilibili.com"}, {"TED", "ted.com"}, {"NHK", "nhk.or.jp"}, {"niconico", "nicovideo.jp"}, {"BBC", "bbc.co.uk"}} {
		if !app.Exists("id:browse-sites-tiles >> label:" + msgs.SitesTileLabel(s[0], s[1])) {
			t.Fatalf("no tile of %s on the start: %q", s[1], app.Texts())
		}
	}
	app.Press("label:" + msgs.SitesTileLabel("TED", "ted.com"))
	app.Press("id:browse-sites")
	if !slices.Equal(a.calls, []string{"open ted.com", "sites"}) {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { onPage(a, "https://www.ted.com/") })
	if app.Exists("id:browse-sites-tiles") {
		t.Fatal("the start's sites show over a page")
	}
}

// While another page shows, the page's videos pause, so they stop downloading and autoplay does not move on, and nothing plays them again; the web view's own switch keeps the sound off (BrowseMuted).
func TestPauseScriptOnlyPauses(t *testing.T) {
	if !strings.Contains(PauseScript, "m.pause()") || strings.Contains(PauseScript, "play(") || strings.Contains(PauseScript, "muted") {
		t.Fatalf("the pause script does more than pause: %s", PauseScript)
	}
}

// The pages' user agent is read from a page only as a header's value that ends in Safari's version where the pages add it, whatever else a page's script says it is.
func TestPageAgent(t *testing.T) {
	ua := "Mozilla/5.0 (Kando Test) AppleWebKit/605.1.15 (KHTML, like Gecko)"
	if suffix := UserAgentSuffix(); suffix != "" {
		ua += " " + suffix
	}
	if got, ok := PageAgent(`"` + ua + `"`); !ok || got != ua {
		t.Fatalf("PageAgent(%q) = %q, %v", ua, got, ok)
	}
	bad := []string{``, `""`, `42`, `"Mozilla/5.0\r\nCookie: x"`, `"` + strings.Repeat("a", 600) + `"`, `"Mozilla/5.0 ünicode"`}
	if UserAgentSuffix() != "" {
		bad = append(bad, `"Mozilla/5.0 (Kando Test)"`)
	}
	for _, raw := range bad {
		if got, ok := PageAgent(raw); ok {
			t.Errorf("PageAgent(%s) took %q", raw, got)
		}
	}
}

// The downloads get the cookies of the source's whole site, its registrable domain and the hosts under it, or for an address without one those a request to it sends, with the pages' user agent.
func TestCookieSource(t *testing.T) {
	var filters []CookieFilter
	expires := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	want := media.Cookie{Domain: ".bilibili.com", Path: "/", Secure: true, HTTPOnly: true, Expires: expires, Name: "SESSDATA", Value: "v"}
	list := func(_ context.Context, f CookieFilter) ([]media.Cookie, error) {
		filters = append(filters, f)
		return []media.Cookie{want}, nil
	}
	agent := func() string { return "Mozilla/5.0 Kando-Test" }
	source := CookieSource(list, agent)
	cookies, ua, err := source(context.Background(), "https://www.bilibili.com/video/BV1xx411c7mD")
	if err != nil || len(cookies) != 1 || cookies[0] != want || ua != "Mozilla/5.0 Kando-Test" {
		t.Fatalf("cookies %+v, ua %q, err %v", cookies, ua, err)
	}
	for _, s := range []string{"https://news.bbc.co.uk/video/1", "http://127.0.0.1:8080/v.mp4", "http://localhost:3000/v", "file:///Users/me/a.mp4", "https://[::1]/v"} {
		if _, _, err := source(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	wantFilters := []CookieFilter{{Domain: "bilibili.com"}, {Domain: "bbc.co.uk"}, {URL: "http://127.0.0.1:8080/v.mp4"}, {URL: "http://localhost:3000/v"}, {URL: "https://[::1]/v"}}
	if !slices.Equal(filters, wantFilters) {
		t.Fatalf("filters %+v", filters)
	}
	failing := CookieSource(func(context.Context, CookieFilter) ([]media.Cookie, error) { return nil, errors.New("no web view") }, agent)
	if cookies, _, err := failing(context.Background(), "https://example.com/v"); err == nil || cookies != nil {
		t.Fatalf("cookies %v, err %v", cookies, err)
	}
}

// The cookies of the user's own browser go to the web views' store with their HttpOnly, Secure and expiry; one that expired, and one the store would refuse, which would fail the whole write, stay out.
func TestWebCookiesKeepWhatTheStoreTakes(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)
	got := WebCookies([]media.Cookie{
		{Domain: ".YouTube.com", Path: "/", Secure: true, HTTPOnly: true, Expires: later, Name: "__Secure-3PSID", Value: "sid"},
		{Domain: "www.bilibili.com", Path: "/", Name: "buvid3", Value: "b, c"},
		{Domain: "www.youtube.com", Path: "/", Secure: true, Name: "__Host-GAPS", Value: "gaps"},
		{Domain: ".youtube.com", Path: "/", Expires: now.Add(-time.Second), Name: "old", Value: "x"},
		{Domain: ".youtube.com", Path: "/", Name: "__Secure-plain", Value: "x"},
		{Domain: ".youtube.com", Path: "/", Secure: true, Name: "__Host-dot", Value: "x"},
		{Domain: ".youtube.com", Path: "/", Name: "a,b", Value: "x"},
		{Domain: ".youtube.com", Path: "/", Name: "semi", Value: "a;b"},
		{Domain: ".youtube.com", Path: "watch", Name: "relative", Value: "x"},
		{Domain: ".", Path: "/", Name: "nohost", Value: "x"},
	}, now)
	want := []core.WebCookie{
		{Name: "__Secure-3PSID", Value: "sid", Domain: ".youtube.com", Path: "/", Expires: later, Secure: true, HTTPOnly: true},
		{Name: "buvid3", Value: "b, c", Domain: "www.bilibili.com", Path: "/"},
		{Name: "__Host-GAPS", Value: "gaps", Domain: "www.youtube.com", Path: "/", Secure: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("WebCookies = %+v\nwant %+v", got, want)
	}
}

// The sign-in Browse takes from the user's own browser covers every site its start page offers and every site its own buttons know (catalog.SignInCookies), so a site added to either is added there too.
func TestSignInCoversBrowsesSites(t *testing.T) {
	domains := slices.Clone(startSites)
	for _, s := range videoSites {
		domains = append(domains, s.Domain)
	}
	for _, d := range domains {
		if take, _ := catalog.SignInCookies("."+d, "any"); !take {
			t.Errorf("the sign-in leaves out %s", d)
		}
	}
}
