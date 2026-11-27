package catalog

import (
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/sync"
)

// episode is film id of computer d, the index-th of River Walk, work w1.
func episode(d, id string, index int) Entry {
	return Entry{Desktop: d, Film: sync.Film{ID: id, Title: "River Walk " + id, DurationMS: 60_000, Work: "w1", WorkTitle: "River Walk", WorkIndex: index}}
}

func rowKeys(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Work != nil {
			ids := []string{}
			for _, e := range r.Work.Films {
				ids = append(ids, e.Film.ID)
			}
			out = append(out, "work:"+r.Work.Title+":"+strings.Join(ids, ","))
			continue
		}
		out = append(out, r.Film.ID)
	}
	return out
}

// The list folds a work's films into one row at the place of its first, in the order the work plays; a work of one film shown, a film of no work, and the same work's ID on another computer are rows of their own.
func TestFoldFoldsAWorksFilmsAtTheFirstsPlace(t *testing.T) {
	entries := []Entry{
		{Desktop: "d1", Film: sync.Film{ID: "talk", Title: "Morning Talk"}},
		episode("d1", "e3", 3),
		episode("d1", "e1", 1),
		episode("d2", "e1", 1),
		episode("d1", "e2", 2),
	}
	got := rowKeys(Fold(Search(entries, nil, "")))
	if want := []string{"talk", "work:River Walk:e1,e2,e3", "e1"}; !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if r := Fold(Search(entries[:2], nil, "")); len(r) != 2 || r[1].Work != nil {
		t.Fatalf("a work of one film shown is %+v", r)
	}
	rows := Fold(Search(entries, nil, ""))
	if rows[1].Key() == rows[2].Key() || rows[1].Key() == entries[1].Key() {
		t.Fatalf("keys %q %q", rows[1].Key(), rows[2].Key())
	}
}

// A search folds the films it found by their work's name; a film it found by its own title or by a line of it is a row of its own, so the page shows which it found.
func TestASearchFoldsOnlyTheFilmsFoundByTheirWork(t *testing.T) {
	entries := []Entry{episode("d1", "e1", 1), episode("d1", "e2", 2), episode("d1", "e3", 3)}
	entries[0].Film.Title, entries[1].Film.Title, entries[2].Film.Title = "Harbor Dawn", "Harbor Dusk", "Lighthouse"
	if got := rowKeys(Fold(Search(entries, nil, "river"))); !slices.Equal(got, []string{"work:River Walk:e1,e2,e3"}) {
		t.Fatalf("a search for the work finds %v", got)
	}
	if got := rowKeys(Fold(Search(entries, nil, "harbor"))); !slices.Equal(got, []string{"e1", "e2"}) {
		t.Fatalf("a search for two films' titles finds %v", got)
	}
	lines := map[string][]sync.Cue{entries[0].Key(): {{StartMS: 4000, Original: "the tide comes in"}}, entries[2].Key(): {{Original: "the tide goes out"}}}
	got := Fold(Search(entries, lines, "tide"))
	if len(got) != 2 || got[0].Work != nil || got[0].Hit == nil || got[1].Hit == nil {
		t.Fatalf("a search for lines finds %v", rowKeys(got))
	}
}

// A work goes on as its computer's library does: from the film played last, left partway; after the one watched last, a film watched on the computer counting as watched until a device leaves it elsewhere; from the first when none played, and again once all were watched.
func TestGoOnGoesOnWithTheWorkWhereItWasLeft(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 11, 27, 9, m, 0, 0, time.UTC) }
	w := Work{Films: []Entry{episode("d1", "e1", 1), episode("d1", "e2", 2), episode("d1", "e3", 3)}}
	if e, how := GoOn(w); e.Film.ID != "e1" || how != GoOnStart || Unwatched(w) != 3 {
		t.Fatalf("none played: %s %d", e.Film.ID, how)
	}
	// e1 marked watched on the computer without ever playing (zero PlayedAt): goes on with e2; all marked watched starts again.
	w.Films[0].Film.Watched = true
	if e, how := GoOn(w); e.Film.ID != "e2" || how != GoOnNext || Unwatched(w) != 2 {
		t.Fatalf("marked watched without played_at: %s %d", e.Film.ID, how)
	}
	w.Films[1].Film.Watched, w.Films[2].Film.Watched = true, true
	if e, how := GoOn(w); e.Film.ID != "e1" || how != GoOnAgain || Unwatched(w) != 0 {
		t.Fatalf("all marked watched without played_at: %s %d", e.Film.ID, how)
	}
	w.Films[0].Film.Watched, w.Films[1].Film.Watched, w.Films[2].Film.Watched = false, false, false
	// e1 watched on the computer, e2 left partway on the phone before it.
	w.Films[0].Film.Watched, w.Films[0].Film.PlayedAt = true, at(10)
	w.Films[1].Position = Position{At: 20 * time.Second, When: at(5), Mine: true}
	if e, how := GoOn(w); e.Film.ID != "e2" || how != GoOnNext || Unwatched(w) != 2 {
		t.Fatalf("after the computer's: %s %d", e.Film.ID, how)
	}
	// The phone leaves e1 partway after the computer watched it: it goes on there.
	w.Films[0].Position = Position{At: 10 * time.Second, When: at(20), Mine: true}
	if e, how := GoOn(w); e.Film.ID != "e1" || how != GoOnResume || Watched(w.Films[0]) {
		t.Fatalf("left again: %s %d", e.Film.ID, how)
	}
	// The phone watches e3 to its end; e1 and e2 are not watched, so it goes on with the first of them.
	w.Films[2].Position = Position{At: 59 * time.Second, When: at(30), Mine: true}
	if e, how := GoOn(w); e.Film.ID != "e1" || how != GoOnNext {
		t.Fatalf("after the last: %s %d", e.Film.ID, how)
	}
	for i := range w.Films {
		w.Films[i].Position, w.Films[i].Film.Watched, w.Films[i].Film.PlayedAt = Position{}, true, at(40+i)
	}
	if e, how := GoOn(w); e.Film.ID != "e1" || how != GoOnAgain || Unwatched(w) != 0 {
		t.Fatalf("all watched: %s %d", e.Film.ID, how)
	}
}

// A work's page lists every film of the work, in its order, named as most of them name it.
func TestWorkOfListsTheWorksFilms(t *testing.T) {
	entries := []Entry{episode("d1", "e2", 2), {Desktop: "d1", Film: sync.Film{ID: "talk"}}, episode("d1", "e1", 1), episode("d2", "e1", 1)}
	entries[0].Film.WorkTitle = "River Walk (old)"
	entries = append(entries, episode("d1", "e3", 3))
	w, ok := WorkOf(entries, "d1", "w1")
	if !ok || w.Title != "River Walk" || len(w.Films) != 3 || w.Films[0].Film.ID != "e1" || w.Films[2].Film.ID != "e3" {
		t.Fatalf("work %+v, %v", w, ok)
	}
	if _, ok := WorkOf(entries, "d1", "none"); ok {
		t.Fatal("a work of no films")
	}
	// On a title count tie, the newer listing's title wins.
	older := episode("d1", "e1", 1)
	older.Film.WorkTitle, older.Film.Updated = "Old Name", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	newer := episode("d1", "e2", 2)
	newer.Film.WorkTitle, newer.Film.Updated = "New Name", time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	if tied, ok := WorkOf([]Entry{older, newer}, "d1", "w1"); !ok || tied.Title != "New Name" {
		t.Fatalf("tied titles picked %q", tied.Title)
	}
}
