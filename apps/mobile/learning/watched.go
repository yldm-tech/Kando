package learning

import (
	"time"

	"kando/packages/study"
)

// GoalMinutes are the daily goals the Learn tab offers, in minutes of films played, and DefaultGoal the one it starts with.
var GoalMinutes = []int{10, 20, 30, 60}

// DefaultGoal is the daily goal until the learner picks one.
const DefaultGoal = 20

// Watched is how long films played on the phone each day, by the day in the phone's calendar (study.Day).
type Watched map[string]time.Duration

// Add adds d, time a film played, to the day of at.
func (w Watched) Add(at time.Time, d time.Duration) {
	if d > 0 {
		w[study.Day(at)] += d
	}
}

// Day is how long films played on the day of at.
func (w Watched) Day(at time.Time) time.Duration { return w[study.Day(at)] }

// Week is the last seven days up to the day of now, the earliest first, each how long films played on it.
func (w Watched) Week(now time.Time) [7]time.Duration {
	var out [7]time.Duration
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	for i := range out {
		out[i] = w[study.Day(noon.AddDate(0, 0, i-6))]
	}
	return out
}

// WeekDays are the days Week counts, the earliest first, at noon in now's location.
func WeekDays(now time.Time) [7]time.Time {
	var out [7]time.Time
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	for i := range out {
		out[i] = noon.AddDate(0, 0, i-6)
	}
	return out
}

// Streak is how many days in a row up to the day of now films played at least study.StudyDay, today counting once it has; until then the run up to yesterday still counts, as the computer counts it.
func (w Watched) Streak(now time.Time) int {
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	studied := func(back int) bool { return w[study.Day(noon.AddDate(0, 0, -back))] >= study.StudyDay }
	from, n := 0, 0
	if !studied(0) {
		from = 1
	}
	for back := from; studied(back); back++ {
		n++
	}
	return n
}
