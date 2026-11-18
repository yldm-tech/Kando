package phonesync

import (
	"strconv"
	"testing"
	"time"

	psync "kando/packages/sync"
)

func position(film string, ms int64, device string, at time.Time) psync.Record {
	return psync.Record{Kind: psync.KindPosition, ID: film, Film: film, Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Value(ms), Stamp: psync.Stamp{At: at, Device: device}}}}
}

func TestHandoffIsWhereAPhoneLastLeftAFilm(t *testing.T) {
	t0 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	l := psync.NewLedger()
	n := 0
	id := func() string { n++; return "c" + strconv.Itoa(n) }
	if _, ok := Handoff(l, "mac"); ok {
		t.Fatal("an empty ledger hands off")
	}
	l.Put([]psync.Record{position("talk", 65000, "mac", t0), position("tide", 125000, "phone", t0.Add(time.Minute)), position("old", 3000, "pad", t0.Add(-time.Hour))}, id)
	got, ok := Handoff(l, "mac")
	if !ok || got.Film != "tide" || got.At != 125*time.Second || got.Stamp.Device != "phone" {
		t.Fatalf("handoff %+v, %v", got, ok)
	}
	if LeftOn(l, "tide", "mac") != "phone" || LeftOn(l, "talk", "mac") != "" || LeftOn(l, "none", "mac") != "" {
		t.Fatal("LeftOn names the wrong devices")
	}
	// The desktop played a film since: the latest place is its own, and nothing is handed off.
	l.Put([]psync.Record{position("talk", 70000, "mac", t0.Add(2*time.Minute))}, id)
	if got, ok := Handoff(l, "mac"); ok {
		t.Fatalf("handed off %+v after the desktop played", got)
	}
	// A phone that watched a film to its end leaves no place to continue from.
	l.Put([]psync.Record{{Kind: psync.KindPosition, ID: "end", Film: "end", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Null, Stamp: psync.Stamp{At: t0.Add(3 * time.Minute), Device: "phone"}}}}}, id)
	if got, ok := Handoff(l, "mac"); ok {
		t.Fatalf("handed off %+v at a film's start", got)
	}
	// A phone leaves another film later; the desktop then plays a film to its end, which leaves no position: the desktop played last, so nothing is handed off.
	l.Put([]psync.Record{position("tide", 140000, "phone", t0.Add(4*time.Minute))}, id)
	if got, ok := Handoff(l, "mac"); !ok || got.Film != "tide" {
		t.Fatalf("handoff %+v, %v after a phone left a film", got, ok)
	}
	l.Put([]psync.Record{{Kind: psync.KindPosition, ID: "talk", Film: "talk", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Null, Stamp: psync.Stamp{At: t0.Add(5 * time.Minute), Device: "mac"}}}}}, id)
	if got, ok := Handoff(l, "mac"); ok {
		t.Fatalf("handed off %+v after the desktop played a film to its end", got)
	}
	words, lines, positions := Counts(l)
	if words != 0 || lines != 0 || positions != 2 {
		t.Fatalf("counts %d %d %d", words, lines, positions)
	}
}

func TestCountsLeaveTombstonesOut(t *testing.T) {
	t0 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	l := psync.NewLedger()
	s := psync.Stamp{At: t0, Device: "phone"}
	l.Put([]psync.Record{
		{Kind: psync.KindWord, ID: "w1", Fields: map[string]psync.Field{psync.FieldWord: {Value: psync.Value("span"), Stamp: s}}},
		{Kind: psync.KindWord, ID: "w2", Removed: &s},
		{Kind: psync.KindLine, ID: "l1", Film: "talk", Fields: map[string]psync.Field{psync.FieldOriginal: {Value: psync.Value("Hello"), Stamp: s}}},
	}, func() string { return "c" })
	if words, lines, positions := Counts(l); words != 1 || lines != 1 || positions != 0 {
		t.Fatalf("counts %d %d %d", words, lines, positions)
	}
}

func TestJoinedKeepsBothNotes(t *testing.T) {
	c := psync.Conflict{Kind: psync.KindLine, Field: psync.FieldNote, Kept: psync.Field{Value: psync.Value("a")}, Lost: psync.Field{Value: psync.Value("b")}}
	if got := Text(Joined(c)); got != "a\nb" {
		t.Fatalf("joined %q", got)
	}
	if !Joinable(psync.KindLine, psync.FieldNote) || !Joinable(psync.KindWord, psync.FieldMeaning) || Joinable(psync.KindLine, psync.FieldTranslation) {
		t.Fatal("Joinable")
	}
	if Text(psync.Value(5)) != "" {
		t.Fatal("a number read as text")
	}
}
