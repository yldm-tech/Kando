package catalog

import (
	"cmp"
	"slices"
	"time"
)

// Works: the Library shows the films of a work, a show's episodes, a course's lessons, a playlist that is one, as one row, as their computer folds them into one card (sync.Film.Work). The phone cuts no work itself: it takes the computer's, which a film the phone keeps remembers from the computer's last listing (session.keepListed), so the phone folds them away from any network too.
//
//   - The list folds the films the chip and the search let through (Fold), as the computer's library page folds after its filters: a work with one film let through is that film's row. A search folds the films it found by their work's name; a film found by its own title or a line of it stays a row of its own, so the films it found show.
//   - A work's row opens the work's page, which lists every film of the work the phone lists (Work.Films), whatever the chip or the search.
//   - Watching goes by the latest of the computer's and the devices' (Watched, PlayedAt): a film watched to its end on the computer is watched unless a device left it elsewhere since.

// Work is the films of one work of a computer the phone lists, in the order the work plays.
type Work struct {
	Desktop, ID, Title string
	Films              []Entry
}

// Key names the work's row, apart from every film's (Entry.Key), which a work's never is: a film's key has no NUL in it.
func (w Work) Key() string { return "work\x00" + w.Desktop + "\x00" + w.ID }

// workKey is what tells e's work from the others: its computer and the work's ID, "" for a film of none.
func workKey(e Entry) string {
	if e.Film.Work == "" {
		return ""
	}
	return e.Desktop + "\x00" + e.Film.Work
}

// WorkOf is the work of the film of computer desktop whose work is id, made of those of entries in it, false for none.
func WorkOf(entries []Entry, desktop, id string) (Work, bool) {
	w := Work{Desktop: desktop, ID: id}
	for _, e := range entries {
		if e.Desktop == desktop && e.Film.Work == id && id != "" {
			w.Films = append(w.Films, e)
		}
	}
	if len(w.Films) == 0 {
		return Work{}, false
	}
	return ordered(w), true
}

// ordered is w with its films in the order the work plays, and the title most of them name it by, the latest listing's first on a tie, as a film kept from an older listing may name it otherwise.
func ordered(w Work) Work {
	slices.SortStableFunc(w.Films, func(a, b Entry) int { return cmp.Compare(a.Film.WorkIndex, b.Film.WorkIndex) })
	count := map[string]int{}
	latest := map[string]time.Time{}
	for _, e := range w.Films {
		t := e.Film.WorkTitle
		if t == "" {
			continue
		}
		count[t]++
		if e.Film.Updated.After(latest[t]) {
			latest[t] = e.Film.Updated
		}
		if w.Title == "" || count[t] > count[w.Title] || count[t] == count[w.Title] && latest[t].After(latest[w.Title]) {
			w.Title = t
		}
	}
	return w
}

// Row is a row of the Library's list: a film the search found, or, with Work set, a work's films folded into one, the Result then being the first of them the list showed.
type Row struct {
	Result
	Work *Work
}

// Key names the row: its work's, or its film's.
func (r Row) Key() string {
	if r.Work != nil {
		return r.Work.Key()
	}
	return r.Result.Key()
}

// Fold is results as the list shows them: the films of each work folded into one row, at the place of the first of them, with the films of the work among results in the order it plays; a work with one film among them, and every film of a search found otherwise than by its work's name (Result.ByWork), is a row of its own.
func Fold(results []Result) []Row {
	groups := map[string][]Entry{}
	for _, r := range results {
		if k := workKey(r.Entry); k != "" && r.foldable() {
			groups[k] = append(groups[k], r.Entry)
		}
	}
	var out []Row
	placed := map[string]bool{}
	for _, r := range results {
		k := workKey(r.Entry)
		if k == "" || !r.foldable() || len(groups[k]) < 2 {
			out = append(out, Row{Result: r})
			continue
		}
		if placed[k] {
			continue
		}
		placed[k] = true
		w := ordered(Work{Desktop: r.Desktop, ID: r.Film.Work, Films: groups[k]})
		out = append(out, Row{Result: r, Work: &w})
	}
	return out
}

// foldable reports whether r folds into its work's row: a film the list shows without a search, or one a search found by its work's name.
func (r Result) foldable() bool { return r.Hit == nil && (r.ByWork || !r.searched) }

// Watched reports whether e's film was watched to its end: on its computer, unless a device left it elsewhere since, or by where it was left last, within resumeMargin of its end.
func Watched(e Entry) bool {
	if e.Film.Watched && !e.Position.When.After(e.Film.PlayedAt) {
		return true
	}
	length := time.Duration(e.Film.DurationMS) * time.Millisecond
	return length > 0 && e.Position.At > 0 && e.Position.At >= length-resumeMargin
}

// PlayedAt is when e's film last played on any device, zero for never: on its computer, or where it was left last, whichever is later.
func PlayedAt(e Entry) time.Time {
	if e.Position.When.After(e.Film.PlayedAt) {
		return e.Position.When
	}
	return e.Film.PlayedAt
}

// The ways a work goes on (GoOn): from the film played last, left partway; with the film after the one watched to its end last; from its first film, none having played; or from its first again, every one being watched.
const (
	GoOnResume = iota
	GoOnNext
	GoOnStart
	GoOnAgain
)

// GoOn is the film of w a work goes on with, and how (GoOnResume and the others), as its computer's library goes on with it: the film played last when it was not watched to its end; else the first after it not watched; else the first not watched; else its first, again.
func GoOn(w Work) (Entry, int) {
	if len(w.Films) == 0 {
		return Entry{}, GoOnStart
	}
	last, at := -1, time.Time{}
	for i, e := range w.Films {
		if played := PlayedAt(e); !played.IsZero() && played.After(at) {
			last, at = i, played
		}
	}
	if last < 0 {
		anyWatched := false
		for _, e := range w.Films {
			if Watched(e) {
				anyWatched = true
				break
			}
		}
		if !anyWatched {
			return w.Films[0], GoOnStart
		}
		for _, e := range w.Films {
			if !Watched(e) {
				return e, GoOnNext
			}
		}
		return w.Films[0], GoOnAgain
	}
	if !Watched(w.Films[last]) {
		return w.Films[last], GoOnResume
	}
	for _, e := range slices.Concat(w.Films[last+1:], w.Films[:last]) {
		if !Watched(e) {
			return e, GoOnNext
		}
	}
	return w.Films[0], GoOnAgain
}

// Unwatched is how many of w's films were not watched to their end (Watched).
func Unwatched(w Work) int {
	n := 0
	for _, e := range w.Films {
		if !Watched(e) {
			n++
		}
	}
	return n
}
