package session

import (
	"context"
	"maps"
	"slices"
	"time"

	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// The word marks of the films' lines (docs/plugin-system.md, E2; sync.OpFilmMarks): readings over the words of the originals, a line in a level's tone under the words of the word list chosen on the computer and a dashed one under those in the vocabulary. The phone runs no plugin and reads no dictionary for them: the film's computer computes them with its own switches (字幕外观 › 逐词标注), and the phone keeps its answer with the film (store.SaveMarks), so a film played offline shows them too. They are asked for on the computer's network as a film opens and once it is downloaded; a computer from before them answers no marks, and the film shows none.

// filmMarks are the films' marks as the phone last kept or was answered them, by film key, the films whose marks are being read or asked for, and those to ask for again once that ask ends, as their lines changed meanwhile (Follow).
type filmMarks struct {
	films  *core.Signal[map[string]sync.FilmMarks]
	asking map[string]bool
	again  map[string]bool
}

func newFilmMarks(cx *core.Scope) *filmMarks {
	return &filmMarks{films: core.NewSignalFunc(cx, map[string]sync.FilmMarks{}, nil), asking: map[string]bool{}, again: map[string]bool{}}
}

// marksTimeout is how long asking a computer for a film's marks waits; the computer answers marks it is still computing pending after a while of its own, shorter than this, and the phone asks again up to marksTries times, marksRetry apart.
const (
	marksTimeout = 20 * time.Second
	marksTries   = 3
	marksRetry   = time.Second
)

// FilmMarks are the word marks of the film under key's lines as kept with it or as its computer last answered them, false before either. Read it in a binding.
func (s *Session) FilmMarks(key string) (sync.FilmMarks, bool) {
	m, ok := s.marks.films.Get()[key]
	return m, ok
}

// OpenMarks reads the marks kept with the film under key, off the engine's goroutine, then asks its computer for them on its network, which the phone keeps in their place: a computer out of reach, or one from before the marks, leaves those kept; one that marks nothing on the film, with its switches off, drops them. Called while an ask of the film is under way, it asks again once that one ends, as the film's lines changed meanwhile.
func (s *Session) OpenMarks(key string) {
	f, ok := s.Film(key)
	if !ok {
		return
	}
	if s.marks.asking[key] {
		s.marks.again[key] = true
		return
	}
	var c *sync.Client
	if q, known := s.remote.queues.GetUntracked()[f.Desktop]; !known || len(q.Ops) == 0 || slices.Contains(q.Ops, sync.OpFilmMarks) {
		c = s.Client(f.Desktop)
	}
	_, read := s.marks.films.GetUntracked()[key]
	if read && c == nil {
		return
	}
	s.marks.asking[key] = true
	s.work.Go(func() {
		if !read {
			if m, ok, _ := s.st.Marks(f.Desktop, f.Film.ID); ok {
				s.cx.Post(func() {
					if _, answered := s.marks.films.GetUntracked()[key]; !answered {
						s.keepMarks(key, m)
					}
				})
			}
		}
		m, ok := askMarks(s.ctx, c, f.Film.ID)
		if ok {
			_ = s.st.SaveMarks(f.Desktop, f.Film.ID, m)
		}
		s.cx.Post(func() {
			delete(s.marks.asking, key)
			if ok {
				s.keepMarks(key, m)
			}
			if s.marks.again[key] {
				delete(s.marks.again, key)
				s.OpenMarks(key)
			}
		})
	})
}

// askMarks asks c, nil for no computer to ask, for film id's marks, again while they are being computed, and reports whether it answered them.
func askMarks(ctx context.Context, c *sync.Client, id string) (sync.FilmMarks, bool) {
	if c == nil {
		return sync.FilmMarks{}, false
	}
	for try := range marksTries {
		if try > 0 {
			select {
			case <-time.After(marksRetry):
			case <-ctx.Done():
				return sync.FilmMarks{}, false
			}
		}
		ask, cancel := context.WithTimeout(ctx, marksTimeout)
		m, err := c.FilmMarks(ask, id)
		cancel()
		if err != nil {
			return sync.FilmMarks{}, false
		}
		if !m.Pending {
			return m, true
		}
	}
	return sync.FilmMarks{}, false
}

// keepMarks keeps m as the marks of the film under key.
func (s *Session) keepMarks(key string, m sync.FilmMarks) {
	all := maps.Clone(s.marks.films.GetUntracked())
	all[key] = m
	s.marks.films.Set(all)
}

// sameOriginals reports whether two films' lines have the same originals, in the same order, which their marks depend on.
func sameOriginals(a, b []sync.Cue) bool {
	return slices.EqualFunc(a, b, func(x, y sync.Cue) bool { return x.Original == y.Original })
}
