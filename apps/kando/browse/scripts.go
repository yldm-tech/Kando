package browse

import (
	"encoding/json"
	"runtime"
	"slices"
	"strings"

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

// PauseScript pauses the page's videos and sounds while another page of the window shows, so a hidden video stops downloading and decoding and the site's autoplay does not move on to the next one. The web view's own switch keeps the page quiet meanwhile, frames and what starts later included (the app's BrowseMuted); once the browser's page shows again, what this paused stays paused, for the learner to play again.
const PauseScript = `for (const m of document.querySelectorAll("video, audio")) m.pause();`

// UserAgentSuffix is what the pages' user agent ends with after the platform's own: on macOS Safari's version, as sites serve WKWebView's, which names no browser, older pages and refuse it their sign-in. WebKitGTK and WebView2 already name their browsers ("").
func UserAgentSuffix() string {
	if runtime.GOOS == "darwin" {
		return "Version/26.0 Safari/605.1.15"
	}
	return ""
}

// AgentQuestion is what the app asks a page for the pages' user agent, which a download sends with their cookies; PageAgent reads its answer, raw, the JSON the page gave.
const AgentQuestion = "navigator.userAgent"

// PageAgent reads the user agent a page gave for AgentQuestion: a header's value of printable ASCII, ending in UserAgentSuffix, as a page's script can say what it likes there. It reports false for any other answer.
func PageAgent(raw string) (string, bool) {
	var ua string
	if json.Unmarshal([]byte(raw), &ua) != nil {
		return "", false
	}
	ua = strings.TrimSpace(ua)
	if ua == "" || len(ua) > 512 || !strings.HasSuffix(ua, UserAgentSuffix()) || strings.ContainsFunc(ua, func(r rune) bool { return r < 0x20 || r > 0x7e }) {
		return "", false
	}
	return ua, true
}
