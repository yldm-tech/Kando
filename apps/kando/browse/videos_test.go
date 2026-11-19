package browse

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"kando/apps/kando/sites/catalog"
	kcore "kando/packages/core"
)

// The page's buttons know a video by the same shapes VideoLink reads, which the page's script is built from, and every link they take for one video's ParseLink reads as one video of a site the catalog lists, while its pages that list videos, its channels and its playlists are none.
func TestVideoLinkFollowsParseLink(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", true},
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw&t=42s&pp=ygUF", true},
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", true},
		{"https://m.youtube.com/watch?v=jNQXAC9IVRw", true},
		{"https://music.youtube.com/watch?v=jNQXAC9IVRw", true},
		{"https://www.youtube.com/shorts/TRWgBxVAb1M", true},
		{"https://www.youtube.com/live/TRWgBxVAb1M", true},
		{"https://youtu.be/jNQXAC9IVRw", true},
		{"https://www.bilibili.com/video/BV1xx411c7mD/?spm_id_from=333.1007", true},
		{"https://www.bilibili.com/video/BV1xx411c7mD?p=2", true},
		{"https://m.bilibili.com/video/av170001", true},
		{"https://www.bilibili.com/zh-Hans/video/BV1nx4y1x7t1/?spm_id_from=333.1007", true},
		{"https://www.bilibili.com/en/video/BV1nx4y1x7t1", true},
		{"https://www.bilibili.com/zh-Hans", false},
		{"https://www.bilibili.com/zh-Hans/", false},
		{"https://search.bilibili.com/zh-Hans/all?keyword=french", false},
		{"https://www.bilibili.com/zh-Hans/list/watchlater", false},
		{"https://vimeo.com/76979871", true},
		{"https://www.ted.com/talks/ken_robinson_do_schools_kill_creativity", true},
		{"https://www.nicovideo.jp/watch/sm9", true},
		{"https://www.dailymotion.com/video/x8abcd1", true},
		{"https://www.tiktok.com/@scout2015/video/6718335390845095173", true},

		{"https://www.youtube.com/", false},
		{"https://www.youtube.com/results?search_query=french", false},
		{"https://www.youtube.com/feed/subscriptions", false},
		{"https://www.youtube.com/@veritasium", false},
		{"https://www.youtube.com/@veritasium/videos", false},
		{"https://www.youtube.com/channel/UC6nSFpj9HTCZ5t-N3Rm3-HA", false},
		{"https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", false},
		{"https://www.youtube.com/watch?v=", false},
		{"https://www.youtube.com/watch?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", false},
		{"https://www.bilibili.com/", false},
		{"https://search.bilibili.com/all?keyword=french", false},
		{"https://space.bilibili.com/123456", false},
		{"https://space.bilibili.com/123456/video", false},
		{"https://www.bilibili.com/list/watchlater", false},
		{"https://vimeo.com/channels/staffpicks", false},
		{"https://player.vimeo.com/76979871", false},
		{"https://www.ted.com/talks", false},
		{"https://example.com/watch?v=jNQXAC9IVRw", false},
		{"https://notyoutube.com/watch?v=jNQXAC9IVRw", false},
		{"http://www.youtube.com.evil.example/watch?v=jNQXAC9IVRw", false},
		{"javascript:alert(1)//www.youtube.com/watch?v=jNQXAC9IVRw", false},
		{"file:///watch?v=jNQXAC9IVRw", false},
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw&x=" + strings.Repeat("a", MaxVideoURL), false},
	}
	for _, c := range cases {
		if got := VideoLink(c.url); got != c.want {
			t.Errorf("VideoLink(%q) = %v, want %v", c.url, got, c.want)
		}
		link, ok := kcore.ParseLink(c.url)
		if c.want && (!ok || !link.OneVideo() || link.IsList() || !catalog.Load().On(c.url)) {
			t.Errorf("%q is a video to the page's buttons, but ParseLink reads %+v, on the catalog: %v", c.url, link, catalog.Load().On(c.url))
		}
	}
}

// The page's script reads the very shapes VideoLink does, which both Go and JavaScript read alike, of sites the catalog lists.
func TestPageScriptReadsTheVideoShapes(t *testing.T) {
	if !strings.Contains(Script(), "const sites = ("+videoSitesJSON()+")") {
		t.Fatal("the page's script does not read videoSites")
	}
	var back []videoSite
	if err := json.Unmarshal([]byte(videoSitesJSON()), &back); err != nil || len(back) != len(videoSites) {
		t.Fatalf("videoSites as the script reads them: %v", err)
	}
	// Go's and JavaScript's regular expressions differ in flags, named groups and lookaround, which the shapes leave out.
	shared := regexp.MustCompile(`^[\w\\^$/|()\[\]{}+*?.,@-]*$`)
	for _, s := range videoSites {
		if !catalog.Load().On("https://" + s.Domain) {
			t.Errorf("%s is not a site of the catalog", s.Domain)
		}
		for _, v := range s.Videos {
			for _, p := range []string{v.Path, v.Value} {
				if strings.Contains(p, "(?") || !shared.MatchString(p) {
					t.Errorf("%s: %q is not read alike by Go and JavaScript", s.Domain, p)
				}
			}
			if !strings.HasPrefix(v.Path, "^/") || !strings.HasSuffix(v.Path, "$") || (v.Param != "") != (v.Value != "") {
				t.Errorf("%s: shape %+v", s.Domain, v)
			}
		}
	}
	for _, want := range []string{AskMessage, AddMessage, StateMessage, "ev.isTrusted", "MutationObserver", "attachShadow"} {
		if !strings.Contains(Script(), want) {
			t.Errorf("the page's script has no %q", want)
		}
	}
}

// What the page asks about and adds is read with care, as any script of the page can post it: only links to one video, each once, at most MaxAsk, and a title on one line, without control characters and kept short.
func TestPageMessagesAreRead(t *testing.T) {
	video, other := "https://www.youtube.com/watch?v=jNQXAC9IVRw", "https://www.bilibili.com/video/BV1xx411c7mD"
	ask, _ := json.Marshal([]string{video, "https://www.youtube.com/", video, other, "file:///etc/passwd"})
	if got, ok := ParseAsk(AskMessage + string(ask)); !ok || len(got) != 2 || got[0] != video || got[1] != other {
		t.Fatalf("ParseAsk = %q, %v", got, ok)
	}
	many := make([]string, MaxAsk+20)
	for i := range many {
		many[i] = "https://www.youtube.com/watch?v=video" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	data, _ := json.Marshal(many)
	if got, _ := ParseAsk(AskMessage + string(data)); len(got) != MaxAsk {
		t.Fatalf("ParseAsk kept %d of %d links", len(got), len(many))
	}
	for _, bad := range []string{"kando-ask:{", "kando-ask:[1]", "kando-title", AddMessage + "[]"} {
		if got, ok := ParseAsk(bad); ok && len(got) != 0 {
			t.Errorf("ParseAsk(%q) = %q", bad, got)
		}
	}

	link, title, ok := ParseAdd(AddMessage + `{"url":"` + video + `","title":"  Me at\n the\u0000zoo  "}`)
	if !ok || link != video || title != "Me at the zoo" {
		t.Fatalf("ParseAdd = %q, %q, %v", link, title, ok)
	}
	long, _ := json.Marshal(map[string]string{"url": video, "title": strings.Repeat("长", 500)})
	if _, title, _ := ParseAdd(AddMessage + string(long)); utf8.RuneCountInString(title) != maxTitle+1 || !strings.HasSuffix(title, "…") {
		t.Fatalf("a long title kept %d characters", utf8.RuneCountInString(title))
	}
	for _, bad := range []string{
		AddMessage + `{"url":"https://www.youtube.com/playlist?list=PL1"}`,
		AddMessage + `{"url":"https://www.youtube.com/@veritasium"}`,
		AddMessage + `{"url":"https://example.com/video.mp4"}`,
		AddMessage + `{"url":"file:///etc/passwd"}`,
		AddMessage + `"` + video + `"`,
		`{"url":"` + video + `"}`,
	} {
		if _, _, ok := ParseAdd(bad); ok {
			t.Errorf("ParseAdd(%q) took it", bad)
		}
	}

	state := StateData(PageState{Text: PageText{Add: "下载视频"}, Have: map[string]string{video: HaveDone}})
	var back PageState
	if rest, ok := strings.CutPrefix(state, StateMessage); !ok || json.Unmarshal([]byte(rest), &back) != nil || back.Have[video] != HaveDone || back.Text.Add != "下载视频" {
		t.Fatalf("StateData = %q", state)
	}
}

// 下载视频 says where the page's player was, which AddedAt reads to the second; a time that is none, or no AddMessage, is nowhere.
func TestAddedAt(t *testing.T) {
	video := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	for data, want := range map[string]time.Duration{
		AddMessage + `{"url":"` + video + `","at":84}`:   84 * time.Second,
		AddMessage + `{"url":"` + video + `","at":84.9}`: 84 * time.Second,
		AddMessage + `{"url":"` + video + `"}`:           0,
		AddMessage + `{"url":"` + video + `","at":-3}`:   0,
		AddMessage + `{"url":"` + video + `","at":1e12}`: 0,
		AddMessage + `{"url":"` + video + `","at":"84"}`: 0,
		`{"url":"` + video + `","at":84}`:                0,
	} {
		if got := AddedAt(data); got != want {
			t.Errorf("AddedAt(%q) = %v, want %v", data, got, want)
		}
	}
}
