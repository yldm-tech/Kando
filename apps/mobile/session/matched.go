package session

import (
	"context"
	"path/filepath"
	"time"

	"kando/apps/mobile/store"
)

// A film of a NAS or a server matched to a paired computer's film of the same file (store.Film.Lines, source_lines.go) is one film with two ways to its video (docs/media-sources.md 10.5): the Library lists it once, as the computer's film (catalog.Merge), and the phone keeps up to two copies of it, the record of the film of the source, which always is, and the computer's film downloaded from the computer, which may be. The film of the source is the one the player opens (PlayKey): it holds where the film was left, which it sends to the computer as the computer's film's (savePosition), so whichever video plays goes on from the same place, and it plays, in this order, a video downloaded to the phone from either (VideoFile), the source's file streamed while the source can be read, and else the computer's video, on its network or out of its storage as a computer's film streams (StreamLink). Which copy's download a list shows is Copy's; DownloadCopy downloads from the source when it answers, else from the computer; DeleteCopies removes both copies and leaves the computer's film alone.

// matched is the film of a source the phone keeps that took its lines from the computer's film under key (store.Film.Lines), the first of them; false for none. Read it in a binding.
func (s *Session) matched(key string) (store.Film, bool) {
	for _, f := range s.films.Get() {
		if f.FromSource() && f.Lines != nil && f.Lines.Key() == key {
			return f, true
		}
	}
	return store.Film{}, false
}

// pair is the film of a source of the film under key, n, and the computer's film it took its lines from, c, where the phone keeps it (kept): for a film of a source matched to a computer's film, the film itself, and for that computer's film, the film of a source matched to it; false for a film of no such pair. Read it in a binding.
func (s *Session) pair(key string) (n, c store.Film, kept, ok bool) {
	if f, own := s.Film(key); own && f.FromSource() && f.Lines != nil {
		n = f
	} else if n, ok = s.matched(key); !ok {
		return store.Film{}, store.Film{}, false, false
	}
	c, kept = s.Film(n.Lines.Key())
	return n, c, kept, true
}

// PlayKey is the key the player opens the film under key with: for a computer's film matched to a film of a source on the phone, that film of the source, which plays either video and holds where both were left; any other film's own key. Read it in a binding.
func (s *Session) PlayKey(key string) string {
	if n, ok := s.matched(key); ok {
		return n.Key()
	}
	return key
}

// Matched reports whether the film under key is one of a pair of a film of a source and a computer's film of the same file (store.Film.Lines): either may be named. Read it in a binding.
func (s *Session) Matched(key string) bool {
	_, _, _, ok := s.pair(key)
	return ok
}

// Copy is the key of the phone's copy whose download state stands for the film under key: for a pair of a film of a source and a computer's film, the one whose video is on the phone, the film of the source's first, else the one whose download was asked for, else the film of the source, streamed; any other film's own key. Read it in a binding.
func (s *Session) Copy(key string) string {
	n, c, kept, ok := s.pair(key)
	switch {
	case !ok:
		return key
	case n.HasVideo():
		return n.Key()
	case kept && c.HasVideo():
		return c.Key()
	case !n.Streamed:
		return n.Key()
	case kept && !c.Done() && !c.Streamed:
		return c.Key()
	}
	return n.Key()
}

// VideoFile is the file on the phone the film under key plays: its own video once it is whole, or, for a film of a source matched to a computer's film, that film's video downloaded from the computer; "" for none. Read it in a binding.
func (s *Session) VideoFile(key string) string {
	f, ok := s.Film(key)
	if ok && !f.HasVideo() && f.FromSource() && f.Lines != nil {
		if c, kept := s.Film(f.Lines.Key()); kept && c.HasVideo() {
			f = c
		}
	}
	if !ok || !f.HasVideo() {
		return ""
	}
	return filepath.Join(s.st.FilmDir(f.Desktop, f.Film.ID), f.Video)
}

// DeleteCopies removes every copy the phone keeps of the film under key (Delete): for a pair of a film of a source and a computer's film, both, which leaves the computer's film on the computer, listed as any of its films is.
func (s *Session) DeleteCopies(key string) {
	n, c, kept, ok := s.pair(key)
	if !ok {
		s.Delete(key)
		return
	}
	s.Delete(n.Key())
	if kept {
		s.Delete(c.Key())
	}
}

// probeFor is how long DownloadCopy waits for a source to answer before it downloads from the computer instead.
const probeFor = 10 * time.Second

// DownloadCopy downloads the film under key, of a pair of a film of a source and a computer's film, to the phone: from the source when it answers and still has the file, as its film downloads (DownloadSource), else from the computer, on its network or out of its storage, as the computer's film downloads (Download). A film of no pair downloads as it is (Resume).
func (s *Session) DownloadCopy(key string) {
	n, _, _, ok := s.pair(key)
	if !ok {
		s.Resume(key)
		return
	}
	lf := n.LinesFilm()
	fromComputer := func() {
		f := lf.Film
		if listed, ok := s.Listed(lf.Key()); ok {
			f = listed.Film
		}
		s.Download(lf.Desktop, lf.Library, f)
	}
	src, ok := s.Source(n.Source)
	if !ok {
		fromComputer()
		return
	}
	if s.src.probing[n.Key()] {
		return
	}
	s.src.probing[n.Key()] = true
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, probeFor)
		defer cancel()
		r, err := s.reader(ctx, src)
		if err == nil {
			if _, err = r.Stat(ctx, n.Path); err != nil {
				s.dropReader(src.ID, r)
			}
		}
		s.cx.Post(func() {
			delete(s.src.probing, n.Key())
			cur, ok := s.Film(n.Key())
			switch {
			case !ok:
			case err == nil:
				s.start(cur)
			default:
				fromComputer()
			}
		})
	})
}

// sharedPosition is n, a film of a source matched to a computer's film, with where that film was left as the phone kept it, when the phone kept it later than n's: the place both share, from before they were matched.
func (s *Session) sharedPosition(n store.Film) store.Film {
	if n.Lines == nil {
		return n
	}
	if c, ok := s.Film(n.Lines.Key()); ok && c.PositionAt.After(n.PositionAt) {
		n.PositionMS, n.PositionAt = c.PositionMS, c.PositionAt
	}
	return n
}

// shareKeptPositions gives each film of a source matched to a computer's film the place that film was left later on the phone (sharedPosition), as the store is read.
func (s *Session) shareKeptPositions() {
	for _, f := range s.films.GetUntracked() {
		if shared := s.sharedPosition(f); shared.PositionAt != f.PositionAt {
			s.replace(shared)
			s.saveFilm(shared)
		}
	}
}

// Offline are the films the phone keeps, as Offline Films lists them, each once: a computer's film kept beside a film of a source matched to it is listed as that film, whose row stands for both copies (Copy, DeleteCopies). Read it in a binding.
func (s *Session) Offline() []store.Film {
	var out []store.Film
	for _, f := range s.films.Get() {
		if n, ok := s.matched(f.Key()); ok && n.Key() != f.Key() {
			continue
		}
		out = append(out, f)
	}
	return out
}
