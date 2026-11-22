package browse

import (
	"runtime"
	"slices"

	kcore "kando/packages/core"
)

// The scripts the browser runs in its pages, and the user agent it gives them.

// Script runs in each page before its own scripts: the page's own buttons (buttonsScript). The web view itself tells the app the rest: a window a page asks to open, with whether the user asked for it (page, the app's browseOpenWindow), and a page that changes its address or its title without loading another, as a site that is one page does (the app's browseNavigated).
func Script() string { return ScriptWith(nil) }

// ScriptWith is Script whose buttons also know the sites of extra after Kando's own, the sites of the site packs the user allowed: 下载视频 on their pages of one video, over the corner of the player as they name no row of buttons, and + over their thumbnails. A site of extra that kcore.ValidateVideoSite refuses is left out, as its patterns would be compiled in the page. What the buttons post about those sites the app checks again with the same sites (the app's browsePageAsk, browsePageAdd). When extra changes, the web view loads its page again with the new script, once the browser's page shows (page, the app's browseTakeScript).
func ScriptWith(extra []kcore.VideoSite) string {
	sites := slices.Clone(videoSites)
	for _, s := range extra {
		if kcore.ValidateVideoSite(s) == nil {
			sites = append(sites, s)
		}
	}
	return buttonsScriptOf(webViewBridge, sitesJSON(sites))
}

// QuietScript quiets the page's videos and sounds while another page of the window shows (quiet), or lets them be heard again once the browser's shows. Quieting pauses what plays, so a hidden video stops downloading and decoding and the site's autoplay does not move on to the next one, and mutes what is not muted, marking it, as a page may start playing again by itself. Letting them be heard unmutes what it marked, so what the page muted itself stays muted, and leaves what it paused paused, for the learner to play again.
func QuietScript(quiet bool) string {
	if quiet {
		return `for (const m of document.querySelectorAll("video, audio")) { m.pause(); if (!m.muted) { m.muted = true; m.dataset.kandoMuted = "1"; } }`
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
