package onboarding

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestLinkHint(t *testing.T) {
	for _, tc := range []struct {
		raw, hint string
		ok        bool
	}{
		{"", "", true},
		{"hello", "这不像一个视频链接", false},
		{"file:///Users/me/a.mp4", "这不像一个视频链接", false},
		{"youtube.com/watch?v=k9Yt2LmQ0pX", "识别为 YouTube 视频", true},
		{"https://www.youtube.com/watch?v=k9Yt2LmQ0pX&list=PL123", "识别为 YouTube 视频，只加入这一个，不加入整个列表", true},
		{"https://www.youtube.com/playlist?list=PL123", "这是YouTube列表的链接。现在一次加入一个视频，请粘贴其中一个视频的链接", false},
		{"https://www.youtube.com/@SomeChannel", "这是YouTube频道的链接。现在一次加入一个视频，请粘贴其中一个视频的链接", false},
		{"https://www.bilibili.com/video/BV1xK4y1A7nM", "识别为 哔哩哔哩 视频", true},
		{"https://vimeo.com/123456", "识别为 Vimeo 视频", true},
		{"https://example.org/clip", "识别为网页视频", true},
	} {
		hint, ok := LinkHint(tc.raw)
		if hint != tc.hint || ok != tc.ok {
			t.Errorf("LinkHint(%q) = %q, %v; want %q, %v", tc.raw, hint, ok, tc.hint, tc.ok)
		}
	}
}

func TestEffectiveLeavesOutTheTarget(t *testing.T) {
	if got := Effective([]string{"zh", "ja", "en", "xx"}, "简体中文"); !slices.Equal(got, []string{"en", "ja"}) {
		t.Fatalf("Effective = %q", got)
	}
	if got := tiles("English"); len(got) != 6 || slices.Contains(got, "en") {
		t.Fatalf("tiles(English) = %q", got)
	}
}

// TestStepsNameThisComputer checks that the second step names the machine as the platform calls it, so Linux and Windows do not read 这台 Mac.
func TestStepsNameThisComputer(t *testing.T) {
	_, description := heading(2)
	for _, text := range []string{caption(2), description} {
		if !strings.Contains(text, thisComputer) {
			t.Fatalf("%q does not name %q", text, thisComputer)
		}
		if runtime.GOOS != "darwin" && strings.Contains(text, "Mac") {
			t.Fatalf("%q says Mac on %s", text, runtime.GOOS)
		}
	}
}
