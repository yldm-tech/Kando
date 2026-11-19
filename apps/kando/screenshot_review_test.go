package main

import (
	"os"
	"testing"

	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsReview renders the prototype's film, How Rivers Shape Landscapes (j1), with its final proofread (终校) in the subtitle list: a consistency change that applied by itself, a fluency change that waits for the user and a meaning change, made to the film's first lines; and the 术语 tab asking whether j1 and a film named as its next episode are a series.
func TestScreenshotsReview(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "player-proofread", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j1", 0)
		screenshotLoad(tt, a)
		cues := a.player.subs.machine
		if len(cues) < 3 {
			t.Fatalf("the film has %d lines", len(cues))
		}
		proof := func(i int, kind core.ProofKind, to, why string, status core.ProofStatus) core.Proof {
			translation, _ := core.SplitCue(cues[i].Text)
			return core.Proof{Span: core.Span{Start: cues[i].Start, End: cues[i].End}, Kind: kind, Old: translation, New: to, Why: why, Status: status}
		}
		path := media.ProofsPath(a.player.subs.path)
		if err := media.SaveProofs(path, []core.Proof{
			proof(0, core.ProofConsistency, "河流塑造了我们看到的地貌。", "与全片「地貌」的写法统一", core.ProofApplied),
			proof(1, core.ProofFluency, "我们先从山上的源头说起。", "语序更自然", core.ProofPending),
			proof(2, core.ProofMeaning, "水往低处流，一路带走泥沙。", "原文说的是带走泥沙，不是石头", core.ProofPending),
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
		apply(tt, a, func() { a.review = newReviewUI() })
		screenshotWait(t, tt, a, updates, "the film's final proofread", func() bool { return len(a.review.proofs) == 3 })
		tt.Press("id:film-subs-proofs")
		tt.HoverOut()
		if !tt.Exists("id:film-proofs") {
			t.Fatalf("no proofread view: %q", tt.Texts())
		}
	})
	f.shoot(t, "player-series-guess", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		// j1 and another finished film are named as two parts of one show.
		other := false
		for i, job := range a.library.Jobs {
			switch {
			case job.ID == "j1":
				a.library.Jobs[i].Title = "How Rivers Shape Landscapes - Part 1"
			case !other && job.Stage == core.StageComplete:
				a.library.Jobs[i].Title = "How Rivers Shape Landscapes - Part 2"
				other = true
			}
		}
		screenshotOpen(t, a, tt, updates, "j1", 0)
		screenshotLoad(tt, a)
		screenshotWait(t, tt, a, updates, "the film's terms", func() bool { return a.gloss.filmRead })
		tt.Press("id:film-tab-terms")
		tt.HoverOut()
		if !tt.Exists("id:film-terms-guess") {
			t.Fatalf("no series question: %q", tt.Texts())
		}
	})
}
