package browse

import (
	"kando/apps/mobile/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// site is one of the sites the start offers: its name, the page it opens, and its mark, the brand's color behind a letter.
type site struct {
	id     string
	name   func(cx *core.Scope) string
	url    string
	mark   core.Color
	letter string
}

// named is a site's name that is the same in every language: a product's name, not a word of the interface.
func named(name string) func(*core.Scope) string { return func(*core.Scope) string { return name } }

// sites are the sites learners study from most whose videos Browse knows (kcore.VideoSites), so a page of one of them offers to send its video, in the order the start shows them. The design's Coursera is left out: Kando knows none of its pages as a video's.
var sites = []site{
	{id: "youtube", name: named("YouTube"), url: "https://m.youtube.com/", mark: core.RGB(0xE62117), letter: "▶"},
	{id: "bilibili", name: func(cx *core.Scope) string { return locales.BrowseSiteBilibili(cx) }, url: "https://m.bilibili.com/", mark: core.RGB(0x00A1D6), letter: "B"},
	{id: "ted", name: named("TED"), url: "https://www.ted.com/talks", mark: core.RGB(0xE62B1E), letter: "T"},
	{id: "vimeo", name: named("Vimeo"), url: "https://vimeo.com/watch", mark: core.RGB(0x1AB7EA), letter: "V"},
	{id: "nicovideo", name: func(cx *core.Scope) string { return locales.BrowseSiteNiconico(cx) }, url: "https://sp.nicovideo.jp/", mark: core.RGB(0x3A3A3C), letter: "ニ"},
	{id: "dailymotion", name: named("Dailymotion"), url: "https://www.dailymotion.com/", mark: core.RGB(0x0066DC), letter: "d"},
}
