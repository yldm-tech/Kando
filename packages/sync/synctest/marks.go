package synctest

import (
	"context"
	"slices"

	"kando/packages/sync"
)

// marks are the stand-in desktop's films' word marks (sync.MarkService), by film ID, as its plugins would compute them; pending answers every film's as still being computed, none answers the call unsupported and leaves it out of Hello (SetNoMarks), and asked counts the calls.
type marks struct {
	films   map[string]sync.FilmMarks
	pending bool
	none    bool
	asked   int
}

// SetMarks sets the marks the desktop answers for film id.
func (d *Desktop) SetMarks(id string, m sync.FilmMarks) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.marks.films == nil {
		d.marks.films = map[string]sync.FilmMarks{}
	}
	m.Lines = slices.Clone(m.Lines)
	d.marks.films[id] = m
}

// SetMarksPending makes the desktop answer every film's marks as still being computed while pending is set.
func (d *Desktop) SetMarksPending(pending bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.marks.pending = pending
}

// SetNoMarks makes the desktop answer the marks' call unsupported, and leave it out of Hello, as a Kando from before it marked its films' words.
func (d *Desktop) SetNoMarks(none bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.marks.none = none
}

// MarksAsked is how many times phones asked the desktop for a film's marks.
func (d *Desktop) MarksAsked() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.marks.asked
}

// markService is the stand-in's marks as OpFilmMarks reads them: a film it does not serve is not found, and one it has no marks set for has none.
type markService struct{ d *Desktop }

func (s markService) FilmMarks(_ context.Context, _ sync.Caller, ref sync.FilmRef) (sync.FilmMarks, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.marks.none {
		return sync.FilmMarks{}, sync.Errorf(sync.CodeUnsupported, "no marks")
	}
	d.marks.asked++
	if i := slices.IndexFunc(d.films, func(f Film) bool { return f.ID == ref.ID }); i < 0 || !slices.ContainsFunc(d.libs, func(l sync.Library) bool { return l.ID == d.films[i].Library }) {
		return sync.FilmMarks{}, sync.Errorf(sync.CodeNotFound, "no film %q", ref.ID)
	}
	if d.marks.pending {
		return sync.FilmMarks{Pending: true}, nil
	}
	m := d.marks.films[ref.ID]
	m.Lines = slices.Clone(m.Lines)
	return m, nil
}
