package catalog

import "testing"

// Kando's browser takes the cookies of the video sites Browse is for, of hosts under them and of Google's for YouTube's sign-in, as yt-dlp writes a cookie's domain, and no other site's, though the catalog lists it; a cookie the site sets HttpOnly is marked so, and one its scripts read is not.
func TestSignInCookiesTakeOnlyBrowsesSites(t *testing.T) {
	for _, tc := range []struct {
		domain, name   string
		take, httpOnly bool
	}{
		{".youtube.com", "YSC", true, true},
		{".youtube.com", "VISITOR_INFO1_LIVE", true, true},
		{"www.youtube.com", "PREF", true, false},
		{"accounts.youtube.com", "SAPISID", true, false},
		{"accounts.google.com", "__Host-GAPS", true, true},
		{".google.com", "NID", true, true},
		{".google.com", "SAPISID", true, false},
		{".google.com", "SEARCH_SAMESITE", true, false},
		{".bilibili.com", "buvid3", true, false},
		{"passport.bilibili.com", "SESSDATA", true, false},
		{".tiktok.com", "ttwid", true, true},
		{".tiktok.com", "ttwid2", true, false},
		{".x.com", "ct0", true, false},
		{".twitter.com", "auth_token", true, false},
		{".YouTube.com.", "YSC", true, true},
		{"youtu.be", "x", true, false},
		{".nhk.or.jp", "x", true, false},
		{".bbc.co.uk", "x", true, false},
		{".ted.com", "x", true, false},
		{".nicovideo.jp", "user_session", true, false},
		{".vimeo.com", "vuid", true, false},
		{".dailymotion.com", "ts", true, false},
		{".amazon.com", "session-id", false, false},
		{".apple.com", "myacinfo", false, false},
		{".bank.example", "session", false, false},
		{"notyoutube.com", "YSC", false, false},
		{"youtube.com.evil.test", "YSC", false, false},
		{".com", "YSC", false, false},
		{"", "YSC", false, false},
	} {
		take, httpOnly := SignInCookies(tc.domain, tc.name)
		if take != tc.take || httpOnly != tc.httpOnly {
			t.Errorf("SignInCookies(%q, %q) = %v, %v; want %v, %v", tc.domain, tc.name, take, httpOnly, tc.take, tc.httpOnly)
		}
	}
	if !Load().On("https://www.amazon.com/") {
		t.Fatal("the catalog no longer lists amazon.com, which the test leaves out as a site of the catalog Browse is not for")
	}
}
