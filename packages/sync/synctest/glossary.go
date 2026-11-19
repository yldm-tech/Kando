package synctest

import (
	"context"
	"slices"
	"strings"

	"kando/packages/sync"
)

// glossaries are the stand-in desktop's films' glossaries (sync.GlossaryService), by library and film ID, and whether it shares them at all (SetNoGlossary).
type glossaries struct {
	films map[string]sync.Glossary
	none  bool
}

// glossaryKey names film of library among the glossaries.
func glossaryKey(library, film string) string { return library + "/" + film }

// SetGlossary sets the glossary of film of library as the desktop's 术语 tab would, terms in the order it lists them; writable says whether the film keeps terms of its own.
func (d *Desktop) SetGlossary(library, film, series string, writable bool, terms ...sync.Term) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gloss.films == nil {
		d.gloss.films = map[string]sync.Glossary{}
	}
	d.gloss.films[glossaryKey(library, film)] = sync.Glossary{Library: library, Film: film, Series: series, Writable: writable, Terms: slices.Clone(terms)}
}

// Glossary is the glossary of film of library, as a phone's terms left it.
func (d *Desktop) Glossary(library, film string) sync.Glossary {
	d.mu.Lock()
	defer d.mu.Unlock()
	g := d.gloss.films[glossaryKey(library, film)]
	g.Terms = slices.Clone(g.Terms)
	return g
}

// SetNoGlossary makes the desktop answer the glossary's calls unsupported, and leave them out of Hello, as a Kando from before it shared them.
func (d *Desktop) SetNoGlossary(none bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gloss.none = none
}

// glossaryService is the stand-in's glossaries as v2's calls read and change them.
type glossaryService struct{ d *Desktop }

func (s glossaryService) Terms(_ context.Context, _ sync.Caller, req sync.FilmRef) (sync.Glossary, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gloss.none {
		return sync.Glossary{}, sync.Errorf(sync.CodeUnsupported, "no glossary")
	}
	g, ok := d.gloss.films[glossaryKey(req.Library, req.Film)]
	if !ok {
		g = sync.Glossary{Library: req.Library, Film: req.Film, Writable: true}
	}
	if g.Terms == nil {
		g.Terms = []sync.Term{}
	}
	return g, nil
}

// PutTerm gives the term for the same source the film has its translation and lock, or adds it to the film's own terms, first, as the desktop does.
func (s glossaryService) PutTerm(_ context.Context, _ sync.Caller, req sync.TermPut) (sync.Glossary, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gloss.none {
		return sync.Glossary{}, sync.Errorf(sync.CodeUnsupported, "no glossary")
	}
	t := sync.Term{Source: strings.TrimSpace(req.Term.Source), Target: strings.TrimSpace(req.Term.Target), Locked: req.Term.Locked, Scope: sync.TermFilm}
	if t.Source == "" || t.Target == "" {
		return sync.Glossary{}, sync.Errorf(sync.CodeInvalid, "a term needs a source and a target")
	}
	k := glossaryKey(req.Library, req.Film)
	g, ok := d.gloss.films[k]
	if !ok {
		g = sync.Glossary{Library: req.Library, Film: req.Film, Writable: true}
	}
	if !g.Writable {
		return sync.Glossary{}, sync.Errorf(sync.CodeNotReady, "the film keeps no terms")
	}
	g.Terms = slices.Clone(g.Terms)
	if i := slices.IndexFunc(g.Terms, func(o sync.Term) bool { return strings.EqualFold(o.Source, t.Source) }); i >= 0 {
		t.Scope = g.Terms[i].Scope
		g.Terms[i] = t
	} else {
		g.Terms = append([]sync.Term{t}, g.Terms...)
	}
	if d.gloss.films == nil {
		d.gloss.films = map[string]sync.Glossary{}
	}
	d.gloss.films[k] = g
	return g, nil
}
