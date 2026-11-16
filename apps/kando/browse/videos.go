package browse

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// videoSite is a site whose links to one video the browser knows by their shape, so its pages get the browser's own buttons (PageScript): 下载视频 on a page of one video, and + over the thumbnail of each video a page lists. Domain is the site's host and its subdomains, but for Not, hosts of the site whose pages are no video, as ParseLink writes them (without www. and m.). Videos are the shapes of its videos' links; Rows are where the site keeps the buttons under a video, by CSS selector, the first one found taking 下载视频 at its start or at its end (End).
//
// The JavaScript of the pages reads this same data (PageScript), so the page and the app cannot tell a video apart differently; every shape is a link kcore.ParseLink reads as one video (videos_test.go). The patterns are regular expressions both Go and JavaScript read alike: no flags, no named groups, no lookaround.
type videoSite struct {
	Domain string       `json:"domain"`
	Not    []string     `json:"not,omitempty"`
	Videos []videoShape `json:"videos"`
	Rows   []videoRow   `json:"rows,omitempty"`
}

// videoShape is a link to one video: its path, as the URL escapes it, matching Path, and, when Param is set, that query parameter's first value matching Value.
type videoShape struct {
	Path  string `json:"path"`
	Param string `json:"param,omitempty"`
	Value string `json:"value,omitempty"`
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
			{Path: `^/watch/?$`, Param: "v", Value: `^[\w-]{6,}$`},
			{Path: `^/(shorts|live)/[\w-]{6,}/?$`},
		},
		Rows: []videoRow{{Selector: "ytd-watch-metadata #top-level-buttons-computed"}},
	},
	{Domain: "youtu.be", Videos: []videoShape{{Path: `^/[\w-]{6,}/?$`}}},
	{
		Domain: "bilibili.com",
		Not:    []string{"space.bilibili.com", "search.bilibili.com", "live.bilibili.com"},
		Videos: []videoShape{{Path: `^/video/(BV[0-9A-Za-z]{10}|av[0-9]+)/?$`}},
		Rows:   []videoRow{{Selector: ".video-toolbar-left-main", End: true}, {Selector: ".video-toolbar-left", End: true}},
	},
	{Domain: "vimeo.com", Not: []string{"player.vimeo.com"}, Videos: []videoShape{{Path: `^/[0-9]+/?$`}}},
	{Domain: "ted.com", Videos: []videoShape{{Path: `^/talks/[\w-]+/?$`}}},
	{Domain: "nicovideo.jp", Videos: []videoShape{{Path: `^/watch/(sm|nm|so)[0-9]+/?$`}}},
	{Domain: "dailymotion.com", Videos: []videoShape{{Path: `^/video/[0-9A-Za-z]+/?$`}}},
	{Domain: "tiktok.com", Videos: []videoShape{{Path: `^/@[^/]+/video/[0-9]+/?$`}}},
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

// videoSitesJSON is videoSites as the page's script reads them.
func videoSitesJSON() string {
	data, err := json.Marshal(videoSites)
	if err != nil {
		panic("browse: videoSites: " + err.Error())
	}
	return string(data)
}
