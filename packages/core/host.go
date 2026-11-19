package core

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// WithScheme is text with https:// before it when it is a web address typed without a scheme, as people type and paste them: youtube.com/watch?v=…, b23.tv/BV…, localhost:8080. Anything else is text as it is.
func WithScheme(text string) string {
	if LooksLikeHost(text) {
		return "https://" + text
	}
	return text
}

// WebAddress reports whether rawURL is an address on the web a browser's page may go to: http or https with a host, not a file, an app's own scheme or a page of a page's own making (about:, data:, blob:).
func WebAddress(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != ""
}

// AddressTarget is the address a browser's address field opens for text in the interface's language (a BCP 47 tag): the text itself when it is an http or https address, the text after https:// when it reads as a host, as example.com/watch or localhost:8080, and otherwise a search for it, on Bing in simplified Chinese, where Google does not answer, and on Google elsewhere. Kando's browser on the computer and the phone's open what they are typed alike.
func AddressTarget(text, language string) string {
	if WebAddress(text) {
		return text
	}
	if LooksLikeHost(text) {
		return "https://" + text
	}
	search := "https://www.google.com/search?q="
	if language == "zh-Hans" {
		search = "https://www.bing.com/search?q="
	}
	return search + url.QueryEscape(text)
}

// LooksLikeHost reports whether text, without a scheme, starts with a host a browser would open rather than search for: localhost, an IP address, or a name of dot-separated labels whose last is a top-level domain of letters, with a port and a path after it or not. It is how an address typed without https:// is told from other text, in Kando's link field and its browser's address field alike.
func LooksLikeHost(text string) bool {
	if strings.ContainsAny(text, " \t\r\n") || strings.Contains(text, "://") {
		return false
	}
	host := text
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if strings.Contains(host, "@") {
		return false
	}
	if h, port, err := net.SplitHostPort(host); err == nil {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return false
		}
		host = h
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return true
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if r != '-' && !isHostLetter(r) && (r < '0' || r > '9') {
				return false
			}
		}
	}
	tld := labels[len(labels)-1]
	if strings.HasPrefix(strings.ToLower(tld), "xn--") {
		return true
	}
	letters := 0
	for _, r := range tld {
		if !isHostLetter(r) {
			return false
		}
		letters++
	}
	return letters >= 2
}

// isHostLetter reports whether r is a letter a host name may hold: ASCII, or any letter of an internationalized domain name.
func isHostLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 0x7F && r != 0xFFFD
}
