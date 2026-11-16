package onboarding

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"
)

// TestTilesAreSpeechLanguages keeps every language this guide offers among the ones the speech settings offer as the one spoken in the videos (media.SpeechLanguages), so a language added here can be told to whisper and is not dropped from settings.json.
func TestTilesAreSpeechLanguages(t *testing.T) {
	for _, code := range tileCodes {
		if !slices.Contains(media.SpeechLanguages(), code) {
			t.Fatalf("the guide offers %q, which the speech settings do not", code)
		}
	}
}

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
		// A collection on a channel's page names the channel, not one of its videos.
		{"https://space.bilibili.com/946974/channel/collectiondetail?sid=1", "这是哔哩哔哩列表的链接。现在一次加入一个视频，请粘贴其中一个视频的链接", false},
		// A site's home page and its search results list videos without being one.
		{"https://www.youtube.com/", "这不像一个视频链接", false},
		{"https://search.bilibili.com/all?keyword=french", "这不像一个视频链接", false},
		{"https://vimeo.com/123456", "识别为 Vimeo 视频", true},
		{"https://example.org/clip", "识别为网页视频", true},
	} {
		hint, ok := linkHint(testlocale.Messages(), tc.raw)
		if hint != tc.hint || ok != tc.ok || Addable(tc.raw) != tc.ok {
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
	// A target language none of the tiles is written in leaves six tiles, and only those can be chosen.
	if got := tiles("Português"); len(got) != 6 || slices.Contains(got, "zh") {
		t.Fatalf("tiles(Português) = %q", got)
	}
	if got := Effective([]string{"zh", "ja"}, "Português"); !slices.Equal(got, []string{"ja"}) {
		t.Fatalf("Effective for Português = %q", got)
	}
	if code, original, translation := demo([]string{"ja"}, "Português"); code != "ja" || original != demoLines["ja"] || translation != demoLines["pt"] || translation == "" {
		t.Fatalf("demo for Português = %q %q %q", code, original, translation)
	}
}
