package study

import (
	"slices"
	"strings"
	"time"
)

// StudyDay is how long films must have played on a day, on all devices together, for the day to count as studied in a streak.
const StudyDay = time.Minute

// Stats is what the book says of the learner's study at a moment (Book.Stats).
type Stats struct {
	// Streak is how many days in a row up to today were studied, today counting once it is; until then the run up to yesterday still counts. Longest is the longest such run ever.
	Streak, Longest int
	// Week is how long films played in the last seven days, today with them, and LastWeek in the seven before.
	Week, LastWeek time.Duration
	// Days is the last seven days, the earliest first, today last.
	Days [7]DayTime
	// Words and Lines are how many words and lines the book has, WordsDue and LinesDue how many of them are due for review.
	Words, WordsDue, Lines, LinesDue int
	// Mastered, Learning and New count the words and the lines together by how well they are known: from MasteredBox, in a box between, and in box 0.
	Mastered, Learning, New int
	// Devices is how many devices added to the book: saved a line, added a word or played a film.
	Devices int
}

// DayTime is how long films played on Day, on the book's device (Here) and on the others (Elsewhere).
type DayTime struct {
	Day             string
	Here, Elsewhere time.Duration
}

// Stats is the book's study as of now, the days counted in now's location.
func (b Book) Stats(now time.Time) Stats {
	var s Stats
	perDay := map[string]time.Duration{}
	devices := map[string]bool{}
	for _, w := range b.Watched {
		perDay[w.Day] += w.Time
		devices[w.Device] = true
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	dayAt := func(back int) string { return Day(today.AddDate(0, 0, -back)) }
	for i := range s.Days {
		d := dayAt(6 - i)
		s.Days[i].Day = d
		for _, w := range b.Watched {
			if w.Day != d {
				continue
			}
			if w.Device == b.Device {
				s.Days[i].Here += w.Time
			} else {
				s.Days[i].Elsewhere += w.Time
			}
		}
		s.Week += perDay[d]
	}
	for back := 7; back < 14; back++ {
		s.LastWeek += perDay[dayAt(back)]
	}
	studied := func(d string) bool { return perDay[d] >= StudyDay }
	from := 0
	if !studied(dayAt(0)) {
		from = 1
	}
	for back := from; studied(dayAt(back)); back++ {
		s.Streak++
	}
	// The longest run goes over the days watched in order, b.Watched being sorted by day.
	run, last := 0, time.Time{}
	for _, w := range b.Watched {
		if !studied(w.Day) {
			continue
		}
		d, err := time.ParseInLocation(time.DateOnly, w.Day, now.Location())
		if err != nil || d.Equal(last) {
			continue
		}
		if !last.IsZero() && last.AddDate(0, 0, 1).Equal(d) {
			run++
		} else {
			run = 1
		}
		last = d
		s.Longest = max(s.Longest, run)
	}
	s.Longest = max(s.Longest, s.Streak)
	know := func(box int) {
		switch {
		case box >= MasteredBox:
			s.Mastered++
		case box > 0:
			s.Learning++
		default:
			s.New++
		}
	}
	for _, w := range b.Words {
		s.Words++
		if !w.Due.After(now) {
			s.WordsDue++
		}
		know(w.Box)
		devices[w.Device] = true
	}
	for _, l := range b.Lines {
		s.Lines++
		if !l.Due.After(now) {
			s.LinesDue++
		}
		know(l.Box)
		devices[l.Device] = true
	}
	delete(devices, "")
	s.Devices = len(devices)
	return s
}

// StatsOf is the study of several books together as of now, as of the libraries the learner can open: a day's time is all of theirs, so a day counts in a streak once their minutes together make StudyDay, and the words and lines are all of theirs; device is this device, whose time is a day's Here. The books are left as they are.
func StatsOf(now time.Time, device string, books ...Book) Stats {
	all := Book{Device: device}
	for _, b := range books {
		all.Lines = append(all.Lines, b.Lines...)
		all.Words = append(all.Words, b.Words...)
		all.Watched = append(all.Watched, b.Watched...)
	}
	// Stats goes over the days watched in order for the longest run.
	slices.SortStableFunc(all.Watched, func(a, b Watched) int { return strings.Compare(a.Day, b.Day) })
	return all.Stats(now)
}
