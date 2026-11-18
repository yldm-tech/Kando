package study

import (
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
