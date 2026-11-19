package browse

import (
	"encoding/json"

	kcore "kando/packages/core"
)

// The sites the browser knows the videos of are Kando's, which the phone's browser knows them by too (kcore.VideoSites); the page's script reads the same data (sitesScript).
type videoSite = kcore.VideoSite

var videoSites = kcore.VideoSites

// MaxVideoURL is the longest link the page's buttons speak of; a longer one is no video's.
const MaxVideoURL = kcore.MaxVideoURL

// VideoLink reports whether rawURL is a link to one video of a site the page's buttons know (videoSites), as the page's script decides it too.
func VideoLink(rawURL string) bool { return kcore.VideoLink(rawURL) }

// sitesScript is the JavaScript that reads videoSites as VideoLink does: sites, siteOf(hostname), the site of a host or null, and videoOf(raw), the link raw resolves to against base, a JavaScript expression, when it is one video's, and "" otherwise. It expects MAX_URL to be declared before it. The page's buttons (buttonsScript) and the browser extension's background and popup (ExtensionFiles' sites.js) share it.
func sitesScript(base string) string {
	return `  const sites = (` + videoSitesJSON() + `).map((s) => ({
    domain: s.domain, not: s.not || [], rows: s.rows || [], titles: s.titles || [], cut: s.cut || [],
    videos: s.videos.map((v) => ({ path: new RegExp(v.path), param: v.param || "", value: v.param ? new RegExp(v.value) : null })),
  }));
  const siteOf = (hostname) => {
    const host = hostname.toLowerCase().replace(/\.$/, "");
    const bare = host.replace(/^www\./, "").replace(/^m\./, "");
    for (const s of sites) {
      if (host !== s.domain && !host.endsWith("." + s.domain)) continue;
      return s.not.includes(bare) ? null : s;
    }
    return null;
  };
  // videoOf is the link raw resolves to when it is one video's, as VideoLink decides it, and "" otherwise.
  const videoOf = (raw) => {
    if (!raw || raw.length > MAX_URL) return "";
    let u;
    try { u = new URL(raw, ` + base + `); } catch (e) { return ""; }
    if ((u.protocol !== "http:" && u.protocol !== "https:") || u.href.length > MAX_URL) return "";
    const s = siteOf(u.hostname);
    if (!s) return "";
    for (const v of s.videos) {
      if (v.path.test(u.pathname) && (!v.param || v.value.test(u.searchParams.get(v.param) || ""))) return u.href;
    }
    return "";
  };
`
}

// videoSitesJSON is videoSites as the page's script reads them.
func videoSitesJSON() string {
	data, err := json.Marshal(videoSites)
	if err != nil {
		panic("browse: videoSites: " + err.Error())
	}
	return string(data)
}
