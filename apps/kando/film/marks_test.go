package film

import (
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/lookup"

	ns "github.com/nsgo-dev/nsgo/core"
)

// mark is a mark over the run of line that is word, with the fields of m.
func mark(line, word string, m subtitle.Mark) subtitle.Mark {
	m.Start = strings.Index(line, word)
	m.End = m.Start + len(word)
	return m
}

// TestLineSpansDrawTheMarks checks the spans of a line with marks: a reading is a view over its characters, which a click looks up as the word it is in; a tone draws a thick solid line in its color under its run, which keeps the line's color, and a word in the vocabulary a dashed one, so the two never look alike; a mark off a character's start or outside the line is left out; and a copy under the line, without look, has the readings alone, which take no click.
func TestLineSpansDrawTheMarks(t *testing.T) {
	line := "猫が行った。"
	tokens := []lookup.Token{{Text: "猫", Word: true}, {Text: "が", Word: true}, {Text: "行った", Word: true}, {Text: "。"}}
	marks := []subtitle.Mark{
		mark(line, "猫", subtitle.Mark{Ruby: "ねこ"}),
		mark(line, "が", subtitle.Mark{Tone: subtitle.ToneAccent}),
		mark(line, "行った", subtitle.Mark{Underline: true}),
		{Start: 1, End: 2, Ruby: "x"},
		{Start: 0, End: len(line) + 1, Tone: subtitle.ToneWarn},
	}
	accent := ns.RGB(0x123456)
	var looked []string
	ink := spanInk{style: ns.Style{FontSize: 22}, look: func(w string) { looked = append(looked, w) }, tone: func(tone string) (ns.Color, bool) {
		return accent, tone == subtitle.ToneAccent
	}}
	spans := lineSpans(line, tokens, marks, ink)
	var contents []string
	for _, sp := range spans {
		contents = append(contents, sp.Content)
	}
	if strings.Join(contents, "|") != "猫|が|行った|。" {
		t.Fatalf("the spans: %q", contents)
	}
	if spans[0].Attachment == nil || spans[1].Attachment != nil {
		t.Fatalf("the reading's view: %+v", spans[:2])
	}
	if st := spans[1].Style; st.Color != 0 || st.TextDecoration != ns.TextDecorationUnderline || st.TextDecorationColor != accent || st.TextDecorationStyle != 0 || st.TextDecorationThickness != markThickness {
		t.Fatalf("the toned run: %+v", st)
	}
	if st := spans[2].Style; st.TextDecoration != ns.TextDecorationUnderline || st.TextDecorationStyle != ns.DecorationDashed || st.TextDecorationColor != 0 || st.Color != 0 {
		t.Fatalf("the underlined run: %+v", st)
	}
	if spans[3].OnPress != nil {
		t.Fatal("the punctuation looks something up")
	}
	spans[2].OnPress(ns.PressEvent{})
	ruby := spans[0].Attachment.(ns.View)
	ruby.OnPress(ns.PressEvent{})
	if strings.Join(looked, "|") != "行った|猫" {
		t.Fatalf("the clicks looked up %q", looked)
	}
	if got := ruby.Children[0].(ns.Text).Content; got != "ねこ" {
		t.Fatalf("the reading: %q", got)
	}

	copied := lineSpans(line, nil, marks, spanInk{style: ns.Style{FontSize: 22}})
	if len(copied) != 2 || copied[0].Attachment == nil || copied[1].Content != "が行った。" || copied[1].Style.TextDecoration != 0 || copied[1].OnPress != nil {
		t.Fatalf("the copy's spans: %+v", copied)
	}
	if v := copied[0].Attachment.(ns.View); v.OnPress != nil || v.PointerEvents != ns.PointerEventsNone {
		t.Fatalf("the copy's reading takes clicks: %+v", v)
	}
}

// TestLineSpansCutAtTheTones checks that a tone whose mark ends inside a token draws its line under its own characters alone: a kanji-only line the tokens leave whole, while the marks cut it into two words of different levels, has each word's line in its level's color, dashed under the word in the vocabulary, and a click on either half looks up the token.
func TestLineSpansCutAtTheTones(t *testing.T) {
	line := "東京大学"
	tokens := []lookup.Token{{Text: line, Word: true}}
	marks := []subtitle.Mark{
		mark(line, "東京", subtitle.Mark{Tone: subtitle.ToneAccent}),
		mark(line, "大学", subtitle.Mark{Tone: subtitle.ToneWarn, Underline: true}),
	}
	accent, warn := ns.RGB(0x123456), ns.RGB(0x654321)
	var looked []string
	ink := spanInk{look: func(w string) { looked = append(looked, w) }, tone: func(tone string) (ns.Color, bool) {
		if tone == subtitle.ToneWarn {
			return warn, true
		}
		return accent, tone == subtitle.ToneAccent
	}}
	spans := lineSpans(line, tokens, marks, ink)
	if len(spans) != 2 || spans[0].Content != "東京" || spans[1].Content != "大学" {
		t.Fatalf("the spans: %+v", spans)
	}
	if st := spans[0].Style; st.TextDecorationColor != accent || st.TextDecoration != ns.TextDecorationUnderline || st.TextDecorationStyle != 0 {
		t.Fatalf("the first word's run: %+v", st)
	}
	if st := spans[1].Style; st.TextDecorationColor != warn || st.TextDecoration != ns.TextDecorationUnderline || st.TextDecorationStyle != ns.DecorationDashed {
		t.Fatalf("the second word's run: %+v", st)
	}
	spans[0].OnPress(ns.PressEvent{})
	spans[1].OnPress(ns.PressEvent{})
	if strings.Join(looked, "|") != line+"|"+line {
		t.Fatalf("the clicks looked up %q", looked)
	}
}

// TestLineSpansWithoutMarks checks that a line without marks is cut at its words as before, each word looking itself up.
func TestLineSpansWithoutMarks(t *testing.T) {
	tokens := []lookup.Token{{Text: "Hello", Word: true}, {Text: ", "}, {Text: "there", Word: true}, {Text: "."}}
	var looked []string
	spans := lineSpans("Hello, there.", tokens, nil, spanInk{look: func(w string) { looked = append(looked, w) }})
	if len(spans) != 4 || spans[1].OnPress != nil || spans[1].Style.Cursor == ns.CursorPointer || spans[2].Style.Cursor != ns.CursorPointer {
		t.Fatalf("the spans: %+v", spans)
	}
	spans[2].OnPress(ns.PressEvent{})
	if len(looked) != 1 || looked[0] != "there" {
		t.Fatalf("looked up %q", looked)
	}
}

// TestMarksShowOnThePicture checks that the marks the app gives the original of the cue playing are drawn on the picture: its reading shows over its word, and follows the app's marks as they change.
func TestMarksShowOnThePicture(t *testing.T) {
	a := newFakeApp()
	line := "猫が行った。"
	a.subs.Cues[0].Text = "The cat went.\n" + line
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	app.Progress("id:film-video", 1500*time.Millisecond, 10*time.Second)
	reading := `id:film-subtitle-lines >> text:"ねこ"`
	if app.Exists(reading) {
		t.Fatal("a reading shows with no marks")
	}
	change(app, a, func() { a.marks = map[int][]subtitle.Mark{0: {mark(line, "猫", subtitle.Mark{Ruby: "ねこ"})}} })
	app.WaitUntil(time.Minute, func() bool { return app.Exists(reading) })
	change(app, a, func() { a.marks = nil })
	if app.Exists(reading) {
		t.Fatal("the reading stayed once the app had no marks")
	}
}
