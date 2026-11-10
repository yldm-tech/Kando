package core

import (
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
)

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
		{"file:///Users/me/Movies/clip.mp4", Link{URL: "file:///Users/me/Movies/clip.mp4", Platform: PlatformLocal, Kind: LinkVideo, ID: filepath.FromSlash("/Users/me/Movies/clip.mp4")}},
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
	for _, raw := range []string{"", "   ", "not a link", "example.com/v", "ftp://example.com/v", "file://", "file://other-machine/clip.mp4", "https://", "mailto:a@b.c"} {
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

func TestLinkOneVideo(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts":                                         true,
		"https://www.youtube.com/watch?v=Rv3Hn7Qm2Ts&list=PLx0sYbCqOb8TBPRdmBHs5Iftvv9TPboYG": true,
		"https://youtu.be/TRWgBxVAb1M?list=PLx0sYbCqOb8":                                      true,
		"https://www.youtube.com/playlist?list=PLx0sYbCqOb8":                                  false,
		"https://www.youtube.com/watch?list=PLx0sYbCqOb8":                                     false,
		"https://www.youtube.com/@veritasium":                                                 false,
		"https://www.youtube.com/@veritasium/videos":                                          false,
		"https://www.youtube.com/channel/UCHnyfMqiRRG1u-2MsSQLbXA":                            false,
		"https://www.youtube.com/c/veritasium":                                                false,
		"https://www.youtube.com/user/1veritasium":                                            false,
		"https://www.bilibili.com/video/BV1GJ411x7h7?p=3":                                     true,
		"https://space.bilibili.com/946974":                                                   false,
		"https://space.bilibili.com/946974/channel/collectiondetail?sid=1":                    false,
		"https://www.bilibili.com/list/watchlater":                                            false,
		"https://vimeo.com/76979871":                                                          true,
		"file:///Users/me/Movies/clip.mp4":                                                    true,
	}
	for raw, want := range cases {
		link, ok := ParseLink(raw)
		if !ok {
			t.Fatalf("ParseLink(%q) reported false", raw)
		}
		if got := link.OneVideo(); got != want {
			t.Errorf("ParseLink(%q).OneVideo() = %v, want %v (%+v)", raw, got, want, link)
		}
	}
}

func TestFileURL(t *testing.T) {
	cases := []struct {
		path, want string
		windows    bool // the path is a Windows one, with backslashes only Windows reads as separators
	}{
		{"/Users/me/Movies/clip.mp4", "file:///Users/me/Movies/clip.mp4", false},
		{"/Users/me/a b/片 1.mp4", "file:///Users/me/a%20b/%E7%89%87%201.mp4", false},
		{"/Users/me/a#b?c%.mp4", "file:///Users/me/a%23b%3Fc%25.mp4", false},
		{"C:/Users/a b/clip.mp4", "file:///C:/Users/a%20b/clip.mp4", false},
		{`C:\Users\a b\clip.mp4`, "file:///C:/Users/a%20b/clip.mp4", true},
		{`d:\视频\会议 01.mp4`, "file:///d:/%E8%A7%86%E9%A2%91/%E4%BC%9A%E8%AE%AE%2001.mp4", true},
		{`\\server\share\clip.mp4`, "file:////server/share/clip.mp4", true},
	}
	for _, c := range cases {
		if c.windows && runtime.GOOS != "windows" {
			continue
		}
		if got := FileURL(c.path); got != c.want {
			t.Errorf("FileURL(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestFilePath(t *testing.T) {
	cases := []struct {
		raw           string
		unix, windows string
	}{
		{"file:///Users/me/Movies/clip.mp4", "/Users/me/Movies/clip.mp4", `\Users\me\Movies\clip.mp4`},
		{"file://localhost/Users/me/clip.mp4", "/Users/me/clip.mp4", `\Users\me\clip.mp4`},
		{"file:/Users/me/clip.mp4", "/Users/me/clip.mp4", `\Users\me\clip.mp4`},
		{"file:///Users/me/a%20b/%E7%89%87%201.mp4", "/Users/me/a b/片 1.mp4", `\Users\me\a b\片 1.mp4`},
		{"file:///C:/Users/a%20b/clip.mp4", "/C:/Users/a b/clip.mp4", `C:\Users\a b\clip.mp4`},
		{"file://localhost/d:/%E8%A7%86%E9%A2%91/x.mp4", "/d:/视频/x.mp4", `d:\视频\x.mp4`},
		{"file:////server/share/clip.mp4", "//server/share/clip.mp4", `\\server\share\clip.mp4`},
	}
	for _, c := range cases {
		u, err := url.Parse(c.raw)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", c.raw, err)
		}
		want := c.unix
		if runtime.GOOS == "windows" {
			want = c.windows
		}
		if got, ok := FilePath(u); !ok || got != want {
			t.Errorf("FilePath(%q) = %q, %v, want %q", c.raw, got, ok, want)
		}
	}
}

func TestFilePathRejects(t *testing.T) {
	for _, raw := range []string{"file://", "file:C:/Users/clip.mp4", "file://other-machine/clip.mp4", "https://example.com/clip.mp4", "/Users/me/clip.mp4"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", raw, err)
		}
		if got, ok := FilePath(u); ok {
			t.Errorf("FilePath(%q) = %q, want false", raw, got)
		}
	}
	if got, ok := FilePath(nil); ok {
		t.Errorf("FilePath(nil) = %q, want false", got)
	}
}

// TestFileURLRoundTrips checks that a local video's source, as the app makes it from the path of a file on this machine, is a job source NewJob accepts and leads back to the same file.
func TestFileURLRoundTrips(t *testing.T) {
	paths := []string{"/Users/me/Movies/clip.mp4", "/Users/me/a b/片 1.mp4", "/tmp/a#b?c%.mp4", "/C:/odd/dir.mp4"}
	if runtime.GOOS == "windows" {
		paths = []string{`C:\Users\a b\clip.mp4`, `d:\视频\会议 01.mp4`, `C:\Users\me\a#b%.mp4`, `\\server\share\clip.mp4`}
	}
	for _, path := range paths {
		source := FileURL(path)
		if _, err := NewJob(source); err != nil {
			t.Errorf("NewJob(FileURL(%q) = %q): %v", path, source, err)
		}
		u, err := url.Parse(source)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", source, err)
		}
		if got, ok := FilePath(u); !ok || got != path {
			t.Errorf("FilePath(FileURL(%q) = %q) = %q, %v", path, source, got, ok)
		}
		if link, ok := ParseLink(source); !ok || link.Platform != PlatformLocal || link.ID != path {
			t.Errorf("ParseLink(FileURL(%q) = %q) = %+v, %v", path, source, link, ok)
		}
	}
}
