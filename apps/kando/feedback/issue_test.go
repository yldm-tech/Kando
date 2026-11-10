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
		"open /Users/kim/Movies/a.mp4 failed":             "open <路径> failed",
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
	for in, want := range map[string]string{
		`C:\Users\kim\Videos\a.mp4 missing`: "<路径> missing",
		`home is C:\Users\kim`:              "home is ~",
	} {
		if got := win.Clean(in); got != want {
			t.Errorf("windows home: Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

// NoSecrets hides what Clean hides of credentials, and keeps the paths and links a log on this machine needs.
func TestNoSecretsKeepsPathsAndLinks(t *testing.T) {
	s := Scrubber{Home: "/Users/kim", Secrets: []string{"sk-live-abcdef123456", "https://llm.example.internal/v1", "abc"}}
	cases := map[string]string{
		"open /Users/kim/Movies/a.mp4 failed":                              "open /Users/kim/Movies/a.mp4 failed",
		"see https://www.youtube.com/watch?v=abc":                          "see https://www.youtube.com/watch?v=abc",
		"key sk-live-abcdef123456 rejected":                                "key *** rejected",
		`Post "https://llm.example.internal/v1/chat/completions": refused`: `Post "***/chat/completions": refused`,
		"Authorization: Bearer abcdefghijklmnop":                           "Authorization: Bearer ***",
		"api_key=xyz123456 in query":                                       "api_key=*** in query",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123 leaked":                  "token *** leaked",
	}
	for in, want := range cases {
		if got := s.NoSecrets(in); got != want {
			t.Errorf("NoSecrets(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestScrubberHidesWholePaths checks that a path leaves nothing of itself behind: not the names under the home folder, not the parts after a space in a folder or a video's title, and not a path that follows Chinese text or a full-width colon.
func TestScrubberHidesWholePaths(t *testing.T) {
	s := Scrubber{Home: "/Users/kim", Paths: []string{"/Volumes/My Disk/影视库，2024.kando"}}
	cases := map[string]string{
		"下载失败：/Volumes/Lexar/私人/电影.mp4":                                         "下载失败：<路径>",
		"打开失败：/Volumes/Lexar/私人相册/家庭.mp4":                                       "打开失败：<路径>",
		"视频在/Volumes/Lexar/私人/a.mp4":                                            "视频在<路径>",
		"ffmpeg: error from '/Users/kim/Movies/Kando/Private Family Video.mp4'": "ffmpeg: error from '<路径>'",
		`open "/Users/kim/Google Drive/Secret Project/clip.mp4"`:                `open "<路径>"`,
		"open /Users/kim/Google Drive/Secret Project/clip.mp4 now":              "open <路径> now",
		"home /Users/kim/Movies/Private Film.mp4":                               "home <路径>",
		"[download] Destination: /Volumes/Lexar/Kando/Some Title [abc].mp4":     "[download] Destination: <路径>",
		"我把视频放在 /Volumes/Lexar/Kando 里面，打不开":                                    "我把视频放在 <路径> 里面，打不开",
		"see (/tmp/x/y.log).":          "see (<路径>).",
		"typed ~/Movies/a.mp4 by hand": "typed <路径> by hand",
		"库在 /Volumes/My Disk/影视库，2024.kando/videos/a b.mp4 里": "库在 <路径> 里",
		"按 / 键，或者 /help":                  "按 / 键，或者 /help",
		"「/Volumes/Lexar/私人 相册/家.mp4」打不开": "「<路径>」打不开",
	}
	for in, want := range cases {
		if got := s.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNoPathsRunsToTheEndOfTheLine checks that a tool's message loses the whole path it names, however many words the video's title has, up to the end of its line, a closing quote or the ": " of the error after it.
func TestNoPathsRunsToTheEndOfTheLine(t *testing.T) {
	cases := map[string]string{
		"download: ERROR: unable to write /Volumes/Lexar/Kando/A Long Title With Many Words：Part 1.mp4\nnext line": "download: ERROR: unable to write <路径>\nnext line",
		"open /Users/kim/a b/c d.mp4: no such file or directory":                                                   "open <路径>: no such file or directory",
		"ffmpeg: from '/Volumes/x/My Film.mp4' failed":                                                             "ffmpeg: from '<路径>' failed",
		"下载失败：/Volumes/Lexar/私人/电影 第二部.mp4":                                                                        "下载失败：<路径>",
		"HTTP Error 403: Forbidden":                                                                                "HTTP Error 403: Forbidden",
	}
	for in, want := range cases {
		if got := NoPaths(in); got != want {
			t.Errorf("NoPaths(%q) = %q, want %q", in, got, want)
		}
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
	if strings.Contains(issue.Body, "video.mp4") || !strings.Contains(issue.Body, "错误：open <路径>: no such file") {
		t.Fatalf("the path under the home folder is not hidden: %q", issue.Body)
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
