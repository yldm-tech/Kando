package browse

import "runtime"

// The scripts the browser runs in its pages, and the user agent it gives them.

// HistoryMessage starts what Script has a page post when it changes its address without loading another page: "push", "replace" or "pop" follows it.
const HistoryMessage = "kando-history:"

// Script runs in each page before its own scripts. A link or a script that opens a window loads its address in the same view when the user clicked it, and does nothing otherwise, as a pop-up a script opens by itself, so the page never opens a window of the app; the web view's Access keeps it to http and https addresses. A page that changes its address without loading another, as a site that is one page does, says so (HistoryMessage), and the app reads the address from the page (the app's browseHeard).
func Script() string {
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
  const post = (kind) => { try { window.nsgo.postMessage("` + HistoryMessage + `" + kind); } catch (e) {} };
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

// MuteScript mutes the page's videos and sounds, marking the ones it muted, or unmutes those it marked, so what the page muted itself stays muted.
func MuteScript(muted bool) string {
	if muted {
		return `for (const m of document.querySelectorAll("video, audio")) { if (!m.muted) { m.muted = true; m.dataset.kandoMuted = "1"; } }`
	}
	return `for (const m of document.querySelectorAll("[data-kando-muted]")) { m.muted = false; delete m.dataset.kandoMuted; }`
}

// UserAgent is the pages' user agent: on macOS WKWebView's own, which names no browser, with Safari's version after it, as sites serve such a user agent older pages and refuse it their sign-in. WebKitGTK and WebView2 already name their browsers and keep their own ("").
func UserAgent() string {
	if runtime.GOOS == "darwin" {
		// TODO(nsgo#114): append Safari's version to the platform's own user agent, as MyGo's PageOptions.UserAgentSuffix did, rather than replacing it with WKWebView's as macOS writes it today https://github.com/nsgo-dev/nsgo/issues/114
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15"
	}
	return ""
}
