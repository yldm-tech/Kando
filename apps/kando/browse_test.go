package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/browse"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
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

// browseReport reports nav for the web view as the platform does once a page starts loading and once it has loaded, and settles. The web view reports while it is hidden behind the start too (after home), which nsgotest's Navigate allows as it finds the mounted view.
func browseReport(tt *nsgotest.App, nav ns.WebNavigation) {
	tt.Navigate(browsePage, nav)
}

// browseOpen loads rawURL as the address field would and lets the page finish loading it.
func browseOpen(t *testing.T, a *app, tt *nsgotest.App, rawURL string) {
	t.Helper()
	a.browseLoad(rawURL)
	refresh(tt, a)
	browseReport(tt, ns.WebNavigation{URL: rawURL, CanGoBack: a.browse.canBack, CanGoForward: a.browse.canForward})
	if got := tt.FindMounted(browsePage).Web.Source.Src; a.browse.url != rawURL || (got != rawURL && len(browseCommands(tt, nsgotest.WebLoad)) == 0) {
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

// A download gets the cookies the pages' store holds for its video's whole site, HttpOnly ones included, with the pages' user agent, and none of another site's.
func TestBrowseDownloadsGetThePagesCookies(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{})
	expires := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	tt.WebCookies = []ns.WebCookie{
		{Name: "SESSDATA", Value: "v", Domain: ".bilibili.com", Path: "/", Expires: expires, Secure: true, HTTPOnly: true},
		{Name: "other", Value: "o", Domain: "example.com", Path: "/"},
	}
	a.browse.agent.set("Mozilla/5.0 Kando-Test")
	// The tests' app never runs ready, where the browser gives the pipeline the pages' cookies.
	a.browseHooks().ready()
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.SiteCookies == nil {
		t.Fatal("the downloads are given none of the pages' cookies")
	}
	type result struct {
		cookies []media.Cookie
		ua      string
		err     error
	}
	got := make(chan result, 1)
	go func() {
		cookies, ua, err := tools.SiteCookies(t.Context(), "https://www.bilibili.com/video/BV1xx411c7mD")
		got <- result{cookies, ua, err}
	}()
	waitUpdate(t, tt, a, updates)
	select {
	case r := <-got:
		want := media.Cookie{Domain: ".bilibili.com", Path: "/", Secure: true, HTTPOnly: true, Expires: expires, Name: "SESSDATA", Value: "v"}
		if r.err != nil || len(r.cookies) != 1 || r.cookies[0] != want || r.ua != "Mozilla/5.0 Kando-Test" {
			t.Fatalf("cookies %+v, ua %q, err %v", r.cookies, r.ua, r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the pages' cookies were never listed")
	}
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

// An address the web view's source holds already, which the page has left since, loads again through the web view, as the source has not changed, and not through a script of the page's.
func TestBrowseLoadsTheSameAddressAgain(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/a")
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/b", CanGoBack: true})
	a.browseLoad("https://example.com/a")
	refresh(tt, a)
	if got := browseCommands(tt, nsgotest.WebLoad); len(got) != 1 || len(browseScripts(tt)) != 0 {
		t.Fatalf("loads %q, scripts %q", got, browseScripts(tt))
	}
	if load := tt.FindMounted(browsePage).WebCommands; load[len(load)-1].Command != nsgotest.WebLoad || !strings.Contains(fmt.Sprint(load[len(load)-1].Value, load[len(load)-1].Data), "https://example.com/a") {
		t.Fatalf("the web view loaded %+v", load[len(load)-1])
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

// browseMoved reports that the page changed its address, its title or its history without loading another page, as a site that is one page does, which the platform reports as a change of the navigation's state alone.
func browseMoved(tt *nsgotest.App, nav ns.WebNavigation) {
	tt.Report(nsgotest.Event{Kind: nsgotest.EventNavigationStateChange, ID: tt.FindMounted(browsePage).ID, Navigation: &nav})
	tt.Settle()
}

// A page that changes its address without loading another has the web view say so, and the address field and back follow it.
func TestBrowseFollowsAPageThatChangesItsAddress(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/")
	browseMoved(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", CanGoBack: true})
	if a.browse.address != "https://www.youtube.com/watch?v=jNQXAC9IVRw" || !tt.Enabled("id:browse-back") || a.browse.loading {
		t.Fatalf("address %q, back %v, loading %v", a.browse.address, a.browse.canBack, a.browse.loading)
	}
	// An address that is not on the web is never taken for the page's.
	browseMoved(tt, ns.WebNavigation{URL: "file:///etc/passwd", CanGoBack: true})
	if a.browse.url != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Fatalf("url %q", a.browse.url)
	}
}

// A link or a script that opens a window loads its address in the same view when the user clicked it; a pop-up a script opens by itself, or one that is not on the web, loads nothing, and a window the web view refused for want of a web address says nothing.
func TestBrowseOpensAWindowInTheSameView(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/")
	web := tt.Find(browsePage)
	// nsgotest names no kind for a page's new-window request, which shares a video's EventEnd (the web view and the video use the same bit).
	open := func(req ns.WebWindowRequest) {
		tt.Report(nsgotest.Event{Kind: nsgotest.EventEnd, ID: web.ID, Data: req})
		tt.Settle()
	}
	open(ns.WebWindowRequest{URL: "https://example.com/ad"})
	open(ns.WebWindowRequest{URL: "kando://open", UserInitiated: true})
	if a.browse.url != "https://example.com/" || len(browseCommands(tt, nsgotest.WebLoad)) != 0 || tt.Find(browsePage).Web.Source.Src != "https://example.com/" {
		t.Fatalf("a window loaded: page %q, source %q", a.browse.url, tt.Find(browsePage).Web.Source.Src)
	}
	open(ns.WebWindowRequest{URL: "https://example.com/watch", UserInitiated: true})
	if a.browse.url != "https://example.com/watch" || !a.browse.loading || tt.Find(browsePage).Web.Source.Src != "https://example.com/watch" {
		t.Fatalf("the window the user opened: page %q, source %q", a.browse.url, tt.Find(browsePage).Web.Source.Src)
	}
	browseReport(tt, ns.WebNavigation{URL: "https://example.com/watch", CanGoBack: true})
	for _, refused := range []string{"", browseBlank} {
		tt.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: "nsgo: the web view may not navigate to " + refused, Navigation: &ns.WebNavigation{URL: refused}})
		tt.Settle()
	}
	if a.browse.notice.Open {
		t.Fatalf("a refused window said %q", a.browse.notice.Text)
	}
}

// 下载视频 adds the page shown to the queue, read from the page as it is now, as a site that changes its page without loading another may not have said so, and says so in the message bar.
func TestBrowseDownloadEnqueuesTheShownPage(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/watch?v=TRWgBxVAb1M")
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

// 下载视频 is off on a page that lists videos without being one, a site's home page, its search results or a channel, where yt-dlp would download whichever video it found first, and its tooltip says why; it comes on once a video of the page opens. A page that moved to such a page without saying so adds nothing either, and the message bar says why.
func TestBrowseDownloadIsOffOnAPageOfVideos(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	for _, u := range []string{"https://www.youtube.com/", "https://www.youtube.com/results?search_query=french", "https://www.youtube.com/@veritasium", "https://search.bilibili.com/all?keyword=french", "https://www.bilibili.com/zh-Hans", "https://search.bilibili.com/zh-Hans/all?keyword=french"} {
		browseOpen(t, a, tt, u)
		if tt.Enabled("id:browse-download") {
			t.Fatalf("下载视频 is on at %s", u)
		}
	}
	tt.Hover("id:browse-download")
	tt.Advance(time.Second)
	if !hasText(tt, msgs.BrowseDownloadNotVideo()) || hasText(tt, msgs.BrowseDownloadTooltip()) {
		t.Fatalf("the tooltip of 下载视频 off: %q", tt.Texts())
	}
	tt.HoverOut()
	browseReport(tt, ns.WebNavigation{URL: "https://www.bilibili.com/video/BV1xx411c7mD"})
	if !tt.Enabled("id:browse-download") {
		t.Fatal("下载视频 is off on a video")
	}
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"https://www.youtube.com/results?search_query=french"`, "")
	tt.Settle()
	if len(a.library.Jobs) != 0 || !a.browse.notice.Open || a.browse.notice.Text != msgs.BrowseDownloadNotVideo() {
		t.Fatalf("jobs %+v, notice %+v", a.library.Jobs, a.browse.notice)
	}
}

// The message bar says what is still so: what it says of a page closes once another page shows, a message that something worked closes by itself after a while, and a setting it asks for stays across pages until it is set, saying what is missing now as that changes.
func TestBrowseNoticeFollowsWhatItSays(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	video := "https://www.youtube.com/watch?v=TRWgBxVAb1M"
	browseOpen(t, a, tt, video)
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"`+video+`"`, "")
	settle(t, a, tt, updates)
	wantText(t, tt, msgs.BrowseAdded())
	tt.Advance(noticeFor - time.Second)
	wantText(t, tt, msgs.BrowseAdded())
	tt.Advance(2 * time.Second)
	if a.browse.notice.Open {
		t.Fatalf("%q still shows after %v", a.browse.notice.Text, noticeFor+time.Second)
	}

	// The same video again, on a page that moved to it without saying so, is in the library already, which says so until another page shows.
	browseReport(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts"})
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"`+video+`"`, "")
	tt.Settle()
	if !a.browse.notice.Open || a.browse.notice.Kind != "warning" {
		t.Fatalf("notice %+v", a.browse.notice)
	}
	tt.Advance(2 * noticeFor)
	if !a.browse.notice.Open {
		t.Fatal("a warning closed by itself")
	}
	browseReport(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw"})
	if a.browse.notice.Open {
		t.Fatalf("%q still shows on another page", a.browse.notice.Text)
	}

	model := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(model, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready := media.Tools{YTDLP: "yt-dlp", FFmpeg: "ffmpeg", Whisper: "whisper-cli", WhisperModel: model, Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", APIKey: "k"}}
	missing := ready
	missing.YTDLP = ""
	a.pipeline = missing
	refresh(tt, a)
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"https://www.youtube.com/watch?v=jNQXAC9IVRw"`, "")
	tt.Settle()
	if a.browse.notice.Text != msgs.PrecheckToolsMissing("yt-dlp") || a.browse.notice.Section != settings.SectionGeneral {
		t.Fatalf("notice %+v", a.browse.notice)
	}
	browseReport(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts"})
	if !a.browse.notice.Open {
		t.Fatal("what the app misses closed with the page")
	}
	noModel := ready
	noModel.WhisperModel = ""
	a.pipeline = noModel
	refresh(tt, a)
	if !a.browse.notice.Open || a.browse.notice.Text != msgs.PrecheckNoSpeechModel() || a.browse.notice.Section != settings.SectionSpeech {
		t.Fatalf("with yt-dlp in: notice %+v", a.browse.notice)
	}
	a.pipeline = ready
	refresh(tt, a)
	if a.browse.notice.Open {
		t.Fatalf("%q still shows with everything in", a.browse.notice.Text)
	}
}

// wantTooltip fails t unless 下载视频's tooltip says want, once the pointer rests on it.
func wantTooltip(t *testing.T, tt *nsgotest.App, want string) {
	t.Helper()
	tt.Hover("id:browse-download")
	tt.Advance(time.Second)
	defer tt.HoverOut()
	if !hasText(tt, want) {
		t.Fatalf("下载视频's tooltip does not say %q: %q", want, tt.Texts())
	}
}

// 下载视频's tooltip names the page's video by the title the web view reported once the page loaded, and, on a site that is one page, by the title the page set after it moved to another video, not by the one before's it still reported with the new address; a title of an address that is not on the web names nothing.
func TestBrowseNamesThePagesVideo(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	browseOpen(t, a, tt, "https://www.youtube.com/watch?v=TRWgBxVAb1M")
	wantTooltip(t, tt, msgs.BrowseDownloadTooltip())
	browseReport(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Title: " French in 10 Minutes - YouTube "})
	wantTooltip(t, tt, msgs.BrowseDownloadTitled("French in 10 Minutes - YouTube"))
	// The page moved to another video, reporting the title of the one before until it sets its own.
	browseMoved(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Title: "French in 10 Minutes - YouTube", CanGoBack: true})
	wantTooltip(t, tt, msgs.BrowseDownloadTooltip())
	browseMoved(tt, ns.WebNavigation{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Title: "Me at the zoo - YouTube", CanGoBack: true})
	wantTooltip(t, tt, msgs.BrowseDownloadTitled("Me at the zoo - YouTube"))
	browseMoved(tt, ns.WebNavigation{URL: "file:///etc/passwd", Title: "passwd", CanGoBack: true})
	if a.browse.title != "Me at the zoo - YouTube" {
		t.Fatalf("title %q from a page that is not on the web", a.browse.title)
	}
}

// After 下载视频 the message bar names the video it added and shows it in the library; while the library has it, 下载视频 reads 已在队列中 and then 已在资料库中, adds nothing again and shows it instead: its card in 处理中, then the film.
func TestBrowseDownloadSaysWhatBecameOfTheVideo(t *testing.T) {
	p := &fakePipeline{block: true}
	a, tt, updates, _ := browseApp(t, p)
	msgs := testlocale.Messages()
	video := "https://www.youtube.com/watch?v=TRWgBxVAb1M"
	browseOpen(t, a, tt, video)
	browseReport(tt, ns.WebNavigation{URL: video, Title: "French in 10 Minutes"})
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"`+video+`"`, "")
	tt.Settle()
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	id := a.library.Jobs[0].ID
	wantText(t, tt, msgs.BrowseAddedTitled("French in 10 Minutes"))
	if a.browse.notice.Film != id || !tt.Exists("id:browse-notice-film") {
		t.Fatalf("notice %+v", a.browse.notice)
	}
	if !tt.Exists("id:browse-download >> " + whole(msgs.BrowseQueued())) {
		t.Fatalf("下载视频 does not read %q with the video in the queue: %q", msgs.BrowseQueued(), tt.Texts())
	}
	wantTooltip(t, tt, msgs.BrowseQueuedTitled("French in 10 Minutes"))
	tt.Press("id:browse-notice-film")
	if a.page != pageFilms || !a.lib.queueOpen || a.openKey != "" || a.browse.notice.Open {
		t.Fatalf("page %q, queue open %v, film %q, notice %+v", a.page, a.lib.queueOpen, a.openKey, a.browse.notice)
	}

	a.Select(pageBrowse)
	a.lib.queueOpen = false
	refresh(tt, a)
	tt.Press("id:browse-download")
	if len(a.library.Jobs) != 1 || a.page != pageFilms || !a.lib.queueOpen {
		t.Fatalf("pressing 已在队列中: jobs %d, page %q, queue open %v", len(a.library.Jobs), a.page, a.lib.queueOpen)
	}

	a.Cancel(id)
	settle(t, a, tt, updates)
	// The cancelled job's goroutine read block as its download began; settle has applied the update it sent as it returned, so this write comes after that read, and the retry's goroutine starts after it.
	p.block = false
	a.Retry(id)
	settle(t, a, tt, updates)
	if job, _ := a.library.Find(id); job.Stage != core.StageComplete {
		t.Fatalf("job %+v", job)
	}
	a.Select(pageBrowse)
	refresh(tt, a)
	if !tt.Exists("id:browse-download >> " + whole(msgs.BrowseInLibrary())) {
		t.Fatalf("下载视频 does not read %q with the video in the library: %q", msgs.BrowseInLibrary(), tt.Texts())
	}
	tt.Press("id:browse-download")
	if len(a.library.Jobs) != 1 || a.page != pageFilms || a.openKey != id {
		t.Fatalf("pressing 已在资料库中: jobs %d, page %q, film %q", len(a.library.Jobs), a.page, a.openKey)
	}
}

// ⌘D is 下载视频 in the File menu: it adds a video's page, and does nothing on a page that lists videos or on a video the library has.
func TestBrowseDownloadShortcut(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/results?search_query=french")
	tt.Key("mod+d")
	if len(browseCommands(tt, nsgotest.WebInjectJavaScript)) != 0 {
		t.Fatal("⌘D asked a page of videos for its address")
	}
	video := "https://www.youtube.com/watch?v=TRWgBxVAb1M"
	browseReport(tt, ns.WebNavigation{URL: video})
	tt.Key("mod+d")
	tt.AnswerScript(browsePage, `"`+video+`"`, "")
	settle(t, a, tt, updates)
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != video {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	asked := len(browseCommands(tt, nsgotest.WebInjectJavaScript))
	tt.Key("mod+d")
	if len(browseCommands(tt, nsgotest.WebInjectJavaScript)) != asked || a.page != pageBrowse {
		t.Fatalf("⌘D on a video the library has: page %q", a.page)
	}
}

// A page the web view had not reported moving to may show a video the library has: the message bar says so and shows it.
func TestBrowseDownloadShowsAVideoAlreadyThere(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	done := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	if err := a.Enqueue(done); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	browseOpen(t, a, tt, "https://www.youtube.com/watch?v=TRWgBxVAb1M")
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"`+done+`"`, "")
	tt.Settle()
	id := a.library.Jobs[0].ID
	if len(a.library.Jobs) != 1 || a.browse.notice.Film != id || a.browse.notice.Text != testlocale.Messages().QueueAlreadyInLibrary() {
		t.Fatalf("jobs %d, notice %+v", len(a.library.Jobs), a.browse.notice)
	}
	tt.Press("id:browse-notice-film")
	if a.page != pageFilms || a.openKey != id {
		t.Fatalf("page %q, film %q", a.page, a.openKey)
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

// browseLoading reports that the page started loading rawURL, as the platform does before the page loads or fails.
func browseLoading(tt *nsgotest.App, rawURL string) {
	web := tt.FindMounted(browsePage)
	tt.Report(nsgotest.Event{Kind: nsgotest.EventNavigationStateChange, ID: web.ID, Navigation: &ns.WebNavigation{URL: rawURL, Loading: true, CanGoBack: true}})
	tt.Settle()
}

// browseCancelled reports the failure the platform reports for a load cut short, as WKWebView does for NSURLErrorCancelled (-999).
func browseCancelled(tt *nsgotest.App, rawURL string) {
	tt.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: tt.FindMounted(browsePage).ID, Text: "The operation couldn’t be completed. (NSURLErrorDomain error -999.)", Navigation: &ns.WebNavigation{URL: rawURL}})
	tt.Settle()
}

// A load the user cuts short with stop, home or another address says nothing when the platform reports it cancelled, and the next load of a page says again what goes wrong.
func TestBrowseSaysNothingOfALoadItCutShort(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/")
	browseLoading(tt, "https://example.com/slow")
	tt.Press(testlocale.Messages().BrowseStop())
	browseCancelled(tt, "https://example.com/slow")
	if a.browse.notice.Open {
		t.Fatalf("stop said %q", a.browse.notice.Text)
	}

	browseLoading(tt, "https://example.com/slow")
	tt.Press("id:browse-home")
	browseCancelled(tt, "https://example.com/slow")
	browseReport(tt, ns.WebNavigation{URL: browseBlank, CanGoBack: true})
	browseCancelled(tt, browseBlank)
	if a.browse.notice.Open || a.browse.url != browseBlank {
		t.Fatalf("home said %q, at %q", a.browse.notice.Text, a.browse.url)
	}

	browseOpen(t, a, tt, "https://example.com/a")
	browseLoading(tt, "https://example.com/slow")
	a.browseLoad("https://example.org/")
	refresh(tt, a)
	browseCancelled(tt, "https://example.com/slow")
	if a.browse.notice.Open || !a.browse.loading {
		t.Fatalf("another address said %q, loading %v", a.browse.notice.Text, a.browse.loading)
	}
	browseLoading(tt, "https://example.org/")
	reason := "A server with the specified hostname could not be found."
	tt.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: tt.Find(browsePage).ID, Text: reason, Navigation: &ns.WebNavigation{URL: "https://example.org/"}})
	tt.Settle()
	if !a.browse.notice.Open || a.browse.notice.Text != testlocale.Messages().BrowseLoadFailed(reason) {
		t.Fatalf("a page that failed after a cut-short load said %+v", a.browse.notice)
	}
	browseLoading(tt, "https://example.org/again")
	if a.browse.notice.Open {
		t.Fatalf("the failure stayed once another load started: %+v", a.browse.notice)
	}
}

// A page that goes to a page of its own making (data:, blob:), which the web view lets it, keeps the last web address in the address field and for 下载视频.
func TestBrowseKeepsTheWebAddressOverAPageOfItsOwn(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/watch")
	for _, own := range []string{"data:text/html,hello", "blob:https://example.com/0b1c"} {
		browseReport(tt, ns.WebNavigation{URL: own, CanGoBack: true})
		if a.browse.url != "https://example.com/watch" || a.browse.address != "https://example.com/watch" {
			t.Fatalf("after %s: page %q, address %q", own, a.browse.url, a.browse.address)
		}
	}
}

// The pages' user agent is kept for the downloads: on macOS WKWebView's own with Safari's version, which the web view sends.
func TestBrowseKeepsTheUserAgent(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://example.com/")
	if got := a.browse.agent.get(); got != browse.UserAgent() || tt.Find(browsePage).Web.UserAgent != got {
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

// The web view, kept while another page shows, would play on a video in it: its videos pause and its sound goes off while another page shows, and the sound comes back with the browser's page without the videos playing again. A web view that went with its column plays nothing.
func TestBrowseQuietWhileAnotherPageShows(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	browseOpen(t, a, tt, "https://www.youtube.com/watch?v=x")
	if len(browseScripts(tt)) != 0 {
		t.Fatalf("quieted on its own page: %q", browseScripts(tt))
	}
	tt.Press("id:source-" + sidebar.SourceAll)
	if !a.browse.quiet {
		t.Fatal("the page's videos played on")
	}
	if kept := len(tt.QueryMounted(browsePage)) == 1; kept && !slices.Equal(browseScripts(tt), []string{browse.QuietScript(true)}) {
		t.Fatalf("on the library: scripts %q", browseScripts(tt))
	}
	tt.Press("id:source-" + sidebar.PageBrowse)
	if a.browse.quiet {
		t.Fatal("the page's sound stayed off on its own page")
	}
	if scripts := browseScripts(tt); len(scripts) == 0 || scripts[len(scripts)-1] != browse.QuietScript(false) {
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

// browsePageStates are what the app told the page's buttons (browse.StateMessage), oldest first.
func browsePageStates(t *testing.T, tt *nsgotest.App) []browse.PageState {
	t.Helper()
	var out []browse.PageState
	for _, data := range browseCommands(tt, nsgotest.WebPostMessage) {
		rest, ok := strings.CutPrefix(data, browse.StateMessage)
		if !ok {
			t.Fatalf("the app posted %q to the page", data)
		}
		var s browse.PageState
		if err := json.Unmarshal([]byte(rest), &s); err != nil {
			t.Fatalf("the app posted %q: %v", data, err)
		}
		out = append(out, s)
	}
	return out
}

// browsePagePost has the page at pageURL post data, as its buttons do, from pageURL's origin, the web view shown or hidden behind another page of the window.
func browsePagePost(tt *nsgotest.App, pageURL, data string) {
	u, _ := url.Parse(pageURL)
	origin := "null"
	if u != nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	tt.Report(nsgotest.Event{Kind: nsgotest.EventMessage, ID: tt.FindMounted(browsePage).ID, Text: data, Origin: origin, URL: pageURL, MainFrame: true})
	tt.Settle()
}

func browseAskData(links ...string) string {
	data, _ := json.Marshal(links)
	return browse.AskMessage + string(data)
}

func browseAddData(link, title string) string {
	data, _ := json.Marshal(map[string]string{"url": link, "title": title})
	return browse.AddMessage + string(data)
}

// The page's buttons ask what the library has of the videos the page shows, and the app answers a page of a site the catalog lists with their words, as 下载视频 says them, and what the library has of each; a page of another site gets nothing.
func TestBrowsePageButtonsAskWhatTheLibraryHas(t *testing.T) {
	a, tt, updates, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	done, other := "https://www.youtube.com/watch?v=jNQXAC9IVRw", "https://www.youtube.com/shorts/TRWgBxVAb1M"
	if err := a.Enqueue("https://youtu.be/jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	a.Select(pageBrowse)
	refresh(tt, a)
	home := "https://www.youtube.com/"
	browseOpen(t, a, tt, home)
	browsePagePost(tt, home, browseAskData(done, other, "https://www.youtube.com/@veritasium"))
	states := browsePageStates(t, tt)
	want := browse.PageState{Text: browse.PageText{Add: msgs.BrowseDownload(), Queued: msgs.BrowseQueued(), Done: msgs.BrowseInLibrary()}, Have: map[string]string{done: browse.HaveDone, other: ""}}
	if len(states) != 1 || !reflect.DeepEqual(states[0], want) {
		t.Fatalf("answers %+v, want %+v", states, want)
	}
	for _, origin := range []string{"https://example.com/", "null"} {
		browsePagePost(tt, origin, browseAskData(done))
	}
	if n := len(browsePageStates(t, tt)); n != 1 {
		t.Fatalf("pages of no site of the catalog got %d answers", n-1)
	}
	// Nothing changed, so nothing more is told.
	a.Changed()
	refresh(tt, a)
	if n := len(browsePageStates(t, tt)); n != 1 {
		t.Fatalf("%d answers without a change", n)
	}
}

// A click on a page's button adds its video as 下载视频 does: one job, the message bar naming the video by the title the page gave it, with 在资料库中显示; the page then hears the video is queued, and done once it is. Clicking it again shows the video in the library, as the toolbar's pill does on a video the library has.
func TestBrowsePageButtonAddsAsDownloadDoes(t *testing.T) {
	p := &fakePipeline{block: true}
	a, tt, updates, _ := browseApp(t, p)
	msgs := testlocale.Messages()
	home, video := "https://www.youtube.com/", "https://www.youtube.com/watch?v=jNQXAC9IVRw&pp=ygUF"
	browseOpen(t, a, tt, home)
	browsePagePost(tt, home, browseAskData(video))
	browsePagePost(tt, home, browseAddData(video, "Me at the zoo"))
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != video {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	id := a.library.Jobs[0].ID
	wantText(t, tt, msgs.BrowseAddedTitled("Me at the zoo"))
	if a.browse.notice.Film != id || !tt.Exists("id:browse-notice-film") || a.page != pageBrowse {
		t.Fatalf("notice %+v, page %q", a.browse.notice, a.page)
	}
	states := browsePageStates(t, tt)
	if last := states[len(states)-1]; last.Have[video] != browse.HaveQueued {
		t.Fatalf("the page heard %+v", states)
	}
	// The toolbar's 下载视频 is off on the home page, which lists videos without being one.
	if tt.Enabled("id:browse-download") {
		t.Fatal("下载视频 is on on YouTube's home")
	}

	browsePagePost(tt, home, browseAddData(video, "Me at the zoo"))
	if len(a.library.Jobs) != 1 || a.page != pageFilms || !a.lib.queueOpen {
		t.Fatalf("a second click: jobs %d, page %q, queue open %v", len(a.library.Jobs), a.page, a.lib.queueOpen)
	}

	a.Cancel(id)
	settle(t, a, tt, updates)
	// The cancelled job's goroutine read block as its download began; settle has applied the update it sent as it returned, so this write comes after that read, and the retry's goroutine starts after it.
	p.block = false
	a.Retry(id)
	settle(t, a, tt, updates)
	a.Select(pageBrowse)
	refresh(tt, a)
	states = browsePageStates(t, tt)
	if last := states[len(states)-1]; last.Have[video] != browse.HaveDone {
		t.Fatalf("the page heard %+v once the film was done", states)
	}
}

// The app takes a click on a page's button only for a video a page of a site the catalog lists shows while the browser's page shows, as any script of a page can post one: not from another site, not a playlist, a channel or a site's home, nor while another page of the window shows.
func TestBrowsePageButtonAddsOnlyWhatItShould(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	home, video := "https://www.youtube.com/", "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	browseOpen(t, a, tt, home)
	browsePagePost(tt, "https://example.com/", browseAddData(video, ""))
	for _, u := range []string{"https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "https://www.youtube.com/@veritasium", home, "https://example.com/watch?v=jNQXAC9IVRw"} {
		browsePagePost(tt, home, browseAddData(u, ""))
	}
	a.Select(pageFilms)
	refresh(tt, a)
	browsePagePost(tt, home, browseAddData(video, ""))
	if len(a.library.Jobs) != 0 || a.browse.notice.Open {
		t.Fatalf("jobs %+v, notice %+v", a.library.Jobs, a.browse.notice)
	}
	// What precheck finds missing it says in the message bar, as for 下载视频.
	a.Select(pageBrowse)
	a.pipeline = media.Tools{}
	refresh(tt, a)
	browsePagePost(tt, home, browseAddData(video, ""))
	if len(a.library.Jobs) != 0 || !a.browse.notice.Open || a.browse.notice.Section != settings.SectionGeneral {
		t.Fatalf("jobs %+v, notice %+v", a.library.Jobs, a.browse.notice)
	}
}

// The page's buttons speak the interface's language, and the page hears their words again once it changes.
func TestBrowsePageButtonsFollowTheLanguage(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	home := "https://www.bilibili.com/"
	browseOpen(t, a, tt, home)
	browsePagePost(tt, "https://search.bilibili.com/all?keyword=x", browseAskData("https://www.bilibili.com/video/BV1xx411c7mD/"))
	a.SetInterfaceLanguage("en")
	refresh(tt, a)
	states := browsePageStates(t, tt)
	en := browse.PageText{Add: "Download Video", Queued: "Queued", Done: "In Library"}
	if len(states) != 2 || states[0].Text.Add != testlocale.Messages().BrowseDownload() || states[1].Text != en || len(states[1].Have) != 0 {
		t.Fatalf("the page heard %+v", states)
	}
}

// 浏览网站 beside the link field and on the page of an empty library opens the browser.
func TestLibraryOpensTheBrowser(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	wantText(t, tt, testlocale.Messages().FilmsEmptyTitle())
	tt.Press("id:empty-browse")
	if a.page != pageBrowse || !hasText(tt, testlocale.Messages().BrowseEmptyTitle()) {
		t.Fatalf("page %q, texts %q", a.page, tt.Texts())
	}
	a.Select(pageFilms)
	refresh(tt, a)
	tt.Press("id:films-browse")
	if a.page != pageBrowse {
		t.Fatalf("page %q", a.page)
	}
}

// bilibili may serve its pages under the reader's language (www.bilibili.com/zh-Hans/video/BV…/), whose links yt-dlp's bilibili extractor does not read: a page's button adds the video by its canonical link, which is the same video as the localized one, so the page hears it is queued under either.
func TestBrowsePageButtonAddsALocalizedBilibiliVideo(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{block: true})
	home, localized, canonical := "https://www.bilibili.com/zh-Hans", "https://www.bilibili.com/zh-Hans/video/BV1nx4y1x7t1/?p=2", "https://www.bilibili.com/video/BV1nx4y1x7t1/?p=2"
	browseOpen(t, a, tt, home)
	browsePagePost(tt, "https://search.bilibili.com/zh-Hans/all?keyword=x", browseAskData(localized))
	browsePagePost(tt, home, browseAddData(localized, "爆旋陀螺"))
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != canonical {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if queueSourceKey(localized) != queueSourceKey(canonical) {
		t.Fatalf("keys %q and %q", queueSourceKey(localized), queueSourceKey(canonical))
	}
	browsePagePost(tt, home, browseAskData(canonical))
	states := browsePageStates(t, tt)
	if pushed, asked := states[len(states)-2], states[len(states)-1]; pushed.Have[localized] != browse.HaveQueued || asked.Have[canonical] != browse.HaveQueued {
		t.Fatalf("the page heard %+v", states)
	}
}
