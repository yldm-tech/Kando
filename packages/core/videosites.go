package core

import (
	"net/url"
	"regexp"
	"strings"
)

// VideoSite is a site whose links to one video Kando knows by their shape, so the browsers of Kando on a computer and on a phone know a page of one video: the computer's pages get its own buttons (apps/kando/browse PageScript): 下载视频 on a page of one video, and + over the thumbnail of each video a page lists. Domain is the site's host and its subdomains, but for Not, hosts of the site whose pages are no video, as ParseLink writes them (without www. and m.). Videos are the shapes of its videos' links; Rows are where the site keeps the buttons under a video, by CSS selector, the first one found taking 下载视频 at its start or at its end (End). Titles are the headings that name the video of its page, the first one showing; without one the document's title names it, without a leading count and without the site's name after it (Cut).
//
// The JavaScript of the computer's pages reads this same data (apps/kando/browse PageScript), so the page and the app cannot tell a video apart differently; every shape is a link ParseLink reads as one video (apps/kando/browse/videos_test.go). The patterns are regular expressions both Go and JavaScript read alike: no flags, no named groups, no lookaround.
type VideoSite struct {
	Domain string       `json:"domain"`
	Not    []string     `json:"not,omitempty"`
	Videos []VideoShape `json:"videos"`
	Rows   []VideoRow   `json:"rows,omitempty"`
	Titles []string     `json:"titles,omitempty"`
	Cut    []string     `json:"cut,omitempty"`
}

// VideoShape is a link to one video: its path, as the URL escapes it, matching Path, and, when Param is set, that query parameter's first value matching Value. Menu are the paths of the browser extension's menu item on links (apps/kando/browse ExtensionFiles), as match patterns write them, with * for any characters, query included (https://developer.chrome.com/docs/extensions/develop/concepts/match-patterns): they take in every link of the shape, and as few others as such a pattern can, as the menu shows by them alone and the item adds only a link VideoLink takes.
type VideoShape struct {
	Path  string   `json:"path"`
	Param string   `json:"param,omitempty"`
	Value string   `json:"value,omitempty"`
	Menu  []string `json:"menu"`
}

// VideoRow is a site's row of buttons under a video, which 下载视频 goes into, at its start or, with End, at its end.
type VideoRow struct {
	Selector string `json:"selector"`
	End      bool   `json:"end,omitempty"`
}

// VideoSites are the sites Kando's browsers know the videos of. YouTube's and 哔哩哔哩's follow ParseLink's own rules for a video; the others' are the shapes of their videos' pages, which ParseLink reads as one video like any link of a site it does not know. A site without Rows, or whose rows a page does not have, as YouTube's Shorts, gets 下载视频 over the corner of its player.
var VideoSites = []VideoSite{
	{
		Domain: "youtube.com",
		Videos: []VideoShape{
			{Path: `^/watch/?$`, Param: "v", Value: `^[\w-]{6,}$`, Menu: []string{"/watch?*", "/watch/?*"}},
			{Path: `^/(shorts|live)/[\w-]{6,}/?$`, Menu: []string{"/shorts/*", "/live/*"}},
		},
		Rows:   []VideoRow{{Selector: "ytd-watch-metadata #top-level-buttons-computed"}},
		Titles: []string{"ytd-watch-metadata h1", "ytd-reel-video-renderer[is-active] h2"},
		Cut:    []string{" - YouTube"},
	},
	{Domain: "youtu.be", Videos: []VideoShape{{Path: `^/[\w-]{6,}/?$`, Menu: []string{"/*"}}}},
	{
		Domain: "bilibili.com",
		Not:    []string{"space.bilibili.com", "search.bilibili.com", "live.bilibili.com"},
		// Its pages may be under the reader's language, as /zh-Hans/video/BV…/, which ParseLink reads as the video's canonical link.
		Videos: []VideoShape{{Path: `^/(` + BilibiliLocale + `/)?video/(BV[0-9A-Za-z]{10}|av[0-9]+)/?$`, Menu: []string{"/video/*", "/*/video/*"}}},
		Rows:   []VideoRow{{Selector: ".video-toolbar-left-main", End: true}, {Selector: ".video-toolbar-left", End: true}},
		Titles: []string{"h1.video-title", ".video-info-title h1", ".video-title"},
		Cut:    []string{"_哔哩哔哩_bilibili"},
	},
	{Domain: "vimeo.com", Not: []string{"player.vimeo.com"}, Videos: []VideoShape{{Path: `^/[0-9]+/?$`, Menu: []string{"/0*", "/1*", "/2*", "/3*", "/4*", "/5*", "/6*", "/7*", "/8*", "/9*"}}}},
	{Domain: "ted.com", Videos: []VideoShape{{Path: `^/talks/[\w-]+/?$`, Menu: []string{"/talks/*"}}}},
	{Domain: "nicovideo.jp", Videos: []VideoShape{{Path: `^/watch/(sm|nm|so)[0-9]+/?$`, Menu: []string{"/watch/sm*", "/watch/nm*", "/watch/so*"}}}},
	{Domain: "dailymotion.com", Videos: []VideoShape{{Path: `^/video/[0-9A-Za-z]+/?$`, Menu: []string{"/video/*"}}}},
	{Domain: "tiktok.com", Videos: []VideoShape{{Path: `^/@[^/]+/video/[0-9]+/?$`, Menu: []string{"/@*/video/*"}}}},
}

// compiledVideoShape is a VideoShape as Go reads it.
type compiledVideoShape struct {
	path, value *regexp.Regexp
	param       string
}

var compiledVideoSites = func() [][]compiledVideoShape {
	out := make([][]compiledVideoShape, len(VideoSites))
	for i, s := range VideoSites {
		for _, v := range s.Videos {
			c := compiledVideoShape{path: regexp.MustCompile(v.Path), param: v.Param}
			if v.Param != "" {
				c.value = regexp.MustCompile(v.Value)
			}
			out[i] = append(out[i], c)
		}
	}
	return out
}()

// MaxVideoURL is the longest link Kando's browsers take for a video's; a longer one is no video's.
const MaxVideoURL = 2048

// VideoLink reports whether rawURL is a link to one video of a site Kando's browsers know (VideoSites), as the computer's page script decides it too.
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
	for i, s := range VideoSites {
		if host != s.Domain && !strings.HasSuffix(host, "."+s.Domain) {
			continue
		}
		for _, not := range s.Not {
			if bare == not {
				return false
			}
		}
		for _, v := range compiledVideoSites[i] {
			if v.path.MatchString(u.EscapedPath()) && (v.param == "" || v.value.MatchString(u.Query().Get(v.param))) {
				return true
			}
		}
		return false
	}
	return false
}

// pageCount is the count of unread notifications a site puts before its page's title, as YouTube's "(3) ".
var pageCount = regexp.MustCompile(`^\(\d+\+?\) `)

// VideoTitle is the title of the video on the page at rawURL, a link VideoLink takes, from the document's title: without the count of notifications before it and the site's name after it (VideoSite.Cut), as the computer's page script cuts it. Other pages' titles stay as they are, trimmed.
func VideoTitle(rawURL, title string) string {
	t := strings.TrimSpace(title)
	u, err := url.Parse(rawURL)
	if err != nil || !VideoLink(rawURL) {
		return t
	}
	t = pageCount.ReplaceAllString(t, "")
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	for _, s := range VideoSites {
		if host != s.Domain && !strings.HasSuffix(host, "."+s.Domain) {
			continue
		}
		for _, cut := range s.Cut {
			if rest, ok := strings.CutSuffix(t, cut); ok {
				t = strings.TrimSpace(rest)
			}
		}
		break
	}
	return t
}
