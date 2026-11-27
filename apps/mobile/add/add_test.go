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

func TestSharedTakesTheFirstLinkAmongTheTexts(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"Never Gonna Give You Up", "https://youtu.be/abc"}, "https://youtu.be/abc"},
		{[]string{"Title https://youtu.be/abc", "https://youtu.be/def"}, "https://youtu.be/abc"},
		{[]string{"  ", "no link here", "other words"}, "no link here"},
		{[]string{" "}, ""},
		{nil, ""},
	} {
		if got := Shared(c.in); got != c.want {
			t.Errorf("Shared(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
