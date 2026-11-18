package browserext

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// The bookmarklet opens a link of Kando's scheme naming the page, its title and where its video was, which ReadLink reads back.
func TestBookmarkletOpensKandosLink(t *testing.T) {
	code := Bookmarklet()
	if !strings.HasPrefix(code, "javascript:") || !strings.Contains(code, `"kando://add?url="`) || strings.ContainsAny(code, "\n`") {
		t.Fatalf("bookmarklet %q", code)
	}
	page := "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts"
	link := "kando://add?url=" + url.QueryEscape(page) + "&title=" + url.QueryEscape("Why Glaciers\nMove") + "&t=84"
	got, title, at, ok := ReadLink(link)
	if !ok || got != page || title != "Why Glaciers Move" || at != 84*time.Second {
		t.Fatalf("ReadLink = %q, %q, %v, %v", got, title, at, ok)
	}
	if _, _, at, ok := ReadLink("kando://add?url=" + url.QueryEscape(page) + "&t=soon"); !ok || at != 0 {
		t.Fatalf("a time that is none: %v, %v", at, ok)
	}
	if _, title, _, _ := ReadLink("kando://add?url=" + url.QueryEscape(page) + "&title=" + strings.Repeat("x", 400)); len([]rune(title)) != maxTitle+1 {
		t.Fatalf("long title kept %d characters", len([]rune(title)))
	}
}

// Links that are not the bookmarklet's, or name no web page, are refused.
func TestReadLinkRefuses(t *testing.T) {
	for _, link := range []string{
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts",
		"kando://open?url=https%3A%2F%2Fexample.com%2Fv",
		"kando://add",
		"kando://add?url=file%3A%2F%2F%2Fetc%2Fpasswd",
		"kando://add?url=javascript%3Aalert(1)",
		"other://add?url=https%3A%2F%2Fexample.com%2Fv",
	} {
		if _, _, _, ok := ReadLink(link); ok {
			t.Errorf("ReadLink(%q) took it", link)
		}
	}
}
