package phonesync

import (
	"context"
	"errors"
	"time"

	psync "kando/packages/sync"
)

// watching is the server as protocol v2's psync.WatchService, nil when the app takes no phone's watching (Config.Watched).
func (s *Server) watching() psync.WatchService {
	if s.cfg.Watched == nil {
		return nil
	}
	return watchAnswer{s}
}

// findMaking is the film being made of ID id, read now, as how far it has come changes every few seconds.
func (s *Server) findMaking(ctx context.Context, id string) (served, error) {
	libs, err := s.cfg.Source.Libraries(ctx)
	if err != nil {
		return served{}, err
	}
	for _, lib := range libs {
		if lib.Locked {
			continue
		}
		making, err := s.makingFor(ctx, lib, false)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return served{}, err
		}
		for _, f := range making {
			if f.ID == id {
				return f, nil
			}
		}
	}
	return served{}, ErrNotFound
}

// watchAnswer answers psync.OpFilmWatch: how far the film watched has come, read again at each call (findMaking); and the film being made is told to the app (Config.Watched), which makes it first. A film done answers as one, so the phone reads it again as any other film; one no longer listed, removed or failed, is not found.
type watchAnswer struct{ s *Server }

func (w watchAnswer) WatchFilm(ctx context.Context, caller psync.Caller, req psync.FilmWatch) (psync.FilmWatching, error) {
	f, err := w.s.findMaking(ctx, req.ID)
	if errors.Is(err, ErrNotFound) {
		// Done since the phone listed it, it is found among the films as they are.
		f, err = w.s.find(ctx, req.ID)
	}
	if errors.Is(err, ErrNotFound) {
		return psync.FilmWatching{}, psync.Errorf(psync.CodeNotFound, "no film %q", req.ID)
	}
	if err != nil {
		return psync.FilmWatching{}, err
	}
	out := psync.FilmWatching{Making: f.Making, Updated: f.film.Updated}
	switch {
	case f.SubtitlesOnly:
	case f.ComingSound != "":
		out.Plays = psync.PlaysSound
	case !f.Making || fileThere(f.Video):
		out.Plays = psync.PlaysVideo
	}
	if f.Making {
		w.s.cfg.Watched(caller, f.Film, time.Duration(req.AtMS)*time.Millisecond, req.Claim)
	}
	return out, nil
}
