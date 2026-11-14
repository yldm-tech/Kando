package browse

import (
	"context"
	"net"
	"net/url"
	"strings"

	"kando/packages/media"

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
