package phonesync

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/study"
	psync "kando/packages/sync"
)

func TestLineRecordsReadBackIntoTheStudyRecord(t *testing.T) {
	saved := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	book := study.Book{Device: "desk", Now: func() time.Time { return saved }}
	book.Toggle(study.Line{Video: "Talk/talk.mp4", Start: 1200 * time.Millisecond, End: 2500 * time.Millisecond, Original: "Hello", Translation: "你好", Film: "Talk", Note: "greeting"})
	book.Toggle(study.Line{Video: "Other/other.mp4", Start: time.Second})
	book.Remove("Other/other.mp4", time.Second)
	films := map[string]string{"Talk/talk.mp4": "f1"}
	records := LineRecords(book, func(video string) string { return films[video] })
	if len(records) != 2 {
		t.Fatalf("records %+v, want a line and a tombstone", records)
	}
	line, gone := records[0], records[1]
	if line.Film != "f1" || string(line.Fields[psync.FieldStartMS].Value) != "1200" || line.Fields[psync.FieldNote].Device != "desk" {
		t.Errorf("line %+v", line)
	}
	if gone.Removed == nil || gone.Film != "" {
		t.Errorf("tombstone %+v", gone)
	}

	// The same records taken into a book of another device make the same line.
	other := study.Book{Device: "phone"}
	lines, removed := LinesOf(other, records, func(film string) (string, bool) { return "Talk/talk.mp4", film == "f1" })
	other.Merge(lines, removed)
	if len(other.Lines) != 1 || len(other.Removed) != 1 {
		t.Fatalf("book %+v", other)
	}
	got, want := other.Lines[0], book.Lines[0]
	if got.ID != want.ID || got.Video != want.Video || got.Start != want.Start || got.End != want.End || got.Note != want.Note || got.Original != want.Original || got.Film != want.Film || !got.SavedAt.Equal(want.SavedAt) || got.Device != "desk" {
		t.Errorf("line read back %+v, want %+v", got, want)
	}
}

func TestCorrectionRecordsReadBack(t *testing.T) {
	text := "fixed"
	edits := []core.Correction{
		{Span: core.Span{Start: time.Second, End: 2 * time.Second}, Translation: &text, StartShift: -100 * time.Millisecond},
		{Span: core.Span{Start: 3 * time.Second, End: 4 * time.Second}, NoSpeech: true},
	}
	records := CorrectionRecords("f1", edits)
	if len(records) != 2 || records[0].ID != "f1/1000-2000" {
		t.Fatalf("records %+v", records)
	}
	for i, r := range records {
		c, ok := CorrectionOf(r)
		if !ok || c.Span != edits[i].Span || c.StartShift != edits[i].StartShift || c.NoSpeech != edits[i].NoSpeech || (c.Translation == nil) != (edits[i].Translation == nil) || c.Original != nil {
			t.Errorf("correction %d read back %+v, want %+v", i, c, edits[i])
		}
	}
	// A correction whose fields are all null takes its line's correction away.
	cleared := psync.Record{Kind: psync.KindCorrection, ID: "f1/1000-2000", Fields: map[string]psync.Field{psync.FieldTranslation: {Value: psync.Null}, psync.FieldStartShiftMS: {Value: psync.Null}}}
	c, _ := CorrectionOf(cleared)
	if got := SetCorrection(edits, c); len(got) != 1 || got[0].Span != edits[1].Span {
		t.Errorf("edits once cleared %+v", got)
	}
}

func TestStateAcceptsOnlyRecordsItCanKeep(t *testing.T) {
	known := func(film string) bool { return film == "f1" }
	none := func(string, string) bool { return false }
	ok := []psync.Record{
		{Kind: psync.KindPosition, ID: "f1", Film: "f1"},
		{Kind: psync.KindCorrection, ID: "f1/0-10", Film: "f1"},
		{Kind: psync.KindLine, ID: "l", Film: "f1", Fields: map[string]psync.Field{psync.FieldStartMS: {Value: psync.Value(0)}}},
		{Kind: psync.KindLine, ID: "gone", Removed: &psync.Stamp{}},
	}
	if why := StateAccepts(ok, known, none); why != "" {
		t.Errorf("refused %q", why)
	}
	for _, bad := range []psync.Record{
		{Kind: psync.KindWord, ID: "w"},
		{Kind: psync.KindPosition, ID: "f2", Film: "f2"},
		{Kind: psync.KindPosition, ID: "f1", Film: "f2"},
		{Kind: psync.KindCorrection, ID: "f2/0-10", Film: "f1"},
		{Kind: psync.KindLine, ID: "l", Film: "f1"},
	} {
		if why := StateAccepts([]psync.Record{bad}, known, none); why == "" {
			t.Errorf("accepted %+v", bad)
		}
	}
	// A line the ledger has already is kept whatever its film, as one whose film left the library.
	if why := StateAccepts([]psync.Record{{Kind: psync.KindLine, ID: "l", Film: "f9"}}, known, func(kind, id string) bool { return id == "l" }); why != "" {
		t.Errorf("refused a line the ledger has: %q", why)
	}
	if !strings.Contains(StateAccepts([]psync.Record{{Kind: psync.KindWord, ID: "w"}}, known, none), "word") {
		t.Error("the refusal does not name the kind")
	}
}

func TestFieldValidTakesOnlyValuesOfTheFieldsType(t *testing.T) {
	for _, c := range []struct {
		kind, field, value string
		ok                 bool
	}{
		{psync.KindPosition, psync.FieldPositionMS, `12345`, true},
		{psync.KindPosition, psync.FieldPositionMS, `12345.6`, true},
		{psync.KindPosition, psync.FieldPositionMS, `null`, true},
		{psync.KindPosition, psync.FieldPositionMS, `"12345"`, false},
		{psync.KindLine, psync.FieldNote, `"a note"`, true},
		{psync.KindLine, psync.FieldNote, `""`, true},
		{psync.KindLine, psync.FieldNote, `7`, false},
		{psync.KindLine, psync.FieldNote, `null`, false},
		{psync.KindLine, psync.FieldStartMS, `"1000"`, false},
		{psync.KindLine, psync.FieldSavedAt, `"2026-11-18T09:00:00Z"`, true},
		{psync.KindLine, psync.FieldSavedAt, `"yesterday"`, false},
		{psync.KindLine, psync.FieldReview, `{"box":2,"due":"2026-11-18T09:00:00Z"}`, true},
		{psync.KindLine, psync.FieldReview, `{"box":"two"}`, false},
		{psync.KindLine, psync.FieldReview, `2`, false},
		{psync.KindCorrection, psync.FieldTranslation, `null`, true},
		{psync.KindCorrection, psync.FieldTranslation, `false`, false},
		{psync.KindCorrection, psync.FieldStartShiftMS, `-100`, true},
		{psync.KindCorrection, psync.FieldNoSpeech, `true`, true},
		{psync.KindCorrection, psync.FieldNoSpeech, `1`, false},
		// A field this desktop does not know holds anything.
		{psync.KindLine, "later", `[1]`, true},
	} {
		if got := FieldValid(c.kind, c.field, json.RawMessage(c.value)); got != c.ok {
			t.Errorf("%s %s = %s: valid %v, want %v", c.kind, c.field, c.value, got, c.ok)
		}
	}
	bad := []psync.Record{{Kind: psync.KindLine, ID: "l", Film: "f1", Fields: map[string]psync.Field{psync.FieldStartMS: {Value: psync.Value(0)}, psync.FieldNote: {Value: psync.Value(7)}}}}
	if why := StateAccepts(bad, func(string) bool { return true }, func(string, string) bool { return false }); why == "" {
		t.Error("accepted a note that is a number")
	}
}

// A phone's milliseconds may come as a Double; they are rounded rather than read as 0.
func TestPositionOfRoundsMilliseconds(t *testing.T) {
	r := psync.Record{Kind: psync.KindPosition, ID: "f1", Film: "f1", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: json.RawMessage(`12345.6`)}}}
	if got := PositionOf(r); got != 12346*time.Millisecond {
		t.Errorf("position %v, want 12.346s", got)
	}
}
