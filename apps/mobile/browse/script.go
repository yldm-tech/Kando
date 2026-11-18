package browse

import (
	"encoding/json"
	"net/url"
	"strings"

	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// pageMessage starts what pageScript has a page post as it changes its address without loading another page, or its title: a JSON pageState follows it.
const pageMessage = "kando-page:"

// pageState is the address and the title a page posted.
type pageState struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// pageScript runs in each page before its own scripts. A link or a script that opens a window loads its address in the same view when the user tapped it, and does nothing otherwise, as a pop-up a script opens by itself, so the page never opens a window of its own; and the page posts its address and title (pageMessage) as it pushes or replaces an address in its history, goes back within it, or changes its title, as the sites that are one page do as they move to another video, which the web view does not report.
// TODO(nsgo#113): hear a page open a window, with whether the user asked for it, and change its address and title within the document from the web view itself, as the computer's browser waits for too, then drop this script https://github.com/nsgo-dev/nsgo/issues/113
const pageScript = `(() => {
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
    if (target === "" || target === "_self" || target === "_top" || target === "_parent") return;
    e.preventDefault();
    go(a.href);
  });
  window.open = function (url) {
    if (url && navigator.userActivation && navigator.userActivation.isActive) go(String(url));
    return null;
  };
  const post = () => {
    try { window.nsgo.postMessage("` + pageMessage + `" + JSON.stringify({ url: location.href, title: document.title })); } catch (e) {}
  };
  for (const name of ["pushState", "replaceState"]) {
    const original = history[name];
    history[name] = function (...args) {
      const result = original.apply(this, args);
      post();
      return result;
    };
  }
  addEventListener("popstate", post);
  let title = null;
  const retitled = () => {
    if (document.title === title) return;
    title = document.title;
    post();
  };
  const watchTitle = () => {
    new MutationObserver(retitled).observe(document.head || document.documentElement, { childList: true, subtree: true, characterData: true });
    retitled();
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", watchTitle); else watchTitle();
})();
`

// parsePage reads what a page posted (pageScript): its address and title, when the address is on the web and of the origin that posted it, so no page can claim to be another site's.
func parsePage(msg core.WebMessage) (pageState, bool) {
	rest, ok := strings.CutPrefix(msg.Data, pageMessage)
	if !ok {
		return pageState{}, false
	}
	var p pageState
	if json.Unmarshal([]byte(rest), &p) != nil || !kcore.WebAddress(p.URL) {
		return pageState{}, false
	}
	u, err := url.Parse(p.URL)
	if err != nil || !strings.EqualFold(u.Scheme+"://"+u.Host, msg.Origin) {
		return pageState{}, false
	}
	return p, true
}

// webOnly holds the web view to the web: its pages go to http and https addresses of any host at the top level, and nowhere else, as a file, the app's own schemes, another app's (a site's link that would open its own app) or a page of their own making (about:, data:, blob:). The rest of the page's scope may do what the app's code may.
type webOnly struct{}

// Allows implements core.Access.
func (webOnly) Allows(req core.AccessRequest) bool {
	if req.Permission != core.AccessWebView || req.Method == core.AccessInput {
		return true
	}
	return kcore.WebAddress(req.Resource)
}

// WebHosts implements core.Access.
func (webOnly) WebHosts() []string { return []string{"*"} }

// WebSchemes implements core.AccessWebSchemes: none of the pages a page makes itself.
func (webOnly) WebSchemes() []string { return nil }
