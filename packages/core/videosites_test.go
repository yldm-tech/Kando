package core

import "testing"

// The phone's browser and the computer's know a video by the same shapes: a page of one video of a site they know, and none of its pages that list videos or of a site they do not know (apps/kando/browse/videos_test.go checks every shape against ParseLink and the catalog).
func TestVideoLinkKnowsThePagesOfOneVideo(t *testing.T) {
	for u, want := range map[string]bool{
		"https://m.youtube.com/watch?v=jNQXAC9IVRw":                         true,
		"https://youtu.be/jNQXAC9IVRw":                                      true,
		"https://m.bilibili.com/video/BV1xx411c7mD":                         true,
		"https://www.ted.com/talks/ken_robinson_do_schools_kill_creativity": true,
		"https://vimeo.com/76979871":                                        true,
		"https://sp.nicovideo.jp/watch/sm9":                                 true,
		"https://m.youtube.com/":                                            false,
		"https://m.youtube.com/results?search_query=french":                 false,
		"https://www.coursera.org/learn/psychology":                         false,
		"https://example.com/watch?v=jNQXAC9IVRw":                           false,
		"intent://watch?v=jNQXAC9IVRw#Intent;scheme=youtube;end":            false,
	} {
		if got := VideoLink(u); got != want {
			t.Errorf("VideoLink(%q) = %v, want %v", u, got, want)
		}
	}
}

// A video's title is the page's without the count of notifications and the site's name; another page's stays as it is.
func TestVideoTitleCutsTheSitesName(t *testing.T) {
	for _, c := range []struct{ url, title, want string }{
		{"https://m.youtube.com/watch?v=jNQXAC9IVRw", "(3) Me at the zoo - YouTube", "Me at the zoo"},
		{"https://www.bilibili.com/video/BV1xx411c7mD", "東京の朝ごはん_哔哩哔哩_bilibili", "東京の朝ごはん"},
		{"https://vimeo.com/76979871", " Coastline ", "Coastline"},
		{"https://m.youtube.com/", "(3) YouTube", "(3) YouTube"},
	} {
		if got := VideoTitle(c.url, c.title); got != c.want {
			t.Errorf("VideoTitle(%q, %q) = %q, want %q", c.url, c.title, got, c.want)
		}
	}
}
