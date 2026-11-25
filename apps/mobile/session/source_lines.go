package session

import (
	"context"
	"time"

	"kando/apps/mobile/download"
	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"
)

// The lines of a film of a source, from a computer (docs/media-sources.md 10.3, 10.5): the phone makes no subtitles itself, so a film it plays from a NAS or a server has none, unless a paired computer made a film of the same file. A computer names the file a film of its media sources was made of by its key (sync.Film.SourceKey, remotefs.SourceKey of where the file is and its size), which the phone computes alike for the file of its own source; once a computer lists a film of the same key (matchLines), the phone takes that film's subtitles as it takes a computer's film's by its lines alone, over the computer's network or out of its storage, and keeps them in that film's folder, films/<desktop>/<film>, which holds no film.json and so is no film of the Library. The film of the source keeps which film it took them from (store.Film.Lines), so it shows them where neither the computer nor the storage can be reached; the player, the word marks, the learning state, lookups and the glossary all go by that film (LinesFilm), and the two are one film from then on: the Library lists it once, as the computer's film, and the film of the source holds the place both were left at and plays the video of either (matched.go). A server named otherwise on the two devices, as nas.local on one and its address on the other, gives other keys, and the film keeps no lines, on purpose.

// linesFor is how long the phone waits for a computer, or its storage, to give the lines of a film of a source.
const linesFor = 30 * time.Second

// LinesFilm is the film the film kept under key takes its lines from, and whose computer it asks of them (store.Film.LinesFilm): the film itself, or a computer's film matched to a film of a source. Read it in a binding.
func (s *Session) LinesFilm(key string) (store.Film, bool) {
	f, ok := s.Film(key)
	return f.LinesFilm(), ok
}

// sourceKeyOf is the key of f's file, a film of a source (remotefs.SourceKey), by where its source is now and the size the file had as it was kept; "" for a source removed, or one remotefs cannot name.
func (s *Session) sourceKeyOf(f store.Film) string {
	for _, src := range s.src.list.GetUntracked() {
		if src.ID == f.Source {
			return remotefs.SourceKey(nas.Remote(src, nil), f.Path, f.Film.VideoSize)
		}
	}
	return ""
}

// matchLines gives each film of a source that has no lines yet those of the film a paired computer lists of the same file, the first computer's in the order they were paired, as the computers last listed them on their networks or through their storage (Shelf); a film being made is matched once it is done. It runs as the computers' shelves change and as a film of a source is kept. Engine goroutine only.
func (s *Session) matchLines() {
	var films []store.Film
	for _, f := range s.films.GetUntracked() {
		if f.FromSource() && f.Lines == nil {
			films = append(films, f)
		}
	}
	if len(films) == 0 {
		return
	}
	shelves := s.shelves.GetUntracked()
	listed := map[string]store.Lines{}
	for _, d := range s.desktops.GetUntracked() {
		for _, lib := range shelves[d.ID].Listing.Libraries {
			for _, f := range lib.Films {
				if _, dup := listed[f.SourceKey]; f.SourceKey != "" && !f.Making && !dup {
					listed[f.SourceKey] = store.Lines{Desktop: d.ID, Library: lib.ID, LibraryName: lib.Name, Film: f}
				}
			}
		}
	}
	if len(listed) == 0 {
		return
	}
	for _, f := range films {
		if l, ok := listed[s.sourceKeyOf(f)]; ok {
			s.takeLines(f.Key(), l, nil)
		}
	}
}

// linesSource is where the lines of film id of desktop come from: out of its storage while the phone reaches it through the storage and the storage holds the film (relayed), else the computer on its network; false for a computer not paired.
func (s *Session) linesSource(desktop, id string) (src download.Source, relayed, ok bool) {
	if s.shelves.GetUntracked()[desktop].Relayed {
		if rf, there := s.remote.shelf.GetUntracked()[desktop]; there {
			if _, listed := rf.films.Films[id]; listed {
				return rf.films, true, true
			}
		}
	}
	c := s.Client(desktop)
	return c, false, c != nil
}

// takeLines has the film of a source kept under key take the lines of l, a computer's film of the same file: they are read off the engine's goroutine and kept in l's folder, and then the film keeps l as what it took its lines from, and changed runs, as for a player showing the film. A film whose lines are being taken or brought up to date already (Follow) is left to that, with changed run once it is done.
func (s *Session) takeLines(key string, l store.Lines, changed func()) {
	if fw, running := s.following[key]; running {
		if changed != nil {
			fw.changed = append(fw.changed, changed)
		}
		return
	}
	src, relayed, ok := s.linesSource(l.Desktop, l.Film.ID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, linesFor)
	fw := &follow{cancel: cancel}
	if changed != nil {
		fw.changed = append(fw.changed, changed)
	}
	s.following[key] = fw
	s.work.Go(func() {
		defer cancel()
		cues, err := src.Subtitles(ctx, l.Film.ID)
		if err == nil {
			err = s.st.SaveCues(l.Desktop, l.Film.ID, cues)
		}
		s.cx.Post(func() { s.tookLines(key, l, fw, err, relayed) })
	})
}

// tookLines is the end of takeLines, on the engine's goroutine: the film of the source, as it stands now, keeps l as what it took its lines from, and asks for their word marks and its learning state; a film deleted meanwhile leaves no lines behind it. Lines that could not be read through the storage (relayed) or from the computer on its network are asked for again once the phone reaches the computer the other way, as when it left the computer's network while it asked, and the computer's shelf changed meanwhile.
func (s *Session) tookLines(key string, l store.Lines, fw *follow, err error, relayed bool) {
	if s.following[key] == fw {
		delete(s.following, key)
	}
	cur, ok := s.Film(key)
	if !ok {
		s.dropLines(l, key)
		return
	}
	if err != nil {
		if s.shelves.GetUntracked()[l.Desktop].Relayed != relayed {
			s.matchLines()
		}
		return
	}
	cur.Lines = &l
	// Where the computer's film was left on the phone before they were matched is where both go on from, when it is the later (matched.go).
	cur = s.sharedPosition(cur)
	s.replace(cur)
	s.saveFilm(cur)
	s.OpenMarks(key)
	s.OpenStudy(key)
	for _, fn := range fw.changed {
		fn()
	}
}

// followLines brings the lines of f, a film of a source, kept under key, up to date with the computer's film it took them from (store.Film.Lines), as Follow does a computer's film on its network: lines the computer changed since, or that are no longer on the phone, as the computer's film kept here was deleted, are taken anew. A film that has no lines yet looks among the computers' films (matchLines).
func (s *Session) followLines(key string, f store.Film, changed func()) {
	if f.Lines == nil {
		s.matchLines()
	}
	if fw, running := s.following[key]; running {
		fw.changed = append(fw.changed, changed)
		return
	}
	if f.Lines == nil {
		return
	}
	lf := f.LinesFilm()
	c := s.Client(lf.Desktop)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), followFor)
	fw := &follow{cancel: cancel, changed: []func(){changed}}
	s.following[key] = fw
	s.work.Go(func() {
		defer cancel()
		if _, kept, _ := s.st.Cues(lf.Desktop, lf.Film.ID); !kept {
			// Lines no longer on the phone are taken again, whenever the computer changed them.
			lf.Film.Updated = time.Time{}
		}
		listed, cues, updated, _ := download.Follow(ctx, c, lf)
		var err error
		if updated {
			err = s.st.SaveCues(lf.Desktop, lf.Film.ID, cues)
		}
		s.cx.Post(func() {
			if !updated {
				if s.following[key] == fw {
					delete(s.following, key)
				}
				return
			}
			l := *f.Lines
			if cur, ok := s.Film(key); ok && cur.Lines != nil {
				l = *cur.Lines
			}
			l.Film = listed
			s.tookLines(key, l, fw, err, false)
		})
	})
}

// dropLines removes the folder of l's lines, which the film under key, a film of a source no longer kept, took them into, unless another film still keeps them: the computer's film kept on the phone itself, or another film of a source that took the same lines.
func (s *Session) dropLines(l store.Lines, key string) {
	for _, f := range s.films.GetUntracked() {
		if f.Key() == l.Key() || f.Key() != key && f.Lines != nil && f.Lines.Key() == l.Key() {
			return
		}
	}
	s.work.Go(func() { _ = s.st.DeleteFilm(l.Desktop, l.Film.ID) })
}
