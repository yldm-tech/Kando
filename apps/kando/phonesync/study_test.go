package phonesync

import (
	"testing"
	"time"

	psync "kando/packages/sync"
)

// The line explained goes with at most before lines before it, trimmed, and never one after it.
func TestExplainLinesLeaveOutWhatComesAfter(t *testing.T) {
	req := psync.LineExplain{Index: 2, Lines: []psync.Cue{
		{StartMS: 0, Original: "One."}, {StartMS: 1000, Original: " Two. ", Translation: " 二。 "}, {StartMS: 2000, Original: "Three."}, {StartMS: 3000, Original: "Four."}}}
	before, line := ExplainLines(req, 1)
	if len(before) != 1 || before[0].Original != "Two." || before[0].Translation != "二。" || before[0].Start != time.Second {
		t.Fatalf("before %+v", before)
	}
	if line.Original != "Three." || line.Start != 2*time.Second {
		t.Fatalf("line %+v", line)
	}
	if before, _ := ExplainLines(psync.LineExplain{Lines: req.Lines}, 3); len(before) != 0 {
		t.Fatalf("the first line went with %+v", before)
	}
}
