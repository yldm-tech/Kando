package study

import (
	"testing"
	"time"
)

// day is midnight UTC of November d, 2026, and a minute past it.
func day(d int) time.Time { return time.Date(2026, 11, d, 0, 1, 0, 0, time.UTC) }

// TestWatchAddsUpPerDayAndDevice checks that time watched adds up on the day it was watched, on this device's entry of that day, and that nothing or a negative time adds nothing.
func TestWatchAddsUpPerDayAndDevice(t *testing.T) {
	b := Book{Device: "mac"}
	if !b.Watch(day(16), 30*time.Second) || !b.Watch(day(16).Add(time.Hour), 45*time.Second) || !b.Watch(day(17), time.Minute) {
		t.Fatal("Watch reported no change")
	}
	if b.Watch(day(17), 0) || b.Watch(day(17), -time.Second) {
		t.Fatal("nothing watched changed the book")
	}
	want := []Watched{{Day: "2026-11-16", Device: "mac", Time: 75 * time.Second}, {Day: "2026-11-17", Device: "mac", Time: time.Minute}}
	if len(b.Watched) != 2 || b.Watched[0] != want[0] || b.Watched[1] != want[1] {
		t.Fatalf("watched %+v", b.Watched)
	}
}

// TestMergeWatchedKeepsEachDevicesLongest checks that merging two devices' time watched keeps each device's own day, the longer of two copies of it, as a device only ever adds to its own, and that merging again changes nothing.
func TestMergeWatchedKeepsEachDevicesLongest(t *testing.T) {
	mac, phone := Book{Device: "mac"}, Book{Device: "phone"}
	mac.Watch(day(16), 10*time.Minute)
	phone.Watch(day(16), 5*time.Minute)
	if !phone.MergeWatched(mac.Watched) || !mac.MergeWatched(phone.Watched) {
		t.Fatal("a merge took nothing")
	}
	mac.Watch(day(16), 2*time.Minute)
	stale := []Watched{{Day: "2026-11-16", Device: "mac", Time: 10 * time.Minute}}
	if !phone.MergeWatched(mac.Watched) || phone.MergeWatched(stale) {
		t.Fatal("the later copy did not win")
	}
	if phone.Stats(day(16)).Days[6].Here != 5*time.Minute || phone.Stats(day(16)).Days[6].Elsewhere != 12*time.Minute {
		t.Fatalf("phone watched %+v", phone.Watched)
	}
	if mac.MergeWatched(phone.Watched) || phone.MergeWatched(mac.Watched) {
		t.Fatal("merging again changed a book")
	}
}

// TestStats checks what the stats dialog shows: the days in a row studied, a day counting once a minute was watched on it on any device, and the longest such run; the time watched in the last seven days and the seven before, each day's on this device and on the others; how many words and lines are due; and how well the words and lines are known.
func TestStats(t *testing.T) {
	b := Book{Device: "mac", Now: func() time.Time { return day(16) }}
	watch := func(device string, d int, m float64) {
		b.MergeWatched([]Watched{{Day: Day(day(d)), Device: device, Time: time.Duration(m * float64(time.Minute))}})
	}
	for d := 1; d <= 6; d++ {
		watch("mac", d, 10)
	}
	watch("mac", 12, 5)
	watch("mac", 13, 0.5)
	watch("mac", 14, 10)
	watch("mac", 15, 20)
	watch("mac", 16, 30)
	watch("phone", 16, 15)
	watch("phone", 10, 40)

	b.AddWord(Word{Text: "known"})
	b.AddWord(Word{Text: "learning"})
	b.AddWord(Word{Text: "new"})
	b.Words[b.WordIndex("known", "")].Box, b.Words[b.WordIndex("known", "")].Due = MasteredBox, day(30)
	b.Words[b.WordIndex("learning", "")].Box, b.Words[b.WordIndex("learning", "")].Due = 2, day(16).Add(-time.Hour)
	b.Toggle(Line{Video: "a", Start: time.Second, SavedAt: day(16)})
	b.Toggle(Line{Video: "a", Start: 2 * time.Second, SavedAt: day(16)})
	b.Lines[0].Box, b.Lines[0].Due = 1, day(20)

	s := b.Stats(day(16))
	if s.Streak != 3 || s.Longest != 6 {
		t.Fatalf("streak %d, longest %d", s.Streak, s.Longest)
	}
	if s.Week != 120*time.Minute+30*time.Second || s.LastWeek != 40*time.Minute {
		t.Fatalf("week %v, last week %v", s.Week, s.LastWeek)
	}
	if s.Days[0].Day != "2026-11-10" || s.Days[0].Here != 0 || s.Days[0].Elsewhere != 40*time.Minute || s.Days[6].Day != "2026-11-16" || s.Days[6].Here != 30*time.Minute || s.Days[6].Elsewhere != 15*time.Minute {
		t.Fatalf("days %+v", s.Days)
	}
	if s.Words != 3 || s.WordsDue != 2 || s.Lines != 2 || s.LinesDue != 1 {
		t.Fatalf("words %d (%d due), lines %d (%d due)", s.Words, s.WordsDue, s.Lines, s.LinesDue)
	}
	if s.Mastered != 1 || s.Learning != 2 || s.New != 2 {
		t.Fatalf("mastered %d, learning %d, new %d", s.Mastered, s.Learning, s.New)
	}
	if s.Devices != 2 {
		t.Fatalf("devices %d", s.Devices)
	}
	if s := b.Stats(day(17)); s.Streak != 3 {
		t.Fatalf("a day not studied yet broke the streak: %d", s.Streak)
	}
	if s := b.Stats(day(18)); s.Streak != 0 || s.Longest != 6 {
		t.Fatalf("a day missed: streak %d, longest %d", s.Streak, s.Longest)
	}
}
