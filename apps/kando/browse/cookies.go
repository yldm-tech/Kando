package browse

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"golang.org/x/net/publicsuffix"
)

// CookieFilter is which cookies of the pages' store CookieSource asks for: those of Domain and every host under it, or, without a domain, those a request to URL sends.
type CookieFilter struct {
	URL    string
	Domain string
}

// CookieSource is media.Tools.SiteCookies over list, which lists the cookies of the browser's pages: for a source on a site, the cookies of its registrable domain and every host under it, so a video on www.example.com gets the sign-in example.com's accounts page left; for an IP address or a host without a public suffix, as localhost, those a request to it would send. It runs on the pipeline's goroutine; agent gives the pages' user agent, "" when it is not known. A source that is not an http or https address gets none.
func CookieSource(list func(CookieFilter) ([]media.Cookie, error), agent func() string) func(context.Context, string) ([]media.Cookie, string, error) {
	return func(_ context.Context, source string) ([]media.Cookie, string, error) {
		u, err := url.Parse(source)
		if err != nil || !Allowed(source) || u.Hostname() == "" {
			return nil, "", nil
		}
		filter := CookieFilter{URL: source}
		if host := strings.ToLower(u.Hostname()); net.ParseIP(host) == nil {
			if site, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
				filter = CookieFilter{Domain: site}
			}
		}
		cookies, err := list(filter)
		if err != nil {
			return nil, "", err
		}
		return cookies, agent(), nil
	}
}

// WebCookies are cookies of the user's own browser as the web views' store takes them (core.Scope.SetWebCookies), for the pages to be signed in as the user is there: HttpOnly, Secure and the expiry kept, and SameSite left to the platform, as yt-dlp's file does not say it (media.ReadCookies). A cookie that expired by now is left out, and so is one the store would refuse, which would fail the whole write (webCookieOK).
func WebCookies(cookies []media.Cookie, now time.Time) []core.WebCookie {
	var out []core.WebCookie
	for _, c := range cookies {
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			continue
		}
		w := core.WebCookie{Name: c.Name, Value: c.Value, Domain: strings.ToLower(c.Domain), Path: c.Path, Expires: c.Expires, Secure: c.Secure, HTTPOnly: c.HTTPOnly}
		if webCookieOK(w) {
			out = append(out, w)
		}
	}
	return out
}

// webCookieOK reports whether the web views' store takes c, as NSGo checks each cookie before it writes any (core/webcookie.go, checkWebCookie): a name without '=', ';', ',', spaces or control characters, a value without ';' or control characters, a path that starts with '/' and has no ';' or spaces, a host for a domain, Secure for a "__Secure-" name, and Secure, a host without a leading dot and the path "/" for a "__Host-" one. A browser can hold a cookie that breaks one of these, as an old site's comma in a name.
func webCookieOK(c core.WebCookie) bool {
	bad := func(s, also string) bool {
		return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f || strings.ContainsRune(also, r) })
	}
	host := c.Host()
	switch {
	case c.Name == "" || bad(c.Name, "=;, \t") || bad(c.Value, ";"):
		return false
	case c.Path != "" && (!strings.HasPrefix(c.Path, "/") || bad(c.Path, "; ")):
		return false
	case host == "" || bad(host, "/;, []:") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, "."):
		return false
	case strings.HasPrefix(c.Name, "__Secure-") && !c.Secure:
		return false
	case strings.HasPrefix(c.Name, "__Host-") && (!c.Secure || !c.HostOnly() || c.CookiePath() != "/"):
		return false
	}
	return true
}
