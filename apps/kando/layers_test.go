package main

import (
	"testing"

	"kando/apps/kando/film"
	"kando/packages/core"
)

// The progress badge follows the open film's job: translating it again, its speech is heard already and its lines are as far as the translation says, then its final proofread; done with a line left untranslated, it says how much is translated; done in full, there is no badge.
func TestSubtitleLayersFollowTheOpenFilmsJob(t *testing.T) {
	a, tt, _ := playerOpen(t)
	if l := a.SubtitleLayers(); l.Show {
		t.Fatalf("a done film shows %+v", l)
	}
	apply(tt, a, func() {
		job := a.library.Jobs[0]
		job.Stage, job.Redo, job.Progress = core.StageTranslating, core.RedoTranslation, core.Progress{Lines: 30, Of: 100}
		a.library.Upsert(job)
	})
	l := a.SubtitleLayers()
	if !l.Show || !l.Running || l.Percent != 30 || len(l.Rows) != 3 || l.Rows[0].Done != 1 || l.Rows[1].Done != 0.3 || l.Rows[2].Kind != film.LayerReview || l.Rows[2].Done != 0 {
		t.Fatalf("translating: %+v", l)
	}
	apply(tt, a, func() {
		job := a.library.Jobs[0]
		job.Progress = core.Progress{Lines: 50, Of: 100, Reviewing: true}
		a.library.Upsert(job)
	})
	if l := a.SubtitleLayers(); l.Rows[1].Done != 1 || l.Rows[2].Done != 0.5 || l.Percent != 100 {
		t.Fatalf("proofreading: %+v", l)
	}
	apply(tt, a, func() {
		job := a.library.Jobs[0]
		job.Stage, job.Redo, job.Progress, job.Untranslated = core.StageComplete, "", core.Progress{}, 1
		a.library.Upsert(job)
	})
	if l := a.SubtitleLayers(); !l.Show || l.Running || l.Untranslated != 1 || l.Percent != 50 {
		t.Fatalf("done with a line untranslated: %+v", l)
	}
	*a.Review() = false
	apply(tt, a, func() {
		job := a.library.Jobs[0]
		job.Stage, job.Untranslated = core.StageTranscribing, 0
		job.Progress = core.Fraction(0.25)
		a.library.Upsert(job)
	})
	if l := a.SubtitleLayers(); len(l.Rows) != 2 || l.Rows[0].Done != 0.25 || l.Percent != 0 {
		t.Fatalf("transcribing without the final proofread: %+v", l)
	}
	if !tt.Exists("id:film-badge") {
		t.Fatal("the badge does not show while the film is made")
	}
}
