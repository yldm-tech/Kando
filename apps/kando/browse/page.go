package browse

import (
	"kando/apps/kando/live"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// pageFill is the page's background before the page paints its own, as a browser's is.
var pageFill = core.RGB(0xFFFFFF)

// page is the web view showing the web page. Its pages are someone else's: they may go to http and https addresses at the top level, and nowhere else (webOnly), not to pages of their own making (about:, data:, blob:) either, and they reach the app only through what they post to window.nsgo, which BrowseMessage hears as a plain string with the origin of the page that posted it. Every page reaches it, of whichever site the user went on to, as the app checks what each message asks of it.
type page struct{ a App }

// Render implements core.Node.
func (p page) Render(cx *core.Scope) core.Node {
	a := p.a
	// The web view refuses the other navigations as they start, a file dropped on it among them, and reports them to OnError.
	cx.ProvideAccess(webOnly{})
	return core.WebView{
		TestID:                          "browse-page",
		Ref:                             a.BrowseWeb(),
		SourceFunc:                      live.Func(a, func() core.WebSource { return core.WebSrc(a.BrowseSource()) }),
		UserAgent:                       a.BrowseUserAgent(),
		InjectedJavaScriptBeforeContent: a.BrowseScript(),
		BackForwardGestures:             true,
		PinchZoom:                       true,
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

// webOnly holds the web view to the web: its pages go to http and https addresses of any host at the top level, and nowhere else, as a file, the app's own schemes, another app's or a page of their own making (about:, data:, blob:). The rest of the page's scope may do what the app's code may.
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

// WebSchemes implements core.AccessWebSchemes: none of the pages a page makes itself.
func (webOnly) WebSchemes() []string { return nil }

// Allowed reports whether a page may go to rawURL: only http and https addresses, not files, the app's own schemes or other apps'.
func Allowed(rawURL string) bool { return kcore.WebAddress(rawURL) }
