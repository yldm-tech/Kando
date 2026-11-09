// Package sitename names the sites core.ParseLink knows (core.Link.Site) in the interface's language. It imports no interface package, so the library page and the guide both name sites with it.
package sitename

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/i18n"
)

// latin are the sites whose names are brands written in Latin letters, the same in every language.
var latin = map[string]string{
	"youtube.com": "YouTube", "vimeo.com": "Vimeo", "dailymotion.com": "Dailymotion", "ted.com": "TED", "twitch.tv": "Twitch", "archive.org": "Internet Archive", "rumble.com": "Rumble", "cnn.com": "CNN", "bbc.co.uk": "BBC",
	"x.com": "X", "tiktok.com": "TikTok", "instagram.com": "Instagram", "facebook.com": "Facebook", "reddit.com": "Reddit",
	"tver.jp": "TVer", "abema.tv": "ABEMA", "nhk.or.jp": "NHK", "acfun.cn": "AcFun",
	"soundcloud.com": "SoundCloud", "khanacademy.org": "Khan Academy", "coursera.org": "Coursera", "udemy.com": "Udemy", "patreon.com": "Patreon",
	"netflix.com": "Netflix", "disneyplus.com": "Disney+", "primevideo.com": "Prime Video", "hulu.com": "Hulu", "max.com": "Max", "u-next.jp": "U-NEXT",
}

// Of is the name people know site by, a core.Link.Site such as "bilibili.com", in the interface's language; "" for a site it does not know.
func Of(site string) string { return In(locales.In(i18n.Locale()), site) }

// In is Of in the language of m, as the feedback issue names a site in the language it is written in.
func In(m locales.Messages, site string) string {
	switch site {
	case "bilibili.com":
		return m.SourceBilibili()
	case "nicovideo.jp":
		return m.SourceNiconico()
	case "tv.naver.com":
		return m.SourceNaverTV()
	case "douyin.com":
		return m.SourceDouyin()
	case "ixigua.com":
		return m.SourceXigua()
	case "weibo.com":
		return m.SourceWeibo()
	case "youku.com":
		return m.SourceYouku()
	case "huya.com":
		return m.SourceHuya()
	case "douyu.com":
		return m.SourceDouyu()
	case "podcasts.apple.com":
		return m.SourceApplePodcasts()
	case "iqiyi.com":
		return m.SourceIqiyi()
	case "v.qq.com":
		return m.SourceTencentVideo()
	case "mgtv.com":
		return m.SourceMangoTV()
	}
	return latin[site]
}
