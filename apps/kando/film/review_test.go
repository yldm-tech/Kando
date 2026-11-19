package film

import (
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
)

// fakeReviewer is the fake app's final proofread: the changes the list shows and what it asks of them, decided as a real app would.
type fakeReviewer struct {
	proofs    []Proof
	reviewOff bool
	proofView bool
	busy      bool
}

func (f *fakeReviewer) Proofs() []Proof  { return f.proofs }
func (f *fakeReviewer) ReviewOn() bool   { return !f.reviewOff }
func (f *fakeReviewer) ProofView() *bool { return &f.proofView }
func (f *fakeReviewer) ProofsBusy() bool { return f.busy }
func (f *fakeReviewer) AcceptProofs()    { f.set(core.AcceptProofs(f.core())) }
func (f *fakeReviewer) RollBackProofs()  { f.set(core.RollBackProofs(f.core())) }
func (f *fakeReviewer) DecideProof(i int, accept bool) {
	f.set(core.DecideProof(f.core(), i, accept))
}

func (f *fakeReviewer) core() []core.Proof {
	out := make([]core.Proof, len(f.proofs))
	for i, p := range f.proofs {
		out[i] = p.Proof
	}
	return out
}

func (f *fakeReviewer) set(proofs []core.Proof) {
	for i := range proofs {
		f.proofs[i].Proof = proofs[i]
	}
}

func sampleProofs() []Proof {
	span := func(s int) core.Span {
		return core.Span{Start: time.Duration(s) * time.Second, End: time.Duration(s+2) * time.Second}
	}
	return []Proof{
		{Proof: core.Proof{Span: span(1), Kind: core.ProofConsistency, Old: "你好，杰克。", New: "你好，捷克。", Why: "统一人名", Status: core.ProofApplied}},
		{Proof: core.Proof{Span: span(4), Kind: core.ProofFluency, Old: "再见了你。", New: "再见。", Why: "更自然", Status: core.ProofPending}},
		{Proof: core.Proof{Span: span(8), Kind: core.ProofMeaning, Old: "他走了。", New: "她走了。", Status: core.ProofPending}, Mine: true},
	}
}

// The 终校 chip shows once the film has changes and lists them in place of the lines: what the pass changed and what waits, each change's kind, where it stands, the old and new translation and the reason. Accept and Don't Use decide one that waits, Undo This rolls back one that is live, a line the user corrected offers neither, and Accept All and Roll Back decide the rest.
func TestProofViewDecidesTheFinalProofread(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-subs-proofs") {
		t.Fatal("a film without changes shows the 终校 chip")
	}
	change(app, a, func() { a.proofs = sampleProofs() })
	app.Press("id:film-subs-proofs")
	if !a.proofView || !app.Exists("id:film-proofs") || app.Exists("id:film-cues") {
		t.Fatalf("the chip did not show the changes: view %v, texts %q", a.proofView, app.Texts())
	}
	if !app.Exists(whole(msgs.ReviewHeadWaiting(3, 1))) {
		t.Fatalf("the head: %q", app.Texts())
	}
	for _, want := range []string{msgs.ReviewKindConsistency(), msgs.ReviewKindFluency(), msgs.ReviewKindMeaning(), msgs.ReviewStatusApplied(), msgs.ReviewStatusPending(), msgs.ReviewStatusMine(), "你好，杰克。", "你好，捷克。", "更自然"} {
		if !app.Exists(whole(want)) {
			t.Fatalf("%q is not shown: %q", want, app.Texts())
		}
	}
	if n := len(app.Query("id:film-proof-accept")); n != 1 {
		t.Fatalf("%d changes offer Accept, want the one that waits and is not the user's line", n)
	}
	app.Press("id:film-proof-accept")
	if a.proofs[1].Status != core.ProofAccepted || !app.Exists(whole(msgs.ReviewHeadDone(3))) {
		t.Fatalf("accept: %+v, texts %q", a.proofs, app.Texts())
	}
	app.Press("id:film-proofs-roll-back")
	if a.proofs[0].Status != core.ProofRejected || a.proofs[1].Status != core.ProofAccepted {
		t.Fatalf("roll back: %+v", a.proofs)
	}
	app.Press("id:film-subs-count")
	if a.proofView || !app.Exists("id:film-cues") {
		t.Fatal("全部 did not bring the lines back")
	}
}

// While a job makes the film's subtitles again the changes cannot be decided, and with the final proofread off the head says where to turn it on.
func TestProofViewWhileBusyAndOff(t *testing.T) {
	a := newFakeApp()
	a.proofs, a.busy, a.reviewOff, a.proofView = sampleProofs(), true, true, true
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.ReviewOff())) {
		t.Fatalf("the head: %q", app.Texts())
	}
	for _, id := range []string{"film-proof-accept", "film-proof-reject", "film-proof-undo", "film-proofs-accept-all", "film-proofs-roll-back"} {
		if !app.Find("id:" + id).Props.Disabled {
			t.Errorf("%s is on while the subtitles are being made again", id)
		}
	}
}

// fakeProgresser is the fake app's progress badge: how far the film's subtitles are, as a test sets it.
type fakeProgresser struct{ layers Layers }

func (f *fakeProgresser) SubtitleLayers() Layers { return f.layers }

// The progress badge shows while the film's subtitles are being made, a ring of how much is translated; it opens a panel with each layer. A done film with lines left untranslated shows it with !, and its panel's button translates them; Settings… opens Settings › 翻译. Otherwise there is none.
func TestProgressBadgeShowsTheLayers(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-badge") {
		t.Fatal("a done film shows the badge")
	}
	change(app, a, func() {
		a.layers = Layers{Show: true, Running: true, Percent: 40, Rows: []Layer{{Kind: LayerSpeech, Engine: "small", Done: 1}, {Kind: LayerTranslation, Engine: "gpt", Done: 0.4}, {Kind: LayerReview, Done: 0}}}
		a.jobStatus = "翻译中 · 40/100 句"
	})
	if got := app.Find("id:film-badge").Props.Label; got != msgs.FilmBadgeRunning(40) {
		t.Fatalf("the badge is named %q", got)
	}
	app.Press("id:film-badge")
	for _, want := range []string{msgs.FilmBadgeSubtitles(40), "翻译中 · 40/100 句", msgs.FilmLayerSpeech(), msgs.FilmLayerTranslation(), msgs.FilmLayerReview(), "small", "gpt", msgs.FilmBadgePercent(40)} {
		if !app.Exists(whole(want)) {
			t.Fatalf("%q is not shown: %q", want, app.Texts())
		}
	}
	app.Press("id:film-badge-settings")
	if len(a.calls) == 0 || a.calls[len(a.calls)-1] != "settings translate" {
		t.Fatalf("calls %q", a.calls)
	}

	change(app, a, func() {
		a.layers = Layers{Show: true, Percent: 96, Untranslated: 3, Rows: []Layer{{Kind: LayerSpeech, Done: 1}, {Kind: LayerTranslation, Done: 0.96}}}
	})
	if got := app.Find("id:film-badge").Props.Label; got != msgs.FilmBadgeAttention(96, 3) {
		t.Fatalf("the badge is named %q", got)
	}
	app.Press("id:film-badge")
	if !app.Exists(whole(msgs.FilmBadgeUntranslated(3))) {
		t.Fatalf("the summary: %q", app.Texts())
	}
	app.Press("id:film-badge-translate")
	if a.calls[len(a.calls)-1] != "translate missing" {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() { a.layers = Layers{} })
	if app.Exists("id:film-badge") {
		t.Fatal("the badge stayed once the subtitles were done")
	}
}
