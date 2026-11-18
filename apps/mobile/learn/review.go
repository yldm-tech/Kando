package learn

import (
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// reviewRun is a review under way: the items to go through, those the learner did not know put back at its end, and how far it is.
type reviewRun struct {
	queue []learning.Item
	at    int
	// total is how many items the review started with, again how many times one was not known.
	total, again int
	// shown is set while the answer shows.
	shown bool
}

// ReviewPage reviews the items due when it opens, a card at a time: the word or the line, then on a press its meaning, the line it came from and its translation, and whether the learner knew it, which schedules it (session.Grade). An item not known comes back at the end, until it is known. back leaves; the computers take the review at the next sync, which leaving starts.
func ReviewPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	due := learning.Due(s.LearnItems(), s.Now())
	run := core.NewSignalFunc(cx, reviewRun{queue: due, total: len(due)}, nil)
	current := func() (learning.Item, bool) {
		r := run.Get()
		if r.at >= len(r.queue) {
			return learning.Item{}, false
		}
		return r.queue[r.at], true
	}
	done := func() bool { _, ok := current(); return !ok }
	grade := func(good bool) {
		r := run.GetUntracked()
		it, ok := current()
		if !ok {
			return
		}
		s.Grade(it, good)
		r.queue = append([]learning.Item(nil), r.queue...)
		if !good {
			r.again++
			if !slicesContainsLater(r.queue, r.at, it) {
				it.Box = 0
				r.queue = append(r.queue, it)
			}
		}
		r.at++
		r.shown = false
		run.Set(r)
	}
	leave := func() {
		s.SyncLearning()
		back()
	}
	text := func(get func(learning.Item) string) func() string {
		return func() string { it, _ := current(); return get(it) }
	}
	has := func(get func(learning.Item) string) func() bool {
		return func() bool { it, _ := current(); return get(it) != "" }
	}
	shown := func() bool { return run.Get().shown }
	line := func(style core.Style, color func() core.Color, id string, get func(learning.Item) string) core.Node {
		return core.Show{When: has(get), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: id, ContentFunc: text(get), Style: style.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(color)}
		}}
	}
	front := core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{TestID: "review-text", ContentFunc: text(func(it learning.Item) string { return it.Text }), Style: look.Display.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
		line(look.Subhead, colors.Muted, "review-reading", func(it learning.Item) string {
			if !run.Get().shown {
				return ""
			}
			return it.Reading
		}),
		line(look.Body, colors.Muted, "review-context", func(it learning.Item) string { return it.Context }),
	}}
	answer := core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "review-answer", Style: core.Style{Gap: 8, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 16}}, Children: []core.Node{
			core.View{Style: core.Style{Height: core.Pt(0.5), AlignSelf: core.AlignStretch}, StyleFunc: ui.Filled(colors.Line)},
			line(look.Heading, colors.Accent, "review-meaning", func(it learning.Item) string { return it.Meaning }),
			line(look.Body, colors.Muted, "review-context-translation", func(it learning.Item) string { return it.ContextTranslation }),
			line(look.Subhead, colors.Muted, "review-note", func(it learning.Item) string { return it.Note }),
		}}
	}}
	from := line(look.Caption, colors.Faint, "review-film", func(it learning.Item) string {
		if it.Title == "" {
			return ""
		}
		return locales.ReviewFrom(cx, it.Title)
	})
	cardView := core.View{TestID: "review-card", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, Padding: core.All(24), Gap: 8, CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} },
		Children:  []core.Node{front, answer, from}}
	actions := core.Show{When: shown,
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
				look.Button{TestID: "review-again", Kind: look.Tinted, Flex: 1, Text: func() string { return locales.ReviewAgain(cx) }, OnPress: func() { grade(false) }},
				look.Button{TestID: "review-known", Kind: look.Primary, Flex: 1, Text: func() string { return locales.ReviewKnown(cx) }, OnPress: func() { grade(true) }},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return look.Button{TestID: "review-show", Kind: look.Primary, Text: func() string { return locales.ReviewShow(cx) },
				OnPress: func() { r := run.GetUntracked(); r.shown = true; run.Set(r) }}
		}}
	progress := func() string {
		r := run.Get()
		return locales.ReviewProgress(cx, min(r.at+1, len(r.queue)), len(r.queue))
	}
	body := core.Show{When: done,
		Then: func(cx *core.Scope) core.Node { return finished(cx, func() reviewRun { return run.Get() }, leave) },
		Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, Gap: 16}, Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, Padding: core.Edges{Top: 8, Right: 16, Left: 16}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea},
					MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 12}, Children: []core.Node{
					ui.Tabular(core.Text{TestID: "review-progress", ContentFunc: progress, Style: look.Caption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)}),
					cardView,
				}},
				look.ActionBar(cx, actions),
			}}
		}}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.ReviewTitle(cx) }, TestID: "review-page", StyleFunc: ui.Filled(colors.Background), Children: []core.Node{body}},
		ui.NavHeader{OnBack: leave})
}

// finished is what the review says once every item is through: how many it went through and how many to see again.
func finished(cx *core.Scope, run func() reviewRun, leave func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "review-done", Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 8, Padding: core.All(32)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.ReviewDoneTitle(cx) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.Text{TestID: "review-done-count", ContentFunc: func() string { return locales.ReviewDoneCount(cx, run().total) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
			core.Text{TestID: "review-done-again", ContentFunc: func() string { return locales.ReviewDoneAgain(cx, run().again) }, Style: look.Body.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
		}},
		look.ActionBar(cx, look.Button{TestID: "review-finish", Kind: look.Primary, Text: func() string { return locales.ReviewDone(cx) }, OnPress: leave}),
	}}
}

// slicesContainsLater reports whether it comes again in queue after at, as one put back already.
func slicesContainsLater(queue []learning.Item, at int, it learning.Item) bool {
	for _, q := range queue[at+1:] {
		if q.Key() == it.Key() {
			return true
		}
	}
	return false
}
