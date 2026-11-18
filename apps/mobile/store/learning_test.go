package store

import (
	"fmt"
	"testing"
	"time"

	"kando/apps/mobile/study"
	"kando/packages/sync"
)

func TestTheLearningStateIsKeptByLibrary(t *testing.T) {
	st := Open(t.TempDir())
	if l, err := st.Learning("desk", "lib"); err != nil || l.Epoch != "" || len(l.Pending) != 0 {
		t.Fatalf("before any: %+v, %v", l, err)
	}
	l := study.Change(study.Library{Epoch: "e", Rev: 2}, study.Save("a", "f1", "Talk", study.Cue{Start: time.Second, Original: "Hi."}, "", time.Unix(1_700_000_000, 0).UTC()))
	if err := st.SaveLearning("desk", "lib/../x", l); err != nil {
		t.Fatal(err)
	}
	got, err := Open(st.dir).Learning("desk", "lib/../x")
	if err != nil || got.Rev != 2 || len(study.Lines(got, "f1")) != 1 {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if other, _ := st.Learning("desk", "x"); len(other.Pending) != 0 {
		t.Fatal("an ID with a path in it reached another library's file")
	}
	if err := st.SaveLearning("other/desk", "lib", study.Library{Epoch: "o"}); err != nil {
		t.Fatal(err)
	}
	all, err := st.Learnings()
	if err != nil || len(all) != 2 || all[LearningKey{"desk", "lib/../x"}].Rev != 2 || all[LearningKey{"other/desk", "lib"}].Epoch != "o" {
		t.Fatalf("all of them: %+v, %v", all, err)
	}
	if err := st.RemoveLearning("desk", "lib/../x"); err != nil {
		t.Fatal(err)
	}
	if all, _ := st.Learnings(); len(all) != 1 {
		t.Fatalf("after one was removed: %+v", all)
	}
}

func TestLookupsKeepTheLatest(t *testing.T) {
	st := Open(t.TempDir())
	m := map[string]Lookup{}
	t0 := time.Unix(1_700_000_000, 0).UTC()
	for i := range MaxLookups + 2 {
		m[fmt.Sprint(i)] = Lookup{Entry: sync.WordEntry{Word: fmt.Sprint(i)}, At: t0.Add(time.Duration(i) * time.Second)}
	}
	if err := st.SaveLookups(m); err != nil {
		t.Fatal(err)
	}
	got, err := st.Lookups()
	if err != nil || len(got) != MaxLookups {
		t.Fatalf("kept %d, %v", len(got), err)
	}
	if _, ok := got["0"]; ok {
		t.Fatal("the oldest lookup was kept")
	}
	if LookupKey(" River ", "EN", "a line") != LookupKey("river", "en", "a line ") {
		t.Fatal("LookupKey tells one word apart from itself")
	}
}
