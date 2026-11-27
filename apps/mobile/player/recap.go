package player

import (
	"strings"
	"time"

	"kando/apps/mobile/cues"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// recapItem is a moment of the recap: the start of the line it is at, in the lines' time, and what happened there, or the line itself for the lines watched shown in its place.
type recapItem struct {
	start time.Duration
	text  string
}

// recapCard is the 前情提要 sheet of a film opened held where it was left (yldm-tech/Kando#215): where it was left (at), in the picture's time; pending until the film's lines are read, then the part watched summed up by the computer's AI service, busy while it is asked, or the lines watched themselves (excerpts) when it cannot, failed with why when it was asked and could not; the model that wrote it and whether it runs on the computer.
type recapCard struct {
	open, pending  bool
	at             time.Duration
	items          []recapItem
	busy, excerpts bool
	failed         bool
	problem        session.StudyProblem
	model          string
	local          bool
}

// recapper is the recap of the film the player plays; gen numbers the recaps asked for, so the answer to one the learner left is dropped.
type recapper struct {
	p     *play
	card  *core.Signal[recapCard]
	gen   int
	watch []sync.Cue
}

// recapEndMargin is how close to its end a film left there is taken as watched to its end: it opens without a recap, as the player starts it from the beginning again.
const recapEndMargin = 2 * time.Second

// newRecapper is the recap of the film p plays. A film opened held where it was left (held), as from its page's Play, holds there and waits for its lines: when some of them were watched, it shows the recap of them, and otherwise plays on. A film opened playing, as from Continue Watching or a line found, shows none.
func newRecapper(cx *core.Scope, p *play, held bool) *recapper {
	f, _ := p.s.Film(p.key)
	r := &recapper{p: p, card: core.NewSignalFunc(cx, recapCard{}, nil)}
	at := time.Duration(f.PositionMS) * time.Millisecond
	length := time.Duration(f.Film.DurationMS) * time.Millisecond
	if !held || at <= 0 || (length > 0 && at >= length-recapEndMargin) {
		return r
	}
	p.playing.Set(false)
	r.card.Set(recapCard{pending: true, at: at})
	cx.Effect(func() {
		if st := p.cues.State(); st == core.ResourcePending || st == core.ResourceUnresolved {
			return
		}
		list := p.list()
		cx.Untrack(func() { r.begin(list) })
	})
	cx.Cleanup(func() { r.gen++ })
	return r
}

// begin opens the recap once the film's lines are read, list: the lines watched, those that started by where the film was left, summed up by the computer; a film of which none was watched plays on.
func (r *recapper) begin(list []cues.Cue) {
	c := r.card.GetUntracked()
	if !c.pending {
		return
	}
	p := r.p
	sub := c.at - p.delay.GetUntracked()
	var watched []cues.Cue
	for _, l := range list {
		if l.Start > sub {
			break
		}
		if strings.TrimSpace(l.Original) != "" || strings.TrimSpace(l.Translation) != "" {
			watched = append(watched, l)
		}
	}
	if len(watched) == 0 {
		r.card.Set(recapCard{})
		p.playing.Set(true)
		return
	}
	r.watch = make([]sync.Cue, len(watched))
	for i, l := range watched {
		r.watch[i] = sync.Cue{StartMS: l.Start.Milliseconds(), EndMS: l.End.Milliseconds(), Original: l.Original, Translation: l.Translation}
	}
	r.card.Set(recapCard{open: true, at: c.at, busy: true})
	r.gen++
	gen := r.gen
	p.noteStudy()
	p.s.Recap(p.key, r.watch, func(got sync.Recap, err error) {
		c := r.card.GetUntracked()
		if gen != r.gen || !c.open {
			return
		}
		c.busy = false
		for _, it := range got.Items {
			if text := strings.TrimSpace(it.Text); text != "" {
				c.items = append(c.items, recapItem{start: time.Duration(it.StartMS) * time.Millisecond, text: text})
			}
		}
		switch {
		case err != nil:
			c.items, c.excerpts, c.failed, c.problem = recapExcerpts(watched), true, true, session.StudyProblemOf(err)
		case len(c.items) == 0:
			c.items, c.excerpts, c.failed, c.problem = recapExcerpts(watched), true, true, session.StudyFailed
		default:
			c.model, c.local = got.Model, got.Local
		}
		r.card.Set(c)
	})
}

// recapExcerpts are the lines the sheet shows when the computer sums none up: the first, the middle and the last line watched, as the computer's recap card picks them, each its translation or else its original; every line when there are no more than sync.RecapItems.
func recapExcerpts(watched []cues.Cue) []recapItem {
	picks := watched
	if len(watched) > sync.RecapItems {
		picks = []cues.Cue{watched[0], watched[len(watched)/2], watched[len(watched)-1]}
	}
	out := make([]recapItem, 0, len(picks))
	for _, l := range picks {
		text := strings.TrimSpace(l.Translation)
		if text == "" {
			text = strings.TrimSpace(l.Original)
		}
		out = append(out, recapItem{start: l.Start, text: text})
	}
	return out
}

// close closes the recap: the film stays held where it was left.
func (r *recapper) close() {
	r.gen++
	r.card.Set(recapCard{})
}

// playFrom closes the recap and plays the film from at, in the picture's time.
func (r *recapper) playFrom(at time.Duration) {
	r.close()
	r.p.held.Set(false)
	r.p.seek(at)
	r.p.playing.Set(true)
}

// resume is Continue from X: the film plays on from where it was left.
func (r *recapper) resume() { r.playFrom(r.card.GetUntracked().at) }

// startOver is Start Over: the film plays from its start.
func (r *recapper) startOver() { r.playFrom(0) }

// jump plays the film from item i, at the start of the line it is at, with the film's subtitle delay.
func (r *recapper) jump(i int) {
	items := r.card.GetUntracked().items
	if i < 0 || i >= len(items) {
		return
	}
	r.playFrom(max(items[i].start+r.p.delay.GetUntracked(), 0))
}

// recapNote is the sheet's footnote: which model summed up the part watched and where it ran, or that the lines shown are the lines watched, not summed up, and why.
func recapNote(cx *core.Scope, p *play, c recapCard) string {
	name := computer(p)
	switch {
	case c.busy:
		return ""
	case !c.excerpts && c.local:
		return locales.PlayerRecapNoteOn(cx, c.model, name)
	case !c.excerpts:
		return locales.PlayerRecapNote(cx, c.model)
	}
	switch c.problem {
	case session.StudyNoAI:
		return locales.PlayerRecapExcerptsNoAI(cx, name)
	case session.StudyUnpaired:
		return locales.PlayerRecapExcerptsUnpaired(cx)
	case session.StudyFailed:
		return locales.PlayerRecapExcerptsFailed(cx, name)
	}
	return locales.PlayerRecapExcerptsUnreachable(cx, name)
}

// recapSheet is the recap, in a sheet over the player held where the film was left (design Main, 前情提要): where it was left, three moments of the part watched, each of which plays the film from there, the footnote that says who summed them up, or that they are the lines watched; Continue from X and Start Over. Closing it leaves the film held.
func recapSheet(cx *core.Scope, p *play, r *recapper) core.Node {
	colors := ui.ColorsOf(cx)
	c := func() recapCard { return r.card.Get() }
	ai := func() core.Color { return theme.AI }
	items := func() []recapItem { return c().items }
	// Keyed by the whole moment: two of the lines watched shown in its place may start together.
	rows := core.Each(items, func(it recapItem) recapItem { return it }, func(cx *core.Scope, it func() recapItem, i func() int) core.Node {
		return base.Button{TestID: "player-recap-item", LabelFunc: func() string { return locales.PlayerRecapJump(cx, clock(it().start), it().text) },
			OnPress: func(core.PressEvent) { r.jump(i()) },
			Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignStart, Padding: core.All(12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
			Children: []core.Node{
				ui.Tabular(core.Text{ContentFunc: func() string { return clock(it().start) }, MaxLines: 1, PointerEvents: core.PointerEventsNone,
					Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Accent)}),
				core.Text{TestID: "player-recap-text", ContentFunc: func() string { return it().text }, PointerEvents: core.PointerEventsNone,
					Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: ui.Colored(colors.Text)},
			}}
	})
	at := func() string { return clock(c().at) }
	return appSheet{Dark: true, Open: func() bool { return c().open }, OnClose: r.close, Label: func() string { return locales.PlayerRecapTitle(cx) }, Children: []core.Node{
		core.Scroll{TestID: "player-recap", Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 8}}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 9, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: theme.AI.Faded(0.18)},
						Children: []core.Node{ui.IconOf(lucide.Clock, 18, ai)}},
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.PlayerRecapTitle(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
						core.Text{TestID: "player-recap-at", ContentFunc: func() string { return locales.PlayerRecapStoppedAt(cx, at()) }, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
					}},
					ui.IconButton{TestID: "player-recap-close", Icon: lucide.X, LabelFunc: func() string { return locales.PlayerClose(cx) }, Variant: ui.IconButtonGhost, Size: 36, OnPress: r.close},
				}},
				core.Show{When: func() bool { return c().busy }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
						core.Text{TestID: "player-recap-busy", ContentFunc: func() string { return locales.PlayerRecapBusy(cx, computer(p)) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
						ui.Skeleton{Lines: 3},
					}}
				}},
				core.View{TestID: "player-recap-items", Style: core.Style{Gap: 8}, Children: []core.Node{rows}},
				core.Show{When: func() bool { return !c().busy }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "player-recap-note", ContentFunc: func() string { return recapNote(cx, p, c()) }, LiveRegion: core.LiveRegionPolite, Style: look.Caption, StyleFunc: ui.Colored(colors.Faint)}
				}},
				look.Button{TestID: "player-recap-continue", Kind: look.Primary, Icon: lucide.Play, Text: func() string { return locales.PlayerRecapContinue(cx, at()) }, OnPress: r.resume},
				look.Button{TestID: "player-recap-start-over", Kind: look.Plain, Text: func() string { return locales.PlayerRecapStartOver(cx) }, OnPress: r.startOver},
			}},
		}},
	}}
}
