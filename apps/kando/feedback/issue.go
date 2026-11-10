// Package feedback is the 反馈问题 dialog: what kind of feedback it is, a title and a description, what the app may attach, and a preview of the GitHub issue it opens in the browser. This file writes the issue; dialog.go draws the dialog.
package feedback

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"kando/apps/kando/locales"
)

// Repo is the GitHub repository feedback goes to, and NewIssue its new-issue page, which takes the title, body and labels from the query.
const (
	Repo     = "yldm-tech/Kando"
	NewIssue = "https://github.com/" + Repo + "/issues/new"
)

// MaxURL is the longest new-issue link the dialog opens. Measured against github.com in November 2026: a browser that is not signed in is sent to the sign-in page with the link as its return address, which fails with 500 once the link passes about 6,950 bytes, and a request line past about 8 KB fails with 414 URI Too Long. The link stays under the lower of the two with room to spare. MaxTitle is GitHub's limit on an issue title.
const (
	MaxURL   = 6000
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

// Kinds are the kinds' names as the issue writes them, by Kind. The issue goes to the maintainers' tracker in one language, Chinese, whatever the interface's language; the segmented control shows kindLabels.
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

// Body is the issue's text in Markdown, before Issue scrubs and fits it. Its headings, the kind and the facts are written in Chinese whatever the interface's language, so the maintainers read every issue on the tracker the same way; only what the user wrote is in their own words.
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

// Preview is the issue as the dialog shows it: the title and the label the link fills in, in the interface's language, then the body it carries, as it is sent.
func (is Issue) Preview() string {
	return locales.FeedbackPreviewTitle(orValue(is.Title, locales.FeedbackPreviewUntitled())) + "\n" + locales.FeedbackPreviewLabel(is.Label) + "\n\n" + is.Body
}

// Scrubber takes out of text what must not reach a public issue: every absolute path becomes <路径>, whole, with the names under it and the spaces in it, the home folder on its own becomes ~, and the secrets it knows and anything shaped like a key or token become ***.
type Scrubber struct {
	// Home is the user's home folder; Paths are the files and folders the app knows, such as the libraries, the videos and their subtitles, which are hidden wherever they are written, whatever they have in their names; Secrets are values never to send, such as API keys, access keys and the environment's credentials.
	Home    string
	Paths   []string
	Secrets []string
}

// PathMark is what a path is written as.
const PathMark = "<路径>"

var (
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
	// extension is a file name's extension, which ends a path: once a path names a file, the words after it are not part of it.
	extension = regexp.MustCompile(`[^/\\.]\.[A-Za-z0-9]{1,6}$`)
)

// links are web and file addresses, which a film's error can carry: the video's own link, or a link of the site it came from.
var links = regexp.MustCompile(`(?i)\b(?:https?|ftp|file)://[^\s"'<>()（）]+`)

// NoLinks writes every link in text as <链接>, for what the app attaches about a film, which the issue sends without the film's link.
func NoLinks(text string) string { return links.ReplaceAllString(text, "<链接>") }

// Clean scrubs text.
func (s Scrubber) Clean(text string) string {
	text = s.knownSecrets(text)
	text = s.knownPaths(text)
	text = s.homeAsTilde(text)
	text = scrubPaths(text, false)
	return noTokens(text)
}

// NoSecrets writes the secrets s knows and anything shaped like a key or token as ***, and leaves the paths and links in text as they are, for the app's log file, which stays on this machine.
func (s Scrubber) NoSecrets(text string) string {
	return noTokens(s.knownSecrets(text))
}

// knownSecrets writes each of Secrets as ***. One shorter than six characters is left alone, as it would hide parts of ordinary words.
func (s Scrubber) knownSecrets(text string) string {
	for _, secret := range s.Secrets {
		if len(secret) >= 6 {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	return text
}

// noTokens writes whatever is shaped like a credential as ***.
func noTokens(text string) string {
	for _, t := range tokens {
		text = t.re.ReplaceAllString(text, t.with)
	}
	return text
}

// NoPaths writes every absolute path in a tool's message as <路径>, for what the app attaches about a film. A tool writes a path with the video's title in it and does not quote it, so a path runs to the end of its line, a closing quote or the ": " of the error after it.
func NoPaths(text string) string { return scrubPaths(text, true) }

// knownRoot stands in for a path of Paths until scrubPaths hides it with whatever path goes on below it. It starts as an absolute path does and has no space, so scrubPaths takes it as the start of one.
const knownRoot = "/\x00"

// knownPaths writes each of Paths, the longest first, as knownRoot wherever a path starts with it.
func (s Scrubber) knownPaths(text string) string {
	paths := make([]string, 0, len(s.Paths))
	for _, p := range s.Paths {
		if p = strings.TrimRight(p, `/\`); len(p) >= 2 {
			paths = append(paths, p)
		}
	}
	slices.SortFunc(paths, func(a, b string) int { return len(b) - len(a) })
	for _, p := range paths {
		text = replacePath(text, p, knownRoot)
	}
	return text
}

// homeAsTilde writes the home folder as ~ wherever a path starts with it, so the user's name is not in the issue even where the rest of a path is.
func (s Scrubber) homeAsTilde(text string) string {
	home := strings.TrimRight(s.Home, `/\`)
	if len(home) < 2 {
		return text
	}
	return replacePath(text, home, "~")
}

// replacePath writes dir as with wherever it is followed by a separator or by the end of the path, so /Users/kim does not turn /Users/kimberly into ~berly.
func replacePath(text, dir, with string) string {
	var b strings.Builder
	for {
		i := strings.Index(text, dir)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		end := i + len(dir)
		if end == len(text) || !pathChar(text[end]) {
			b.WriteString(text[:i])
			b.WriteString(with)
		} else {
			b.WriteString(text[:end])
		}
		text = text[end:]
	}
}

// pathChar reports whether c can go on a folder's name, so a folder followed by it is a longer name, not the folder.
func pathChar(c byte) bool {
	return c != '/' && c != '\\' && c != ' ' && c != '\t' && c != '\n' && c != '"' && c != '\'' && c != ':' && c != ')' && c != ',' && c != ';'
}

// quotes are the quotes a path can be written in, by their closing quote. A quoted path runs to its closing quote, spaces and all.
var quotes = map[rune]rune{'"': '"', '\'': '\'', '`': '`', '“': '”', '‘': '’', '「': '」', '『': '』'}

// scrubPaths writes every absolute path in text as <路径>: one that starts at the start of the text or after anything but a letter, a digit or a character of a path or an address, so a link's // and a ratio such as 1/2 stay, and that has a folder and a name under its root, so a lone / or a /help does not count; a ~ path and a Windows path count from their root. A quoted path runs to its closing quote. Otherwise, with toLineEnd a path runs to the end of its line or the ": " after it, and without it to the end of its word, and on over the next words of the sentence while a later one has a separator or an extension and the path so far names no file, as a folder or a title with spaces does.
func scrubPaths(text string, toLineEnd bool) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		root := pathRoot(text[i:])
		if root == 0 || !pathStart(text[:i]) {
			i += size
			continue
		}
		end := pathEnd(text, i, root, toLineEnd)
		if end <= i {
			i += size
			continue
		}
		b.WriteString(text[last:i])
		b.WriteString(PathMark)
		last, i = end, end
	}
	b.WriteString(text[last:])
	return b.String()
}

// pathRoot is the length of the absolute path's root text starts with: / , ~/ or ~\, or a drive such as C:\; 0 when it starts none.
func pathRoot(text string) int {
	switch {
	case strings.HasPrefix(text, "~/"), strings.HasPrefix(text, `~\`):
		return 2
	case strings.HasPrefix(text, "/"):
		return 1
	case len(text) >= 3 && isLetter(text[0]) && text[1] == ':' && text[2] == '\\':
		return 3
	}
	return 0
}

// pathStart reports whether a path can start after before: at the start of the text, or after anything but a letter, a digit or a character that goes on a path or an address.
func pathStart(before string) bool {
	r, _ := utf8.DecodeLastRuneInString(before)
	if r == utf8.RuneError {
		return true
	}
	if r < utf8.RuneSelf && (isLetter(byte(r)) || r >= '0' && r <= '9') {
		return false
	}
	return !strings.ContainsRune("_.~/\\:-", r)
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// pathEnd is where the path at text[i:] with a root of root bytes ends, as scrubPaths says, or i when it is not a path.
func pathEnd(text string, i, root int, toLineEnd bool) int {
	open, _ := utf8.DecodeLastRuneInString(text[:i])
	if closing, ok := quotes[open]; ok {
		line := text[i:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		if at := strings.IndexRune(line, closing); at > 0 && isPath(line[:at], root) {
			return i + at
		}
	}
	if toLineEnd {
		line := text[i:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		if at := strings.Index(line, ": "); at >= 0 {
			line = line[:at]
		}
		line = trimPathEnd(strings.TrimRightFunc(line, unicode.IsSpace))
		if !isPath(line, root) {
			return i
		}
		return i + len(line)
	}
	end, stop := wordEnd(text, i+root)
	if !isPath(trimPathEnd(text[i:end]), root) {
		return i
	}
	// On over the next words while one of them still has a separator or an extension, as the rest of a folder's or a title's name with spaces does.
	for at := end; !stop && !extension.MatchString(trimPathEnd(text[i:end])); {
		if at >= len(text) || text[at] != ' ' {
			break
		}
		next, nextStop := wordEnd(text, at+1)
		word := trimPathEnd(text[at+1 : next])
		if word == "" || pathRoot(word) != 0 {
			break
		}
		if strings.ContainsAny(word, `/\`) || extension.MatchString(word) {
			end = next
		}
		at, stop = next, nextStop
	}
	return i + len(trimPathEnd(text[i:end]))
}

// wordEnd is where the word at text[i:] ends: at a space or a punctuation mark that ends a sentence or a clause. stop says it ended at a punctuation mark, which ends the sentence a path can go on in.
func wordEnd(text string, i int) (end int, stop bool) {
	for end = i; end < len(text); {
		r, size := utf8.DecodeRuneInString(text[end:])
		if unicode.IsSpace(r) {
			return end, r == '\n'
		}
		if _, quote := quotes[r]; quote || strings.ContainsRune("”’」』<>|,;:，。；：、！？（）《》", r) {
			return end, true
		}
		end += size
	}
	return end, true
}

// trimPathEnd takes off the end of a path what ends the sentence around it: a full stop, and a closing bracket the path did not open.
func trimPathEnd(p string) string {
	for p != "" {
		switch c := p[len(p)-1]; {
		case c == '.':
			p = p[:len(p)-1]
		case c == ')' && strings.Count(p, "(") < strings.Count(p, ")"), c == ']' && strings.Count(p, "[") < strings.Count(p, "]"):
			p = p[:len(p)-1]
		default:
			return p
		}
	}
	return p
}

// isPath reports whether p, starting with a root of root bytes, is a path: a ~ or a drive path with anything under its root, a known path, or a / path with a folder and a name under it.
func isPath(p string, root int) bool {
	if len(p) <= root {
		return false
	}
	if root > 1 || strings.HasPrefix(p, knownRoot) {
		return true
	}
	first, rest, ok := strings.Cut(p[1:], "/")
	return ok && first != "" && rest != ""
}
