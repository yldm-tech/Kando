package media

import (
	"strings"

	"kando/packages/core"
)

// failSigns are the phrases of yt-dlp's errors that tell why a download failed, in the order they are tried: a phrase that is more telling comes before one that many errors share. yt-dlp ends an error that may be its own bug with "Confirm you are on the latest version", so that one is tried last, after the errors that ask for a login and also carry it.
var failSigns = []struct {
	kind    core.FailKind
	phrases []string
}{
	// The browser's cookies could not be read: Safari's file is closed to an app without Full Disk Access, and a browser that is not installed has none.
	{core.FailCookies, []string{"Cookies.binarycookies", "cookies database", "could not find chrome", "could not find firefox", "failed to decrypt"}},
	{core.FailDRM, []string{"DRM protection", "DRM protected"}},
	{core.FailUnsupported, []string{"Unsupported URL"}},
	{core.FailRegion, []string{"geo restriction", "geo-restricted", "not available in your country", "not available from your location", "available in your country"}},
	{core.FailAge, []string{"confirm your age", "age-restricted", "age restricted", "inappropriate for some users"}},
	{core.FailMembers, []string{"members-only", "members only", "premium members", "Join this channel", "channel members", "大会员"}},
	{core.FailPrivate, []string{"Private video", "This video is private", "video is unavailable", "Video unavailable", "has been removed", "does not exist", "HTTP Error 404", "HTTP Error 410"}},
	{core.FailRateLimit, []string{"HTTP Error 429", "Too Many Requests", "rate-limit", "rate limit", "not a bot", "try again later"}},
	{core.FailLogin, []string{"registered users", "login required", "log in", "--cookies-from-browser or --cookies"}},
	{core.FailNetwork, []string{"Failed to resolve", "nodename nor servname", "Name or service not known", "Temporary failure in name resolution", "Connection refused", "Unable to connect to proxy", "Network is unreachable", "timed out", "Connection reset", "No route to host"}},
	{core.FailOutdated, []string{"Confirm you are on the latest version", "please report this issue", "Unable to extract", "No video formats found", "nsig extraction failed", "Signature extraction failed"}},
}

// ClassifyDownload reads why yt-dlp failed from what it printed on stderr, or from an error that carries those lines, as Download's does. Only its ERROR lines count: its warnings ask for an update for reasons that do not stop a download. It returns "" for an error it does not recognize.
func ClassifyDownload(stderr string) core.FailKind {
	var errs []string
	for _, line := range strings.Split(stderr, "\n") {
		if i := strings.Index(line, "ERROR:"); i >= 0 {
			errs = append(errs, line[i:])
		}
	}
	text := strings.ToLower(strings.Join(errs, "\n"))
	if text == "" {
		return ""
	}
	for _, sign := range failSigns {
		for _, phrase := range sign.phrases {
			if strings.Contains(text, strings.ToLower(phrase)) {
				return sign.kind
			}
		}
	}
	return ""
}
