// Package explain is the side of Explain Line that needs no window: what the AI service set in 「AI 服务」 is asked about a subtitle line and how its reply reads (Prompt, Request, Parse), the follow-up questions about the line (FollowPrompt, FollowRequest), and the terms of the film's glossary the line holds (Terms). It sends only the line and the lines before it, so an explanation gives nothing away of what comes after. It imports no area of the window.
package explain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"kando/apps/kando/lines"
	"kando/packages/core"
)

// Sections are what the AI service said of a line, each "" when it said nothing of it: what the line means, its words and phrases one a line, what is worth knowing of its translation, and, for 展开, its tone and related expressions.
type Sections struct {
	Meaning, Breakdown, Translation, Tone, Related string
}

// HasMore reports whether there is more than the first sections to show: the tone or related expressions.
func (s Sections) HasMore() bool { return s.Tone != "" || s.Related != "" }

// QA is a follow-up question about a line and the AI service's answer.
type QA struct {
	Question, Answer string
}

// ErrEmpty is what Parse returns for a reply that says nothing.
var ErrEmpty = errors.New("the AI service said nothing about the line")

// defaultLanguage is what the AI service writes in when no language is given.
const defaultLanguage = "简体中文"

// Prompt is the instruction a line is explained with: what it means, its words and phrases, what is worth knowing of its translation when translated is set, its tone and related expressions, in language, as the JSON object Parse reads.
func Prompt(language string, translated bool) string {
	if strings.TrimSpace(language) == "" {
		language = defaultLanguage
	}
	translation := `"translation" 写空字符串。`
	if translated {
		translation = `"translation" 写这句的译文里值得注意的地方，比如为什么这样译、语气是否贴切，没什么可说就写空字符串。`
	}
	return "你在帮用户借助影片学外语。用户消息里是一部影片的几行字幕，每行以 [分:秒] 开头，后面是译文和原文，最后是要讲解的那一句。" +
		"用" + language + "讲解这一句的原文，只依据这句和它之前的字幕，不要猜测或透露之后的内容。" +
		`"meaning" 用一两句话说整句的意思；"breakdown" 逐个拆解关键的词或短语，每行一个，写成「原文 · 意思和用法」；` + translation +
		`"tone" 用一句话说这句的语气或说话场合；"related" 列一两个相关的说法，每行一个。都要简短。` +
		`只输出一个 JSON 对象：{"meaning": "", "breakdown": "", "translation": "", "tone": "", "related": ""}。`
}

// Request is the message line of the film named film is explained with: the lines before it as context, then the line itself, its original and its translation.
func Request(film string, before []lines.Line, line lines.Line) string {
	var b strings.Builder
	writeContext(&b, film, before)
	b.WriteString("要讲解的一句：[" + clock(line.Start) + "] " + quoted(line))
	if line.Original != "" && line.Translation != "" {
		b.WriteString("\n译文：" + line.Translation)
	}
	return b.String()
}

// Parse reads the AI service's reply to Request: the JSON object Prompt asks for, which a model may have put in a code block, or, from a model that answered in plain text, the text as the meaning. A reply that says nothing is ErrEmpty.
func Parse(reply string) (Sections, error) {
	reply = strings.TrimSpace(reply)
	var parsed struct {
		Meaning     string `json:"meaning"`
		Breakdown   string `json:"breakdown"`
		Translation string `json:"translation"`
		Tone        string `json:"tone"`
		Related     string `json:"related"`
	}
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start >= 0 && end > start && json.Unmarshal([]byte(reply[start:end+1]), &parsed) == nil {
		s := Sections{Meaning: strings.TrimSpace(parsed.Meaning), Breakdown: strings.TrimSpace(parsed.Breakdown), Translation: strings.TrimSpace(parsed.Translation),
			Tone: strings.TrimSpace(parsed.Tone), Related: strings.TrimSpace(parsed.Related)}
		if s == (Sections{}) {
			return Sections{}, ErrEmpty
		}
		return s, nil
	}
	if reply == "" {
		return Sections{}, ErrEmpty
	}
	return Sections{Meaning: reply}, nil
}

// FollowPrompt is the instruction a follow-up question about a line is sent with: answer it in language, from the line and the lines before it only, in plain text.
func FollowPrompt(language string) string {
	if strings.TrimSpace(language) == "" {
		language = defaultLanguage
	}
	return "你在帮用户借助影片学外语。用户消息里是一部影片的几行字幕、要讲解的那一句、之前对它的讲解和问答，最后是用户对这句的追问。" +
		"用" + language + "回答追问，两三句话以内；只依据这句和它之前的字幕，之后才讲到的不要猜测或透露。只输出回答的文字。"
}

// FollowRequest is the message question about line is asked with: what Request sent, what was explained of the line and the questions asked of it before, the question last.
func FollowRequest(film string, before []lines.Line, line lines.Line, explained Sections, asked []QA, question string) string {
	var b strings.Builder
	b.WriteString(Request(film, before, line))
	if explained.Meaning != "" || explained.Breakdown != "" {
		b.WriteString("\n\n之前的讲解：\n" + strings.TrimSpace(explained.Meaning+"\n"+explained.Breakdown))
	}
	for _, qa := range asked {
		b.WriteString("\n\n问：" + qa.Question + "\n答：" + qa.Answer)
	}
	b.WriteString("\n\n追问：" + strings.TrimSpace(question))
	return b.String()
}

// Terms are the terms of glossary that original holds, written "source → target", in the glossary's order, a source once: a term of words where the line has them as words of its own, or followed by an ending such as a plural's, so "meander" is found in "meanders" but "an" not in "plan" or "and".
func Terms(original string, glossary []core.Term) []string {
	folded := strings.ToLower(original)
	var out []string
	seen := map[string]bool{}
	for _, t := range glossary {
		source := strings.TrimSpace(t.Source)
		key := strings.ToLower(source)
		if !t.Valid() || seen[key] || !holdsWord(folded, key) {
			continue
		}
		seen[key] = true
		out = append(out, source+" → "+strings.TrimSpace(t.Target))
	}
	return out
}

// endings are what may follow a term in a line that still has it: none, or an ending of the English plural or verb.
var endings = []string{"", "s", "es", "ed", "ing"}

// holdsWord reports whether text holds word as Terms finds it, both in lower case.
func holdsWord(text, word string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(word)
		if bounded(text, i, word, end) {
			return true
		}
		from = i + 1
	}
}

// bounded reports whether word, found at text[i:end], stands as a word there: no letter of a spaced script runs into it before, and none after but one of endings.
func bounded(text string, i int, word string, end int) bool {
	first, _ := utf8.DecodeRuneInString(word)
	last, _ := utf8.DecodeLastRuneInString(word)
	if spaced(first) {
		if r, _ := utf8.DecodeLastRuneInString(text[:i]); i > 0 && spaced(r) {
			return false
		}
	}
	if !spaced(last) {
		return true
	}
	for _, ending := range endings {
		if !strings.HasPrefix(text[end:], ending) {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(text[end+len(ending):]); end+len(ending) == len(text) || !spaced(r) {
			return true
		}
	}
	return false
}

// spaced reports whether r is a letter or digit of a script written with spaces between words, where a word has bounds to find.
func spaced(r rune) bool {
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return false
	}
	return !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai)
}

// writeContext writes the film's name and the lines before the line explained, one a line as "[m:ss] translation / original".
func writeContext(b *strings.Builder, film string, before []lines.Line) {
	if film = strings.TrimSpace(film); film != "" {
		b.WriteString("影片：" + film + "\n")
	}
	if len(before) == 0 {
		return
	}
	b.WriteString("之前的字幕：\n")
	for _, l := range before {
		text := l.Translation
		switch {
		case text == "":
			text = l.Original
		case l.Original != "":
			text += " / " + l.Original
		}
		b.WriteString("[" + clock(l.Start) + "] " + text + "\n")
	}
	b.WriteString("\n")
}

// quoted is what of a line is explained: its original, or its translation for a line that has none.
func quoted(l lines.Line) string {
	if l.Original != "" {
		return l.Original
	}
	return l.Translation
}

// clock writes d as the player does, "1:05" or "1:02:03".
func clock(d time.Duration) string {
	d = max(d, 0)
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
