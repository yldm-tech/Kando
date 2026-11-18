package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Reviewer is the open film's final proofread (终校, core.Proof): the changes it made or suggests to the machine's translation, which the subtitle list's 终校 chip lists for the user to accept, turn down or roll back.
type Reviewer interface {
	// Proofs is the open film's changes, in the order the pass made them.
	Proofs() []Proof
	// ReviewOn reports whether Settings › 翻译 has the final proofread on.
	ReviewOn() bool
	// ProofView is whether the subtitle list shows the changes in place of the lines.
	ProofView() *bool
	// ProofsBusy reports whether the changes cannot be decided now, as while a job makes the film's subtitles again.
	ProofsBusy() bool
	// DecideProof accepts change i, or with accept false turns it down, or rolls it back when it is live; AcceptProofs accepts every change that waits; RollBackProofs rolls the pass back, keeping what the user accepted.
	DecideProof(i int, accept bool)
	AcceptProofs()
	RollBackProofs()
}

// Proof is a change of the final proofread as the list shows it: Mine when the user corrected its line's translation, which the change then no longer touches.
type Proof struct {
	core.Proof
	Mine bool
}

// waiting reports whether p waits for the user.
func (p Proof) waiting() bool { return p.Status == core.ProofPending && !p.Mine }

// proofKindName is what the list calls a change's kind in the language the interface shows under cx.
func proofKindName(cx *ns.Scope, kind core.ProofKind) string {
	switch kind {
	case core.ProofFluency:
		return locales.ReviewKindFluency(cx)
	case core.ProofMeaning:
		return locales.ReviewKindMeaning(cx)
	}
	return locales.ReviewKindConsistency(cx)
}

// proofKindColors are the pill of a change's kind under cx (prototype script 2836): consistency a plain glass, fluency the accent's blue and meaning the warning's amber, each with its text. It reads the palette, so it goes in a StyleFunc.
func proofKindColors(cx *ns.Scope, kind core.ProofKind) (bg, fg ns.Color) {
	switch kind {
	case core.ProofFluency:
		return ui.Alpha(theme.Of(cx).AccentInk(), 0.25), ui.Mix(theme.Of(cx).AccentInk(), onGlass, 0.6)
	case core.ProofMeaning:
		return ui.Alpha(theme.Of(cx).WarnInk(), 0.25), ui.Mix(theme.Of(cx).WarnInk(), onGlass, 0.6)
	}
	return ns.RGBA(0xFFFFFF, 0.16), onGlass
}

// proofStatus says where a change stands in the language the interface shows under cx.
func proofStatus(cx *ns.Scope, p Proof) string {
	switch {
	case p.Mine:
		return locales.ReviewStatusMine(cx)
	case p.Status == core.ProofApplied:
		return locales.ReviewStatusApplied(cx)
	case p.Status == core.ProofPending:
		return locales.ReviewStatusPending(cx)
	case p.Status == core.ProofAccepted:
		return locales.ReviewStatusAccepted(cx)
	}
	return locales.ReviewStatusRejected(cx)
}

// proofView is the open film's changes in place of the subtitle list (prototype template 900-920): what the pass changed and what waits, with Accept All and Roll Back, then each change with its time, which plays from its line, its kind, where it stands, the old and new translation, the reason, and the buttons that decide it, and a note on what the pass touches.
func (s *screen) proofView(cx *ns.Scope) ns.Node {
	a := s.a
	proofs := live.Func(a, a.Proofs)
	busy := live.Func(a, a.ProofsBusy)
	head := live.Func(a, func() string {
		if !a.ReviewOn() {
			return locales.ReviewOff(cx)
		}
		list := a.Proofs()
		waiting := 0
		for _, p := range list {
			if p.waiting() {
				waiting++
			}
		}
		if waiting > 0 {
			return locales.ReviewHeadWaiting(cx, len(list), waiting)
		}
		return locales.ReviewHeadDone(cx, len(list))
	})
	anyWaiting := live.Func(a, func() bool {
		for _, p := range a.Proofs() {
			if p.waiting() {
				return true
			}
		}
		return false
	})
	anyLive := live.Func(a, func() bool {
		for _, p := range a.Proofs() {
			if (p.Status == core.ProofApplied || p.Status == core.ProofPending) && !p.Mine {
				return true
			}
		}
		return false
	})
	rows := ns.Each(func() []int {
		out := make([]int, len(proofs()))
		for i := range out {
			out[i] = i
		}
		return out
	}, func(i int) int { return i }, func(cx *ns.Scope, i func() int, _ func() int) ns.Node {
		return s.proofRow(cx, func() Proof {
			if list := proofs(); i() < len(list) {
				return list[i()]
			}
			return Proof{}
		}, i, busy)
	})
	return ns.View{TestID: "film-proofs", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 6, Padding: ns.Edges{Right: 6, Bottom: 8, Left: 6}}, Children: []ns.Node{
			ns.Text{TestID: "film-proofs-head", ContentFunc: head, Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.7)}},
			pillButton(cx, pill{id: "film-proofs-accept-all", label: func() string { return locales.ReviewAcceptAll(cx) }, color: func() ns.Color { return theme.Of(cx).AccentInk() },
				disabled: func() bool { return busy() || !anyWaiting() }, press: live.Do(a, a.AcceptProofs)}),
			pillButton(cx, pill{id: "film-proofs-roll-back", label: func() string { return locales.ReviewRollBack(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.14) },
				disabled: func() bool { return busy() || !anyLive() }, press: live.Do(a, a.RollBackProofs)}),
		}},
		ns.Scroll{Style: ns.Style{FlexGrow: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
			ns.View{Style: ns.Style{Gap: 6, Padding: ns.Edges{Bottom: 12}}, Children: []ns.Node{
				rows,
				ns.Text{ContentFunc: func() string { return locales.ReviewNote(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.55), Margin: ns.Edges{Top: 6, Right: 4, Left: 4}}},
			}},
		}},
	}}
}

// proofRow is one change of the final proofread: its line's time, which plays from it, its kind and where it stands, the translation it replaced struck through over the one it gives, the reason, and Accept and Don't Use for one that waits or Undo This for one that is live.
func (s *screen) proofRow(cx *ns.Scope, proof func() Proof, index func() int, busy func() bool) ns.Node {
	a := s.a
	clock := func() string { return cueClockTenths(proof().Span.Start) }
	kind := func() string { return proofKindName(cx, proof().Kind) }
	decide := func(accept bool) func() {
		return live.Do(a, func() { a.DecideProof(index(), accept) })
	}
	waiting := func() bool { return proof().waiting() }
	liveNow := func() bool { p := proof(); return p.Live() && !p.Mine }
	return ns.View{TestID: "film-proof", Role: ns.RoleGroup, LabelFunc: func() string { return clock() + " " + kind() + " " + proofStatus(cx, proof()) },
		Style: ns.Style{CornerRadius: 10, Padding: ns.Symmetric(9, 10), Gap: 4},
		StyleFunc: func() ns.Style {
			if waiting() {
				return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.1)}
			}
			return ns.Style{Background: ns.RGBA(0xFFFFFF, 0.05)}
		},
		Children: []ns.Node{
			ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 6}, Children: []ns.Node{
				base.Button{TestID: "film-proof-time", LabelFunc: func() string { return locales.ReviewPlayLine(cx, clock()) },
					OnPress: live.On(a, func(ns.PressEvent) { a.Seek(proof().Span.Start + a.Delay()) }),
					Style: ns.Style{Height: ns.Pt(20), Padding: ns.Symmetric(0, 6), CornerRadius: 6, JustifyContent: ns.JustifyCenter,
						Hover: &ns.Style{Background: panelLine}, FocusVisible: ui.FocusRing(cx, -2)},
					Children: []ns.Node{ns.Text{ContentFunc: clock, MaxLines: 1, Style: ns.Style{FontSize: 11, LineHeight: 14, FontVariant: ns.FontTabularNums, Color: onGlass, Opacity: ns.Alpha(0.8)}}}},
				ns.View{Style: ns.Style{Padding: ns.Symmetric(0, 6), CornerRadius: 6},
					StyleFunc: func() ns.Style { bg, _ := proofKindColors(cx, proof().Kind); return ns.Style{Background: bg} },
					Children: []ns.Node{ns.Text{TestID: "film-proof-kind", ContentFunc: kind, MaxLines: 1, Style: ns.Style{FontSize: 11, LineHeight: 16, FontWeight: ns.FontWeightBold},
						StyleFunc: func() ns.Style { _, fg := proofKindColors(cx, proof().Kind); return ns.Style{Color: fg} }}}},
				ns.Text{TestID: "film-proof-status", ContentFunc: func() string { return proofStatus(cx, proof()) }, MaxLines: 1,
					Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), FontSize: 11, LineHeight: 14, Color: onGlass, Opacity: ns.Alpha(0.65)}},
			}},
			ns.View{Selectable: true, Style: ns.Style{Gap: 4}, Children: []ns.Node{
				ns.Text{TestID: "film-proof-old", ContentFunc: func() string { return proof().Old },
					Style: ns.Style{FontSize: 12, LineHeight: 17, Color: onGlass, Opacity: ns.Alpha(0.55), TextDecoration: ns.TextDecorationLineThrough}},
				ns.Text{TestID: "film-proof-new", ContentFunc: func() string { return proof().New }, Style: ns.Style{FontSize: 14, LineHeight: 19, Color: onGlass}},
				ns.Show{When: func() bool { return proof().Why != "" }, Then: func(*ns.Scope) ns.Node {
					return ns.Text{TestID: "film-proof-why", ContentFunc: func() string { return proof().Why }, Style: ns.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: ns.Alpha(0.65)}}
				}},
			}},
			ns.Show{When: waiting, Then: func(cx *ns.Scope) ns.Node {
				return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, Margin: ns.Edges{Top: 2}}, Children: []ns.Node{
					pillButton(cx, pill{id: "film-proof-accept", label: func() string { return locales.ReviewAccept(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.28) },
						disabled: busy, press: decide(true)}),
					pillButton(cx, pill{id: "film-proof-reject", label: func() string { return locales.ReviewReject(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.14) },
						disabled: busy, press: decide(false)}),
				}}
			}},
			ns.Show{When: liveNow, Then: func(cx *ns.Scope) ns.Node {
				return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, Margin: ns.Edges{Top: 2}}, Children: []ns.Node{
					pillButton(cx, pill{id: "film-proof-undo", label: func() string { return locales.ReviewUndo(cx) }, color: func() ns.Color { return ns.RGBA(0xFFFFFF, 0.14) },
						disabled: busy, press: decide(false)}),
				}}
			}},
		}}
}
