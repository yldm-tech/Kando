package core

import (
	"testing"
	"time"
)

func TestProofAutoApplies(t *testing.T) {
	cases := []struct {
		auto ProofAuto
		kind ProofKind
		want bool
	}{
		{ProofAutoConsistency, ProofConsistency, true},
		{ProofAutoConsistency, ProofFluency, false},
		{ProofAutoConsistency, ProofMeaning, false},
		{"", ProofConsistency, true},
		{"bogus", ProofMeaning, false},
		{ProofAutoAll, ProofMeaning, true},
		{ProofAutoNone, ProofConsistency, false},
	}
	for _, c := range cases {
		if got := c.auto.Applies(c.kind); got != c.want {
			t.Errorf("%q.Applies(%q) = %v, want %v", c.auto, c.kind, got, c.want)
		}
	}
}

func TestProofValid(t *testing.T) {
	ok := Proof{Span: Span{0, time.Second}, Kind: ProofFluency, Old: "a", New: "b"}
	if !ok.Valid() {
		t.Fatal("a change of a known kind should be valid")
	}
	for _, p := range []Proof{
		{Kind: "style", Old: "a", New: "b"},
		{Kind: ProofFluency, Old: "a", New: " a "},
		{Kind: ProofFluency, Old: "a", New: ""},
		{Kind: ProofFluency, Old: "", New: "b"},
	} {
		if p.Valid() {
			t.Errorf("%+v should not be valid", p)
		}
	}
}

func TestSwapProofs(t *testing.T) {
	cues := []Segment{
		{Start: 0, End: time.Second, Text: "你好，杰克\nHello, Jack"},
		{Start: time.Second, End: 2 * time.Second, Text: "再见\nBye"},
	}
	pending := []Proof{
		{Span: Span{0, time.Second}, Kind: ProofConsistency, Old: "你好，杰克", New: "你好，捷克", Status: ProofPending},
		{Span: Span{time.Second, 2 * time.Second}, Kind: ProofFluency, Old: "不对", New: "回头见", Status: ProofPending},
	}
	accepted := AcceptProofs(pending)
	if !SwapProofs(cues, pending, accepted) {
		t.Fatal("accepting a change should rewrite its line")
	}
	if cues[0].Text != "你好，捷克\nHello, Jack" {
		t.Fatalf("line 1 = %q", cues[0].Text)
	}
	// The second change expects a translation the line no longer has: it changes nothing.
	if cues[1].Text != "再见\nBye" {
		t.Fatalf("line 2 = %q", cues[1].Text)
	}
	back := RollBackProofs(accepted)
	if back[0].Status != ProofAccepted {
		t.Fatalf("rolling back keeps what the user accepted, got %q", back[0].Status)
	}
	rejected := DecideProof(accepted, 0, false)
	if !SwapProofs(cues, accepted, rejected) || cues[0].Text != "你好，杰克\nHello, Jack" {
		t.Fatalf("turning a change down puts the old line back, got %q", cues[0].Text)
	}
	if SwapProofs(cues, rejected, rejected) {
		t.Fatal("nothing moved, nothing should change")
	}
}

func TestRollBackProofs(t *testing.T) {
	proofs := []Proof{{Status: ProofApplied}, {Status: ProofPending}, {Status: ProofAccepted}, {Status: ProofRejected}}
	got := RollBackProofs(proofs)
	want := []ProofStatus{ProofRejected, ProofRejected, ProofAccepted, ProofRejected}
	for i := range got {
		if got[i].Status != want[i] {
			t.Errorf("proof %d = %q, want %q", i, got[i].Status, want[i])
		}
	}
	if proofs[0].Status != ProofApplied {
		t.Fatal("RollBackProofs changed its argument")
	}
	if d := DecideProof(proofs, 0, true); d[0].Status != ProofApplied {
		t.Fatalf("accepting an applied change keeps it applied, got %q", d[0].Status)
	}
}
