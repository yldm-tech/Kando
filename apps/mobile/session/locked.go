package session

import (
	"maps"
	"slices"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/store"
)

// Locked libraries on the phone: a password-protected library a computer lists as locked for the phone (catalog.Load, sync.WithLocked) serves none of its films, but the films the phone downloaded or kept from it before it was locked stay on the phone. While their library is locked they are hidden everywhere the phone shows films: the Library's list, Continue Watching and search (Entries), Settings › Offline Films (Offline), the words and lines of Learn (LearnItems) and the widgets that read them, and the player, which closes (Hidden). Nothing of them is deleted: once the library is unlocked (UnlockLibrary), they show again.
//
// The phone keeps which libraries each computer last listed as locked (store.Locked), so they stay hidden after the app starts again away from the computer, and after the computer is unpaired, as its films stay. Only a computer that answers itself tells it: a listing through the storage (relay_films.go) holds no locked library, as the computer puts none of their films there, and leaves what the phone kept.

// takeLocked keeps the libraries listing, which computer desktop answered itself, names as locked, in place of those it named before.
func (s *Session) takeLocked(desktop string, listing catalog.Listing) {
	var now []store.LockedLibrary
	for _, lib := range listing.Libraries {
		if lib.Locked {
			now = append(now, store.LockedLibrary{ID: lib.ID, Name: lib.Name})
		}
	}
	was := s.locked.GetUntracked()
	if slices.Equal(was[desktop], now) {
		return
	}
	m := maps.Clone(was)
	if m == nil {
		m = map[string][]store.LockedLibrary{}
	}
	if len(now) == 0 {
		delete(m, desktop)
	} else {
		m[desktop] = now
	}
	s.locked.Set(m)
	s.saveLocked(m)
}

// saveLocked writes m off the engine's goroutine, unless a later one is written already.
func (s *Session) saveLocked(m map[string][]store.LockedLibrary) {
	s.lockedMu.Lock()
	s.lockedSaving++
	gen := s.lockedSaving
	s.lockedMu.Unlock()
	s.work.Go(func() {
		s.lockedMu.Lock()
		defer s.lockedMu.Unlock()
		if gen <= s.lockedSaved {
			return
		}
		if s.st.SaveLocked(m) == nil {
			s.lockedSaved = gen
		}
	})
}

// LibraryLocked reports whether computer desktop last listed its library id, named name, as locked for the phone; a library the phone knows by name alone (id "") is matched by its name. Read it in a binding.
func (s *Session) LibraryLocked(desktop, id, name string) bool {
	return slices.ContainsFunc(s.locked.Get()[desktop], func(l store.LockedLibrary) bool {
		if id != "" && l.ID != "" {
			return l.ID == id
		}
		return name != "" && l.Name == name
	})
}

// hidden reports whether f is a film of a locked library: its own, or for a film of a source matched to a computer's film, that film's library (store.Film.Lines), whose title, poster and lines it shows. Read it in a binding.
func (s *Session) hidden(f store.Film) bool {
	if l := f.Lines; l != nil && s.LibraryLocked(l.Desktop, l.Library, l.LibraryName) {
		return true
	}
	return !f.FromSource() && s.LibraryLocked(f.Desktop, f.LibraryID, f.Library)
}

// Hidden reports whether the film kept under key is of a locked library, which the phone shows nothing of until it is unlocked. Read it in a binding.
func (s *Session) Hidden(key string) bool {
	f, ok := s.Film(key)
	return ok && s.hidden(f)
}

// shown are the films kept on the phone but those of a locked library. Read it in a binding.
func (s *Session) shown() []store.Film {
	all := s.films.Get()
	if len(s.locked.Get()) == 0 {
		return all
	}
	return slices.DeleteFunc(slices.Clone(all), s.hidden)
}
