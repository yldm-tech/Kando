package study

import (
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

// at is t0 and m minutes.
func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

// bookOn is an empty book whose changes device makes at the minute *clock.
func bookOn(device string, clock *int) Book {
	return Book{Device: device, Now: func() time.Time { return at(*clock) }}
}

// TestSetNoteStampsTheNote checks that a note is kept trimmed and stamped with when and on which device it was written, that writing the same note again changes nothing, and that a line the book does not have is reported.
func TestSetNoteStampsTheNote(t *testing.T) {
	clock := 0
	b := bookOn("mac", &clock)
	b.Toggle(Line{Video: "a", Start: time.Second, SavedAt: at(0)})
	id := b.Lines[0].ID
	clock = 5
	if !b.SetNote(id, "  picks up = 裹挟  ") {
		t.Fatal("no change")
	}
	if l := b.Lines[0]; l.Note != "picks up = 裹挟" || l.Stamps.Note != (Stamp{At: at(5), Device: "mac"}) || !l.Stamps.Original.At.Equal(at(0)) {
		t.Fatalf("line %+v", l)
	}
	clock = 6
	if b.SetNote(id, "picks up = 裹挟") || !b.Lines[0].Stamps.Note.At.Equal(at(5)) {
		t.Fatal("the same note again changed the line")
	}
	if b.SetNote("nope", "x") {
		t.Fatal("a note on no line")
	}
}

// TestMergeTakesEachFieldsLatest checks the merge of two devices' books field by field: the note written last on one and the review done last on the other both stay, and merging again changes nothing, on either side.
func TestMergeTakesEachFieldsLatest(t *testing.T) {
	clock := 0
	mac := bookOn("mac", &clock)
	mac.Toggle(Line{Video: "a", Start: time.Second, Original: "Hi", SavedAt: at(0)})
	phone := bookOn("phone", &clock)
	if !phone.Merge(mac.Lines, mac.Removed) || len(phone.Lines) != 1 || phone.Lines[0].ID != mac.Lines[0].ID {
		t.Fatalf("phone took %+v", phone.Lines)
	}
	id := mac.Lines[0].ID

	clock = 10
	phone.Grade("a", time.Second, true, at(10))
	clock = 20
	mac.SetNote(id, "from the mac")
	clock = 30
	phone.SetNote(id, "from the phone, later")
	clock = 40
	mac.Grade("a", time.Second, false, at(40))

	if !mac.Merge(phone.Lines, phone.Removed) {
		t.Fatal("the mac took nothing")
	}
	if !phone.Merge(mac.Lines, mac.Removed) {
		t.Fatal("the phone took nothing")
	}
	for name, b := range map[string]Book{"mac": mac, "phone": phone} {
		l := b.Lines[0]
		if l.Note != "from the phone, later" || l.Box != 0 || !l.Due.Equal(at(40)) {
			t.Fatalf("%s has %+v", name, l)
		}
	}
	if mac.Merge(phone.Lines, phone.Removed) || phone.Merge(mac.Lines, mac.Removed) {
		t.Fatal("merging again changed a book")
	}
}

// TestMergeKeepsOneOfALineSavedTwice checks that a line both devices saved on their own becomes one line on both, under the smaller of the two ids, with the other id removed, so a third merge finds nothing to do.
func TestMergeKeepsOneOfALineSavedTwice(t *testing.T) {
	clock := 0
	mac := bookOn("mac", &clock)
	phone := bookOn("phone", &clock)
	mac.Toggle(Line{ID: "b-mac", Video: "a", Start: time.Second, Original: "Hi", SavedAt: at(0)})
	phone.Toggle(Line{ID: "a-phone", Video: "a", Start: time.Second, Original: "Hi", Translation: "你好", SavedAt: at(1)})

	mac.Merge(phone.Lines, phone.Removed)
	phone.Merge(mac.Lines, mac.Removed)
	for name, b := range map[string]Book{"mac": mac, "phone": phone} {
		if len(b.Lines) != 1 || b.Lines[0].ID != "a-phone" || b.Lines[0].Translation != "你好" || !b.Lines[0].SavedAt.Equal(at(0)) {
			t.Fatalf("%s has %+v", name, b.Lines)
		}
		if !slices.ContainsFunc(b.Removed, func(r Removed) bool { return r.ID == "b-mac" }) {
			t.Fatalf("%s removed %+v", name, b.Removed)
		}
	}
	if mac.Merge(phone.Lines, phone.Removed) || phone.Merge(mac.Lines, mac.Removed) {
		t.Fatal("merging again changed a book")
	}
}

// TestMergeRemovesLines checks that a line removed on one device goes on the other, edits made to it there notwithstanding, and does not come back from a book that still has it; a line saved again after its removal is a line of its own, which stays.
func TestMergeRemovesLines(t *testing.T) {
	clock := 0
	mac := bookOn("mac", &clock)
	mac.Toggle(Line{Video: "a", Start: time.Second, SavedAt: at(0)})
	phone := bookOn("phone", &clock)
	phone.Merge(mac.Lines, mac.Removed)
	stale := slices.Clone(phone.Lines)

	clock = 5
	mac.Remove("a", time.Second)
	clock = 6
	phone.SetNote(phone.Lines[0].ID, "edited after")
	if !phone.Merge(mac.Lines, mac.Removed) || len(phone.Lines) != 0 {
		t.Fatalf("the phone kept %+v", phone.Lines)
	}
	if mac.Merge(stale, nil) || len(mac.Lines) != 0 {
		t.Fatalf("a removed line came back: %+v", mac.Lines)
	}

	clock = 7
	mac.Toggle(Line{Video: "a", Start: time.Second, SavedAt: at(7)})
	if !phone.Merge(mac.Lines, mac.Removed) || len(phone.Lines) != 1 || !phone.Lines[0].SavedAt.Equal(at(7)) {
		t.Fatalf("the line saved again: %+v", phone.Lines)
	}
}
