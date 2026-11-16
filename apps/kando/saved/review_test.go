package saved

import (
	"slices"
	"testing"
	"time"
)

// TestGradeFollowsTheBoxes checks the schedule: a known line goes up a box each time and comes back a day, then 3, 7, 14, 30 and 60 days later, and stays at 60; a line not known goes back to box 0, due at once.
func TestGradeFollowsTheBoxes(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	var b Book
	b.Toggle(Line{Video: "a", Start: time.Second, SavedAt: now})
	days := []int{1, 3, 7, 14, 30, 60, 60}
	for i, d := range days {
		if !b.Grade("a", time.Second, true, now) {
			t.Fatal("no line to grade")
		}
		if l := b.Lines[0]; l.Due != now.Add(time.Duration(d)*24*time.Hour) || l.Box != min(i+1, len(Intervals)) {
			t.Fatalf("after %d Goods: box %d due %v, want %d days", i+1, l.Box, l.Due, d)
		}
	}
	b.Grade("a", time.Second, false, now)
	if l := b.Lines[0]; l.Box != 0 || !l.Due.Equal(now) {
		t.Fatalf("after Again: box %d due %v", l.Box, l.Due)
	}
	if b.Grade("b", time.Second, true, now) {
		t.Fatal("graded a line that is not saved")
	}
}

// TestDueListsTheLinesDue checks that only the lines due by now are reviewed, the one due longest first, and the new ones in the book's order.
func TestDueListsTheLinesDue(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	b := Book{Lines: []Line{
		{Video: "new2", Due: now},
		{Video: "new1", Due: now},
		{Video: "later", Due: now.Add(time.Hour)},
		{Video: "old", Due: now.Add(-48 * time.Hour)},
	}}
	if got := b.Due(now); !slices.Equal(got, []int{3, 0, 1}) {
		t.Fatalf("due %v", got)
	}
}
