package core

import (
	"testing"
	"time"
)

func TestStartAt(t *testing.T) {
	for raw, want := range map[string]time.Duration{
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=84":       84 * time.Second,
		"https://youtu.be/Rv3Hn7Qm2Ts?t=84s":                     84 * time.Second,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=1m24s":    84 * time.Second,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=1h2m3s":   time.Hour + 2*time.Minute + 3*time.Second,
		"https://www.youtube.com/embed/Rv3Hn7Qm2Ts?start=90":     90 * time.Second,
		"https://www.bilibili.com/video/BV1xx411c7mD?t=12.6":     13 * time.Second,
		"https://vimeo.com/123456#t=30s":                         30 * time.Second,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts":            0,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=0":        0,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=-5":       0,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=soon":     0,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=99999999": 0,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=5ms":      0,
		"not a link %": 0,
	} {
		if got := StartAt(raw); got != want {
			t.Errorf("StartAt(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestLinkDRM(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://www.netflix.com/watch/80100172":      true,
		"https://www.disneyplus.com/video/abc":        true,
		"https://www.iq.com/play/abc":                 true,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts": false,
		"https://vimeo.com/123456":                    false,
		"https://example.com/video":                   false,
	} {
		link, ok := ParseLink(raw)
		if !ok {
			t.Fatalf("ParseLink(%q) failed", raw)
		}
		if got := link.DRM(); got != want {
			t.Errorf("DRM(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestJobBrowser(t *testing.T) {
	for cookies, want := range map[string]string{"": "", CookiesNone: "", "chrome": "chrome"} {
		if got := (Job{Cookies: cookies}).Browser(); got != want {
			t.Errorf("Browser() of Cookies %q = %q, want %q", cookies, got, want)
		}
	}
}
