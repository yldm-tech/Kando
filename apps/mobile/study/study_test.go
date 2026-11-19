package study

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/sync"
)

var t0 = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

var hello = Cue{Start: time.Second, End: 2 * time.Second, Original: "Hello.", Translation: "你好。"}

func TestALineSavedShowsAtOnceAndGoesOnceTaken(t *testing.T) {
	var l Library
	rec := Save("a", "f1", "Talk", hello, "", t0)
	l = Change(l, rec)
	got := Lines(l, "f1")
	if len(got) != 1 || got[0].ID != "a" || got[0].Original != "Hello." || got[0].Start != time.Second || !got[0].SavedAt.Equal(t0) {
		t.Fatalf("lines %+v", got)
	}
	if _, ok := At(l, "f1", time.Second+20*time.Millisecond); !ok {
		t.Fatal("At did not find the line by its start")
	}
	if len(Lines(l, "f2")) != 0 {
		t.Fatal("another film's lines hold it")
	}
	// A note written before it was sent goes with it.
	l = Change(l, Note(l, "a", "f1", "  greeting ", t0.Add(time.Second)))
	if len(l.Pending) != 1 || Lines(l, "f1")[0].Note != "greeting" {
		t.Fatalf("pending %+v", l.Pending)
	}
	sent := l.Pending
	merged := clone(sent[0])
	merged.Rev = 3
	l = Took(l, sent, sync.StatePutResult{Epoch: "e", Rev: 3, Records: []sync.Record{merged}}, false)
	if len(l.Pending) != 0 || len(l.Records) != 1 || l.Records[0].Rev != 3 || Lines(l, "f1")[0].Note != "greeting" {
		t.Fatalf("after the put: %+v", l)
	}
	// The note changed again is made from the computer's.
	n := Note(l, "a", "f1", "hi", t0.Add(time.Minute))
	if b := n.Fields[sync.FieldNote].Base; b == nil || !b.At.Equal(t0.Add(time.Second)) {
		t.Fatalf("base %+v", b)
	}
	l = Change(l, n)
	l = Change(l, Remove("a", "f1", t0.Add(2*time.Minute)))
	if len(Lines(l, "f1")) != 0 || len(l.Pending) != 1 || l.Pending[0].Removed == nil || l.Pending[0].Fields != nil {
		t.Fatalf("removed: %+v", l.Pending)
	}
}

func TestALineTakenBackBeforeItWasSentGoesWithoutAWord(t *testing.T) {
	l := Change(Library{}, Save("a", "f1", "Talk", hello, "", t0))
	l = Change(l, Remove("a", "f1", t0.Add(time.Second)))
	if len(l.Pending) != 0 || len(Lines(l, "f1")) != 0 {
		t.Fatalf("pending %+v", l.Pending)
	}
}

func TestAChangeMadeWhileAPutRanStaysPending(t *testing.T) {
	l := Change(Library{}, Save("a", "f1", "Talk", hello, "", t0))
	sent := l.Pending
	l = Change(l, Note(l, "a", "f1", "later", t0.Add(time.Second)))
	l = Took(l, sent, sync.StatePutResult{Records: []sync.Record{sent[0]}}, false)
	if len(l.Pending) != 1 || Lines(l, "f1")[0].Note != "later" {
		t.Fatalf("pending %+v", l.Pending)
	}
	// A put refused for good drops what it sent.
	l = Took(l, l.Pending, sync.StatePutResult{}, true)
	if len(l.Pending) != 0 {
		t.Fatalf("refused: %+v", l.Pending)
	}
}

func TestApplyTakesInTheComputersChanges(t *testing.T) {
	desk := Save("d", "f1", "Talk", Cue{Start: 3 * time.Second, Original: "Bye."}, "", t0)
	l := Change(Library{}, Save("a", "f1", "Talk", hello, "", t0))
	l = Apply(l, sync.StateChanges{Epoch: "e", Rev: 5, Reset: true, Records: []sync.Record{desk},
		Conflicts: []sync.Conflict{{ID: "c1", Kind: sync.KindLine, Record: "d", Field: sync.FieldNote}}})
	if l.Epoch != "e" || l.Rev != 5 || len(Lines(l, "f1")) != 2 || len(l.Conflicts) != 1 {
		t.Fatalf("reset: %+v", l)
	}
	gone := sync.Record{Kind: sync.KindLine, ID: "d", Film: "f1", Removed: &sync.Stamp{At: t0}}
	l = Apply(l, sync.StateChanges{Epoch: "e", Rev: 6, Records: []sync.Record{gone}, Conflicts: []sync.Conflict{{ID: "c1", Resolved: &sync.Stamp{At: t0}}}})
	if got := Lines(l, "f1"); len(got) != 1 || got[0].ID != "a" || len(l.Conflicts) != 0 || l.Rev != 6 {
		t.Fatalf("changes: %+v, %+v", got, l)
	}
}

// A save handed to the computer and taken back before it answered goes to it as a tombstone, and the line stays taken back once the computer's copy of it comes back; one never handed goes without a word.
func TestALineTakenBackOnceSentTellsTheComputer(t *testing.T) {
	var l Library
	l = Change(l, Save("a", "f1", "Talk", hello, "", t0))
	sent := slices.Clone(l.Pending)
	l = Sending(l, sent)
	l = Change(l, Remove("a", "f1", t0.Add(time.Second)))
	if len(l.Pending) != 1 || l.Pending[0].Removed == nil || len(Lines(l, "f1")) != 0 {
		t.Fatalf("pending %+v", l.Pending)
	}
	desk := clone(sent[0])
	desk.Rev = 1
	l = Took(l, sent, sync.StatePutResult{Epoch: "e", Rev: 1, Records: []sync.Record{desk}}, false)
	if len(l.Pending) != 1 || l.Pending[0].Removed == nil || len(Lines(l, "f1")) != 0 || len(l.Sent) != 1 {
		t.Fatalf("after the put: %+v", l)
	}
	l = Took(l, slices.Clone(l.Pending), sync.StatePutResult{}, false)
	if len(l.Pending) != 0 || len(l.Sent) != 0 {
		t.Fatalf("after the tombstone's put: %+v", l)
	}

	var never Library
	never = Change(never, Save("b", "f1", "Talk", hello, "", t0))
	never = Change(never, Remove("b", "f1", t0.Add(time.Second)))
	if len(never.Pending) != 0 {
		t.Fatalf("a save never sent left %+v", never.Pending)
	}
}

// A refused change is dropped alone: the others stay pending.
func TestARefusedChangeIsDroppedAlone(t *testing.T) {
	var l Library
	l = Change(l, Save("a", "f1", "Talk", hello, "", t0))
	l = Change(l, Save("b", "gone", "Gone", hello, "", t0))
	l = Sending(l, l.Pending)
	l = Took(l, []sync.Record{l.Pending[1]}, sync.StatePutResult{}, true)
	if len(l.Pending) != 1 || l.Pending[0].ID != "a" || len(l.Sent) != 1 || l.Sent[0].ID != "a" {
		t.Fatalf("after the refusal: %+v", l)
	}
}

// Batches keep each put within the limit, in order, a record longer than it alone.
func TestBatchesKeepEachPutWithinTheLimit(t *testing.T) {
	var records []sync.Record
	for i := range 10 {
		records = append(records, Save(fmt.Sprint(i), "f1", "Talk", hello, strings.Repeat("n", 100), t0))
	}
	big := Save("big", "f1", "Talk", hello, strings.Repeat("n", 5000), t0)
	records = append(records[:5], append([]sync.Record{big}, records[5:]...)...)
	one, _ := json.Marshal(records[0])
	limit := 3 * (len(one) + 1)
	batches := Batches(records, limit)
	var got []sync.Record
	for _, b := range batches {
		data, _ := json.Marshal(b)
		if len(b) > 1 && len(data) > limit+1 {
			t.Fatalf("a batch of %d takes %d bytes, over %d", len(b), len(data), limit)
		}
		got = append(got, b...)
	}
	if len(got) != len(records) || !slices.EqualFunc(got, records, func(a, b sync.Record) bool { return a.ID == b.ID }) {
		t.Fatalf("batches %d records", len(got))
	}
	if !slices.ContainsFunc(batches, func(b []sync.Record) bool { return len(b) == 1 && b[0].ID == "big" }) {
		t.Fatal("the long record is not alone")
	}
}

func TestUnpendTakesBackOnlyTheKindAsked(t *testing.T) {
	l := Change(Library{}, Save("a", "f1", "Talk", hello, "", t0))
	l = Change(l, sync.Record{Kind: sync.KindWatched, ID: sync.WatchedID("p1", "2026-11-18"), Fields: map[string]sync.Field{sync.FieldWatchedMS: {Value: sync.Value(1000), Stamp: sync.Stamp{At: t0}}}})
	got := Unpend(l, sync.KindWatched)
	if len(got.Pending) != 1 || got.Pending[0].Kind != sync.KindLine || len(l.Pending) != 2 {
		t.Fatalf("pending %+v, the library it came from %+v", got.Pending, l.Pending)
	}
}

func TestAWordAddedIsAWordRecordDueAtOnceAndGoesWhenTakenBack(t *testing.T) {
	e := sync.WordEntry{Word: "go", Reading: "ɡoʊ", Meaning: "去", Language: "zh-Hans"}
	r := AddWord("w", "f1", "Talk", "en", "went", e, hello, t0)
	if r.Kind != sync.KindWord || r.Film != "f1" {
		t.Fatalf("record %+v", r)
	}
	text := func(name string) string {
		var s string
		_ = json.Unmarshal(r.Fields[name].Value, &s)
		return s
	}
	if text(sync.FieldWord) != "go" || text(sync.FieldSpoken) != "en" || text(sync.FieldMeaning) != "去" || text(sync.FieldContext) != "Hello." || text(sync.FieldContextTranslation) != "你好。" || text(sync.FieldTitle) != "Talk" {
		t.Fatalf("fields %+v", r.Fields)
	}
	var rv sync.Review
	_ = json.Unmarshal(r.Fields[sync.FieldReview].Value, &rv)
	if rv.Box != 0 || !rv.Due.Equal(t0) {
		t.Fatalf("review %+v", rv)
	}
	// Without a dictionary form the word is as tapped.
	if r := AddWord("w2", "f1", "Talk", "en", " went ", sync.WordEntry{}, hello, t0); !strings.Contains(string(r.Fields[sync.FieldWord].Value), `"went"`) {
		t.Fatalf("word %s", r.Fields[sync.FieldWord].Value)
	}
	l := Change(Library{}, r)
	if _, ok := View(l, sync.KindWord, "w"); !ok {
		t.Fatal("the word added does not show")
	}
	l = Change(l, RemoveWord("w", "f1", t0.Add(time.Second)))
	if _, ok := View(l, sync.KindWord, "w"); ok || len(l.Pending) != 0 {
		t.Fatalf("taken back before it was sent: %+v", l.Pending)
	}
}

// A line's translation fixed on the phone is a correction of the machine's line it shows as, or of the correction the computer has of it when one moved it there; the phone reads its fixes back by where their lines show.
func TestCorrectAndCorrections(t *testing.T) {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	desk := sync.Stamp{At: now.Add(-time.Hour), Device: "mac"}
	moved := sync.CorrectionID("f1", 4000, 6000)
	l := Library{Records: []sync.Record{{Kind: sync.KindCorrection, ID: moved, Film: "f1", Fields: map[string]sync.Field{
		sync.FieldStartShiftMS: {Value: sync.Value(500), Stamp: desk}, sync.FieldEndShiftMS: {Value: sync.Value(0), Stamp: desk},
		sync.FieldTranslation: {Value: sync.Value("旧的"), Stamp: desk}, sync.FieldOriginal: {Value: sync.Null, Stamp: desk},
	}}}}

	fresh := Correct(l, "f1", Cue{Start: time.Second, End: 2 * time.Second, Original: "Hi.", Translation: "嗨。"}, " 你好。 ", now)
	if fresh.ID != sync.CorrectionID("f1", 1000, 2000) || sync.Text(fresh.Fields[sync.FieldTranslation].Value) != "你好。" || fresh.Fields[sync.FieldTranslation].Base != nil {
		t.Fatalf("a fresh correction %+v", fresh)
	}
	again := Correct(l, "f1", Cue{Start: 4500 * time.Millisecond, End: 6 * time.Second}, "新的", now)
	if again.ID != moved || again.Fields[sync.FieldTranslation].Base == nil || !again.Fields[sync.FieldTranslation].Base.Equal(desk) {
		t.Fatalf("a correction of a moved line %+v", again)
	}
	l = Change(Change(l, fresh), again)
	got := Corrections(l, "f1")
	if len(got) != 2 {
		t.Fatalf("corrections %+v", got)
	}
	c, ok := CorrectionAt(l, "f1", 4500*time.Millisecond, 6*time.Second)
	if !ok || c.Translation == nil || *c.Translation != "新的" || c.Original != nil {
		t.Fatalf("the moved line's correction %+v, %v", c, ok)
	}
	if c, ok := CorrectionAt(l, "f1", time.Second, 2*time.Second); !ok || *c.Translation != "你好。" {
		t.Fatalf("the fresh correction %+v, %v", c, ok)
	}
	if len(Corrections(l, "f2")) != 0 {
		t.Fatal("another film has corrections")
	}
}
