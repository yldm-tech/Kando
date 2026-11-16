package onboarding

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"
)

// TestSpeechLanguagesAreTheTiles keeps the languages the speech settings offer as the one spoken in the videos (media.SpeechLanguages) the same as the ones this guide offers, so a language added here can be told to whisper and is not dropped from settings.json.
func TestSpeechLanguagesAreTheTiles(t *testing.T) {
	if got, want := slices.Sorted(slices.Values(media.SpeechLanguages())), slices.Sorted(slices.Values(tileCodes)); !slices.Equal(got, want) {
		t.Fatalf("speech languages %q, the guide's languages %q", got, want)
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
}
