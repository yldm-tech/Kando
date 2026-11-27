package session

import (
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// What plays after a film (sync.Film.Next), as its computer goes on from it: a course's next lesson, a disc's next track, a playlist's next video; the work the computer folds it into (sync.Film.Work); and whether it was watched there and when it last played (sync.Film.Watched, PlayedAt). The phone keeps them with each film it keeps, from every listing the computer gives, so a film played away from any network still knows what comes after it, as the next one is downloaded later than it was, and the Library folds the films it keeps as their computer last did.

// keepListed writes, on the films the phone keeps of computer desktop, what listing says of each that the phone keeps no other way (listedOf), where that changed; on a film of a NAS or a server matched to one of the computer's films, on the computer's film it keeps (store.Film.Lines), which the Library lists it as away from the computer. What else the phone took of a film, its title and when its files changed among it, stays as it was when the phone took its files.
func (s *Session) keepListed(desktop string, listing catalog.Listing) {
	listed := map[string]sync.Film{}
	for _, lib := range listing.Libraries {
		for _, f := range lib.Films {
			listed[f.ID] = f
		}
	}
	for _, f := range s.films.GetUntracked() {
		changed := false
		if l, ok := listed[f.Film.ID]; ok && f.Desktop == desktop {
			changed = listedOf(&f.Film, l)
		}
		if f.Lines != nil && f.Lines.Desktop == desktop {
			if l, ok := listed[f.Lines.Film.ID]; ok {
				lines := *f.Lines
				if listedOf(&lines.Film, l) {
					f.Lines, changed = &lines, true
				}
			}
		}
		if changed {
			s.replace(f)
			s.saveFilm(f)
		}
	}
}

// listedOf sets on kept what listed, the same film as its computer lists it now, says of it that the phone keeps from every listing (keepListed), and reports whether any of it changed.
func listedOf(kept *sync.Film, listed sync.Film) bool {
	before := *kept
	kept.Next = listed.Next
	kept.Work, kept.WorkTitle, kept.WorkIndex = listed.Work, listed.WorkTitle, listed.WorkIndex
	kept.Watched, kept.PlayedAt = listed.Watched, listed.PlayedAt
	return kept.Next != before.Next || kept.Work != before.Work || kept.WorkTitle != before.WorkTitle || kept.WorkIndex != before.WorkIndex ||
		kept.Watched != before.Watched || !kept.PlayedAt.Equal(before.PlayedAt)
}

// Next is the key of the film that plays after the film under key, as its computer last named it, and whether the phone keeps that film with its video, which it can play then; "" for a film with none. A film of a source matched to a computer's film goes on as that film does, and a next film matched to a film of a source plays as that one (PlayKey), from the phone, its source or its computer (matched.go).
func (s *Session) Next(key string) (string, bool) {
	f, ok := s.Film(key)
	lf := f.LinesFilm()
	if !ok || lf.Film.Next == "" {
		return "", false
	}
	next := s.PlayKey(store.Key(lf.Desktop, lf.Film.Next))
	kept, ok := s.Film(next)
	return next, ok && (kept.HasVideo() || kept.FromSource() || (kept.Streamed && s.StreamLink(next) != ""))
}
