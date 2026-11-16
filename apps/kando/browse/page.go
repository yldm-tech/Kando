package browse

import (
	"net/url"
	"strings"

	"kando/apps/kando/live"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// pageFill is the page's background before the page paints its own, as a browser's is.
var pageFill = core.RGB(0xFFFFFF)

// page is the web view showing the web page. Its pages are someone else's: they may go to http and https addresses and to pages of their own (about:, data:, blob:), and nowhere else (webOnly), and they reach the app only through what they post to window.nsgo, which BrowseMessage hears as a plain string with the origin of the page that posted it. Every page reaches it, of whichever site the user went on to, as the app checks what each message asks of it.
type page struct{ a App }

// Render implements core.Node.
func (p page) Render(cx *core.Scope) core.Node {
	a := p.a
	// The web view refuses the other navigations as they start, a file dropped on it among them, and reports them to OnError.
	cx.ProvideAccess(webOnly{})
	// TODO(nsgo#203): let Access refuse a web view's top-level navigations to about:, data: and blob: addresses, which WebConfig.Navigable allows whatever the Access says, as MyGo's OnWillNavigate refused them; until then a page may go to one, and the app keeps the last web address for the address field and 下载视频 (BrowseNavigated) https://github.com/nsgo-dev/nsgo/issues/203
	// TODO(nsgo#202): let a web view zoom its page with a pinch, as MyGo's PageOptions.PinchZoom did (WKWebView's allowsMagnification) https://github.com/nsgo-dev/nsgo/issues/202
	return core.WebView{
		TestID:                          "browse-page",
		Ref:                             a.BrowseWeb(),
		SourceFunc:                      live.Func(a, func() core.WebSource { return core.WebSrc(a.BrowseSource()) }),
		UserAgent:                       a.BrowseUserAgent(),
		InjectedJavaScriptBeforeContent: a.BrowseScript(),
		BackForwardGestures:             true,
		OnNavigationStateChange:         live.On(a, a.BrowseNavigated),
		OnError:                         live.On(a, a.BrowseFailed),
		MessageOrigins:                  []string{"*"},
		OnMessageFrom:                   live.On(a, a.BrowseMessage),
		Style:                           core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Background: pageFill},
		// The start shows in its place after home, while the web view keeps the pages before it.
		StyleFunc: live.Func(a, func() core.Style {
			if !a.BrowseState().Page {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}),
	}
}

// webOnly holds the web view to the web: its pages go to http and https addresses of any host at the top level, besides the pages of their own the platform always lets them make, and nowhere else, as a file, the app's own schemes or another app's. The rest of the page's scope may do what the app's code may.
type webOnly struct{}

// Allows implements core.Access.
func (webOnly) Allows(req core.AccessRequest) bool {
	if req.Permission != core.AccessWebView || req.Method == core.AccessInput {
		return true
	}
	return Allowed(req.Resource)
}

// WebHosts implements core.Access.
func (webOnly) WebHosts() []string { return []string{"*"} }

// Allowed reports whether a page may go to rawURL: only http and https addresses, not files, the app's own schemes or other apps'.
func Allowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != ""
}

// Target is the address the address field's text opens in the interface's language: the text itself when it is an http or https address, the text after https:// when it reads as a host, as example.com/watch or localhost:8080, and otherwise a search for it, on Bing in simplified Chinese, where Google does not answer, and on Google elsewhere.
func Target(text, language string) string {
	if u, err := url.Parse(text); err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != "" {
		return text
	}
	if kcore.LooksLikeHost(text) {
		return "https://" + text
	}
	search := "https://www.google.com/search?q="
	if language == "zh-Hans" {
		search = "https://www.bing.com/search?q="
	}
	return search + url.QueryEscape(text)
}
