package study

import (
	"slices"
	"time"
)

// Watched is how long films played on one device on one day, Day in the device's own calendar. Only that device adds to it, and only ever adds, so of two copies the longer is the later (MergeWatched).
type Watched struct {
	Day    string
	Device string
	Time   time.Duration
}

// Day is the day of t in its own location, as Watched names it: "2026-11-16".
func Day(t time.Time) string { return t.Format(time.DateOnly) }

// Watch adds d, time a film played, to the book's device's time on the day of at, and reports whether it added any.
func (b *Book) Watch(at time.Time, d time.Duration) bool {
	if d <= 0 {
		return false
	}
	day := Day(at)
	if i := slices.IndexFunc(b.Watched, func(w Watched) bool { return w.Day == day && w.Device == b.Device }); i >= 0 {
		b.Watched[i].Time += d
		return true
	}
	b.insertWatched(Watched{Day: day, Device: b.Device, Time: d})
	return true
}

// MergeWatched takes another device's time watched into the book, each device's day the longer of the two copies of it, and reports whether the book changed. It is the same whichever book merges into which, and merging again changes nothing.
func (b *Book) MergeWatched(in []Watched) bool {
	changed := false
	for _, w := range in {
		if w.Day == "" || w.Time <= 0 {
			continue
		}
		i := slices.IndexFunc(b.Watched, func(have Watched) bool { return have.Day == w.Day && have.Device == w.Device })
		switch {
		case i < 0:
			b.insertWatched(w)
		case w.Time > b.Watched[i].Time:
			b.Watched[i].Time = w.Time
		default:
			continue
		}
		changed = true
	}
	return changed
}

// insertWatched puts w among the days in order, the earliest first, and a day's devices by their ids.
func (b *Book) insertWatched(w Watched) {
	at, _ := slices.BinarySearchFunc(b.Watched, w, func(have, w Watched) int {
		if have.Day != w.Day {
			if have.Day < w.Day {
				return -1
			}
			return 1
		}
		if have.Device < w.Device {
			return -1
		}
		return 1
	})
	b.Watched = slices.Insert(b.Watched, at, w)
}
