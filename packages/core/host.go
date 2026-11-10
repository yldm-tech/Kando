package core

import (
	"net"
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
