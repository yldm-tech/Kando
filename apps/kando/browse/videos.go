package browse

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	kcore "kando/packages/core"
)

// videoSite is a site whose links to one video the browser knows by their shape, so its pages get the browser's own buttons (PageScript): 下载视频 on a page of one video, and + over the thumbnail of each video a page lists. Domain is the site's host and its subdomains, but for Not, hosts of the site whose pages are no video, as ParseLink writes them (without www. and m.). Videos are the shapes of its videos' links; Rows are where the site keeps the buttons under a video, by CSS selector, the first one found taking 下载视频 at its start or at its end (End). Titles are the headings that name the video of its page, the first one showing; without one the document's title names it, without a leading count and without the site's name after it (Cut).
//
// The JavaScript of the pages reads this same data (PageScript), so the page and the app cannot tell a video apart differently; every shape is a link kcore.ParseLink reads as one video (videos_test.go). The patterns are regular expressions both Go and JavaScript read alike: no flags, no named groups, no lookaround.
type videoSite struct {
	Domain string       `json:"domain"`
	Not    []string     `json:"not,omitempty"`
	Videos []videoShape `json:"videos"`
	Rows   []videoRow   `json:"rows,omitempty"`
	Titles []string     `json:"titles,omitempty"`
	Cut    []string     `json:"cut,omitempty"`
}

// videoShape is a link to one video: its path, as the URL escapes it, matching Path, and, when Param is set, that query parameter's first value matching Value. Menu are the paths of the browser extension's menu item on links (ExtensionFiles), as match patterns write them, with * for any characters, query included (https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns): they take in every link of the shape, and as few others as such a pattern can, as the menu shows by them alone and the item adds only a link VideoLink takes.
type videoShape struct {
	Path  string   `json:"path"`
	Param string   `json:"param,omitempty"`
	Value string   `json:"value,omitempty"`
	Menu  []string `json:"menu"`
}

// videoRow is a site's row of buttons under a video, which 下载视频 goes into, at its start or, with End, at its end.
type videoRow struct {
	Selector string `json:"selector"`
	End      bool   `json:"end,omitempty"`
}

// videoSites are the sites the page's buttons know. YouTube's and 哔哩哔哩's follow kcore.ParseLink's own rules for a video; the others' are the shapes of their videos' pages, which ParseLink reads as one video like any link of a site it does not know. A site without Rows, or whose rows a page does not have, as YouTube's Shorts, gets 下载视频 over the corner of its player.
var videoSites = []videoSite{
	{
		Domain: "youtube.com",
		Videos: []videoShape{
			{Path: `^/watch/?$`, Param: "v", Value: `^[\w-]{6,}$`, Menu: []string{"/watch?*", "/watch/?*"}},
			{Path: `^/(shorts|live)/[\w-]{6,}/?$`, Menu: []string{"/shorts/*", "/live/*"}},
		},
		Rows:   []videoRow{{Selector: "ytd-watch-metadata #top-level-buttons-computed"}},
		Titles: []string{"ytd-watch-metadata h1", "ytd-reel-video-renderer[is-active] h2"},
		Cut:    []string{" - YouTube"},
	},
	{Domain: "youtu.be", Videos: []videoShape{{Path: `^/[\w-]{6,}/?$`, Menu: []string{"/*"}}}},
	{
		Domain: "bilibili.com",
		Not:    []string{"space.bilibili.com", "search.bilibili.com", "live.bilibili.com"},
		// Its pages may be under the reader's language, as /zh-Hans/video/BV…/, which ParseLink reads as the video's canonical link.
		Videos: []videoShape{{Path: `^/(` + kcore.BilibiliLocale + `/)?video/(BV[0-9A-Za-z]{10}|av[0-9]+)/?$`, Menu: []string{"/video/*", "/*/video/*"}}},
		Rows:   []videoRow{{Selector: ".video-toolbar-left-main", End: true}, {Selector: ".video-toolbar-left", End: true}},
		Titles: []string{"h1.video-title", ".video-info-title h1", ".video-title"},
		Cut:    []string{"_哔哩哔哩_bilibili"},
	},
	{Domain: "vimeo.com", Not: []string{"player.vimeo.com"}, Videos: []videoShape{{Path: `^/[0-9]+/?$`, Menu: []string{"/0*", "/1*", "/2*", "/3*", "/4*", "/5*", "/6*", "/7*", "/8*", "/9*"}}}},
	{Domain: "ted.com", Videos: []videoShape{{Path: `^/talks/[\w-]+/?$`, Menu: []string{"/talks/*"}}}},
	{Domain: "nicovideo.jp", Videos: []videoShape{{Path: `^/watch/(sm|nm|so)[0-9]+/?$`, Menu: []string{"/watch/sm*", "/watch/nm*", "/watch/so*"}}}},
	{Domain: "dailymotion.com", Videos: []videoShape{{Path: `^/video/[0-9A-Za-z]+/?$`, Menu: []string{"/video/*"}}}},
	{Domain: "tiktok.com", Videos: []videoShape{{Path: `^/@[^/]+/video/[0-9]+/?$`, Menu: []string{"/@*/video/*"}}}},
}

// compiledShape is a videoShape as Go reads it.
type compiledShape struct {
	path, value *regexp.Regexp
	param       string
}

var compiledSites = func() [][]compiledShape {
	out := make([][]compiledShape, len(videoSites))
	for i, s := range videoSites {
		for _, v := range s.Videos {
			c := compiledShape{path: regexp.MustCompile(v.Path), param: v.Param}
			if v.Param != "" {
				c.value = regexp.MustCompile(v.Value)
			}
			out[i] = append(out[i], c)
		}
	}
	return out
}()

// MaxVideoURL is the longest link the page's buttons speak of; a longer one is no video's.
const MaxVideoURL = 2048

// VideoLink reports whether rawURL is a link to one video of a site the page's buttons know (videoSites), as the page's script decides it too.
func VideoLink(rawURL string) bool {
	if len(rawURL) > MaxVideoURL {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	bare := strings.TrimPrefix(strings.TrimPrefix(host, "www."), "m.")
	for i, s := range videoSites {
		if host != s.Domain && !strings.HasSuffix(host, "."+s.Domain) {
			continue
		}
		for _, not := range s.Not {
			if bare == not {
				return false
			}
		}
		for _, v := range compiledSites[i] {
			if v.path.MatchString(u.EscapedPath()) && (v.param == "" || v.value.MatchString(u.Query().Get(v.param))) {
				return true
			}
		}
		return false
	}
	return false
}

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
