package study

import (
	"slices"
	"time"
)

// Intervals is the review schedule, a Leitner system of boxes: a line saved, or one the learner did not know, is in box 0 and due at once; each time it is known (Grade with good) it moves up a box and is due again the box's interval later, the first a day, then 3, 7, 14, 30 and 60 days, and it stays in the last box after that; a line not known goes back to box 0, due at once, so the review shows it again before it ends.
var Intervals = []time.Duration{24 * time.Hour, 3 * 24 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour, 30 * 24 * time.Hour, 60 * 24 * time.Hour}

// Due is the indexes of the lines due for review at now, the one due longest first; lines due at once keep the book's order, newest first.
func (b Book) Due(now time.Time) []int {
	var out []int
	for i, l := range b.Lines {
		if !l.Due.After(now) {
			out = append(out, i)
		}
	}
	slices.SortStableFunc(out, func(x, y int) int { return b.Lines[x].Due.Compare(b.Lines[y].Due) })
	return out
}

// Grade schedules the line of video that starts at start after a review at now (Intervals): good when the learner knew it, which moves it up a box, else back to box 0. The schedule is stamped at now, on the book's device. It reports whether there was such a line.
func (b *Book) Grade(video string, start time.Duration, good bool, now time.Time) bool {
	i := b.Find(video, start)
	if i < 0 {
		return false
	}
	l := &b.Lines[i]
	l.Stamps.Review = Stamp{At: now, Device: b.Device}
	if !good {
		l.Box = 0
		l.Due = now
		return true
	}
	l.Box = min(l.Box+1, len(Intervals))
	l.Due = now.Add(Intervals[l.Box-1])
	return true
}
