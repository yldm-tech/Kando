package main

import (
	"os"
	"testing"

	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsGlossary renders the prototype's film, How Rivers Shape Landscapes (j1), held at its start with the side panel on 术语: the design's terms of the film (prototype script 2290), one locked, one its series' and one every film's, the film in the series Earth Science with two other films.
func TestScreenshotsGlossary(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "player-terms", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		j1 := f.job(t, "j1")
		own := media.TermsPath(j1.VideoPath)
		if err := media.SaveTerms(own, []core.Term{
			{Source: "meander", Target: "河曲", Locked: true},
			{Source: "oxbow lake", Target: "牛轭湖"},
			{Source: "sediment", Target: "泥沙"},
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(own) })
		a.gloss.store.Name("earth-science", "Earth Science")
		a.gloss.store.Set("earth-science", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeSeries})
		a.gloss.store.Set("earth-science", core.Term{Source: "V-shaped valley", Target: "V 形谷", Scope: core.ScopeGlobal})
		in := 0
		for i, job := range a.library.Jobs {
			if job.ID == "j1" || (in < 3 && job.Stage == core.StageComplete) {
				a.library.Jobs[i].Series = "earth-science"
				in++
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", 0)
		screenshotLoad(tt, a)
		screenshotWait(t, tt, a, updates, "the film's terms", func() bool { return a.gloss.filmRead && len(a.gloss.film) == 3 })
		tt.Press("id:film-tab-terms")
		tt.HoverOut()
		if !tt.Exists("id:film-terms-series") || len(a.FilmTerms()) != 5 {
			t.Fatalf("terms %+v; texts %q", a.FilmTerms(), tt.Texts())
		}
	})
}
