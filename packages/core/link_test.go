package core

import "testing"

func TestParseLink(t *testing.T) {
	cases := []struct {
		raw  string
		want Link
	}{
		{"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=84", Link{URL: "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&t=84", Platform: PlatformYouTube, Kind: LinkVideo, ID: "Rv3Hn7Qm2Ts", Host: "youtube.com", Site: "youtube.com"}},
		{"https://youtu.be/TRWgBxVAb1M?si=abc", Link{URL: "https://youtu.be/TRWgBxVAb1M?si=abc", Platform: PlatformYouTube, Kind: LinkVideo, ID: "TRWgBxVAb1M", Host: "youtu.be", Site: "youtube.com"}},
		{"https://m.youtube.com/shorts/aBcDeFgHiJk", Link{URL: "https://m.youtube.com/shorts/aBcDeFgHiJk", Platform: PlatformYouTube, Kind: LinkVideo, ID: "aBcDeFgHiJk", Host: "youtube.com", Site: "youtube.com"}},
		{"https://music.youtube.com/watch?v=Rv3Hn7Qm2Ts", Link{URL: "https://music.youtube.com/watch?v=Rv3Hn7Qm2Ts", Platform: PlatformYouTube, Kind: LinkVideo, ID: "Rv3Hn7Qm2Ts", Host: "music.youtube.com", Site: "youtube.com"}},
		{"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&list=PLx0sYbCqOb8TBPRdmBHs5Iftvv9TPboYG", Link{URL: "https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&list=PLx0sYbCqOb8TBPRdmBHs5Iftvv9TPboYG", Platform: PlatformYouTube, Kind: LinkPlaylist, ID: "Rv3Hn7Qm2Ts", List: "PLx0sYbCqOb8TBPRdmBHs5Iftvv9TPboYG", Host: "youtube.com", Site: "youtube.com"}},
		{"https://www.youtube.com/playlist?list=PLx0sYbCqOb8", Link{URL: "https://www.youtube.com/playlist?list=PLx0sYbCqOb8", Platform: PlatformYouTube, Kind: LinkPlaylist, List: "PLx0sYbCqOb8", Host: "youtube.com", Site: "youtube.com"}},
		{"https://www.youtube.com/@veritasium/videos", Link{URL: "https://www.youtube.com/@veritasium/videos", Platform: PlatformYouTube, Kind: LinkChannel, ID: "@veritasium", Host: "youtube.com", Site: "youtube.com"}},
		{"https://www.youtube.com/channel/UCHnyfMqiRRG1u-2MsSQLbXA", Link{URL: "https://www.youtube.com/channel/UCHnyfMqiRRG1u-2MsSQLbXA", Platform: PlatformYouTube, Kind: LinkChannel, ID: "UCHnyfMqiRRG1u-2MsSQLbXA", Host: "youtube.com", Site: "youtube.com"}},
		{"youtube.com/watch?v=Rv3Hn7Qm2Ts", Link{URL: "https://youtube.com/watch?v=Rv3Hn7Qm2Ts", Platform: PlatformYouTube, Kind: LinkVideo, ID: "Rv3Hn7Qm2Ts", Host: "youtube.com", Site: "youtube.com"}},
		{"https://www.bilibili.com/video/BV1GJ411x7h7/?spm_id_from=333", Link{URL: "https://www.bilibili.com/video/BV1GJ411x7h7/?spm_id_from=333", Platform: PlatformBilibili, Kind: LinkVideo, ID: "BV1GJ411x7h7", Host: "bilibili.com", Site: "bilibili.com"}},
		{"https://www.bilibili.com/video/BV1GJ411x7h7?p=3", Link{URL: "https://www.bilibili.com/video/BV1GJ411x7h7?p=3", Platform: PlatformBilibili, Kind: LinkPlaylist, ID: "BV1GJ411x7h7", Host: "bilibili.com", Site: "bilibili.com"}},
		{"https://m.bilibili.com/video/av170001", Link{URL: "https://m.bilibili.com/video/av170001", Platform: PlatformBilibili, Kind: LinkVideo, ID: "av170001", Host: "bilibili.com", Site: "bilibili.com"}},
		{"https://b23.tv/xYz12Ab", Link{URL: "https://b23.tv/xYz12Ab", Platform: PlatformBilibili, Kind: LinkVideo, ID: "xYz12Ab", Host: "b23.tv", Site: "bilibili.com", Short: true}},
		{"b23.tv/xYz12Ab", Link{URL: "https://b23.tv/xYz12Ab", Platform: PlatformBilibili, Kind: LinkVideo, ID: "xYz12Ab", Host: "b23.tv", Site: "bilibili.com", Short: true}},
		{"https://space.bilibili.com/946974", Link{URL: "https://space.bilibili.com/946974", Platform: PlatformBilibili, Kind: LinkChannel, ID: "946974", Host: "space.bilibili.com", Site: "bilibili.com"}},
		{"https://space.bilibili.com/946974/channel/collectiondetail?sid=1", Link{URL: "https://space.bilibili.com/946974/channel/collectiondetail?sid=1", Platform: PlatformBilibili, Kind: LinkPlaylist, ID: "946974", Host: "space.bilibili.com", Site: "bilibili.com"}},
		{"https://vimeo.com/76979871", Link{URL: "https://vimeo.com/76979871", Platform: PlatformWeb, Kind: LinkVideo, Host: "vimeo.com", Site: "vimeo.com"}},
		{"https://www.ted.com/talks/some_talk", Link{URL: "https://www.ted.com/talks/some_talk", Platform: PlatformWeb, Kind: LinkVideo, Host: "ted.com", Site: "ted.com"}},
		{"https://example.com/v", Link{URL: "https://example.com/v", Platform: PlatformWeb, Kind: LinkVideo, Host: "example.com"}},
		{"file:///Users/me/Movies/clip.mp4", Link{URL: "file:///Users/me/Movies/clip.mp4", Platform: PlatformLocal, Kind: LinkVideo, ID: "/Users/me/Movies/clip.mp4"}},
		{"  https://youtu.be/TRWgBxVAb1M  ", Link{URL: "https://youtu.be/TRWgBxVAb1M", Platform: PlatformYouTube, Kind: LinkVideo, ID: "TRWgBxVAb1M", Host: "youtu.be", Site: "youtube.com"}},
	}
	for _, c := range cases {
		got, ok := ParseLink(c.raw)
		if !ok {
			t.Errorf("ParseLink(%q) reported false", c.raw)
			continue
		}
		if got != c.want {
			t.Errorf("ParseLink(%q)\n got %+v\nwant %+v", c.raw, got, c.want)
		}
	}
}

func TestParseLinkRejects(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a link", "example.com/v", "ftp://example.com/v", "file://", "https://", "mailto:a@b.c"} {
		if got, ok := ParseLink(raw); ok {
			t.Errorf("ParseLink(%q) = %+v, want false", raw, got)
		}
	}
}

// TestParseLinkAcceptsJobSources checks that every source NewJob accepts for a video on the web or on disk has a platform.
func TestParseLinkAcceptsJobSources(t *testing.T) {
	for _, raw := range []string{"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts", "https://example.com/v", "file:///tmp/a.mp4"} {
		if _, err := NewJob(raw); err != nil {
			t.Fatalf("NewJob(%q): %v", raw, err)
		}
		if link, ok := ParseLink(raw); !ok || link.Platform == "" {
			t.Fatalf("ParseLink(%q) = %+v, %v", raw, link, ok)
		}
	}
}
