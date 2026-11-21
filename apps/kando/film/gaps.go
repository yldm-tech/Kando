package film

import (
	"maps"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// Gap is a part of a film being made that has no lines yet, as its speech is not heard yet, or that the speech recognition gave up on (Failed). The subtitle list shows it as a row of its own, so a film made from where it was opened does not read as one whose lines go missing between the parts made.
type Gap struct {
	Start, End time.Duration
	Failed     bool
}

// Gaps are the parts of a film length long that are neither heard nor given up, and those given up, each run of cells one gap, from heard and givenUp, its cells as track.Doc.Heard tells them. A cell heard silent is no gap: it has no lines to come; nor is one any of lines falls in, by its middle, as a cell being heard shows its lines as they are heard, before it is heard whole. A film whose length is not known yet (0) has gaps only up to the last cell heard or given up.
func Gaps(heard, givenUp map[int]bool, lines []core.Segment, length time.Duration) []Gap {
	heard = maps.Clone(heard)
	if heard == nil {
		heard = map[int]bool{}
	}
	for _, l := range lines {
		heard[int((l.Start+l.End)/2/track.Cell)] = true
	}
	cells := int((length + track.Cell - 1) / track.Cell)
	if length <= 0 {
		for c := range heard {
			cells = max(cells, c+1)
		}
		for c := range givenUp {
			cells = max(cells, c+1)
		}
	}
	var out []Gap
	for c := range cells {
		if heard[c] {
			continue
		}
		start, end := time.Duration(c)*track.Cell, time.Duration(c+1)*track.Cell
		if length > 0 {
			end = min(end, length)
		}
		if n := len(out); n > 0 && out[n-1].End == start && out[n-1].Failed == givenUp[c] {
			out[n-1].End = end
			continue
		}
		out = append(out, Gap{Start: start, End: end, Failed: givenUp[c]})
	}
	return out
}

// gapAt is the index of the gap of gaps at, or -1.
func gapAt(gaps []Gap, at time.Duration) int {
	for i, g := range gaps {
		if at >= g.Start && at < g.End {
			return i
		}
	}
	return -1
}
