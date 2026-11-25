package session

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"

	"kando/apps/mobile/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// The glossary of the films kept on the phone and the corrections of their lines (yldm-tech/Kando#213). The player lists a film's glossary as its computer translates it (sync.OpTerms), which the phone shows and does not change; a learner who fixes a line's translation on the phone keeps the fix as a correction of the line in the learning state (sync.KindCorrection), which the computer takes at the next sync and lays over the line wherever it is read, and may add a term to the film's glossary while doing it (sync.OpTermPut), which the computer translates the film with from then on. A computer that made the phone read-only takes neither, so the phone offers neither.

// glossaries are the films' glossaries as their computers last answered them, by film key, and the films whose glossary is being asked for.
type glossaries struct {
	films  *core.Signal[map[string]sync.Glossary]
	asking map[string]bool
}

func newGlossaries(cx *core.Scope) *glossaries {
	return &glossaries{films: core.NewSignalFunc(cx, map[string]sync.Glossary{}, nil), asking: map[string]bool{}}
}

// glossaryTimeout is how long asking a computer for a film's glossary, or adding a term to it, waits.
const glossaryTimeout = 15 * time.Second

// Glossary is the glossary of the film under key as its computer last answered it this run, false until it answered, or when it does not share it. Read it in a binding.
func (s *Session) Glossary(key string) (sync.Glossary, bool) {
	g, ok := s.gloss.films.Get()[key]
	return g, ok
}

// OpenGlossary asks the computer of the film under key for the film's glossary, off the engine's goroutine, once the phone knows the film's library (Studyable); a computer out of reach, or one that does not share it, leaves what the phone has.
func (s *Session) OpenGlossary(key string) {
	f, ok := s.LinesFilm(key)
	if !ok || f.LibraryID == "" || s.gloss.asking[key] {
		return
	}
	c := s.Client(f.Desktop)
	if c == nil {
		return
	}
	s.gloss.asking[key] = true
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, glossaryTimeout)
		defer cancel()
		g, err := c.Terms(ctx, f.LibraryID, f.Film.ID)
		s.cx.Post(func() {
			delete(s.gloss.asking, key)
			if err == nil {
				s.keepGlossary(key, g)
			}
		})
	})
}

// keepGlossary keeps g as the glossary of the film under key.
func (s *Session) keepGlossary(key string, g sync.Glossary) {
	m := maps.Clone(s.gloss.films.GetUntracked())
	m[key] = g
	s.gloss.films.Set(m)
}

// readOnly reports whether computer id made the phone read-only, as it last said. Read it in a binding.
func (s *Session) readOnly(id string) bool {
	settings, _ := s.DesktopSettings(id)
	return settings.ReadOnly || s.Queue(id).ReadOnly
}

// CanCorrect reports whether the lines of the film under key can be fixed on the phone: its library is known (Studyable) and its computer did not make the phone read-only. Read it in a binding.
func (s *Session) CanCorrect(key string) bool {
	f, ok := s.LinesFilm(key)
	return ok && f.LibraryID != "" && !s.readOnly(f.Desktop)
}

// CanAddTerm reports whether a term can be added to the glossary of the film under key: its computer shares the glossary, the film can keep terms of its own, and the computer did not make the phone read-only. Read it in a binding.
func (s *Session) CanAddTerm(key string) bool {
	g, ok := s.Glossary(key)
	return ok && g.Writable && s.CanCorrect(key)
}

// Corrections are the fixes of the film under key's lines, the phone's not yet taken by the computer among them. Read it in a binding.
func (s *Session) Corrections(key string) []study.Correction {
	f, ok := s.LinesFilm(key)
	if !ok || f.LibraryID == "" {
		return nil
	}
	return study.Corrections(s.studies.Get()[studyKey(f.Desktop, f.LibraryID)], f.Film.ID)
}

// Correct keeps translation as the translation of c, a line of the film under key as the player shows it, and sends it to the computer, and reports whether it did: a film whose library is not known yet, or whose computer made the phone read-only, keeps nothing (CanCorrect).
func (s *Session) Correct(key string, c study.Cue, translation string) bool {
	f, ok := s.LinesFilm(key)
	if !ok || !s.CanCorrect(key) || strings.TrimSpace(translation) == "" {
		return false
	}
	l := s.studies.GetUntracked()[studyKey(f.Desktop, f.LibraryID)]
	s.changeStudy(f, study.Correct(l, f.Film.ID, c, translation, s.cfg.Now()))
	return true
}

// errNoGlossary is the error of a term added to the glossary of a film whose computer does not take one now.
var errNoGlossary = errors.New("the film's computer takes no term now")

// AddTerm adds t to the glossary of the film under key through its computer, and calls done on the engine's goroutine with why it was not added, nil once it was; the glossary the computer answers is kept.
func (s *Session) AddTerm(key string, t sync.Term, done func(error)) {
	f, ok := s.LinesFilm(key)
	c := s.Client(f.Desktop)
	if !ok || c == nil || !s.CanAddTerm(key) {
		done(errNoGlossary)
		return
	}
	req := sync.TermPut{Library: f.LibraryID, Film: f.Film.ID, Term: t}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, glossaryTimeout)
		defer cancel()
		g, err := c.PutTerm(ctx, req)
		s.cx.Post(func() {
			if err == nil {
				s.keepGlossary(key, g)
			}
			done(err)
		})
	})
}
