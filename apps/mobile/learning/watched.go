package learning

import (
	"time"

	phonestudy "kando/apps/mobile/study"
	"kando/packages/study"
	"kando/packages/sync"
)

// GoalMinutes are the daily goals the Learn tab offers, in minutes of films played, and DefaultGoal the one it starts with.
var GoalMinutes = []int{10, 20, 30, 60}

// DefaultGoal is the daily goal until the learner picks one.
const DefaultGoal = 20

// Watched is how long films played each day, by the day in the phone's calendar (study.Day): on the phone, as it keeps it, and with AddOthers on the computers and the other devices too.
type Watched map[string]time.Duration

// Add adds d, time a film played, to the day of at.
func (w Watched) Add(at time.Time, d time.Duration) {
	if d > 0 {
		w[study.Day(at)] += d
	}
}

// AddOthers adds the time watched of library l's learning state (sync.KindWatched) on every device but the phone, which mine says the IDs of: the computer's and other devices' time, which the phone's own (Add) does not count.
func (w Watched) AddOthers(l phonestudy.Library, mine func(device string) bool) {
	for _, id := range phonestudy.IDs(l, sync.KindWatched) {
		device, day, ok := sync.ParseWatchedID(id)
		if !ok || mine(device) {
			continue
		}
		if r, ok := phonestudy.View(l, sync.KindWatched, id); ok {
			if d := ms(r.Fields[sync.FieldWatchedMS]); d > 0 {
				w[day] += d
			}
		}
	}
}

// WatchedChange is the change of device's time watched in library l's learning state on the day of at, d more than l has of it: the phone's time watched there, which the computer adds to its stats.
func WatchedChange(l phonestudy.Library, device string, at time.Time, d time.Duration) sync.Record {
	id := sync.WatchedID(device, study.Day(at))
	var have time.Duration
	if r, ok := phonestudy.View(l, sync.KindWatched, id); ok {
		have = ms(r.Fields[sync.FieldWatchedMS])
	}
	return sync.Record{Kind: sync.KindWatched, ID: id, Fields: map[string]sync.Field{
		sync.FieldWatchedMS: {Value: sync.Value((have + d).Milliseconds()), Stamp: sync.Stamp{At: at, Device: device}},
	}}
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
