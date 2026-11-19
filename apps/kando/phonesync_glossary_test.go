package main

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// A phone reads a film's glossary as the 术语 tab lists it, its own terms, its series' and every film's, and adds a term while it corrects a line: new words go into the film's own terms, kept beside its video, and words the glossary has take the phone's translation where they are. A film the desktop does not serve is not found, and a read-only phone reads the glossary but adds nothing.
func TestPhoneSyncSharesTheGlossary(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	lib := phoneLibraryID(dir)
	var video string
	onEngine(a, func() {
		a.glossaryLoad()
		i := slices.IndexFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "yt" })
		a.library.Jobs[i].Series = "geo"
		video = a.library.Jobs[i].VideoPath
		a.gloss.store.Name("geo", "地理课")
		a.gloss.store.Set("geo", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal})
	})
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpTerms) || !slices.Contains(h.Ops, psync.OpTermPut) {
		t.Fatalf("hello %+v, %v; want the glossary among the operations", h, err)
	}
	g, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Glossary, error) { return c.Terms(ctx, lib, "yt") })
	if err != nil || g.Series != "地理课" || !g.Writable || len(g.Terms) != 1 || g.Terms[0] != (psync.Term{Source: "delta", Target: "三角洲", Scope: psync.TermGlobal}) {
		t.Fatalf("glossary %+v, %v", g, err)
	}

	put := func(term psync.Term) (psync.Glossary, error) {
		return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Glossary, error) {
			return c.PutTerm(ctx, psync.TermPut{Library: lib, Film: "yt", Term: term})
		})
	}
	g, err = put(psync.Term{Source: " meander ", Target: "河曲", Locked: true})
	if err != nil || len(g.Terms) != 2 || g.Terms[0] != (psync.Term{Source: "meander", Target: "河曲", Scope: psync.TermFilm, Locked: true}) {
		t.Fatalf("adding a term answered %+v, %v", g, err)
	}
	own, err := media.LoadTerms(media.TermsPath(video))
	if err != nil || len(own) != 1 || own[0].Source != "meander" || own[0].Origin != core.OriginUser || !own[0].Locked {
		t.Fatalf("the film's terms file holds %+v, %v", own, err)
	}
	g, err = put(psync.Term{Source: "Delta", Target: "河口三角洲", Locked: true})
	if err != nil || len(g.Terms) != 2 || g.Terms[1] != (psync.Term{Source: "delta", Target: "河口三角洲", Scope: psync.TermGlobal, Locked: true}) {
		t.Fatalf("changing a term answered %+v, %v", g, err)
	}
	if _, global := a.gloss.store.Terms("geo"); len(global) != 1 || global[0].Target != "河口三角洲" {
		t.Fatalf("every film's terms are %+v", global)
	}
	if own, _ := media.LoadTerms(media.TermsPath(video)); len(own) != 1 {
		t.Fatalf("a global term went into the film's own: %+v", own)
	}
	_, err = put(psync.Term{Source: "levee"})
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Glossary, error) { return c.Terms(ctx, lib, "nope") })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	onEngine(a, func() {
		for _, d := range a.phone.devices {
			a.phoneSyncSetReadOnly(d.ID, true)
		}
	})
	_, err = put(psync.Term{Source: "levee", Target: "堤"})
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	if g, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Glossary, error) { return c.Terms(ctx, lib, "yt") }); err != nil || len(g.Terms) != 2 {
		t.Fatalf("a read-only phone read %+v, %v", g, err)
	}
}
