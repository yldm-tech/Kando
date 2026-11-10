package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakeBrowsePage stands in for the web view's page, which the tests have no window to make: it records what the browser asked of it, and its fields are what it reports.
type fakeBrowsePage struct {
	url              string
	loading          bool
	back, forward    bool
	ua               string
	opened, loads    []string
	backs, forwards  int
	reloads, stops   int
	failLoad, failUp error
}

// open is browseUI.open: it makes no web view, so the page's room stays blank.
func (p *fakeBrowsePage) open(rawURL string) (browsePage, *mygo.WebView, error) {
	if p.failUp != nil {
		return nil, nil, p.failUp
	}
	p.opened = append(p.opened, rawURL)
	p.url, p.loading = rawURL, true
	return p, nil, nil
}

func (p *fakeBrowsePage) LoadURL(rawURL string) error {
	if p.failLoad != nil {
		return p.failLoad
	}
	p.loads = append(p.loads, rawURL)
	p.url, p.loading = rawURL, true
	return nil
}

func (p *fakeBrowsePage) GoBack()            { p.backs++ }
func (p *fakeBrowsePage) GoForward()         { p.forwards++ }
func (p *fakeBrowsePage) Reload()            { p.reloads++ }
func (p *fakeBrowsePage) Stop()              { p.stops++ }
func (p *fakeBrowsePage) URL() string        { return p.url }
func (p *fakeBrowsePage) CanGoBack() bool    { return p.back }
func (p *fakeBrowsePage) CanGoForward() bool { return p.forward }
func (p *fakeBrowsePage) IsLoading() bool    { return p.loading }
func (p *fakeBrowsePage) UserAgent() string  { return p.ua }

// browseApp is a test app on the browser's page, its web view the fake page and its data cleared by a counter.
func browseApp(t *testing.T, p media.Pipeline) (*app, *ui.Tester, chan func(), *fakeBrowsePage, *int) {
	t.Helper()
	a, tt, updates := newTestApp(p)
	fake := &fakeBrowsePage{ua: "Mozilla/5.0 Kando-Test"}
	cleared := 0
	a.browse.open = fake.open
	a.browse.clear = func() error { cleared++; return nil }
	a.Select(pageBrowse)
	tt.Frame()
	return a, tt, updates, fake, &cleared
}

// browseOpen loads rawURL as the address field would and lets the page finish loading it.
func browseOpen(t *testing.T, a *app, tt *ui.Tester, fake *fakeBrowsePage, rawURL string) {
	t.Helper()
	a.browseLoad(rawURL)
	tt.Frame()
	fake.loading = false
	a.browseRefresh()
	tt.Frame()
	if a.browse.page == nil || fake.url != rawURL {
		t.Fatalf("page %v at %q, want %q", a.browse.page, fake.url, rawURL)
	}
}

// drain runs the background updates waiting in updates until done reports true.
func drain(t *testing.T, tt *ui.Tester, updates chan func(), done func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatal("timed out waiting for a background update")
		}
	}
	tt.Frame()
}

func TestBrowseTargetReadsAddressesAndSearches(t *testing.T) {
	for _, c := range []struct{ text, language, want string }{
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", "zh-Hans", "https://www.youtube.com/watch?v=jNQXAC9IVRw"},
		{"HTTP://example.com/a", "en", "HTTP://example.com/a"},
		{"example.com", "en", "https://example.com"},
		{"www.bilibili.com/video/BV1xx411c7mD?p=2", "zh-Hans", "https://www.bilibili.com/video/BV1xx411c7mD?p=2"},
		{"localhost:8080/watch", "en", "https://localhost:8080/watch"},
		{"192.168.1.2", "en", "https://192.168.1.2"},
		{"例子.中国", "zh-Hans", "https://例子.中国"},
		{"cat videos", "zh-Hans", "https://www.bing.com/search?q=cat+videos"},
		{"cat videos", "en", "https://www.google.com/search?q=cat+videos"},
		{"猫", "ja", "https://www.google.com/search?q=%E7%8C%AB"},
		{"3.14", "en", "https://www.google.com/search?q=3.14"},
		{"golang", "en", "https://www.google.com/search?q=golang"},
		{"file:///etc/passwd", "en", "https://www.google.com/search?q=file%3A%2F%2F%2Fetc%2Fpasswd"},
		{"javascript:alert(1)", "en", "https://www.google.com/search?q=javascript%3Aalert%281%29"},
		{"user@example.com", "en", "https://www.google.com/search?q=user%40example.com"},
	} {
		if got := browseTarget(c.text, c.language); got != c.want {
			t.Errorf("browseTarget(%q, %s) = %q, want %q", c.text, c.language, got, c.want)
		}
	}
}

// Enter in the address field opens what it holds: the first address makes the web view, later ones load in it, and words are searched for on the engine of the interface's language, read when Enter is pressed.
func TestBrowseEnterLoadsTheAddress(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	if err := tt.Click("网址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("example.com")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !slices.Equal(fake.opened, []string{"https://example.com"}) || a.browse.page == nil {
		t.Fatalf("opened %q", fake.opened)
	}
	if tt.HasText("打开有视频的网页") {
		t.Fatal("the start still shows over the page")
	}
	a.browse.address = "猫 视频"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	testlocale.Use(t, "en")
	a.browse.address = "cat videos"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	want := []string{"https://www.bing.com/search?q=%E7%8C%AB+%E8%A7%86%E9%A2%91", "https://www.google.com/search?q=cat+videos"}
	if !slices.Equal(fake.loads, want) || len(fake.opened) != 1 {
		t.Fatalf("loads %q, opened %q", fake.loads, fake.opened)
	}
}

// Before a page opens the start shows, 下载视频 does nothing and back, reload and home are off; home brings the start back by loading the blank page, which keeps the page in the history for back.
func TestBrowseStartAndHome(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	for _, text := range []string{"打开有视频的网页", "清除登录信息…", "下载视频"} {
		if !tt.HasText(text) {
			t.Fatalf("no %q on the start: %q", text, tt.Texts())
		}
	}
	_ = tt.Click("下载视频")
	a.BrowseDownload()
	_ = tt.Click("后退")
	_ = tt.Click("主页")
	if len(a.library.Jobs) != 0 || fake.backs != 0 || len(fake.loads) != 0 || a.browse.notice.Open {
		t.Fatalf("the start acted: jobs %v, backs %d, loads %q", a.library.Jobs, fake.backs, fake.loads)
	}
	browseOpen(t, a, tt, fake, "https://www.bilibili.com/video/BV1xx411c7mD")
	if a.browse.address != "https://www.bilibili.com/video/BV1xx411c7mD" || tt.HasText("打开有视频的网页") {
		t.Fatalf("address %q, texts %q", a.browse.address, tt.Texts())
	}
	if err := tt.Click("主页"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(fake.loads, []string{browseBlank}) || fake.stops != 1 || a.browse.address != "" || !tt.HasText("打开有视频的网页") {
		t.Fatalf("home: loads %q, stops %d, address %q, texts %q", fake.loads, fake.stops, a.browse.address, tt.Texts())
	}
	fake.back = true
	fake.loading = false
	a.browseRefresh()
	tt.Frame()
	if err := tt.Click("后退"); err != nil {
		t.Fatal(err)
	}
	if fake.backs != 1 {
		t.Fatalf("back from the start went %d pages back", fake.backs)
	}
}

// The toolbar's buttons drive the page: back and forward while it has somewhere to go, stop in place of reload while it loads.
func TestBrowseToolbarDrivesThePage(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, fake, "https://example.com/a")
	_ = tt.Click("后退")
	_ = tt.Click("前进")
	if fake.backs != 0 || fake.forwards != 0 {
		t.Fatalf("went back %d and forward %d without a history", fake.backs, fake.forwards)
	}
	fake.back, fake.forward = true, true
	a.browseRefresh()
	tt.Frame()
	if err := tt.Click("后退"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if fake.backs != 1 || !tt.HasText("停止载入") && !hasAccess(tt, "停止载入") {
		t.Fatalf("backs %d, loading %v", fake.backs, a.browse.loading)
	}
	if err := tt.Click("停止载入"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("前进"); err != nil {
		t.Fatal(err)
	}
	fake.loading = false
	a.browseRefresh()
	tt.Frame()
	if err := tt.Click("重新载入"); err != nil {
		t.Fatal(err)
	}
	if fake.stops != 1 || fake.forwards != 1 || fake.reloads != 1 {
		t.Fatalf("stops %d, forwards %d, reloads %d", fake.stops, fake.forwards, fake.reloads)
	}
}

func hasAccess(tt *ui.Tester, name string) bool {
	_, ok := tt.Access(name)
	return ok
}

// The address field shows the page's address after each navigation, but not over what the user is typing.
func TestBrowseAddressFollowsThePage(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, fake, "https://example.com/a")
	fake.url = "https://example.com/b"
	a.browseRefresh()
	if a.browse.address != "https://example.com/b" {
		t.Fatalf("address %q after the page moved", a.browse.address)
	}
	if err := tt.Click("网址"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.browseRefresh()
	if a.browse.address != "https://example.com/b" {
		t.Fatalf("address %q after a refresh with the field focused", a.browse.address)
	}
	tt.Type("/x")
	tt.Frame()
	typed := a.browse.address
	fake.url = "https://example.com/c"
	a.browseRefresh()
	if a.browse.address != typed || a.browse.shown != "https://example.com/c" {
		t.Fatalf("the page replaced what was typed: %q (typed %q)", a.browse.address, typed)
	}
}

// 下载视频 adds the page shown to the queue, read from the page as it is now, as a site that changes its page without loading another tells nothing, and says so in the message bar.
func TestBrowseDownloadEnqueuesTheShownPage(t *testing.T) {
	a, tt, updates, fake, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, fake, "https://www.youtube.com/")
	fake.url = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	if err := tt.Click("下载视频"); err != nil {
		t.Fatal(err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	settle(t, a, tt, updates)
	if !tt.HasText("已加入资料库，开始处理。") || a.page != pageBrowse {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
}

// What precheck finds wrong shows in the browser's message bar, with 打开设置 when a setting fixes it.
func TestBrowseDownloadSaysWhatIsMissing(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, media.Tools{})
	browseOpen(t, a, tt, fake, "https://www.bilibili.com/video/BV1xx411c7mD")
	if err := tt.Click("下载视频"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.library.Jobs) != 0 || !a.browse.notice.Open || a.browse.notice.Section != settings.SectionGeneral {
		t.Fatalf("jobs %v, notice %+v", a.library.Jobs, a.browse.notice)
	}
	if !tt.HasText(a.browse.notice.Text) {
		t.Fatalf("the message bar does not show %q: %q", a.browse.notice.Text, tt.Texts())
	}
	if err := tt.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	wantSettings(t, a, settings.SectionGeneral)
}

// 清除登录信息… asks first, holding the window's modal flag while it does; only 清除 clears, and the page shown loads again.
func TestBrowseClearAsksFirst(t *testing.T) {
	a, tt, updates, fake, cleared := browseApp(t, &fakePipeline{})
	if err := tt.Click("清除登录信息…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.browse.clearOpen || !a.modalOpen() || *cleared != 0 {
		t.Fatalf("open %v, modal %v, cleared %d", a.browse.clearOpen, a.modalOpen(), *cleared)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.browse.clearOpen || a.modalOpen() || *cleared != 0 {
		t.Fatalf("after 取消: open %v, modal %v, cleared %d", a.browse.clearOpen, a.modalOpen(), *cleared)
	}
	browseOpen(t, a, tt, fake, "https://example.com/")
	// On a page the confirmation opens from the menu bar's row.
	for _, s := range a.browseKeys() {
		if s.Label == "清除登录信息…" && s.When() {
			s.Run()
		}
	}
	tt.Frame()
	if err := tt.Click("清除"); err != nil {
		t.Fatal(err)
	}
	drain(t, tt, updates, func() bool { return !a.browse.clearing })
	if *cleared != 1 || fake.reloads != 1 || !tt.HasText("已清除登录信息。") || a.modalOpen() {
		t.Fatalf("cleared %d, reloads %d, modal %v, texts %q", *cleared, fake.reloads, a.modalOpen(), tt.Texts())
	}
}

func TestBrowseClearSaysWhyItFailed(t *testing.T) {
	a, tt, updates, _, _ := browseApp(t, &fakePipeline{})
	a.browse.clear = func() error { return errors.New("the web engine did not answer") }
	a.BrowseAskClear()
	tt.Frame()
	if err := tt.Click("清除"); err != nil {
		t.Fatal(err)
	}
	drain(t, tt, updates, func() bool { return !a.browse.clearing })
	if !tt.HasText("没能清除登录信息：the web engine did not answer") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// Going to another page closes the confirmation, which that page does not draw, and gives back the modal flag.
func TestBrowseConfirmationClosesOffItsPage(t *testing.T) {
	a, tt, _, _, _ := browseApp(t, &fakePipeline{})
	a.BrowseAskClear()
	tt.Frame()
	a.Select(pageFilms)
	tt.Frame()
	if a.browse.clearOpen || a.modalOpen() {
		t.Fatalf("open %v, modal %v", a.browse.clearOpen, a.modalOpen())
	}
}

// The browser's shortcuts work only while its page shows.
func TestBrowseShortcutsOnlyOnItsPage(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, fake, "https://example.com/a")
	fake.back, fake.forward = true, true
	a.browseRefresh()
	a.Select(pageFilms)
	tt.Frame()
	for _, key := range []ui.Key{ui.KeyR, ui.KeyBracketLeft, ui.KeyBracketRight, ui.KeyL} {
		tt.Key(ui.Cmd, key)
		tt.Frame()
	}
	if fake.reloads+fake.backs+fake.forwards != 0 || a.browse.focus {
		t.Fatalf("off the page: reloads %d, backs %d, forwards %d, focus %v", fake.reloads, fake.backs, fake.forwards, a.browse.focus)
	}
	a.Select(pageBrowse)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyR)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyBracketLeft)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyBracketRight)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyL)
	tt.Frame()
	tt.Frame()
	if fake.reloads != 1 || fake.backs != 1 || fake.forwards != 1 || !tt.Focused("网址") {
		t.Fatalf("on the page: reloads %d, backs %d, forwards %d, field focused %v", fake.reloads, fake.backs, fake.forwards, tt.Focused("网址"))
	}
}

// A page goes only to http and https addresses, and a window it opens opens in the same view only when the user asked.
func TestBrowseAllowsOnlyTheWeb(t *testing.T) {
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
		if got := browseAllowed(raw); got != want {
			t.Errorf("browseAllowed(%q) = %v", raw, got)
		}
	}
}

// A page that fails to load says why in the message bar, which goes once another page opens; a web view that cannot be made says so too.
func TestBrowseSaysWhenAPageFails(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	fake.failUp = errors.New("no window")
	a.browseLoad("https://example.com/")
	tt.Frame()
	tt.Frame()
	if a.browse.page != nil || !tt.HasText("打不开这个网页：no window") || !tt.HasText("打开有视频的网页") {
		t.Fatalf("page %v, texts %q", a.browse.page, tt.Texts())
	}
	fake.failUp = nil
	browseOpen(t, a, tt, fake, "https://example.com/")
	if a.browse.notice.Open {
		t.Fatalf("the failure stayed: %+v", a.browse.notice)
	}
	a.browseLoadFailed("A server with the specified hostname could not be found.")
	tt.Frame()
	if !tt.HasText("打不开这个网页：A server with the specified hostname could not be found.") {
		t.Fatalf("texts %q", tt.Texts())
	}
	browseOpen(t, a, tt, fake, "https://example.org/")
	if a.browse.notice.Open {
		t.Fatalf("the failure stayed after another page opened: %+v", a.browse.notice)
	}
}

// The downloads get the cookies of the source's whole site, its registrable domain and the hosts under it, or for an address without one those a request to it sends, with the pages' user agent.
func TestBrowseCookieSource(t *testing.T) {
	var filters []mygo.CookieFilter
	expires := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	list := func(f mygo.CookieFilter) ([]mygo.Cookie, error) {
		filters = append(filters, f)
		return []mygo.Cookie{{Name: "SESSDATA", Value: "v", Domain: ".bilibili.com", Path: "/", Expires: expires, Secure: true, HTTPOnly: true}}, nil
	}
	agent := &browseAgent{}
	agent.set("Mozilla/5.0 Kando-Test")
	source := browseCookieSource(list, agent)
	cookies, ua, err := source(context.Background(), "https://www.bilibili.com/video/BV1xx411c7mD")
	want := media.Cookie{Domain: ".bilibili.com", Path: "/", Secure: true, HTTPOnly: true, Expires: expires, Name: "SESSDATA", Value: "v"}
	if err != nil || len(cookies) != 1 || cookies[0] != want || ua != "Mozilla/5.0 Kando-Test" {
		t.Fatalf("cookies %+v, ua %q, err %v", cookies, ua, err)
	}
	for _, s := range []string{"https://news.bbc.co.uk/video/1", "http://127.0.0.1:8080/v.mp4", "http://localhost:3000/v", "file:///Users/me/a.mp4", "https://[::1]/v"} {
		if _, _, err := source(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	wantFilters := []mygo.CookieFilter{{Domain: "bilibili.com"}, {Domain: "bbc.co.uk"}, {URL: "http://127.0.0.1:8080/v.mp4"}, {URL: "http://localhost:3000/v"}, {URL: "https://[::1]/v"}}
	if !slices.Equal(filters, wantFilters) {
		t.Fatalf("filters %+v", filters)
	}
	failing := browseCookieSource(func(mygo.CookieFilter) ([]mygo.Cookie, error) { return nil, errors.New("no web view") }, agent)
	if cookies, _, err := failing(context.Background(), "https://example.com/v"); err == nil || cookies != nil {
		t.Fatalf("cookies %v, err %v", cookies, err)
	}
}

// The page's user agent is kept for the downloads once it has loaded.
func TestBrowseKeepsTheUserAgent(t *testing.T) {
	a, tt, _, fake, _ := browseApp(t, &fakePipeline{})
	if a.browse.agent.get() != "" {
		t.Fatal("a user agent before any page")
	}
	browseOpen(t, a, tt, fake, "https://example.com/")
	if got := a.browse.agent.get(); got != "Mozilla/5.0 Kando-Test" {
		t.Fatalf("user agent %q", got)
	}
}

// 浏览 sits in the sidebar right after 全部影片 without a count; choosing it shows the browser and keeps its page, and the sources go back to the library.
func TestSidebarBrowseRow(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	fake := &fakeBrowsePage{}
	a.browse.open = fake.open
	tt.Frame()
	texts := tt.Texts()
	all, browse, youtube := slices.Index(texts, "全部影片"), slices.Index(texts, "浏览"), slices.Index(texts, "YouTube")
	if all < 0 || browse != all+2 || youtube < browse {
		t.Fatalf("浏览 at %d after 全部影片 at %d: %q", browse, all, texts)
	}
	if err := tt.Click("浏览"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageBrowse || !tt.HasText("打开有视频的网页") {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
	browseOpen(t, a, tt, fake, "https://example.com/")
	if err := tt.Click("全部影片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms || tt.HasText("打开有视频的网页") {
		t.Fatalf("page %q", a.page)
	}
	if err := tt.Click("浏览"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageBrowse || a.browse.page == nil || fake.url != "https://example.com/" || len(fake.opened) != 1 || strings.Contains(strings.Join(tt.Texts(), "\n"), "打开有视频的网页") {
		t.Fatalf("coming back: page %q, url %q, opened %q", a.page, fake.url, fake.opened)
	}
}
