// Package task is the side of a task pack that needs no window (docs/plugin-system.md E6): the instruction and the message the user's AI service is sent for it (Prompt, Request) and how its reply reads (Parse). The host adds to the pack's own prompt the language to answer in, that nothing after the lines given may be guessed at, and the exact JSON the reply must be, so a pack cannot change how its reply is read. Only the lines the learner has reached are sent: the line and the few before it, or the film as far as it was watched, and never one after. It imports no area of the window.
package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kando/apps/kando/lines"
	"kando/apps/kando/plugins"
	"kando/packages/core"
)

// ContextRunes caps the lines of a film-so-far task: past it the earliest are left out, so a long film still fits a small model's context, as the command palette's questions do.
const ContextRunes = 40000

// defaultLanguage is what the reply is written in when no language is given, as Explain Line's.
const defaultLanguage = "简体中文"

// ErrEmpty is a reply that says nothing in any field; ErrMalformed one that is not the JSON the prompt asked for.
var (
	ErrEmpty     = errors.New("the AI service's reply to the task said nothing")
	ErrMalformed = errors.New("the AI service's reply to the task is not the JSON it was asked for")
)

// Input is what a task runs on, by its scope: the film's name; for a line or a word, the line and the lines before it, with the word looked up for a word; for the film so far, the lines watched (Seen), the last of them at most where the film was watched to.
type Input struct {
	Film   string
	Word   string
	Line   lines.Line
	Before []lines.Line
	Seen   []lines.Line
}

// Prompt is the instruction t is sent with: the pack's prompt, with {{language}} replaced by language, or a line naming it after the prompt when the prompt does not name it; then the host's rules, which come last so the pack's cannot override them: use only the subtitles given, which end where the learner is, and reply with only the JSON Parse reads.
func Prompt(t plugins.Task, language string) string {
	if strings.TrimSpace(language) == "" {
		language = defaultLanguage
	}
	prompt := strings.TrimSpace(t.Prompt)
	if strings.Contains(prompt, "{{language}}") {
		prompt = strings.ReplaceAll(prompt, "{{language}}", language)
	} else {
		prompt += "\n\nWrite your answer in " + language + "."
	}
	return prompt + "\n\n" + horizonRule + "\n\n" + ReplyRule(t)
}

// horizonRule keeps the reply to what the learner has watched.
const horizonRule = "Use only the subtitles in the user's message. They end where the learner has watched to; do not guess at or reveal anything that happens later."

// ReplyRule is the JSON the host asks t's reply to be: one object of the fields for sections, else an array of at most the pack's items of such objects; every value a string.
func ReplyRule(t plugins.Task) string {
	keys := make([]string, len(t.Reply.Fields))
	for i, f := range t.Reply.Fields {
		keys[i] = strconv.Quote(f.Key)
	}
	object := "a JSON object with exactly these keys, each a string (write \"\" for one you have nothing for): " + strings.Join(keys, ", ")
	if t.Render == plugins.RenderSections {
		return "Reply with only " + object + "."
	}
	return fmt.Sprintf("Reply with only a JSON array of at most %d items, each %s.", t.Items(), object)
}

// Request is the message t is sent with on in: the film's name, then for a line its lines before it and the line itself, for a word the word too, and for the film so far the lines watched, the earliest left out past ContextRunes. Each line is "[m:ss] translation / original".
func Request(t plugins.Task, in Input) string {
	var b strings.Builder
	if film := strings.TrimSpace(in.Film); film != "" {
		b.WriteString("Film: " + film + "\n\n")
	}
	switch t.Scope {
	case plugins.ScopeFilm:
		b.WriteString("The subtitles watched so far:\n")
		for _, l := range Trim(in.Seen, ContextRunes) {
			b.WriteString(line(l) + "\n")
		}
	default:
		if len(in.Before) > 0 {
			b.WriteString("The lines before it:\n")
			for _, l := range in.Before {
				b.WriteString(line(l) + "\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("The line: " + line(in.Line) + "\n")
		if t.Scope == plugins.ScopeWord {
			b.WriteString("The word: " + strings.TrimSpace(in.Word) + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// Trim is the last of seen that fit in runes characters as Request writes them, the earliest left out; none when not even the last one fits.
func Trim(seen []lines.Line, runes int) []lines.Line {
	total, from := 0, len(seen)
	for from > 0 {
		n := utf8.RuneCountInString(line(seen[from-1])) + 1
		if total+n > runes {
			break
		}
		total += n
		from--
	}
	return seen[from:]
}

// LineAt is line i of cues, split into its translation and original, with up to before lines before it that go with it as its context.
func LineAt(cues []core.Segment, i, before int) (lines.Line, []lines.Line) {
	var earlier []lines.Line
	for _, cue := range cues[max(i-before, 0):i] {
		translation, original := lines.Split(cue.Text)
		earlier = append(earlier, lines.New(cue.Start, translation, original))
	}
	translation, original := lines.Split(cues[i].Text)
	return lines.New(cues[i].Start, translation, original), earlier
}

// line writes l as "[m:ss] translation / original", either alone when the other is empty.
func line(l lines.Line) string {
	text := strings.TrimSpace(l.Original)
	if t := strings.TrimSpace(l.Translation); t != "" && text != "" && t != text {
		text = t + " / " + text
	} else if text == "" {
		text = t
	}
	return "[" + clock(l.Start) + "] " + text
}

// clock writes d as m:ss, or h:mm:ss from an hour.
func clock(d time.Duration) string {
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Item is one item of a reply: a value for each of the task's fields, in their order, "" for a field it left out.
type Item []string

// Parse reads the reply to t: the JSON ReplyRule asks for, which a model may have put in a code block or written text around. Sections are one item; a list or cards are the items with any value, at most the pack's items. A value the model wrote as a number or a list of strings is taken as text, the list a line an element. A reply that is not that JSON is ErrMalformed, one with no value ErrEmpty.
func Parse(t plugins.Task, reply string) ([]Item, error) {
	reply = strings.TrimSpace(reply)
	var raw []map[string]json.RawMessage
	opening, closing := "{", "}"
	if t.Render != plugins.RenderSections {
		opening, closing = "[", "]"
	}
	start, end := strings.Index(reply, opening), strings.LastIndex(reply, closing)
	if start < 0 || end <= start {
		return nil, ErrMalformed
	}
	body := []byte(reply[start : end+1])
	if t.Render == plugins.RenderSections {
		var one map[string]json.RawMessage
		if json.Unmarshal(body, &one) != nil {
			return nil, ErrMalformed
		}
		raw = append(raw, one)
	} else if json.Unmarshal(body, &raw) != nil {
		return nil, ErrMalformed
	}
	var out []Item
	for _, obj := range raw {
		item := make(Item, len(t.Reply.Fields))
		has := false
		for i, f := range t.Reply.Fields {
			item[i] = value(obj[f.Key])
			has = has || item[i] != ""
		}
		if has {
			out = append(out, item)
		}
		if t.Render != plugins.RenderSections && len(out) == t.Items() {
			break
		}
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

// value is a field's JSON as text: a string as it is, a list of strings a line each, a number or a boolean as written; "" for anything else.
func value(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		var kept []string
		for _, l := range list {
			if l = strings.TrimSpace(l); l != "" {
				kept = append(kept, l)
			}
		}
		return strings.Join(kept, "\n")
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return strconv.FormatBool(b)
	}
	return ""
}
