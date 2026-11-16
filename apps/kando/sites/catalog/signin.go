package catalog

import "strings"

// signInDomains are the domains outside the catalog whose cookies a site of the catalog needs to be signed in, as the sign-in of the user's own browser is carried into Kando's (SignInCookies). A site's sign-in pages under its own domain need nothing here, as a domain takes the hosts under it: passport.bilibili.com is bilibili.com's, accounts.youtube.com youtube.com's, sso.douyin.com douyin.com's, passport.twitch.tv twitch.tv's.
var signInDomains = []string{
	// YouTube signs in with a Google account: accounts.google.com keeps the session in .google.com's cookies (SID, HSID, SSID, __Secure-1PSID, __Secure-3PSID), and youtube.com's pages check the account there. The catalog lists google.com only for yt-dlp's Google Drive extractor, which a newer snapshot could drop.
	"google.com",
}

// SignInCookies reports, for the domain of a cookie of the user's own browser as yt-dlp saves it (".youtube.com" or "www.bilibili.com"), whether Kando's browser takes it: a cookie of a site of c, its domain or a domain among its aliases or a host under one, or of a domain of signInDomains. A cookie of any other site stays in the user's browser.
func (c Catalog) SignInCookies() func(domain string) bool {
	domains := map[string]bool{}
	for _, d := range signInDomains {
		domains[d] = true
	}
	for _, s := range c.Sites {
		for _, d := range append([]string{s.Domain}, s.Aliases...) {
			if strings.Contains(d, ".") {
				domains[strings.ToLower(d)] = true
			}
		}
	}
	return func(domain string) bool {
		host := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "."), ".")
		for host != "" {
			if domains[host] {
				return true
			}
			_, parent, ok := strings.Cut(host, ".")
			if !ok {
				return false
			}
			host = parent
		}
		return false
	}
}
