package track

import (
	"maps"
	"slices"
	"testing"
)

// heardDoc is a document of events stamped one after another on one device, as Append would write them.
func heardDoc(events ...Event) Doc {
	for i := range events {
		events[i].Device, events[i].Seq, events[i].At = "mac", int64(i+1), int64(i+1)
	}
	return Doc{Events: events}
}

// TestHeardSaysWhichCellsAreHeard hears cell 0 in one language and cell 3 in another, and fails cell 1 as many times as a unit is tried and cell 2 once: cells 0 and 3 are heard, cell 1 given up, and cell 2, still to be tried again, neither.
func TestHeardSaysWhichCellsAreHeard(t *testing.T) {
	hear := func(cell int, key string) Unit { return Unit{Work: WorkHear, Gen: "g", Cell: cell, Key: key} }
	events := []Event{hear(0, "ja").Made(), hear(3, "auto").Made(), hear(2, "ja").Failed("boom"), Unit{Work: WorkDraft, Gen: "g", Cell: 4}.Made(), hear(5, "ja").Made()}
	events[len(events)-1].Gen = "other"
	for range Attempts {
		events = append(events, hear(1, "ja").Failed("boom"))
	}
	heard, givenUp := heardDoc(events...).Heard("g")
	if got := slices.Sorted(maps.Keys(heard)); !slices.Equal(got, []int{0, 3}) {
		t.Fatalf("heard %v", got)
	}
	if got := slices.Sorted(maps.Keys(givenUp)); !slices.Equal(got, []int{1}) {
		t.Fatalf("given up %v", got)
	}
}
