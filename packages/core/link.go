package core

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Platform is where the video behind a link lives. Its values double as the source IDs the sidebar filters the library by.
type Platform string

const (
	PlatformYouTube  Platform = "youtube"
	PlatformBilibili Platform = "bilibili"
	PlatformWeb      Platform = "web"
	PlatformLocal    Platform = "local"
)

// LinkKind is what a link points at: one video, a list that holds several (a playlist, a collection, a multi-part upload) or a channel.
type LinkKind string

const (
	LinkVideo    LinkKind = "video"
	LinkPlaylist LinkKind = "playlist"
	LinkChannel  LinkKind = "channel"
)

// Link is what ParseLink reads from a pasted address or a job's source.
type Link struct {
	// URL is the address with a scheme, so a bare "youtube.com/watch?v=…" becomes "https://youtube.com/watch?v=…". For a local file it is the file:// URL.
	URL      string
	Platform Platform
	Kind     LinkKind
	// ID is the video's ID (a YouTube video ID, a BV or av number, the code of a b23.tv short link), a channel's handle or number for a channel, and the file's path on this machine for a local file (FilePath). It is empty for other sites and when the link does not name one.
	ID string
	// List is a YouTube playlist ID (the list= parameter); empty otherwise.
	List string
	// Host is the host name in lower case without "www." or "m.". It is empty for a local file.
	Host string
	// Site is the site Kando knows the link's host as, by its main domain, such as "youtube.com" for youtu.be or "bilibili.com" for b23.tv; empty for a site Kando does not know and for a local file. The app names the site from it.
	Site string
	// Short reports a b23.tv short link, whose video is only known once the link is followed.
	Short bool
}

// OneVideo reports whether the link names one video a job can download: false for a channel, and for a list that names none of its videos, which yt-dlp would download whole even with --no-playlist. A list link that also names a video, such as YouTube's watch?v=…&list=…, is that video.
func (l Link) OneVideo() bool {
	switch l.Kind {
	case LinkChannel:
		return false
	case LinkPlaylist:
		// A collection on a bilibili channel's page keeps the channel's number in ID, which is not a video.
		return l.ID != "" && l.Host != "space.bilibili.com"
	}
	return true
}

var (
	bareVideoHost = regexp.MustCompile(`(?i)^(www\.|m\.)?(youtube\.com|youtu\.be|bilibili\.com|b23\.tv)`)
	youTubeID     = regexp.MustCompile(`^[\w-]{6,}$`)
	bilibiliBV    = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	bilibiliAV    = regexp.MustCompile(`(?i)\bav\d+`)
	bilibiliUID   = regexp.MustCompile(`^\d+$`)
)

// knownSites maps a domain to the ID of the site it belongs to, the site's main domain. Sites whose videos are DRM-protected are listed too, so the warning about them can name the site.
var knownSites = map[string]string{
	"youtube.com": "youtube.com", "youtu.be": "youtube.com", "bilibili.com": "bilibili.com", "b23.tv": "bilibili.com",
	"vimeo.com": "vimeo.com", "dailymotion.com": "dailymotion.com", "ted.com": "ted.com", "twitch.tv": "twitch.tv", "archive.org": "archive.org", "rumble.com": "rumble.com", "cnn.com": "cnn.com", "bbc.co.uk": "bbc.co.uk",
	"x.com": "x.com", "twitter.com": "x.com", "tiktok.com": "tiktok.com", "instagram.com": "instagram.com", "facebook.com": "facebook.com", "reddit.com": "reddit.com",
	"nicovideo.jp": "nicovideo.jp", "tver.jp": "tver.jp", "abema.tv": "abema.tv", "nhk.or.jp": "nhk.or.jp", "tv.naver.com": "tv.naver.com",
	"douyin.com": "douyin.com", "ixigua.com": "ixigua.com", "weibo.com": "weibo.com", "youku.com": "youku.com", "acfun.cn": "acfun.cn", "huya.com": "huya.com", "douyu.com": "douyu.com",
	"soundcloud.com": "soundcloud.com", "podcasts.apple.com": "podcasts.apple.com", "khanacademy.org": "khanacademy.org", "coursera.org": "coursera.org", "udemy.com": "udemy.com", "patreon.com": "patreon.com",
	"netflix.com": "netflix.com", "disneyplus.com": "disneyplus.com", "primevideo.com": "primevideo.com", "hulu.com": "hulu.com", "max.com": "max.com", "iq.com": "iqiyi.com", "iqiyi.com": "iqiyi.com", "v.qq.com": "v.qq.com", "mgtv.com": "mgtv.com", "u-next.jp": "u-next.jp",
}

// ParseLink reads which platform, video, list or channel a link names. It accepts http and https links, YouTube and bilibili addresses typed without a scheme, and file:// URLs of local videos. It reports false for anything else, including an empty string.
func ParseLink(raw string) (Link, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Link{}, false
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "file:") {
		if !bareVideoHost.MatchString(raw) {
			return Link{}, false
		}
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Link{}, false
	}
	if strings.EqualFold(u.Scheme, "file") {
		path, ok := FilePath(u)
		if !ok {
			return Link{}, false
		}
		return Link{URL: raw, Platform: PlatformLocal, Kind: LinkVideo, ID: path}, true
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return Link{}, false
	}
	host = strings.TrimPrefix(strings.TrimPrefix(host, "www."), "m.")
	link := Link{URL: raw, Platform: PlatformWeb, Kind: LinkVideo, Host: host, Site: siteOf(host)}
	switch {
	case onDomain(host, "youtube.com") || host == "youtu.be":
		link.Platform = PlatformYouTube
		youTubeLink(&link, u)
	case onDomain(host, "bilibili.com") || host == "b23.tv":
		link.Platform = PlatformBilibili
		bilibiliLink(&link, u)
	}
	return link, true
}

// FileURL is the file:// URL of the file at path, an absolute path on this machine, as a local video's job source: file:///Users/me/clip.mp4 for /Users/me/clip.mp4, and file:///C:/Users/me/clip.mp4 for C:\Users\me\clip.mp4 on Windows. Characters a URL cannot hold, such as spaces, are escaped. FilePath reads it back.
func FileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// FilePath is the path on this machine of the file a file:// URL names, the inverse of FileURL. On Windows the slash before a drive letter is dropped and the slashes become backslashes, so file:///C:/Users/me/clip.mp4 is C:\Users\me\clip.mp4. It reports false for a URL of another scheme, one without a path, and one whose host names another machine (anything but empty or localhost).
func FilePath(u *url.URL) (string, bool) {
	if u == nil || !strings.EqualFold(u.Scheme, "file") || u.Path == "" {
		return "", false
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return "", false
	}
	p := u.Path
	if rest, ok := strings.CutPrefix(p, "/"); ok {
		if volume := filepath.VolumeName(rest); len(volume) == 2 && volume[1] == ':' {
			p = rest
		}
	}
	return filepath.FromSlash(p), true
}

func youTubeLink(link *Link, u *url.URL) {
	parts := pathParts(u.Path)
	if link.Host == "youtu.be" {
		if len(parts) > 0 && youTubeID.MatchString(parts[0]) {
			link.ID = parts[0]
		}
	} else if len(parts) > 0 {
		switch {
		case strings.HasPrefix(parts[0], "@"):
			link.Kind, link.ID = LinkChannel, parts[0]
			return
		case parts[0] == "channel" || parts[0] == "c" || parts[0] == "user":
			link.Kind = LinkChannel
			if len(parts) > 1 {
				link.ID = parts[1]
			}
			return
		case parts[0] == "shorts" || parts[0] == "live" || parts[0] == "embed":
			if len(parts) > 1 && youTubeID.MatchString(parts[1]) {
				link.ID = parts[1]
			}
		}
	}
	query := u.Query()
	if v := query.Get("v"); link.ID == "" && youTubeID.MatchString(v) {
		link.ID = v
	}
	if list := query.Get("list"); list != "" {
		link.Kind, link.List = LinkPlaylist, list
	}
}

func bilibiliLink(link *Link, u *url.URL) {
	parts := pathParts(u.Path)
	if link.Host == "space.bilibili.com" {
		link.Kind = LinkChannel
		if len(parts) > 0 && bilibiliUID.MatchString(parts[0]) {
			link.ID = parts[0]
		}
		if listPath(u.Path) {
			link.Kind = LinkPlaylist
		}
		return
	}
	if link.Host == "b23.tv" {
		link.Short = true
		if len(parts) > 0 {
			link.ID = parts[0]
		}
	}
	if bv := bilibiliBV.FindString(u.Path); bv != "" {
		link.ID = bv
	} else if av := bilibiliAV.FindString(u.Path); av != "" && !link.Short {
		link.ID = strings.ToLower(av)
	}
	if u.Query().Get("p") != "" || listPath(u.Path) {
		link.Kind = LinkPlaylist
	}
}

// listPath reports a bilibili path that names a list, a collection or a season rather than one video.
func listPath(path string) bool {
	path = strings.ToLower(path)
	for _, word := range []string{"list", "channel", "collection", "season"} {
		if strings.Contains(path, word) {
			return true
		}
	}
	return false
}

func pathParts(path string) []string {
	var parts []string
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// onDomain reports whether host is domain or one of its subdomains.
func onDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func siteOf(host string) string {
	for domain, site := range knownSites {
		if onDomain(host, domain) {
			return site
		}
	}
	return ""
}
