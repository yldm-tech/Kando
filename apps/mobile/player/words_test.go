package player

import "testing"

// A breakdown written a word a line becomes rows, blank lines left out; one with prose in it shows as it is.
func TestBreakdownItems(t *testing.T) {
	if got := breakdownItems("how · 怎样\n\nare you · 你好吗"); len(got) != 2 || got[1] != (breakdownItem{"are you", "你好吗"}) {
		t.Fatalf("a breakdown of two words became %+v", got)
	}
	if got := breakdownItems("how · 怎样\nThe rest is plain."); got != nil {
		t.Fatalf("a breakdown with prose in it became %+v", got)
	}
}
