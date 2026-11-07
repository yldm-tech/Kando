package feedback

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	return Report{
		Kind:        KindBug,
		Title:       "导出后字幕错位",
		Description: "导出 ~/Movies/a.kando 里的影片后字幕晚了一秒。",
		Env:         true,
		Tools:       true,
		Facts: Facts{
			App:       "看懂 0.1.0",
			OS:        "macOS 27.2 (arm64)",
			Tools:     []Tool{{Name: "yt-dlp", Version: "2026.09.14"}, {Name: "ffmpeg", Version: "7.1"}, {Name: "whisper-cli", Missing: true}},
			Speech:    "ggml-base.bin",
			Translate: "EveryAPI · claude-haiku-5-5",
			Film:      []string{"来源：本地文件", "状态：已完成"},
			Excerpt:   &Excerpt{At: 12500 * time.Millisecond, Original: "Rivers carve valleys.", Translation: "河流塑造山谷。"},
		},
	}
}

func TestBodyListsWhatIsTicked(t *testing.T) {
	r := sampleReport()
	body := r.Body()
	for _, want := range []string{"问题", "### 描述", "导出 ~/Movies/a.kando", "### 环境", "看懂 0.1.0 · macOS 27.2 (arm64)", "yt-dlp 2026.09.14", "ffmpeg 7.1", "whisper-cli 未安装", "ggml-base.bin", "EveryAPI · claude-haiku-5-5"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"### 当前影片", "### 字幕片段", "Rivers carve valleys."} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body has %q though it was not ticked:\n%s", unwanted, body)
		}
	}
	r.Film, r.Subs = true, true
	body = r.Body()
	for _, want := range []string{"### 当前影片", "- 来源：本地文件", "### 字幕片段", "0:12.5", "Rivers carve valleys.", "河流塑造山谷。"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q once ticked:\n%s", want, body)
		}
	}
	r.Env, r.Tools = false, false
	if body := r.Body(); strings.Contains(body, "### 环境") || strings.Contains(body, "ffmpeg") {
		t.Fatalf("unticked environment still sent:\n%s", body)
	}
}

func TestBodyWithoutDescription(t *testing.T) {
	r := Report{Kind: KindIdea, Title: "x"}
	if body := r.Body(); !strings.Contains(body, "建议") || !strings.Contains(body, "（未填写）") {
		t.Fatalf("body %q", body)
	}
}

func TestIssueURLIsGitHubsNewIssuePage(t *testing.T) {
	r := sampleReport()
	issue := r.Issue(Scrubber{})
	u, err := url.Parse(issue.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.Path != "/yldm-tech/Kando/issues/new" {
		t.Fatalf("url %q", issue.URL)
	}
	q := u.Query()
	if q.Get("title") != r.Title || q.Get("body") != issue.Body || q.Get("labels") != "bug" {
		t.Fatalf("query %v", q)
	}
	if issue.Truncated {
		t.Fatal("a short report was truncated")
	}
	if got := (Report{Kind: KindIdea, Title: "x"}).Issue(Scrubber{}); !strings.Contains(got.URL, "labels=enhancement") {
		t.Fatalf("idea url %q", got.URL)
	}
}

func TestIssueTruncatesToGitHubsLimit(t *testing.T) {
	r := sampleReport()
	r.Description = strings.Repeat("字幕在第二段之后全部晚了一秒，", 600)
	issue := r.Issue(Scrubber{})
	if len(issue.URL) > MaxURL {
		t.Fatalf("url is %d bytes, limit %d", len(issue.URL), MaxURL)
	}
	if !issue.Truncated || issue.Cut <= 0 {
		t.Fatalf("truncated %v cut %d", issue.Truncated, issue.Cut)
	}
	if !strings.HasSuffix(issue.Body, TruncatedMark) {
		t.Fatalf("truncated body does not say so: %q", issue.Body[len(issue.Body)-80:])
	}
	if !strings.HasPrefix(issue.Full, issue.Body[:len(issue.Body)-len(TruncatedMark)]) {
		t.Fatal("the truncated body is not the start of the full one")
	}
	u, _ := url.Parse(issue.URL)
	if u.Query().Get("body") != issue.Body {
		t.Fatal("url body differs from the body shown")
	}
	long := Report{Kind: KindBug, Title: strings.Repeat("长", 400)}
	if got := long.Issue(Scrubber{}); len([]rune(got.Title)) > MaxTitle {
		t.Fatalf("title has %d runes", len([]rune(got.Title)))
	}
}

func TestScrubberHidesHomeOtherPathsAndSecrets(t *testing.T) {
	s := Scrubber{Home: "/Users/kim", Secrets: []string{"sk-live-abcdef123456"}}
	cases := map[string]string{
		"open /Users/kim/Movies/a.mp4 failed":             "open ~/Movies/a.mp4 failed",
		"home is /Users/kim":                              "home is ~",
		"/Users/kimberly/x/y stays out":                   "<路径> stays out",
		"read /Volumes/Disk/films/a.mp4: denied":          "read <路径>: denied",
		`exec: "/opt/homebrew/bin/ffmpeg": not found`:     `exec: "<路径>": not found`,
		"key sk-live-abcdef123456 rejected":               "key *** rejected",
		"Authorization: Bearer abcdefghijklmnop":          "Authorization: Bearer ***",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123 leaked": "token *** leaked",
		"api_key=xyz123456 in query":                      "api_key=*** in query",
		`C:\Users\kim\Videos\a.mp4 missing`:               "<路径> missing",
		"see https://www.youtube.com/watch?v=abc":         "see https://www.youtube.com/watch?v=abc",
		"half and/or a 1/2 ratio":                         "half and/or a 1/2 ratio",
	}
	for in, want := range cases {
		if got := s.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	win := Scrubber{Home: `C:\Users\kim`}
	if got := win.Clean(`C:\Users\kim\Videos\a.mp4 missing`); got != `~\Videos\a.mp4 missing` {
		t.Errorf("windows home: %q", got)
	}
}

func TestIssueScrubsEverythingItSends(t *testing.T) {
	r := sampleReport()
	r.Title = "crash in /Users/kim/Movies"
	r.Description = "key sk-live-abcdef123456 /private/var/tmp/x/y"
	r.Film = true
	r.Facts.Film = []string{"错误：open /Users/kim/Movies/a/video.mp4: no such file"}
	r.Subs = true
	r.Facts.Excerpt = &Excerpt{Original: "my key sk-live-abcdef123456"}
	issue := r.Issue(Scrubber{Home: "/Users/kim", Secrets: []string{"sk-live-abcdef123456"}})
	for _, s := range []string{issue.Title, issue.Body, issue.Full, issue.URL, issue.Preview()} {
		if strings.Contains(s, "/Users/kim") || strings.Contains(s, "sk-live") || strings.Contains(s, "/private/var") {
			t.Fatalf("leaked: %q", s)
		}
	}
	if !strings.Contains(issue.Body, "~/Movies/a/video.mp4") {
		t.Fatalf("home path not kept as ~: %q", issue.Body)
	}
}

func TestPreviewShowsTitleLabelAndBody(t *testing.T) {
	issue := sampleReport().Issue(Scrubber{})
	p := issue.Preview()
	if !strings.HasPrefix(p, "标题：导出后字幕错位\n标签：bug\n\n") || !strings.HasSuffix(p, issue.Body) {
		t.Fatalf("preview %q", p)
	}
	empty := Report{Kind: KindBug}.Issue(Scrubber{})
	if !strings.HasPrefix(empty.Preview(), "标题：（未填写）") {
		t.Fatalf("empty preview %q", empty.Preview())
	}
}

func TestNoLinksTakesOutAddresses(t *testing.T) {
	in := "ERROR: [youtube] abc: Sign in at https://www.youtube.com/watch?v=abc (see file:///tmp/x.log) or ftp://h/x"
	got := NoLinks(in)
	if strings.Contains(got, "youtube.com") || strings.Contains(got, "file://") || strings.Contains(got, "ftp://") {
		t.Fatalf("NoLinks(%q) = %q", in, got)
	}
	if !strings.Contains(got, "ERROR: [youtube] abc: Sign in at <链接> (see <链接>) or <链接>") {
		t.Fatalf("NoLinks(%q) = %q", in, got)
	}
}
