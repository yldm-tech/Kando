package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// reviewingPipeline is fakePipeline whose final proofread counts how often it ran and suggests nothing.
type reviewingPipeline struct {
	fakePipeline
	reviews int
}

func (p *reviewingPipeline) Review(context.Context, []core.Segment, []core.Segment, string, string) ([]core.Proof, error) {
	p.reviews++
	return nil, nil
}

// A job proofreads its translation as a whole while Settings › 翻译 has the final proofread on, as it is by default, and not once it is turned off; the setting and which changes apply by themselves are kept in settings.json.
func TestJobsFollowTheFinalProofreadSetting(t *testing.T) {
	p := &reviewingPipeline{}
	a, tt, updates := newTestApp(t, p)
	if !*a.Review() || a.ReviewAuto() != string(core.ProofAutoConsistency) {
		t.Fatalf("the defaults: on %v, auto %q", *a.Review(), a.ReviewAuto())
	}
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if p.reviews != 1 {
		t.Fatalf("proofread %d times with the setting on", p.reviews)
	}
	*a.Review() = false
	a.SetReviewAuto(string(core.ProofAutoNone))
	if err := a.Enqueue("https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if p.reviews != 1 {
		t.Fatalf("proofread %d times with the setting off", p.reviews)
	}

	var saved savedSettings
	a.saveTranslate(&saved)
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var back savedSettings
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	back.clean()
	a.prefs = newSettingsUI()
	a.restoreTranslate(back)
	if *a.Review() || a.ReviewAuto() != string(core.ProofAutoNone) {
		t.Fatalf("read back: on %v, auto %q from %s", *a.Review(), a.ReviewAuto(), data)
	}
}

// reviewOpen is "talk" open with its final proofread kept beside it: the first line's change applied by itself and the second's waiting. It returns the path of the film's subtitles.
func reviewOpen(t *testing.T) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, nil)
	proofs := []core.Proof{
		{Span: core.Span{Start: time.Second, End: 2500 * time.Millisecond}, Kind: core.ProofConsistency, Old: "您好。", New: "你好。", Why: "统一称呼", Status: core.ProofApplied},
		{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, Kind: core.ProofFluency, Old: "再见了。", New: "回头见。", Why: "更自然", Status: core.ProofPending},
	}
	// The second line has a translation the change was made to.
	srt := item.SubtitlePath
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n再见了。\nBye.\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := media.SaveProofs(media.ProofsPath(item.VideoPath), proofs); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return a.review.ok && len(a.review.proofs) == 2 })
	return a, tt, updates, srt
}

// The subtitle list's 终校 chip lists the film's changes. Accepting the one that waits writes it into the subtitles, which the list then shows; Roll Back puts the one that applied by itself back and keeps what was accepted, on disk as in the list.
func TestFinalProofreadIsDecidedFromTheSubtitleList(t *testing.T) {
	a, tt, updates, srt := reviewOpen(t)
	msgs := testlocale.Messages()
	tt.Press("id:film-subs-proofs")
	wantText(t, tt, msgs.ReviewHeadWaiting(2, 1))
	tt.Press("id:film-proof-accept")
	waitUntil(t, tt, a, updates, func() bool {
		return a.player.subs.ready && len(a.player.subs.cues) == 2 && a.player.subs.cues[1].Text == "回头见。\nBye."
	})
	if kept, err := media.LoadProofs(media.ProofsPath(srt)); err != nil || kept[1].Status != core.ProofAccepted {
		t.Fatalf("kept %+v, %v", kept, err)
	}
	wantText(t, tt, msgs.ReviewHeadDone(2))

	tt.Press("id:film-proofs-roll-back")
	waitUntil(t, tt, a, updates, func() bool {
		return a.player.subs.ready && len(a.player.subs.cues) == 2 && a.player.subs.cues[0].Text == "您好。\nHello."
	})
	if a.player.subs.cues[1].Text != "回头见。\nBye." {
		t.Fatalf("roll back took back what was accepted: %q", a.player.subs.cues[1].Text)
	}
	kept, _ := media.LoadProofs(media.ProofsPath(srt))
	if kept[0].Status != core.ProofRejected || kept[1].Status != core.ProofAccepted {
		t.Fatalf("kept %+v", kept)
	}
	if !*a.ProofView() {
		t.Fatal("deciding a change left the list of changes")
	}
}

// A line the user corrected keeps the user's words: its change says so and offers nothing to decide.
func TestFinalProofreadLeavesTheUsersLine(t *testing.T) {
	a, tt, updates, _ := reviewOpen(t)
	mine := "我的翻译"
	edits := []core.Correction{{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, Translation: &mine}}
	apply(tt, a, func() { a.correctWrite(edits) })
	waitUntil(t, tt, a, updates, func() bool { return len(a.Proofs()) == 2 && a.Proofs()[1].Mine })
	tt.Press("id:film-subs-proofs")
	wantText(t, tt, testlocale.Messages().ReviewStatusMine())
	if tt.Exists("id:film-proof-accept") {
		t.Fatal("a change of the user's line can still be accepted")
	}
	if filepath.Base(media.ProofsPath(a.player.subs.path)) != ".talk.proofs.json" {
		t.Fatalf("the proofs file is %q", media.ProofsPath(a.player.subs.path))
	}
}
