package main

import (
	"time"

	"kando/apps/kando/live"
)

// libraryFollowDay keeps the list's 今天 and 昨天 (films.listDate) true as the day changes: it arms a timer for the next midnight, which lets every binding follow (Changed) and arms the next one. A timer counts no time the computer sleeps, so one armed before a night asleep fires late; libraryHooks' becameActive arms it again, and its watchLifecycle lets the bindings follow, so a day that changed meanwhile shows when the app comes to the front.
func (a *app) libraryFollowDay() {
	if a.lib.dayStop != nil {
		a.lib.dayStop()
	}
	now := time.Now()
	a.lib.dayStop = a.cx.AfterFunc(libraryNextDay(now).Sub(now), live.Do(a, a.libraryFollowDay))
}

// libraryNextDay is when the day after now's starts, in now's time zone: its midnight, or 01:00 where the clocks skip midnight that day, as Chile's do when summer time starts, for time.Date gives a midnight that does not exist as 23:00 the day before, which would arm the timer over and over until then. A timer that fired a moment before it gets the same time again, a moment away.
func libraryNextDay(now time.Time) time.Time {
	y, m, d := now.Date()
	next := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
	if _, _, day := next.Date(); day == d {
		next = time.Date(y, m, d+1, 1, 0, 0, 0, now.Location())
	}
	return next
}
