package core

import (
	"slices"
	"strings"
)

// The final proofread (终校): once a film is translated, the translator reads its translation through again and suggests changes to the machine's lines, each kept with the line it changes, what it was, what it would be and why (Proof). Those of the kinds the user lets apply by themselves (ProofAuto) are written into the film's subtitles at once; the others wait for the user to accept them. Either can be rolled back, and a line the user corrected keeps the user's words whatever the proofread says.

// ProofKind is what a change of the final proofread is about.
type ProofKind string

const (
	// ProofConsistency makes a line write a name, a form of address or a term as the rest of the film does.
	ProofConsistency ProofKind = "consistency"
	// ProofFluency makes a line read naturally without changing what it says.
	ProofFluency ProofKind = "fluency"
	// ProofMeaning corrects a line that says something the original does not.
	ProofMeaning ProofKind = "meaning"
)

// Valid reports whether k is one of the kinds the final proofread knows.
func (k ProofKind) Valid() bool {
	return k == ProofConsistency || k == ProofFluency || k == ProofMeaning
}

// ProofStatus is where a change of the final proofread stands.
type ProofStatus string

const (
	// ProofApplied is a change that applied by itself, as its kind is one the user lets (ProofAuto).
	ProofApplied ProofStatus = "applied"
	// ProofPending is a change that waits for the user.
	ProofPending ProofStatus = "pending"
	// ProofAccepted is a change the user accepted.
	ProofAccepted ProofStatus = "accepted"
	// ProofRejected is a change the user turned down or rolled back.
	ProofRejected ProofStatus = "rejected"
)

// ProofAuto is which of the final proofread's changes apply without the user: those that make the film consistent alone, all of them, or none, each waiting for the user.
type ProofAuto string

const (
	ProofAutoConsistency ProofAuto = "consistency"
	ProofAutoAll         ProofAuto = "all"
	ProofAutoNone        ProofAuto = "none"
)

// ProofAutos are the levels Settings › 翻译 offers, in its order.
var ProofAutos = []ProofAuto{ProofAutoConsistency, ProofAutoAll, ProofAutoNone}

// CleanProofAuto is auto when it is one of ProofAutos, else the default, ProofAutoConsistency.
func CleanProofAuto(auto ProofAuto) ProofAuto {
	for _, a := range ProofAutos {
		if a == auto {
			return auto
		}
	}
	return ProofAutoConsistency
}

// Applies reports whether a change of kind applies by itself under auto.
func (auto ProofAuto) Applies(kind ProofKind) bool {
	switch CleanProofAuto(auto) {
	case ProofAutoAll:
		return true
	case ProofAutoNone:
		return false
	}
	return kind == ProofConsistency
}

// Proof is a change the final proofread made or suggests to the translation of the machine's line at Span: from Old to New, of Kind, for the reason Why, in the language the film is translated into.
type Proof struct {
	Span   Span
	Kind   ProofKind
	Old    string
	New    string
	Why    string
	Status ProofStatus
}

// Live reports whether p's New is the line's translation: it applied by itself or the user accepted it.
func (p Proof) Live() bool { return p.Status == ProofApplied || p.Status == ProofAccepted }

// Valid reports whether p is a change the final proofread can keep: of a kind it knows, to a translation that is not empty and not the one the line has.
func (p Proof) Valid() bool {
	return p.Kind.Valid() && strings.TrimSpace(p.New) != "" && strings.TrimSpace(p.Old) != "" && strings.TrimSpace(p.New) != strings.TrimSpace(p.Old) && p.Span.End >= p.Span.Start
}

// SwapProofs rewrites cues, the cues of a film's bilingual subtitles (SplitCue), for the changes whose status moved between before and after, index for index: a change that became live (Proof.Live) puts its New in place of its Old as its line's translation, and one that no longer is puts Old back. A change whose line, the cue at its span, no longer holds the translation it expects, as after the film was translated again, changes nothing. It reports whether a cue changed.
func SwapProofs(cues []Segment, before, after []Proof) (changed bool) {
	for i := range after {
		if i >= len(before) || before[i].Live() == after[i].Live() {
			continue
		}
		from, to := after[i].Old, after[i].New
		if !after[i].Live() {
			from, to = to, from
		}
		for k := range cues {
			if cues[k].Start != after[i].Span.Start || cues[k].End != after[i].Span.End {
				continue
			}
			translation, original := SplitCue(cues[k].Text)
			if translation != strings.TrimSpace(from) {
				continue
			}
			cues[k].Text = JoinCue(strings.TrimSpace(to), original)
			changed = true
			break
		}
	}
	return changed
}

// DecideProof is proofs with the change at i accepted, or with accept false turned down, or rolled back when it was live. proofs is not changed.
func DecideProof(proofs []Proof, i int, accept bool) []Proof {
	out := slices.Clone(proofs)
	if i < 0 || i >= len(out) {
		return out
	}
	if accept {
		out[i].Status = ProofAccepted
		if proofs[i].Status == ProofApplied {
			out[i].Status = ProofApplied
		}
	} else {
		out[i].Status = ProofRejected
	}
	return out
}

// AcceptProofs is proofs with every change that waits for the user accepted. proofs is not changed.
func AcceptProofs(proofs []Proof) []Proof {
	out := slices.Clone(proofs)
	for i := range out {
		if out[i].Status == ProofPending {
			out[i].Status = ProofAccepted
		}
	}
	return out
}

// RollBackProofs is proofs with the final proofread taken back: the changes that applied by themselves rolled back and those that wait turned down, the ones the user accepted kept. proofs is not changed.
func RollBackProofs(proofs []Proof) []Proof {
	out := slices.Clone(proofs)
	for i := range out {
		if out[i].Status == ProofApplied || out[i].Status == ProofPending {
			out[i].Status = ProofRejected
		}
	}
	return out
}
