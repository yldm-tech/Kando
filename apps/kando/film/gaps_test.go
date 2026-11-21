package film

import (
	"slices"
	"testing"
	"time"

	"kando/packages/core"
)

// TestGapsAreTheRunsOfCellsNotHeard cuts a film of 2 min 40 s, heard in its first and fourth half minute and given up on in its third: the second half minute and the last two are still to come, the third failed; one of unknown length has gaps up to the last cell known only, and a cell with a line heard in it is no gap.
func TestGapsAreTheRunsOfCellsNotHeard(t *testing.T) {
	s := func(n int) time.Duration { return time.Duration(n) * time.Second }
	heard, givenUp := map[int]bool{0: true, 3: true}, map[int]bool{2: true}
	got := Gaps(heard, givenUp, nil, s(160))
	want := []Gap{{Start: s(30), End: s(60)}, {Start: s(60), End: s(90), Failed: true}, {Start: s(120), End: s(160)}}
	if !slices.Equal(got, want) {
		t.Fatalf("gaps %+v, want %+v", got, want)
	}
	if got, want := Gaps(heard, givenUp, nil, 0), want[:2]; !slices.Equal(got, want) {
		t.Fatalf("of unknown length, gaps %+v, want %+v", got, want)
	}
	if got := Gaps(map[int]bool{0: true, 1: true}, nil, nil, s(60)); len(got) != 0 {
		t.Fatalf("a film heard whole has gaps %+v", got)
	}
	// A cell being heard shows its lines before it is heard whole: it is no gap.
	lines := []core.Segment{{Start: s(125), End: s(127)}}
	if got, want := Gaps(heard, givenUp, lines, s(160)), append(want[:2:2], Gap{Start: s(150), End: s(160)}); !slices.Equal(got, want) {
		t.Fatalf("with a line heard at 2:05, gaps %+v, want %+v", got, want)
	}
}
