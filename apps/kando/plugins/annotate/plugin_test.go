package annotate

import (
	"reflect"
	"testing"
)

// TestFromPlugin checks the marks an annotator of a script plugin gives: its offsets, UTF-16 code units as JavaScript counts them, become byte offsets; a mark out of the line, splitting a character, empty, of a tone not known or of neither a reading nor a line is dropped.
func TestFromPlugin(t *testing.T) {
	line := "😀你好 ok"
	got := FromPlugin(line, []any{
		map[string]any{"start": int64(2), "end": int64(4), "ruby": "nǐ hǎo"},
		map[string]any{"start": float64(5), "end": float64(7), "tone": "warn", "underline": true},
		map[string]any{"start": int64(1), "end": int64(3), "ruby": "x"},
		map[string]any{"start": int64(0), "end": int64(99), "tone": "warn"},
		map[string]any{"start": int64(3), "end": int64(3), "tone": "warn"},
		map[string]any{"start": int64(5), "end": int64(7), "tone": "purple"},
		map[string]any{"start": int64(5), "end": int64(7)},
		map[string]any{"start": int64(5), "end": int64(7), "ruby": "a\nb"},
		"not a mark",
	})
	want := []Mark{{Start: 4, End: 10, Ruby: "nǐ hǎo"}, {Start: 11, End: 13, Tone: "warn", Underline: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if FromPlugin(line, "nothing") != nil {
		t.Error("not a list")
	}
}

// TestMerge checks that a plugin's mark joins Kando's own only where it overlaps none of the same kind.
func TestMerge(t *testing.T) {
	have := []Mark{{Start: 0, End: 6, Ruby: "にほん"}, {Start: 0, End: 9, Tone: "accent"}}
	got := Merge(have, []Mark{{Start: 3, End: 6, Ruby: "x"}, {Start: 3, End: 6, Underline: true}, {Start: 9, End: 12, Ruby: "ご"}})
	want := []Mark{{Start: 0, End: 6, Ruby: "にほん"}, {Start: 0, End: 9, Tone: "accent"}, {Start: 9, End: 12, Ruby: "ご"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
