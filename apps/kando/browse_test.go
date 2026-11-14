package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The browser's page is NSGo's web view, which nsgotest keeps as a view of the model: its config (Web) is what it loads, its WebCommands what the app asked of it, and tt.Navigate, tt.Report and tt.AnswerScript report for it as the platform's web view would.

// browsePage is the selector of the browser's web view.
const browsePage = "id:browse-page"

// browseApp is a test app on the browser's page, its pages' data cleared by a counter.
func browseApp(t *testing.T, p media.Pipeline) (*app, *nsgotest.App, chan func(), *int) {
	t.Helper()
	a, tt, updates := newTestApp(t, p)
	cleared := 0
	a.browse.clear = func() error { cleared++; return nil }
	a.Select(pageBrowse)
	refresh(tt, a)
	return a, tt, updates, &cleared
}

// browseReport reports nav for the web view as the platform does once a page starts loading and once it has loaded, and settles.
func browseReport(tt *nsgotest.App, nav ns.WebNavigation) {
	tt.Navigate(browsePage, nav)
	tt.Settle()
}

// browseOpen loads rawURL as the address field would and lets the page finish loading it.
func browseOpen(t *testing.T, a *app, tt *nsgotest.App, rawURL string) {
	t.Helper()
	a.browseLoad(rawURL)
	refresh(tt, a)
	browseReport(tt, ns.WebNavigation{URL: rawURL, CanGoBack: a.browse.canBack, CanGoForward: a.browse.canForward})
	if got := tt.FindMounted(browsePage).Web.Source.Src; a.browse.url != rawURL || (got != rawURL && len(browseScripts(tt)) == 0) {
		t.Fatalf("page at %q loading %q, want %q", a.browse.url, got, rawURL)
	}
}

// browseCommands are the commands of kind the web view was sent, oldest first.
func browseCommands(tt *nsgotest.App, kind nsgotest.WebCommandKind) []string {
	var out []string
	for _, c := range tt.FindMounted(browsePage).WebCommands {
		if c.Command == kind {
			out = append(out, c.Value)
		}
	}
	return out
}

// browseScripts are the scripts the app ran in the page, oldest first, other than its questions to it (Ref.Evaluate).
func browseScripts(tt *nsgotest.App) []string {
	var out []string
	for _, s := range browseCommands(tt, nsgotest.WebInjectJavaScript) {
		if !strings.Contains(s, "nsgo-evaluate:") {
			out = append(out, s)
		}
	}
	return out
}

// Enter in the address field opens what it holds: the first address makes the web view, later ones load in it, and words are searched for on the engine of the interface's language, read when Enter is pressed.
func TestBrowseEnterLoadsTheAddress(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	if tt.Exists(browsePage) {
		t.Fatal("a web view before any page was asked for")
	}
	tt.Type(testlocale.Messages().BrowseAddressLabel(), "example.com")
	tt.Submit("id:browse-address")
	if got := tt.Find(browsePage).Web.Source.Src; got != "https://example.com" || hasText(tt, testlocale.Messages().BrowseEmptyTitle()) {
		t.Fatalf("loading %q, texts %q", got, tt.Texts())
	}
	a.browse.address = "猫 视频"
	refresh(tt, a)
	tt.Submit("id:browse-address")
	if got := tt.Find(browsePage).Web.Source.Src; got != "https://www.bing.com/search?q=%E7%8C%AB+%E8%A7%86%E9%A2%91" {
		t.Fatalf("loading %q", got)
	}
	web := tt.Find(browsePage).ID
	a.SetInterfaceLanguage("en")
	a.browse.address = "cat videos"
	refresh(tt, a)
	tt.Submit("id:browse-address")
	if v := tt.Find(browsePage); v.Web.Source.Src != "https://www.google.com/search?q=cat+videos" || v.ID != web {
		t.Fatalf("loading %q in view %d, was %d", v.Web.Source.Src, v.ID, web)
	}
}

// An address the web view's source holds already, which the page has left since, loads through the page, as the source has not changed.
func TestBrowseLoadsTheSameAddressAgain(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/b", CanGoBack: true})
	a.browseLoad("https://example.com/a")
	refresh(tt, a)
	if got := browseScripts(tt); !slices.Equal(got, []string{`location.assign("https://example.com/a")`}) {
		t.Fatalf("scripts %q", got)
	}
}

// Before a page opens the start shows, 下载视频 is off and back, reload and home are too; home brings the start back by loading the blank page, which keeps the page in the history for back.
func TestBrowseStartAndHome(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.BrowseEmptyTitle(), msgs.BrowseClearData(), msgs.BrowseDownload()} {
		if !hasText(tt, text) {
			t.Fatalf("no %q on the start: %q", text, tt.Texts())
		}
	}
	for _, id := range []string{"browse-download", "browse-back", "browse-home", "browse-reload"} {
		if tt.Enabled("id:" + id) {
			t.Fatalf("%s is on before a page", id)
		}
	}
	a.BrowseDownload()
	a.BrowseHome()
	if len(a.library.Jobs) != 0 || a.browse.source != "" || a.browse.notice.Open {
		t.Fatalf("the start acted: jobs %v, source %q", a.library.Jobs, a.browse.source)
	}
	browseOpen(t, a, tt, "https://www.bilibili.com/video/BV1xx411c7mD")
	if a.browse.address != "https://www.bilibili.com/video/BV1xx411c7mD" || hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("address %q, texts %q", a.browse.address, tt.Texts())
	}
	tt.Press("id:browse-home")
	if got := tt.FindMounted(browsePage).Web.Source.Src; got != browseBlank || len(browseCommands(tt, nsgotest.WebStopLoading)) != 1 || a.browse.address != "" || !hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("home: loading %q, address %q, texts %q", got, a.browse.address, tt.Texts())
	}
	browseReport(tt, ns.WebNavigation{URL: browseBlank, CanGoBack: true})
	if !hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("the blank page does not show the start: %q", tt.Texts())
	}
	tt.Press("id:browse-back")
	if n := len(browseCommands(tt, nsgotest.WebGoBack)); n != 1 {
		t.Fatalf("back from the start went %d pages back", n)
	}
}

// The toolbar's buttons drive the page: back and forward while it has somewhere to go, stop in place of reload while it loads.
func TestBrowseToolbarDrivesThePage(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	if tt.Enabled("id:browse-back") || tt.Enabled("id:browse-forward") {
		t.Fatal("back or forward is on without a history")
	}
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	tt.Press("id:browse-back")
	if !tt.Exists(testlocale.Messages().BrowseStop()) {
		t.Fatalf("no stop while the page loads: loading %v", a.browse.loading)
	}
	tt.Press(testlocale.Messages().BrowseStop())
	tt.Press("id:browse-forward")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	tt.Press(testlocale.Messages().BrowseReload())
	for kind, want := range map[nsgotest.WebCommandKind]int{nsgotest.WebGoBack: 1, nsgotest.WebStopLoading: 1, nsgotest.WebGoForward: 1, nsgotest.WebReload: 1} {
		if got := len(browseCommands(tt, kind)); got != want {
			t.Errorf("command %v sent %d times, want %d", kind, got, want)
		}
	}
}

// The address field shows the page's address after each navigation, but not over what the user is typing.
func TestBrowseAddressFollowsThePage(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/b"})
	if a.browse.address != "https://example.com/b" {
		t.Fatalf("address %q after the page moved", a.browse.address)
	}
	tt.Focus("id:browse-address")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/b"})
	if a.browse.address != "https://example.com/b" || !a.browse.focused {
		t.Fatalf("address %q after a report with the field focused", a.browse.address)
	}
	typeFocused(t, tt, "/x")
	typed := a.browse.address
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/c"})
	if a.browse.address != typed || a.browse.shown != "https://example.com/c" || tt.Find("id:browse-address").Text != typed {
		t.Fatalf("the page replaced what was typed: %q (typed %q)", a.browse.address, typed)
	}
}

// A page that changes its address without loading another says so through the app's script, and the address field and back follow the address read from the page itself.
func TestBrowseFollowsAPageThatChangesItsAddress(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/")
	if !strings.Contains(tt.Find(browsePage).Web.InjectedJavaScriptBeforeContent, browseHistoryMessage) {
		t.Fatal("the pages do not run the browser's script")
	}
	tt.PostMessage(browsePage, browseHistoryMessage+"push")
	tt.AnswerScript(browsePage, `"https://www.youtube.com/watch?v=jNQXAC9IVRw"`, "")
	tt.Settle()
	if a.browse.address != "https://www.youtube.com/watch?v=jNQXAC9IVRw" || !tt.Enabled("id:browse-back") {
		t.Fatalf("address %q, back %v", a.browse.address, a.browse.canBack)
	}
	// What a page posts is never taken for its address.
	tt.PostMessage(browsePage, browseHistoryMessage+"replace")
	tt.AnswerScript(browsePage, `"file:///etc/passwd"`, "")
	tt.Settle()
	if a.browse.url != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Fatalf("url %q", a.browse.url)
	}
}

// 下载视频 adds the page shown to the queue, read from the page as it is now, as a site that changes its page without loading another may not have said so, and says so in the message bar.
func TestBrowseDownloadEnqueuesTheShownPage(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/")
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"https://www.youtube.com/watch?v=jNQXAC9IVRw"`, "")
	tt.Settle()
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if !hasText(tt, testlocale.Messages().BrowseAdded()) || a.page != pageBrowse {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
}

// A page that cannot answer leaves the address the web view last reported.
func TestBrowseDownloadWithoutTheAnswer(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.bilibili.com/video/BV1xx411c7mD")
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, "", "the page went")
	tt.Settle()
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://www.bilibili.com/video/BV1xx411c7mD" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// What precheck finds wrong shows in the browser's message bar, with 打开设置 when a setting fixes it.
func TestBrowseDownloadSaysWhatIsMissing(t *testing.T) {
	a, tt, _, _ := browseApp(t, media.Tools{})
	browseOpen(t, a, tt, "https://www.bilibili.com/video/BV1xx411c7mD")
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"https://www.bilibili.com/video/BV1xx411c7mD"`, "")
	tt.Settle()
	if len(a.library.Jobs) != 0 || !a.browse.notice.Open || a.browse.notice.Section != settings.SectionGeneral {
		t.Fatalf("jobs %v, notice %+v", a.library.Jobs, a.browse.notice)
	}
	if !hasText(tt, a.browse.notice.Text) {
		t.Fatalf("the message bar does not show %q: %q", a.browse.notice.Text, tt.Texts())
	}
	tt.Press(whole(testlocale.Messages().BrowseOpenSettings()))
	wantSettings(t, a, settings.SectionGeneral)
}

// 清除登录信息… asks first, holding the window's modal flag while it does; only 清除 clears, and the page shown loads again.
func TestBrowseClearAsksFirst(t *testing.T) {
	a, tt, updates, cleared := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	tt.Press(whole(msgs.BrowseClearData()))
	if !a.browse.clearOpen || !a.modalOpen() || *cleared != 0 {
		t.Fatalf("open %v, modal %v, cleared %d", a.browse.clearOpen, a.modalOpen(), *cleared)
	}
	tt.Press(whole(msgs.BrowseCancel()))
	if a.browse.clearOpen || a.modalOpen() || *cleared != 0 {
		t.Fatalf("after 取消: open %v, modal %v, cleared %d", a.browse.clearOpen, a.modalOpen(), *cleared)
	}
	browseOpen(t, a, tt, "https://example.com/")
	// On a page the confirmation opens from the menu bar's row.
	for _, s := range a.browseKeys(a.cx) {
		if s.Label == msgs.BrowseClearData() && s.When() {
			s.Run()
		}
	}
	refresh(tt, a)
	tt.Press(whole(msgs.BrowseClearConfirm()))
	waitUntil(t, tt, a, updates, func() bool { return !a.browse.clearing })
	if *cleared != 1 || len(browseCommands(tt, nsgotest.WebReload)) != 1 || !hasText(tt, msgs.BrowseCleared()) || a.modalOpen() {
		t.Fatalf("cleared %d, modal %v, texts %q", *cleared, a.modalOpen(), tt.Texts())
	}
}

func TestBrowseClearSaysWhyItFailed(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	a.browse.clear = func() error { return errors.New("the web engine did not answer") }
	a.BrowseAskClear()
	refresh(tt, a)
	tt.Press(whole(testlocale.Messages().BrowseClearConfirm()))
	waitUntil(t, tt, a, updates, func() bool { return !a.browse.clearing })
	if want := testlocale.Messages().BrowseClearFailed("the web engine did not answer"); !hasText(tt, want) {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
}

// Where the pages' sign-ins cannot be cleared, as NSGo's web view cannot yet, 清除登录信息… shows neither on the start nor in the File menu.
func TestBrowseClearOnlyWhereItCan(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	a.browse.clear = nil
	refresh(tt, a)
	msgs := testlocale.Messages()
	if hasText(tt, msgs.BrowseClearData()) {
		t.Fatal("清除登录信息… shows on the start")
	}
	for _, s := range a.browseKeys(a.cx) {
		if s.Label == msgs.BrowseClearData() {
			t.Fatal("清除登录信息… is a row of the File menu")
		}
	}
	a.BrowseAskClear()
	if a.browse.clearOpen || a.modalOpen() {
		t.Fatal("the confirmation opened")
	}
}

// Going to another page closes the confirmation, which that page does not draw, and gives back the modal flag.
func TestBrowseConfirmationClosesOffItsPage(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	a.BrowseAskClear()
	refresh(tt, a)
	a.Select(pageFilms)
	refresh(tt, a)
	if a.browse.clearOpen || a.modalOpen() {
		t.Fatalf("open %v, modal %v", a.browse.clearOpen, a.modalOpen())
	}
}

// The browser's shortcuts work only while its page shows.
func TestBrowseShortcutsOnlyOnItsPage(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	a.Select(pageFilms)
	refresh(tt, a)
	for _, key := range []string{"mod+r", "mod+[", "mod+]", "mod+l"} {
		tt.Key(key)
	}
	if a.browse.focus || a.browse.loading {
		t.Fatalf("off the page: focus %v, loading %v", a.browse.focus, a.browse.loading)
	}
	a.Select(pageBrowse)
	refresh(tt, a)
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/a", CanGoBack: true, CanGoForward: true})
	tt.Key("mod+r")
	tt.Key("mod+[")
	tt.Key("mod+]")
	tt.Key("mod+l")
	for kind, want := range map[nsgotest.WebCommandKind]int{nsgotest.WebReload: 1, nsgotest.WebGoBack: 1, nsgotest.WebGoForward: 1} {
		if got := len(browseCommands(tt, kind)); got != want {
			t.Errorf("on the page: command %v sent %d times, want %d", kind, got, want)
		}
	}
	if !tt.Find("id:browse-address").Focused {
		t.Fatal("⌘L did not put the keyboard in the address field")
	}
}

// A page goes only to http and https addresses: the web view refuses the rest as they start, and a refused navigation says nothing, while a page that fails to load says why in the message bar, which goes once another page opens.
func TestBrowseSaysWhenAPageFails(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/")
	web := tt.Find(browsePage)
	if !web.Web.Restricted || web.Web.Navigable("file:///etc/passwd") || web.Web.Navigable("kando://open") || !web.Web.Navigable("http://example.com/") {
		t.Fatalf("the web view goes beyond the web: %+v", web.Web)
	}
	for _, refused := range []string{"file:///etc/passwd", "kando://open"} {
		tt.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: "nsgo: the web view may not navigate to " + refused, Navigation: &ns.WebNavigation{URL: refused}})
		tt.Settle()
	}
	if a.browse.notice.Open {
		t.Fatalf("a refused navigation said %q", a.browse.notice.Text)
	}
	reason := "A server with the specified hostname could not be found."
	tt.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: reason, Navigation: &ns.WebNavigation{URL: "https://nowhere.example/"}})
	tt.Settle()
	if want := testlocale.Messages().BrowseLoadFailed(reason); !hasText(tt, want) || a.browse.loading {
		t.Fatalf("no %q in %q", want, tt.Texts())
	}
	browseOpen(t, a, tt, "https://example.org/")
	if a.browse.notice.Open {
		t.Fatalf("the failure stayed after another page opened: %+v", a.browse.notice)
	}
}

// The pages' user agent is kept for the downloads: on macOS WKWebView's own with Safari's version, which the web view sends.
func TestBrowseKeepsTheUserAgent(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/")
	if got := a.browse.agent.get(); got != browseUserAgent() || tt.Find(browsePage).Web.UserAgent != got {
		t.Fatalf("user agent %q, the web view's %q", got, tt.Find(browsePage).Web.UserAgent)
	}
	if runtime.GOOS == "darwin" && !strings.HasSuffix(a.browse.agent.get(), "Version/26.0 Safari/605.1.15") {
		t.Fatalf("user agent %q names no Safari", a.browse.agent.get())
	}
}

// 浏览 sits in the sidebar right after 全部影片 without a count; choosing it shows the browser, and coming back from another page shows the page that was shown, and the sources go back to the library.
func TestSidebarBrowseRow(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	texts := tt.Texts()
	all, browse, youtube := slices.Index(texts, msgs.SidebarSourceAll()), slices.Index(texts, msgs.SidebarBrowse()), slices.Index(texts, "YouTube")
	if all < 0 || browse <= all || youtube < browse {
		t.Fatalf("浏览 at %d after 全部影片 at %d: %q", browse, all, texts)
	}
	tt.Press("id:source-" + sidebar.PageBrowse)
	if a.page != pageBrowse || !hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
	browseOpen(t, a, tt, "https://example.com/")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/watch", CanGoBack: true})
	tt.Press("id:source-" + sidebar.SourceAll)
	if a.page != pageFilms || tt.Exists(browsePage) || hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
	tt.Press("id:source-" + sidebar.PageBrowse)
	if a.page != pageBrowse || tt.Find(browsePage).Web.Source.Src == "" || hasText(tt, msgs.BrowseEmptyTitle()) {
		t.Fatalf("coming back: page %q, url %q, texts %q", a.page, a.browse.url, tt.Texts())
	}
	if v := tt.Find(browsePage); v.Web.Source.Src != "https://example.com/watch" && v.Web.Source.Src != "https://example.com/" {
		t.Fatalf("coming back loads %q", v.Web.Source.Src)
	}
}

// The web view, kept while another page shows, would play on a video in it: its sound goes off while another page shows and comes back with the browser's page. A web view that went with its column plays nothing.
func TestBrowseMutesWhileAnotherPageShows(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/watch?v=x")
	if len(browseScripts(tt)) != 0 {
		t.Fatalf("muted on its own page: %q", browseScripts(tt))
	}
	tt.Press("id:source-" + sidebar.SourceAll)
	if !a.browse.muted {
		t.Fatal("the page's sound stayed on")
	}
	if kept := len(tt.QueryMounted(browsePage)) == 1; kept && !slices.Equal(browseScripts(tt), []string{browseMuteScript(true)}) {
		t.Fatalf("on the library: scripts %q", browseScripts(tt))
	}
	tt.Press("id:source-" + sidebar.PageBrowse)
	if a.browse.muted {
		t.Fatal("the page's sound stayed off on its own page")
	}
	if scripts := browseScripts(tt); len(scripts) == 0 || scripts[len(scripts)-1] != browseMuteScript(false) {
		t.Fatalf("back on the browser: scripts %q", scripts)
	}
}

// The browser's ready removes the cookies folders of downloads that never finished; it runs before the queue's, which starts the first job left waiting from the last run.
func TestBrowseReadySweepsTheCookies(t *testing.T) {
	cache := t.TempDir()
	stale := filepath.Join(cache, "cookies-998877")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(t, media.Tools{CookieDir: cache})
	a.browseHooks().ready()
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale cookies folder kept: %v", err)
	}
	browseAt, queueAt := -1, -1
	for i, h := range a.areas() {
		if h.ready == nil {
			continue
		}
		name := runtime.FuncForPC(reflect.ValueOf(h.ready).Pointer()).Name()
		switch {
		case strings.Contains(name, ".browseHooks."):
			browseAt = i
		case strings.Contains(name, ".queueHooks."):
			queueAt = i
		}
	}
	if browseAt < 0 || queueAt < 0 || browseAt > queueAt {
		t.Fatalf("browse ready at %d, queue ready at %d: the queue's first job would start before the browser's", browseAt, queueAt)
	}
}
