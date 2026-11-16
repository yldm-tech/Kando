package catalog

import "testing"

// Kando's browser takes the cookies of the catalog's sites, of hosts under them and of the domains their sign-in needs, as yt-dlp writes a cookie's domain, and no other site's.
func TestSignInCookiesTakeOnlyTheCatalogsSites(t *testing.T) {
	takes := Load().SignInCookies()
	for domain, want := range map[string]bool{
		".youtube.com":          true,
		"www.youtube.com":       true,
		"accounts.youtube.com":  true,
		".google.com":           true,
		"accounts.google.com":   true,
		".bilibili.com":         true,
		"passport.bilibili.com": true,
		".x.com":                true,
		"v.qq.com":              true,
		".YouTube.com.":         true,
		".bank.example":         false,
		"notyoutube.com":        false,
		"youtube.com.evil.test": false,
		".com":                  false,
		"":                      false,
		".":                     false,
	} {
		if got := takes(domain); got != want {
			t.Errorf("takes(%q) = %v, want %v", domain, got, want)
		}
	}
	empty := Catalog{}.SignInCookies()
	if !empty("accounts.google.com") || empty(".youtube.com") {
		t.Fatal("the sign-in domains are not the catalog's own")
	}
}
