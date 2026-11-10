package media

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Cookie is a cookie the app's own web pages hold for a site, which a download sends to it (Tools.SiteCookies).
type Cookie struct {
	// Domain is the host the cookie is sent to, such as "example.com", or with a leading dot, ".example.com", the domain whose subdomains get it too.
	Domain string
	// Path is the path the cookie is sent for; empty means "/".
	Path     string
	Secure   bool
	HTTPOnly bool
	// Expires is when the cookie expires; zero for one that lasts until the app quits.
	Expires time.Time
	Name    string
	Value   string
}

// cookiesHeader is the first line of a Netscape cookies file, which yt-dlp's --cookies reads.
const cookiesHeader = "# Netscape HTTP Cookie File\n"

// siteCookiesFile asks SiteCookies for the cookies of source's site and writes them to a cookies file for yt-dlp, in a folder of its own made for this run in CookieDir. It returns the file, the user agent that goes with the cookies ("" when unknown) and done, which removes the folder; the caller runs done once yt-dlp has exited. Without SiteCookies, when it fails or gives no cookies, or when the file cannot be written, file is "" and the download goes without cookies. Neither the cookies nor their values ever reach an error or a log.
func (t Tools) siteCookiesFile(ctx context.Context, source string) (file, userAgent string, done func()) {
	done = func() {}
	if t.SiteCookies == nil {
		return "", "", done
	}
	cookies, userAgent, err := t.SiteCookies(ctx, source)
	if err != nil {
		return "", "", done
	}
	data, n := netscapeCookies(cookies)
	if n == 0 {
		return "", "", done
	}
	base := t.CookieDir
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", "", done
	}
	dir, err := os.MkdirTemp(base, "cookies-")
	if err != nil {
		return "", "", done
	}
	done = func() { _ = os.RemoveAll(dir) }
	file = filepath.Join(dir, "cookies.txt")
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		done()
		return "", "", func() {}
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		done()
		return "", "", func() {}
	}
	return file, strings.TrimSpace(userAgent), done
}

// netscapeCookies writes cookies in the Netscape cookies file format yt-dlp reads, one a line: the domain, whether its subdomains get the cookie (TRUE exactly when the domain has a leading dot, as Python's cookie jar insists), the path, whether it goes only over https, when it expires in seconds since 1970 (0 for one that lasts until the app quits), its name and its value. An HttpOnly cookie's domain starts with #HttpOnly_, which yt-dlp reads as curl writes it. A cookie the format cannot hold, without a domain or with a tab or a line break in a field, is left out. n is how many cookies were written.
func netscapeCookies(cookies []Cookie) (data []byte, n int) {
	var b strings.Builder
	b.WriteString(cookiesHeader)
	for _, c := range cookies {
		domain := strings.ToLower(strings.TrimSpace(c.Domain))
		path := c.Path
		if path == "" {
			path = "/"
		}
		if domain == "" || domain == "." || c.Name == "" || !cookieField(domain) || !cookieField(path) || !cookieField(c.Name) || !cookieField(c.Value) {
			continue
		}
		if c.HTTPOnly {
			b.WriteString("#HttpOnly_")
		}
		var expires int64
		if !c.Expires.IsZero() {
			expires = max(c.Expires.Unix(), 1)
		}
		for i, field := range []string{domain, netscapeBool(strings.HasPrefix(domain, ".")), path, netscapeBool(c.Secure), strconv.FormatInt(expires, 10), c.Name, c.Value} {
			if i > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(field)
		}
		b.WriteByte('\n')
		n++
	}
	return []byte(b.String()), n
}

// cookieField reports whether s fits in a field of a Netscape cookies file, which tabs separate and line breaks end.
func cookieField(s string) bool { return !strings.ContainsAny(s, "\t\r\n") }

func netscapeBool(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}
