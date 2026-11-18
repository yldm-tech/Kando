package core

import (
	"testing"
	"time"
)

func sec(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func ptr(s string) *string { return &s }

func TestCorrectLaysEditsOverTheMachinesLines(t *testing.T) {
	lines := []Line{
		{Start: sec(1), End: sec(2), Original: "Hello.", Translation: "你好。"},
		{Start: sec(3), End: sec(4), Original: "Music"},
		{Start: sec(5), End: sec(6), Original: "Bye.", Translation: "再见。"},
	}
	edits := []Correction{
		{Span: Span{sec(5), sec(6)}, Translation: ptr("拜拜。"), StartShift: -sec(2.5)},
		{Span: Span{sec(3), sec(4)}, NoSpeech: true},
	}
	got := Correct(lines, edits)
	if len(got) != 3 {
		t.Fatalf("got %d lines", len(got))
	}
	// The shifted line now starts first but one: 2.5s, after 1s and before the no-speech line at 3s.
	if got[1].Translation != "拜拜。" || got[1].Original != "Bye." || got[1].Start != sec(2.5) || got[1].Edit != 0 || got[1].Machine != (Span{sec(5), sec(6)}) {
		t.Fatalf("edited line %+v", got[1])
	}
	if !got[2].NoSpeech || got[2].Edit != 1 {
		t.Fatalf("no-speech line %+v", got[2])
	}
	if got[0].Edit != -1 || got[0].Translation != "你好。" {
		t.Fatalf("untouched line %+v", got[0])
	}
	if spoken := Spoken(got); len(spoken) != 2 {
		t.Fatalf("spoken %+v", spoken)
	}
}

func TestCorrectFollowsLinesARedoMoved(t *testing.T) {
	// A redo heard the line from 1.1s to 2.2s where it had 1s to 2s; one it moved far off takes nothing.
	lines := []Line{{Start: sec(1.1), End: sec(2.2), Original: "Hello"}, {Start: sec(9), End: sec(10), Original: "Later"}}
	edits := []Correction{
		{Span: Span{sec(1), sec(2)}, Original: ptr("Hello!")},
		{Span: Span{sec(7), sec(7.5)}, Original: ptr("Gone")},
	}
	got := Correct(lines, edits)
	if got[0].Original != "Hello!" || got[0].Edit != 0 || got[0].Machine != (Span{sec(1.1), sec(2.2)}) {
		t.Fatalf("moved line %+v", got[0])
	}
	if got[1].Original != "Later" || got[1].Edit != -1 {
		t.Fatalf("a correction that fits no line took %+v", got[1])
	}
}

func TestCorrectGivesALineTheCorrectionThatFitsBest(t *testing.T) {
	lines := []Line{{Start: sec(1), End: sec(3), Original: "a"}}
	edits := []Correction{
		{Span: Span{sec(1.5), sec(3)}, Original: ptr("partial")},
		{Span: Span{sec(1), sec(3)}, Original: ptr("exact")},
		{Span: Span{sec(1.2), sec(3)}, Original: ptr("closer")},
	}
	if got := Correct(lines, edits); got[0].Original != "exact" || got[0].Edit != 1 {
		t.Fatalf("got %+v", got[0])
	}
}

func TestCorrectSegmentsKeepsTheBilingualForm(t *testing.T) {
	cues := []Segment{
		{Start: sec(1), End: sec(2), Text: "你好。\nHello."},
		{Start: sec(3), End: sec(4), Text: "Bye."},
		{Start: sec(5), End: sec(6), Text: "Noise"},
	}
	edits := []Correction{
		{Span: Span{sec(3), sec(4)}, Translation: ptr("再见。")},
		{Span: Span{sec(5), sec(6)}, NoSpeech: true},
		{Span: Span{sec(1), sec(2)}, EndShift: sec(0.3)},
	}
	got, from := CorrectSegments(cues, edits)
	want := []Segment{{Start: sec(1), End: sec(2.3), Text: "你好。\nHello."}, {Start: sec(3), End: sec(4), Text: "再见。\nBye."}}
	if len(got) != len(want) || len(from) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cue %d is %+v, want %+v", i, got[i], want[i])
		}
	}
	if from[1].Machine != (Span{sec(3), sec(4)}) || from[1].Edit != 0 {
		t.Fatalf("from %+v", from[1])
	}
}

func TestSetCorrection(t *testing.T) {
	a := Correction{Span: Span{sec(1), sec(2)}, NoSpeech: true}
	edits := SetCorrection(nil, -1, a)
	if len(edits) != 1 {
		t.Fatalf("add: %+v", edits)
	}
	if again := SetCorrection(edits, -1, Correction{Span: a.Span}); len(again) != 1 {
		t.Fatalf("an empty correction was added: %+v", again)
	}
	changed := SetCorrection(edits, 0, Correction{Span: a.Span, StartShift: sec(0.1)})
	if changed[0].NoSpeech || changed[0].StartShift != sec(0.1) || !edits[0].NoSpeech {
		t.Fatalf("replace: %+v, before %+v", changed, edits)
	}
	if gone := SetCorrection(changed, 0, Correction{Span: a.Span}); len(gone) != 0 {
		t.Fatalf("an empty correction stayed: %+v", gone)
	}
}
