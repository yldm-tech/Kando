// Package feedback is the 反馈问题 dialog: what kind of feedback it is, a title and a description, what the app may attach, and a preview of the GitHub issue it opens in the browser. This file writes the issue; dialog.go draws the dialog.
package feedback

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Repo is the GitHub repository feedback goes to, and NewIssue its new-issue page, which takes the title, body and labels from the query.
const (
	Repo     = "yldm-tech/Kando"
	NewIssue = "https://github.com/" + Repo + "/issues/new"
)

// MaxURL is the longest new-issue link the dialog opens. GitHub answers a longer request line with 414 URI Too Long somewhere past 8 KB, so the link stays under it with room to spare. MaxTitle is GitHub's limit on an issue title.
const (
	MaxURL   = 8000
	MaxTitle = 256
)

// TruncatedMark ends a body that was cut to fit MaxURL.
const TruncatedMark = "\n\n…（正文太长，超过了 GitHub 链接的长度上限，后面的内容已截掉。）"

// Kind is what the feedback is about: KindBug, KindIdea or KindSubs, in the order of the dialog's segmented control.
type Kind int

const (
	KindBug Kind = iota
	KindIdea
	KindSubs
)

// Kinds are the segmented control's labels, by Kind.
var Kinds = []string{"问题", "建议", "字幕质量"}

// Name is the kind as the dialog and the issue name it.
func (k Kind) Name() string {
	if k < 0 || int(k) >= len(Kinds) {
		return Kinds[KindBug]
	}
	return Kinds[k]
}

// Label is the repository's issue label for the kind: bug for a problem and for subtitle quality, enhancement for an idea. They are labels the repository has; GitHub applies them only for someone who may label issues there and ignores them otherwise.
func (k Kind) Label() string {
	if k == KindIdea {
		return "enhancement"
	}
	return "bug"
}

// Report is what the user wrote and chose to attach, and the facts the app gathered for it.
type Report struct {
	Kind        Kind
	Title       string
	Description string
	// Env, Tools, Film and Subs say which of the facts the issue carries: the app's and the system's versions, the tools' versions and the models in use, the open film's state, and the subtitle line at the playhead.
	Env, Tools, Film, Subs bool
	Facts                  Facts
}

// Facts are what the app knows about itself and the open film, none of it a path, a link or a credential.
type Facts struct {
	// App is the app's name and version, OS the system's name, version and architecture.
	App, OS string
	Tools   []Tool
	// Speech is the speech model's file name, Translate the translation service's name and model.
	Speech, Translate string
	// Film is the open film's state, a line each, or nil with no film open.
	Film []string
	// Excerpt is the subtitle line at the playhead, or nil without one.
	Excerpt *Excerpt
}

// Tool is a tool the pipeline runs and its version: Missing when it is not installed, an empty Version when it did not say.
type Tool struct {
	Name, Version string
	Missing       bool
}

func (t Tool) String() string {
	switch {
	case t.Missing:
		return t.Name + " 未安装"
	case t.Version == "":
		return t.Name + " 版本未知"
	}
	return t.Name + " " + t.Version
}

// Excerpt is one subtitle line: when it starts, its original and its translation, empty when it has none.
type Excerpt struct {
	At                    time.Duration
	Original, Translation string
}

// Body is the issue's text in Markdown, before Issue scrubs and fits it.
func (r Report) Body() string {
	var b strings.Builder
	fmt.Fprintf(&b, "**类型**：%s\n\n### 描述\n", r.Kind.Name())
	if d := strings.TrimSpace(r.Description); d != "" {
		b.WriteString(d)
	} else {
		b.WriteString("（未填写）")
	}
	b.WriteString("\n")
	f := r.Facts
	if r.Env || r.Tools {
		b.WriteString("\n### 环境\n")
		if r.Env {
			fmt.Fprintf(&b, "- %s · %s\n", f.App, f.OS)
		}
		if r.Tools {
			tools := make([]string, len(f.Tools))
			for i, t := range f.Tools {
				tools[i] = t.String()
			}
			fmt.Fprintf(&b, "- 组件：%s\n", strings.Join(tools, " · "))
			fmt.Fprintf(&b, "- 识别：%s · 翻译：%s\n", orNone(f.Speech), orNone(f.Translate))
		}
	}
	if r.Film && len(f.Film) > 0 {
		b.WriteString("\n### 当前影片\n")
		for _, line := range f.Film {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	if r.Subs && f.Excerpt != nil {
		at := Clock(f.Excerpt.At)
		b.WriteString("\n### 字幕片段\n")
		fmt.Fprintf(&b, "- %s 原文：%s\n", at, f.Excerpt.Original)
		fmt.Fprintf(&b, "- %s 译文：%s\n", at, orValue(f.Excerpt.Translation, "（没有译文）"))
	}
	return strings.TrimRight(b.String(), "\n")
}

func orNone(s string) string { return orValue(s, "未设置") }

func orValue(s, none string) string {
	if strings.TrimSpace(s) == "" {
		return none
	}
	return s
}

// Clock writes d as the subtitle list does, minutes, seconds and tenths, as 0:12.5.
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	if h := ms / 3_600_000; h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%d", h, ms/60_000%60, ms/1000%60, ms/100%10)
	}
	return fmt.Sprintf("%d:%02d.%d", ms/60_000, ms/1000%60, ms/100%10)
}

// Issue is the issue as the dialog previews and opens it: scrubbed and fitted to MaxURL.
type Issue struct {
	Title, Label string
	// Body is what the link carries; Full is the whole body, which 拷贝正文 copies. They differ only when the body was cut to fit the link: Truncated is then set, and Cut is how many characters of Full were left out.
	Body, Full string
	Truncated  bool
	Cut        int
	URL        string
}

// Issue scrubs the report's text with s and builds the new-issue link, cutting the end of the body when the link would be longer than MaxURL.
func (r Report) Issue(s Scrubber) Issue {
	title := []rune(s.Clean(strings.TrimSpace(r.Title)))
	if len(title) > MaxTitle {
		title = title[:MaxTitle]
	}
	is := Issue{Title: string(title), Label: r.Kind.Label(), Full: s.Clean(r.Body())}
	is.Body = is.Full
	is.URL = is.link(is.Body)
	if len(is.URL) <= MaxURL {
		return is
	}
	// The longest start of the body that fits, found by halving: the link grows with every character kept.
	full := []rune(is.Full)
	lo, hi := 0, len(full)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if len(is.link(string(full[:mid])+TruncatedMark)) <= MaxURL {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	is.Body = string(full[:lo]) + TruncatedMark
	is.URL = is.link(is.Body)
	is.Truncated, is.Cut = true, len(full)-lo
	return is
}

func (is Issue) link(body string) string {
	q := url.Values{}
	q.Set("title", is.Title)
	q.Set("body", body)
	q.Set("labels", is.Label)
	return NewIssue + "?" + q.Encode()
}

// Preview is the issue as the dialog shows it: the title and the label the link fills in, then the body it carries.
func (is Issue) Preview() string {
	return "标题：" + orValue(is.Title, "（未填写）") + "\n标签：" + is.Label + "\n\n" + is.Body
}

// Scrubber takes out of text what must not reach a public issue: the home folder becomes ~, other absolute paths become <路径>, and the secrets it knows and anything shaped like a key or token become ***.
type Scrubber struct {
	// Home is the user's home folder; Secrets are values never to send, such as API keys, access keys and the environment's credentials.
	Home    string
	Secrets []string
}

var (
	// unixPath is an absolute path of two or more parts after the start of the text, a space, a quote, a bracket or =, so a URL's // and a ratio such as 1/2 stay.
	unixPath = regexp.MustCompile(`(^|[\s"'(\[=（])/[^\s/"'()\[\]:：，。、,;]+/[^\s"'()\[\]:：，。、,;]*`)
	// winPath is an absolute Windows path, as C:\Users.
	winPath = regexp.MustCompile(`(^|[\s"'(\[=（])[A-Za-z]:\\[^\s"'()\[\]:：，。、,;]*`)
	// tokens are shapes of credentials that are not among the known secrets: API keys, GitHub tokens, AWS access keys, bearer tokens and key=value pairs naming a secret.
	tokens = []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/=-]{8,}`), "${1} ***"},
		{regexp.MustCompile(`\b(?:sk|pk|rk)-[A-Za-z0-9_-]{16,}`), "***"},
		{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`), "***"},
		{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "***"},
		{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "***"},
		{regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password|passwd|access[_-]?key)(\s*[=:]\s*)[^\s&"']+`), "${1}${2}***"},
	}
)

// links are web and file addresses, which a film's error can carry: the video's own link, or a link of the site it came from.
var links = regexp.MustCompile(`(?i)\b(?:https?|ftp|file)://[^\s"'<>()（）]+`)

// NoLinks writes every link in text as <链接>, for what the app attaches about a film, which the issue sends without the film's link.
func NoLinks(text string) string { return links.ReplaceAllString(text, "<链接>") }

// Clean scrubs text.
func (s Scrubber) Clean(text string) string {
	for _, secret := range s.Secrets {
		if len(secret) >= 6 {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	text = s.homeAsTilde(text)
	text = unixPath.ReplaceAllString(text, "${1}<路径>")
	text = winPath.ReplaceAllString(text, "${1}<路径>")
	for _, t := range tokens {
		text = t.re.ReplaceAllString(text, t.with)
	}
	return text
}

// homeAsTilde writes the home folder as ~ wherever a path starts with it: followed by a separator or by the end of the path, so /Users/kim does not turn /Users/kimberly into ~berly.
func (s Scrubber) homeAsTilde(text string) string {
	home := strings.TrimRight(s.Home, `/\`)
	if len(home) < 2 {
		return text
	}
	var b strings.Builder
	for {
		i := strings.Index(text, home)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		end := i + len(home)
		if end == len(text) || !pathChar(text[end]) {
			b.WriteString(text[:i])
			b.WriteString("~")
		} else {
			b.WriteString(text[:end])
		}
		text = text[end:]
	}
}

// pathChar reports whether c can go on a folder's name, so a home folder followed by it is a longer name, not the home folder.
func pathChar(c byte) bool {
	return c != '/' && c != '\\' && c != ' ' && c != '\t' && c != '\n' && c != '"' && c != '\'' && c != ':' && c != ')' && c != ',' && c != ';'
}
