package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// phoneGlossary is the films' glossaries a paired phone reads and adds terms to over protocol v2 (psync.GlossaryService): a film's own terms, kept beside its video, its series' and every film's, from glossary.json, as the player's 术语 tab lists them. A phone that corrects a line can add a term while it does; a term for words the film's glossary has already takes the phone's translation in the scope it is in, so a series' or every film's term stays where it is. A film of a locked library is not found.
type phoneGlossary struct{ a *app }

// film is the film of library the call names, as the app files it; a film the desktop does not serve, a locked library's among them, is not found.
func (g phoneGlossary) film(ctx context.Context, library, film string) (films.Film, error) {
	src := phoneSource{g.a}
	libs, err := src.Libraries(ctx)
	if err != nil {
		return films.Film{}, err
	}
	i := slices.IndexFunc(libs, func(l phonesync.Library) bool { return l.ID == library })
	if i < 0 {
		return films.Film{}, psync.Errorf(psync.CodeNotFound, "no library %q", library)
	}
	list, _, err := src.items(ctx, libs[i])
	if errors.Is(err, phonesync.ErrNotFound) {
		return films.Film{}, psync.Errorf(psync.CodeNotFound, "no library %q", library)
	}
	if err != nil {
		return films.Film{}, err
	}
	j := slices.IndexFunc(list, func(item films.Film) bool { return phoneFilmID(item) == film })
	if j < 0 || list[j].VideoPath == "" {
		return films.Film{}, psync.Errorf(psync.CodeNotFound, "no film %q in library %q", film, library)
	}
	return list[j], nil
}

func (g phoneGlossary) Terms(ctx context.Context, _ psync.Caller, req psync.GlossaryRef) (psync.Glossary, error) {
	item, err := g.film(ctx, req.Library, req.Film)
	if err != nil {
		return psync.Glossary{}, err
	}
	// The film's own terms are read off the engine's goroutine; the open film's are the ones the 术语 tab holds.
	own, readErr := media.LoadTerms(media.TermsPath(item.VideoPath))
	a := g.a
	return remote.OnUI(ctx, a.update, func() (psync.Glossary, error) {
		return a.phoneGlossaryOf(req, item, own, readErr == nil), nil
	})
}

// PutTerm adds the term the phone sent to the film's glossary, as the 术语 tab's fields do, and keeps it.
func (g phoneGlossary) PutTerm(ctx context.Context, caller psync.Caller, req psync.TermPut) (psync.Glossary, error) {
	t := core.Term{Source: strings.TrimSpace(req.Term.Source), Target: strings.TrimSpace(req.Term.Target), Scope: core.ScopeFilm, Origin: core.OriginUser, Locked: req.Term.Locked}
	if !t.Valid() {
		return psync.Glossary{}, psync.Errorf(psync.CodeInvalid, "a term needs the words and their translation")
	}
	item, err := g.film(ctx, req.Library, req.Film)
	if err != nil {
		return psync.Glossary{}, err
	}
	a := g.a
	return remote.OnUI(ctx, a.update, func() (psync.Glossary, error) {
		own, err := a.phoneGlossaryPut(item, t)
		if err != nil {
			log.Printf("phonesync: keep the term %s added: %v", caller.Name, err)
			return psync.Glossary{}, err
		}
		return a.phoneGlossaryOf(psync.GlossaryRef{Library: req.Library, Film: req.Film}, item, own, true), nil
	})
}

// phoneGlossaryJob is the job of item, by its ID or else its video, for the series it is in. UI thread only.
func (a *app) phoneGlossaryJob(item films.Film) core.Job {
	if job, ok := a.library.Find(item.JobID); ok && item.JobID != "" {
		return job
	}
	for _, job := range a.library.Jobs {
		if job.VideoPath != "" && filepath.Clean(job.VideoPath) == filepath.Clean(item.VideoPath) {
			return job
		}
	}
	return core.Job{}
}

// phoneGlossaryOf is item's glossary as a phone sees it, own being its own terms as read from beside its video, read whether they could be: the open film's are the ones the 术语 tab holds. UI thread only.
func (a *app) phoneGlossaryOf(ref psync.GlossaryRef, item films.Film, own []core.Term, read bool) psync.Glossary {
	if a.gloss.video != "" && filepath.Clean(a.gloss.video) == filepath.Clean(item.VideoPath) && a.gloss.filmRead {
		own, read = a.gloss.film, true
	}
	job := a.phoneGlossaryJob(item)
	series, global := a.gloss.store.Terms(job.Series)
	out := psync.Glossary{Library: ref.Library, Film: ref.Film, Terms: []psync.Term{}, Writable: read}
	if s, ok := a.gloss.store.Find(job.Series); ok {
		out.Series = s.Name
	}
	for _, t := range core.MergeTerms(own, series, global) {
		out.Terms = append(out.Terms, psync.Term{Source: t.Source, Target: t.Target, Scope: string(t.Scope), Locked: t.Locked})
	}
	return out
}

// phoneGlossaryPut puts t in item's glossary, as glossaryPut does for the open film: a term for the same words already in the film's glossary keeps its scope and takes t's translation and lock; another goes into the film's own terms. It returns the film's own terms as kept. A glossary.json that could not be read, or a terms file a newer Kando wrote, is not written over. UI thread only.
func (a *app) phoneGlossaryPut(item films.Film, t core.Term) ([]core.Term, error) {
	open := a.gloss.video != "" && filepath.Clean(a.gloss.video) == filepath.Clean(item.VideoPath)
	path := media.TermsPath(item.VideoPath)
	var old []core.Term
	switch {
	case open && !a.gloss.filmRead:
		return nil, psync.Errorf(psync.CodeNotReady, "the film's terms could not be read")
	case open:
		old = a.gloss.film
	default:
		var err error
		if old, err = media.LoadTerms(path); err != nil {
			return nil, psync.Errorf(psync.CodeNotReady, "the film's terms could not be read")
		}
	}
	job := a.phoneGlossaryJob(item)
	series, global := a.gloss.store.Terms(job.Series)
	merged := core.MergeTerms(old, series, global)
	if i := slices.IndexFunc(merged, func(o core.Term) bool { return core.SameTerm(o.Source, t.Source) }); i >= 0 {
		t.Scope, t.Source = merged[i].Scope, merged[i].Source
	}
	film := core.RemoveTerm(old, t.Source)
	if t.Scope == core.ScopeFilm {
		film = core.SetTerm(film, t)
	}
	store := a.gloss.store.Clone()
	changed := store.Set(job.Series, t)
	if changed && a.gloss.storeErr != nil {
		return nil, psync.Errorf(psync.CodeNotReady, "glossary.json could not be read")
	}
	filmChanged := !slices.Equal(film, old)
	if filmChanged {
		if err := media.SaveTerms(path, film); errors.Is(err, media.ErrTermsNewer) {
			return nil, psync.Errorf(psync.CodeNotReady, "the film's terms were written by a newer Kando")
		} else if err != nil {
			return nil, err
		}
	}
	if changed {
		if err := store.Save(); err != nil {
			if filmChanged {
				_ = media.SaveTerms(path, old)
			}
			return nil, err
		}
		a.gloss.store = store
	}
	if open {
		a.gloss.film = film
	}
	return film, nil
}
