package synctest

import (
	"context"
	"slices"
	"time"

	"kando/packages/sync"
)

// watching is what phones told the stand-in desktop of the films being made they watch (sync.WatchService), oldest first; none answers the call unsupported and leaves it out of Hello (SetNoWatch).
type watching struct {
	calls []sync.FilmWatch
	none  bool
}

// SetNoWatch makes the desktop answer the watching call unsupported, and leave it out of Hello, as a Kando from before films being made were listed to phones.
func (d *Desktop) SetNoWatch(none bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.watch.none = none
}

// Watches are what phones said of the films being made they watch, oldest first.
func (d *Desktop) Watches() []sync.FilmWatch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.watch.calls)
}

// MakeMore gives film id, being made, the lines cues so far and the video it plays now, as the desktop making it does; plays is what that video is (sync.PlaysVideo, sync.PlaysSound, "" for none yet). Its lines are said to have changed.
func (d *Desktop) MakeMore(id string, cues []sync.Cue, video []byte, plays string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.films {
		if f := &d.films[i]; f.ID == id {
			f.Cues, f.Video, f.Plays = slices.Clone(cues), video, plays
			f.VideoSize = int64(len(video))
			f.Updated = f.Updated.Add(time.Second)
		}
	}
}

// FinishMaking has film id done, with its lines cues: it is listed as any film from then on.
func (d *Desktop) FinishMaking(id string, cues []sync.Cue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.films {
		if f := &d.films[i]; f.ID == id {
			f.Making, f.Plays, f.Cues = false, sync.PlaysVideo, slices.Clone(cues)
			f.Updated = f.Updated.Add(time.Second)
			for j := range d.libs {
				if d.libs[j].ID == f.Library {
					d.libs[j].Films++
				}
			}
		}
	}
}

type watchService struct{ d *Desktop }

func (s watchService) WatchFilm(_ context.Context, _ sync.Caller, w sync.FilmWatch) (sync.FilmWatching, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.watch.none {
		return sync.FilmWatching{}, sync.Errorf(sync.CodeUnsupported, "no watching here")
	}
	for _, f := range d.films {
		if f.ID == w.ID {
			d.watch.calls = append(d.watch.calls, w)
			return sync.FilmWatching{Making: f.Making, Updated: f.Updated, Plays: f.Plays}, nil
		}
	}
	return sync.FilmWatching{}, sync.Errorf(sync.CodeNotFound, "no film %q", w.ID)
}
