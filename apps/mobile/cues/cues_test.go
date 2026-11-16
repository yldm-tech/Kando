package cues

import (
	"testing"
	"time"

	"kando/packages/sync"
)

func sec(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

var film = FromSync([]sync.Cue{
	{StartMS: 1000, EndMS: 2000, Original: "one"},
	{StartMS: 3000, EndMS: 4500, Original: "two"},
	{StartMS: 5000, EndMS: 0, Original: "three"},
	{StartMS: 7000, EndMS: 0, Original: "four"},
})

func TestIndexFindsTheLineShowing(t *testing.T) {
	for _, c := range []struct {
		at   float64
		want int
	}{{0, -1}, {1, 0}, {1.9, 0}, {2, -1}, {3.2, 1}, {4.6, -1}, {6, 2}, {7, 3}, {100, 3}} {
		if got := Index(film, sec(c.at)); got != c.want {
			t.Errorf("Index at %vs = %d, want %d", c.at, got, c.want)
		}
	}
}

func TestSpanEndsALineWithoutAnEndAtTheNext(t *testing.T) {
	for _, c := range []struct {
		i          int
		start, end float64
	}{{0, 1, 2}, {2, 5, 7}, {3, 7, 9}} {
		if s, e := Span(film, c.i, sec(9)); s != sec(c.start) || e != sec(c.end) {
			t.Errorf("Span(%d) = %v-%v, want %vs-%vs", c.i, s, e, c.start, c.end)
		}
	}
}

func TestPrevAndNextStepFromTheLinePlaying(t *testing.T) {
	for _, c := range []struct {
		at         float64
		prev, next int
	}{{0, -1, 0}, {1.5, -1, 1}, {2.5, 0, 1}, {3.5, 0, 2}, {6, 1, 3}, {8, 2, -1}} {
		if p, n := Prev(film, sec(c.at)), Next(film, sec(c.at)); p != c.prev || n != c.next {
			t.Errorf("at %vs: Prev %d Next %d, want %d %d", c.at, p, n, c.prev, c.next)
		}
	}
}

func TestLoopBackAtTheLinesEndOrBeforeIt(t *testing.T) {
	for _, c := range []struct {
		i    int
		at   float64
		want bool
	}{{1, 3, false}, {1, 4.4, false}, {1, 4.5, true}, {1, 2.8, false}, {1, 2, true}, {2, 6.9, false}, {2, 7, true}, {3, 8.9, false}, {3, 9, true}} {
		if got := LoopBack(film, c.i, sec(c.at), sec(9)); got != c.want {
			t.Errorf("LoopBack(%d) at %vs = %v, want %v", c.i, c.at, got, c.want)
		}
	}
	if LoopBack(film, 3, sec(100), 0) {
		t.Error("the last line of a film of unknown length looped back")
	}
}
