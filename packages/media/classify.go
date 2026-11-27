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
	// The browser's cookies could not be read: Safari's file is closed to an app without Full Disk Access, a browser that is not installed has none, and on Windows an open Chromium browser holds them and Chrome's own encryption keeps them from yt-dlp (LoginTrouble tells these apart).
	{core.FailCookies, []string{"Cookies.binarycookies", "cookies database", "could not find chrome", "could not find firefox", "failed to decrypt", "could not copy chrome cookie database"}},
	{core.FailDRM, []string{"DRM protection", "DRM protected"}},
	{core.FailUnsupported, []string{"Unsupported URL"}},
	// YouTube's watch link with an id cut short (youtube:truncated_id), or without its v= part (youtube:truncated_url, which asks whether the shell split the URL at an &).
	{core.FailBadLink, []string{"looks truncated", "Incomplete YouTube ID", "forget to quote the URL"}},
	// The twitter extractor's words for a post whose answer from X has no video (extractor/twitter.py, raise_no_formats), as X answers for a sensitive post or a protected account's a viewer who may not see its media. Tumblr and Bluesky say "in this post", which is not this.
	{core.FailHiddenVideo, []string{"No video could be found in this tweet"}},
	{core.FailRegion, []string{"geo restriction", "geo-restricted", "not available in your country", "not available from your location", "available in your country"}},
	{core.FailAge, []string{"confirm your age", "age-restricted", "age restricted", "inappropriate for some users"}},
	{core.FailMembers, []string{"members-only", "members only", "premium members", "Join this channel", "channel members", "大会员"}},
	{core.FailPrivate, []string{"Private video", "This video is private", "video is unavailable", "Video unavailable", "has been removed", "does not exist", "HTTP Error 404", "HTTP Error 410"}},
	// Bilibili's video CDN answers a client that fetched too much in a short while with its own status 567 rather than 429, which yt-dlp prints as "HTTP Error 567: Unknown Status"; the same file downloads again a while later.
	{core.FailRateLimit, []string{"HTTP Error 429", "Too Many Requests", "HTTP Error 567", "rate-limit", "rate limit", "not a bot", "try again later"}},
	// yt-dlp's _login_hint ends a login error with a hint of the ways to log in, "Use --cookies, --cookies-from-browser, --username and --password, ... to provide account credentials" for a site that also takes a password; the errors above that carry it are told apart first.
	{core.FailLogin, []string{"registered users", "login required", "log in", "logged-in", "to provide account credentials", "Use --cookies for the authentication", "--cookies-from-browser"}},
	// A name that does not resolve for now (EAI_AGAIN) is the network; one that does not resolve at all (EAI_NONAME, macOS's "nodename nor servname") is a host that does not exist, or a network that is down.
	{core.FailNetwork, []string{"Temporary failure in name resolution"}},
	{core.FailHost, []string{"Failed to resolve", "nodename nor servname", "Name or service not known"}},
	{core.FailNetwork, []string{"Connection refused", "Unable to connect to proxy", "Network is unreachable", "timed out", "Connection reset", "No route to host"}},
	// YouTube answers a client whose JavaScript challenge was not solved, as the solver scripts of a yt-dlp older than YouTube's player cannot solve it, with "The page needs to be reloaded" for a signed-in download, and yt-dlp then finds no formats; one with no JavaScript runtime at all is told apart before (downloadFailKind).
	{core.FailOutdated, []string{"Confirm you are on the latest version", "please report this issue", "Unable to extract", "No video formats found", "nsig extraction failed", "Signature extraction failed", "The page needs to be reloaded"}},
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
