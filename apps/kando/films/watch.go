package films

import (
	"time"

	"kando/apps/kando/sidebar"
)

// Progress is what is known of how far each video was watched, by its path, which a film's watch state is worked out from. App has these reads, as does the main package's app.
type Progress interface {
	// Duration is how long the video at path plays, 0 until it is known.
	Duration(path string) time.Duration
	// ResumeAt is where the video at path stopped playing last time, or 0.
	ResumeAt(path string) time.Duration
	// Furthest is how far the video at path has played on from its start, 0 when it never did.
	Furthest(path string) time.Duration
}

// watchSlack is how near either end of a film a point counts as that end: a film stopped within it of its start was not started, one played to within it of its end was watched, as ResumeFrom counts the end.
const watchSlack = 3 * time.Second

// WatchState is how far a film has been watched, as the sidebar filters by it, one of the sidebar's Watch values other than WatchAll:
//   - WatchWatched once it was played to within 3 seconds of its end (watchedOf), as the check over its poster says; not before its length is known, as no check shows then either.
//   - WatchInProgress when it was not, but was played on for 3 seconds or more, or stopped where 继续 picks it up (ResumeFrom), as the progress bar under its poster or the hero says.
//   - WatchUnwatched otherwise, a film opened and closed at its start included, though it has a time it last played.
func WatchState(p Progress, item Film) string {
	if watchedOf(p, item).end {
		return sidebar.WatchWatched
	}
	if p.Furthest(item.VideoPath) >= watchSlack {
		return sidebar.WatchInProgress
	}
	if _, ok := ResumeFrom(p, item); ok {
		return sidebar.WatchInProgress
	}
	return sidebar.WatchUnwatched
}

// ByWatch keeps the films in state (WatchState); sidebar.WatchAll keeps them all.
func ByWatch(items []Film, p Progress, state string) []Film {
	if state == sidebar.WatchAll {
		return items
	}
	var out []Film
	for _, item := range items {
		if WatchState(p, item) == state {
			out = append(out, item)
		}
	}
	return out
}

// WatchCounts is how many of items are in each watch state (WatchState).
func WatchCounts(items []Film, p Progress) sidebar.WatchCounts {
	var n sidebar.WatchCounts
	for _, item := range items {
		switch WatchState(p, item) {
		case sidebar.WatchUnwatched:
			n.Unwatched++
		case sidebar.WatchInProgress:
			n.InProgress++
		case sidebar.WatchWatched:
			n.Watched++
		}
	}
	return n
}
