package session

import (
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/store"
)

// What plays after a film (sync.Film.Next), as its computer goes on from it: a course's next lesson, a disc's next track, a playlist's next video. The phone keeps it with each film it keeps, from every listing the computer gives, so a film played away from any network still knows what comes after it, as the next one is downloaded later than it was.

// keepNexts writes, on the films the phone keeps of computer desktop, what plays after each as listing names it, where that changed.
func (s *Session) keepNexts(desktop string, listing catalog.Listing) {
	next := map[string]string{}
	listed := map[string]bool{}
	for _, lib := range listing.Libraries {
		for _, f := range lib.Films {
			listed[f.ID], next[f.ID] = true, f.Next
		}
	}
	for _, f := range s.films.GetUntracked() {
		if f.Desktop != desktop || !listed[f.Film.ID] || f.Film.Next == next[f.Film.ID] {
			continue
		}
		f.Film.Next = next[f.Film.ID]
		s.replace(f)
		s.saveFilm(f)
	}
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
	return next, ok && (kept.HasVideo() || kept.FromSource())
}
