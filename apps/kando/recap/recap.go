// Package recap is the side of the 前情提要 card that needs no window: what the AI service set in 「AI 服务」 is asked to sum up of the part of a film already watched and how its reply reads (Prompt, Request, Parse), and the lines the card shows instead when no AI service can (Excerpts). It is given only the lines watched, so the recap gives nothing away of what comes after. It imports no area of the window.
package recap

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/lines"
)

// Count is how many items the card shows, as the design's three.
const Count = 3

// contextRunes caps the lines sent; past it the earliest are left out, so a long film still fits a small model's context.
const contextRunes = 40000

// Item is a moment of the part watched and what happened there: a summary, or the line itself for Excerpts. At is the start of a line watched, in the subtitles' time.
type Item struct {
	At   time.Duration
	Text string
}

// ErrEmpty is what Parse returns for a reply that gives no item at a line watched.
var ErrEmpty = errors.New("the AI service gave no recap")

// Prompt is the instruction the recap is asked with: Count short items of what happened in the lines given, in language, each at the start of the line it is about, as the JSON object Parse reads.
func Prompt(language string) string {
	if strings.TrimSpace(language) == "" {
		language = "简体中文"
	}
	return "你在帮用户接着看一部之前看过一部分的影片。用户消息里是这部影片用户已经看过的那部分字幕，每行以 [分:秒] 开头，后面是译文和原文。" +
		fmt.Sprintf("用%s写 %d 条前情提要，按时间先后，每条一句话、不超过 30 个字，说那一段讲了什么；", language, Count) +
		"只依据这些字幕，不要猜测或透露之后的内容。每条的 at 是它所讲那一段开头那行字幕的时间，照字幕里写的抄。" +
		`只输出一个 JSON 对象：{"items": [{"at": "分:秒", "text": "提要"}]}。`
}

// Request is the message the recap of the film named film is asked with: the lines watched, one a line as "[m:ss] translation / original", the earliest left out past contextRunes.
func Request(film string, seen []lines.Line) string {
	rows := make([]string, len(seen))
	for i, l := range seen {
		text := l.Translation
		switch {
		case text == "":
			text = l.Original
		case l.Original != "":
			text += " / " + l.Original
		}
		rows[i] = "[" + clock(l.Start) + "] " + text
	}
	size, from := 0, len(rows)
	for from > 0 {
		n := len([]rune(rows[from-1])) + 1
		if size+n > contextRunes {
			break
		}
		size += n
		from--
	}
	return "影片：" + strings.TrimSpace(film) + "\n已经看过的字幕：\n" + strings.Join(rows[from:], "\n")
}

// Parse reads the AI service's reply to Request: the items of the JSON object Prompt asks for, which a model may have put in a code block, in the order of the film, at most Count. An item is kept only at the start of a line of seen it names to the second, so a click on it never plays past what was watched; an item without text, or one at no line watched, is dropped. A reply that leaves none is ErrEmpty.
func Parse(reply string, seen []lines.Line) ([]Item, error) {
	reply = strings.TrimSpace(reply)
	var parsed struct {
		Items []struct {
			At   string `json:"at"`
			Text string `json:"text"`
		} `json:"items"`
	}
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start || json.Unmarshal([]byte(reply[start:end+1]), &parsed) != nil {
		return nil, ErrEmpty
	}
	var out []Item
	for _, it := range parsed.Items {
		text, at := strings.TrimSpace(it.Text), strings.TrimSpace(it.At)
		if text == "" {
			continue
		}
		i := slices.IndexFunc(seen, func(l lines.Line) bool { return clock(l.Start) == at })
		if i < 0 || slices.ContainsFunc(out, func(x Item) bool { return x.At == seen[i].Start }) {
			continue
		}
		out = append(out, Item{At: seen[i].Start, Text: text})
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	slices.SortStableFunc(out, func(a, b Item) int { return int(a.At - b.At) })
	return out[:min(len(out), Count)], nil
}

// Excerpts are the lines the card shows when no AI service sums them up: the first, the middle and the last line watched, as the design's prototype picks them, each its translation or else its original; every line when there are no more than Count.
func Excerpts(seen []lines.Line) []Item {
	picks := seen
	if len(seen) > Count {
		picks = []lines.Line{seen[0], seen[len(seen)/2], seen[len(seen)-1]}
	}
	out := make([]Item, 0, len(picks))
	for _, l := range picks {
		text := l.Translation
		if text == "" {
			text = l.Original
		}
		out = append(out, Item{At: l.Start, Text: text})
	}
	return out
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
