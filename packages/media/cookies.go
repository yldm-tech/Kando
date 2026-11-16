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

// cookieDirPrefix starts the name of each download's cookies folder in CookieDir.
const cookieDirPrefix = "cookies-"

// SweepCookieDirs removes from dir the cookies folders of downloads that never finished, as when the app was force-quit, crashed or lost power while yt-dlp ran, so the cookies they hold do not stay on disk. Only folders named as siteCookiesFile names them go; everything else in dir stays. The app calls it at startup, before any download can start, as one running would lose its cookies file. It reports nothing: what it cannot read or remove it leaves, and the next startup tries again.
func SweepCookieDirs(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), cookieDirPrefix) {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
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
	dir, err := os.MkdirTemp(base, cookieDirPrefix+"*")
	if err != nil {
		return "", "", done
	}
	// yt-dlp saves the cookie jar it ends with back to this file, and with Cookies set that jar also holds every cookie it read from that browser, so the folder must not outlive the run: done removes it after yt-dlp exits, and SweepCookieDirs removes one a run that never ended left behind.
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

// httpOnlyPrefix starts the domain of an HttpOnly cookie in a Netscape cookies file, as curl writes it and netscapeCookies does.
const httpOnlyPrefix = "#HttpOnly_"

// chromiumExpiryFloor is the least expiry ReadCookies takes for a Chromium browser's: yt-dlp of 2026.08.19 saves a Chromium browser's expires_utc as the browser keeps it (_process_chrome_cookie), in microseconds since 1601, where Firefox's and Safari's are in seconds since 1970. A time in seconds this large is millions of years away, and one in microseconds since 1601 above it is past 1632.
const chromiumExpiryFloor = 1e15

// chromiumEpoch is the start of a Chromium browser's time, 1601-01-01 UTC, in seconds before 1970's.
const chromiumEpoch = 11644473600

// ReadCookies reads a Netscape cookies file, as yt-dlp's --cookies saves the cookies it read from a browser: one cookie a line, its domain, whether its subdomains get it, its path, whether it goes only over https, when it expires (0 for a session cookie, and for a Chromium browser's in that browser's time, chromiumExpiryFloor), its name and its value, separated by tabs. A domain starting with #HttpOnly_ is an HttpOnly cookie's; other lines starting with # are comments. A line without seven fields or a name is left out. yt-dlp writes neither HttpOnly nor SameSite, as it does not read them from the browser.
func ReadCookies(data []byte) []Cookie {
	var cookies []Cookie
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		httpOnly := strings.HasPrefix(line, httpOnlyPrefix)
		line = strings.TrimPrefix(line, httpOnlyPrefix)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 || f[0] == "" || f[5] == "" {
			continue
		}
		domain := strings.ToLower(f[0])
		if f[1] == "TRUE" && !strings.HasPrefix(domain, ".") {
			domain = "." + domain
		}
		c := Cookie{Domain: domain, Path: f[2], Secure: f[3] == "TRUE", HTTPOnly: httpOnly, Name: f[5], Value: f[6]}
		if expires, err := strconv.ParseFloat(f[4], 64); err == nil && expires > 0 {
			if expires >= chromiumExpiryFloor {
				expires = expires/1e6 - chromiumEpoch
			}
			c.Expires = time.Unix(int64(expires), 0)
		}
		cookies = append(cookies, c)
	}
	return cookies
}
