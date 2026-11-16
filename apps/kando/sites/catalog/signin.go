package catalog

import "strings"

// signInSite is a site whose sign-in Kando's browser takes from the user's own browser (SignInCookies): the cookies of Domain and the hosts under it, of which those named in HTTPOnly are marked HttpOnly, as the site sets them. yt-dlp's cookies file does not say which cookies are HttpOnly (media.ReadCookies), so a cookie not named here goes in as page scripts may read it, which is how the site's own scripts read the cookies they need, such as Google's SAPISID.
type signInSite struct {
	Domain   string
	HTTPOnly []string
}

// signInSites are the only sites whose cookies are taken: the video sites Browse is for. Every other domain's cookies stay in the user's browser. Each HTTPOnly name was seen with HttpOnly in the site's own Set-Cookie (an anonymous request on 2026-11-17, `curl -D - https://<host>/`); a cookie that is only set once the user signs in is not listed, as its flags could not be seen that way.
var signInSites = []signInSite{
	// YouTube: Browse's start page and the in-page buttons (browse/videos.go). Set-Cookie of https://www.youtube.com/: YSC, VISITOR_INFO1_LIVE, VISITOR_PRIVACY_METADATA, GPS, __Secure-YNID, __Secure-YEC and __Secure-ROLLOUT_TOKEN all with HttpOnly. youtube.com covers its own sign-in hosts, accounts.youtube.com and m.youtube.com among them.
	{Domain: "youtube.com", HTTPOnly: []string{"YSC", "VISITOR_INFO1_LIVE", "VISITOR_PRIVACY_METADATA", "GPS", "__Secure-YNID", "__Secure-YEC", "__Secure-ROLLOUT_TOKEN"}},
	// youtu.be: YouTube's short links, which redirect to youtube.com.
	{Domain: "youtu.be"},
	// Google: YouTube signs in with a Google account, whose session accounts.google.com keeps in .google.com's cookies. Set-Cookie of https://accounts.google.com/ServiceLogin: __Host-GAPS with HttpOnly; of https://www.google.com/: NID and AEC with HttpOnly, SEARCH_SAMESITE and __Secure-STRP without.
	{Domain: "google.com", HTTPOnly: []string{"__Host-GAPS", "NID", "AEC"}},
	// Bilibili: the start page and the in-page buttons; passport.bilibili.com, where it signs in, is under it. Set-Cookie of https://www.bilibili.com/ and https://passport.bilibili.com/login: buvid3 and b_nut, neither HttpOnly.
	{Domain: "bilibili.com"},
	// TED: the start page and the in-page buttons. https://www.ted.com/ set no cookie.
	{Domain: "ted.com"},
	// NHK: the start page. https://www.nhk.or.jp/ set no cookie.
	{Domain: "nhk.or.jp"},
	// Niconico: the start page and the in-page buttons; account.nicovideo.jp, where it signs in, is under it. Set-Cookie of https://www.nicovideo.jp/: nicosid, not HttpOnly.
	{Domain: "nicovideo.jp"},
	// BBC: the start page. https://www.bbc.co.uk/ set no cookie.
	{Domain: "bbc.co.uk"},
	// Vimeo: the in-page buttons. Set-Cookie of https://vimeo.com/: only Cloudflare's __cf_bm, with HttpOnly.
	{Domain: "vimeo.com", HTTPOnly: []string{"__cf_bm"}},
	// Dailymotion: the in-page buttons. Set-Cookie of https://www.dailymotion.com/: ff, ts and v1st, none HttpOnly.
	{Domain: "dailymotion.com"},
	// TikTok: the in-page buttons. Set-Cookie of https://www.tiktok.com/: ttwid, tt_csrf_token and tt_chain_token, all with HttpOnly.
	{Domain: "tiktok.com", HTTPOnly: []string{"ttwid", "tt_csrf_token", "tt_chain_token"}},
	// X: Set-Cookie of https://x.com/: guest_id, guest_id_ads, guest_id_marketing, personalization_id, gt and ct0 without HttpOnly, Cloudflare's __cf_bm with it.
	{Domain: "x.com", HTTPOnly: []string{"__cf_bm"}},
	// twitter.com: X's old domain, which still serves it.
	{Domain: "twitter.com"},
}

// SignInCookies reports, for a cookie of the user's own browser as yt-dlp saves it, its domain (".youtube.com" or "www.bilibili.com") and name, whether Kando's browser takes it, a cookie of one of signInSites or a host under one, and whether it is HttpOnly there as the site sets it. Any other site's cookie stays in the user's browser.
func SignInCookies(domain, name string) (take, httpOnly bool) {
	host := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "."), ".")
	for _, s := range signInSites {
		if host == s.Domain || strings.HasSuffix(host, "."+s.Domain) {
			for _, n := range s.HTTPOnly {
				if n == name {
					return true, true
				}
			}
			return true, false
		}
	}
	return false, false
}
