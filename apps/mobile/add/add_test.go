package add

import "testing"

func TestFirstLinkIsTheVideoLinkInSharedText(t *testing.T) {
	for in, want := range map[string]string{
		"Look at this https://youtu.be/abc ok":      "https://youtu.be/abc",
		"  https://www.bilibili.com/video/BV1xx \n": "https://www.bilibili.com/video/BV1xx",
		"no link here": "no link here",
		"":             "",
	} {
		if got := firstLink(in); got != want {
			t.Errorf("firstLink(%q) = %q, want %q", in, got, want)
		}
	}
}
