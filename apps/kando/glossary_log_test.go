package main

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"
)

// suggestingPipeline is fakePipeline that suggests terms for each film it translates, counting how often it was asked.
type suggestingPipeline struct {
	fakePipeline
	asked int
}

func (p *suggestingPipeline) SuggestTerms(context.Context, []core.Segment, []core.Segment, string, string) ([]core.Term, error) {
	p.asked++
	return []core.Term{{Source: "Nile", Target: "尼罗河", Scope: core.ScopeFilm, Origin: core.OriginAuto}}, nil
}

// A job asks for terms for its film's glossary once it translates it (media.Run keeps them beside the video).
func TestJobsSuggestTermsForTheGlossary(t *testing.T) {
	p := &suggestingPipeline{}
	a, tt, updates := newTestApp(t, p)
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if p.asked != 1 {
		t.Fatalf("asked for terms %d times", p.asked)
	}
}

// Each change the 术语 tab makes is listed in its edit log, the latest first, and Undo, from the tab's button or the Edit menu, takes it back, the term's file with it; Redo makes it again.
func TestTermsTabEditLogUndoesTermChanges(t *testing.T) {
	a, tt, own := glossaryOpen(t)
	msgs := testlocale.Messages()
	if tt.Exists("id:film-edit-log") {
		t.Fatal("the edit log shows with nothing to take back")
	}
	tt.Type("id:film-term-source", "Hello")
	tt.Type("id:film-term-target", "哈喽")
	tt.Press("id:film-term-add")
	tt.Press("id:film-term-lock")
	if got, want := a.EditLog(), []string{msgs.GlossaryStepLock("哈喽"), msgs.GlossaryStepAdd("哈喽")}; !slices.Equal(got, want) {
		t.Fatalf("the edit log is %q, want %q", got, want)
	}
	wantText(t, tt, msgs.GlossaryStepLock("哈喽"))
	tt.Press("id:film-term-scope")
	wantText(t, tt, msgs.GlossaryStepScope("哈喽", msgs.GlossaryScopeSeries()))

	tt.Press("id:film-edit-log-undo")
	if terms := a.FilmTerms(); len(terms) != 1 || terms[0].Scope != core.ScopeFilm || !terms[0].Locked {
		t.Fatalf("after undoing the move, the terms are %+v", terms)
	}
	if terms, err := media.LoadTerms(own); err != nil || len(terms) != 1 {
		t.Fatalf("after undoing the move, the film's file holds %+v, %v", terms, err)
	}
	tt.Press("id:film-edit-log-undo")
	if terms, _ := media.LoadTerms(own); len(terms) != 1 || terms[0].Locked {
		t.Fatalf("after undoing the lock, the film's file holds %+v", terms)
	}
	tt.Press("id:film-edit-log-undo")
	if _, err := os.Stat(own); !os.IsNotExist(err) || len(a.FilmTerms()) != 0 {
		t.Fatalf("after undoing the add, the terms are %+v, the file %v", a.FilmTerms(), err)
	}
	if tt.Exists("id:film-edit-log") {
		t.Fatal("the edit log stays with nothing to take back")
	}
	apply(tt, a, a.correctRedo)
	if terms := a.FilmTerms(); len(terms) != 1 || terms[0].Target != "哈喽" || terms[0].Locked {
		t.Fatalf("redone, the terms are %+v", terms)
	}
	if got := a.EditLog(); !slices.Equal(got, []string{msgs.GlossaryStepAdd("哈喽")}) {
		t.Fatalf("redone, the edit log is %q", got)
	}
	tt.Press("id:film-term-remove")
	if got := a.EditLog(); len(got) != 2 || got[0] != msgs.GlossaryStepRemove("哈喽") {
		t.Fatalf("the removal is logged as %q", got)
	}
}

// The 术语 tab lists the terms found in the open film that its glossary does not hold: adding one is a change Undo takes back, after which it is suggested again; dismissing one keeps it from coming back.
func TestTermsTabAddsAndDismissesSuggestedTerms(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	found := []core.Term{{Source: "Nile", Target: "尼罗河", Scope: core.ScopeFilm, Origin: core.OriginAuto}, {Source: "delta", Target: "三角洲", Scope: core.ScopeFilm, Origin: core.OriginAuto}, {Source: "Danube", Target: "多瑙河", Scope: core.ScopeFilm, Origin: core.OriginAuto}}
	path := media.SuggestionsPath(item.VideoPath)
	if err := media.SaveSuggestions(path, media.Suggestions{Terms: found}); err != nil {
		t.Fatal(err)
	}
	a.glossaryLoad()
	// Danube is every film's term already, so it is not suggested.
	a.gloss.store.Set("", core.Term{Source: "danube", Target: "多瑙河", Scope: core.ScopeGlobal})
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead && len(a.TermSuggestions()) == 2 })
	tt.Press("id:film-tab-terms")
	msgs := testlocale.Messages()
	wantText(t, tt, "Nile → 尼罗河")
	tt.Press("id:film-term-suggestion-add >> nth=0")
	if terms := a.FilmTerms(); len(terms) != 2 || terms[0].Source != "Nile" || terms[0].Origin != core.OriginAuto || terms[0].Scope != core.ScopeFilm {
		t.Fatalf("added %+v", terms)
	}
	if got := a.TermSuggestions(); len(got) != 1 || got[0].Source != "delta" {
		t.Fatalf("still suggested %+v", got)
	}
	wantText(t, tt, msgs.GlossaryStepAccept("尼罗河"))
	tt.Press("id:film-edit-log-undo")
	if got := a.TermSuggestions(); len(got) != 2 {
		t.Fatalf("undone, suggested %+v", got)
	}
	tt.Press("id:film-term-suggestion-dismiss >> nth=1")
	saved, err := media.LoadSuggestions(path)
	if err != nil || !reflect.DeepEqual(saved.Dismissed, []string{"delta"}) || len(saved.Terms) != 2 {
		t.Fatalf("the file holds %+v, %v", saved, err)
	}
	if got := a.TermSuggestions(); len(got) != 1 || got[0].Source != "Nile" {
		t.Fatalf("after dismissing, suggested %+v", got)
	}
}
