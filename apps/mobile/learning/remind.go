package learning

import "time"

// ReminderHour is the hour of the day, in the phone's time, the review reminder shows at.
const ReminderHour = 21

// NextReminder is when to remind the learner to review items, after now: at ReminderHour on the first day something is due by then, today's if it is not past; false when nothing is to be reviewed.
func NextReminder(now time.Time, items []Item) (time.Time, bool) {
	var first time.Time
	for _, it := range items {
		if first.IsZero() || it.Due.Before(first) {
			first = it.Due
		}
	}
	if len(items) == 0 {
		return time.Time{}, false
	}
	from := now
	if first.After(now) {
		from = first.In(now.Location())
	}
	at := time.Date(from.Year(), from.Month(), from.Day(), ReminderHour, 0, 0, 0, now.Location())
	if !at.After(from) || !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}
