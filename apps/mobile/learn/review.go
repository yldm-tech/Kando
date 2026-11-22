package learn

import (
	"strconv"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"
	"kando/packages/study"

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

// The flip of a card: the design's sheet curve over half a second.
var flip = core.Transition{Duration: 500 * time.Millisecond, Easing: core.Curve{X1: 0.32, Y1: 0.72, X2: 0, Y2: 1}}

// ReviewPage reviews the items due when it opens, a card at a time, over the whole screen, as the Learn tab's review mode says (session.ReviewMode). As cards: the word or the line in the line it came from, which a press turns over to its meaning and the line's translation. As dictation: the line is heard from the film kept on the phone, the learner types it and Check marks what they heard (dictationCard); an item whose film is not on the phone is a card. Then whether the learner knew it, which schedules it (session.Grade); the two buttons say when it comes back, and wait for the answer. An item not known comes back at the end, until it is known. back leaves; the computers take the review at the next sync, which leaving starts.
func ReviewPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	due := learning.Due(s.LearnItems(), s.Now())
	run := core.NewSignalFunc(cx, reviewRun{queue: due, total: len(due)}, nil)
	dictating := s.ReviewMode() == session.ReviewDictation
	// clips are the lines' clips to dictate, by the items' keys, once read (session.LineClips).
	clips := core.NewSignalFunc[map[string]learning.Clip](cx, nil, nil)
	clipsRead := core.NewSignal(cx, !dictating)
	if dictating {
		cx.Post(func() {
			s.LineClips(due, func(m map[string]learning.Clip) {
				clips.Set(m)
				clipsRead.Set(true)
			})
		})
	}
	current := func() (learning.Item, bool) {
		r := run.Get()
		if r.at >= len(r.queue) {
			return learning.Item{}, false
		}
		return r.queue[r.at], true
	}
	done := func() bool { _, ok := current(); return !ok }
	// clip is the current item's clip, false when it is reviewed as a card.
	clip := func() (learning.Clip, bool) {
		it, ok := current()
		if !ok || !dictating {
			return learning.Clip{}, false
		}
		c, ok := clips.Get()[it.Key()]
		return c, ok
	}
	dictated := func() bool { _, ok := clip(); return ok }
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
	item := func() learning.Item { it, _ := current(); return it }
	shown := func() bool { return run.Get().shown }
	show := func() { r := run.GetUntracked(); r.shown = true; run.Set(r) }
	noFilm := core.Show{When: func() bool { return dictating && clipsRead.Get() && !dictated() }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "review-no-film", ContentFunc: func() string { return locales.DictationNoFilm(cx) }, Style: look.Caption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)}
	}}
	// ask is the item's own card: the dictation of its line, built anew for each card (the key of the card is its place in the review), or the card that turns over.
	type at struct {
		key string
		n   int
	}
	ask := core.Show{When: clipsRead.Get,
		Then: func(cx *core.Scope) core.Node {
			return core.Show{When: dictated,
				Then: func(cx *core.Scope) core.Node {
					card := func() []at {
						it, _ := current()
						return []at{{it.Key(), run.Get().at}}
					}
					return core.Each(card, func(a at) string { return a.key + "#" + strconv.Itoa(a.n) }, func(cx *core.Scope, _ func() at, _ func() int) core.Node {
						c, _ := clip()
						return dictationCard(cx, c, shown, show)
					})
				},
				Else: func(cx *core.Scope) core.Node { return flipCard(cx, s, item, shown, show) }}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.View{TestID: "review-loading", Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.Spinner{Size: 24}}}
		}}
	// The grades wait for the answer: the card turned over, the dictation checked.
	days := func() int {
		box := min(item().Box+1, len(study.Intervals))
		return int(study.Intervals[box-1] / (24 * time.Hour))
	}
	actions := core.View{Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Edges{Top: 12, Right: 20, Bottom: 6, Left: 20}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea},
		MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter}, Children: []core.Node{
		gradeButton(cx, "review-again", func() string { return locales.ReviewAgain(cx) }, false, shown, func() { grade(false) }),
		gradeButton(cx, "review-known", func() string { return locales.ReviewKnown(cx, days()) }, true, shown, func() { grade(true) }),
	}}
	r := func() reviewRun { return run.Get() }
	body := core.Show{When: done,
		Then: func(cx *core.Scope) core.Node { return finished(cx, r, leave) },
		Else: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, Padding: core.Edges{Top: 20, Right: 20, Left: 20}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea},
					MaxWidth: core.Pt(520), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 12}, Children: []core.Node{ask, noFilm}},
				actions,
			}}
		}}
	return base.NavPage{TitleFunc: func() string { return locales.ReviewTitle(cx) }, TestID: "review-page", HeaderHidden: true,
		NoPopOnBack: true, OnBackPress: leave, NoPopOnSwipe: true,
		StyleFunc: ui.Filled(colors.Background),
		Children:  []core.Node{progressRow(cx, r, leave), body}}
}

// progressRow is the review's top: the way out, how far the review is as a green bar, and the card's number of how many.
func progressRow(cx *core.Scope, run func() reviewRun, leave func()) core.Node {
	colors := ui.ColorsOf(cx)
	total := func() int { return len(run().queue) }
	through := func() int { return min(run().at, total()) }
	// The counter counts the items, as the summary at the end does: an item put back keeps its number when it comes again.
	counter := func() int {
		r := run()
		seen := map[string]bool{}
		for _, it := range r.queue[:min(r.at+1, len(r.queue))] {
			seen[it.Key()] = true
		}
		return min(len(seen), r.total)
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 8, Right: 16, Left: 8},
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: []core.Node{
		ui.IconButton{TestID: "review-end", Icon: closeIcon, Variant: ui.IconButtonGhost, Size: 44, IconSize: 14,
			LabelFunc: func() string { return locales.ReviewEnd(cx) }, OnPress: leave},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			base.Progress{TestID: "review-bar", LabelFunc: func() string { return locales.ReviewTitle(cx) },
				Value:          func() float32 { return 100 * float32(through()) / float32(max(total(), 1)) },
				TrackStyle:     core.Style{Height: core.Pt(6), CornerRadius: 3, Overflow: core.Hidden},
				TrackStyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(theme.TabInactive, 0.18)} },
				IndicatorStyleFunc: func() core.Style {
					return core.Style{Height: core.Pt(6), CornerRadius: 3, Background: theme.StatusOK,
						Transition: core.Transition{Duration: 300 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionLayout}}
				}},
		}},
		ui.Tabular(core.Text{TestID: "review-progress", ContentFunc: func() string {
			return locales.ReviewProgress(cx, counter(), run().total)
		}, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontFamily: core.FontMonospace}), StyleFunc: ui.Colored(colors.Muted)}),
	}}
}

// gradeButton is one of the two grades under the card, 56 tall: not known, white with danger's text, or known, on the ok green. It waits, faded, until ready reports true.
func gradeButton(cx *core.Scope, testID string, text func() string, good bool, ready func() bool, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, OnPress: func(core.PressEvent) { press() }, Disabled: func() bool { return !ready() },
		Style: core.Style{Height: core.Pt(56), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0),
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 6),
			Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.35)}},
		StyleFunc: func() core.Style {
			if good {
				return core.Style{Background: theme.StatusOK, Color: colors.OnAccent(), FocusVisible: ui.FocusRing(cx, -2)}
			}
			return core.Style{Background: colors.Surface(), Color: colors.Danger(), Shadows: []core.BoxShadow{core.Ring(0.5, ui.Alpha(colors.Text(), 0.08))}, FocusVisible: ui.FocusRing(cx, -2)}
		},
		Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, AdjustsFontSizeToFit: true, MinimumFontScale: 0.8, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightBold, FlexShrink: 1}}}}
}

// flipCard is a card of the review: on its front the film's poster fading into it, the word and how it is said, the line it came from with the word marked, and a press turns it over, in the dark, to the meaning, the line's translation, the learner's note and the film.
func flipCard(cx *core.Scope, s *session.Session, item func() learning.Item, shown func() bool, show func()) core.Node {
	colors := ui.ColorsOf(cx)
	still := ui.ReduceMotion(cx)
	text := func(get func(learning.Item) string) func() string { return func() string { return get(item()) } }
	has := func(get func(learning.Item) string) func() bool { return func() bool { return get(item()) != "" } }
	center := core.Style{TextAlign: core.TextAlignCenter}
	face := core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), CornerRadius: 26, BorderCurve: core.BorderCurveContinuous,
		Overflow: core.Hidden, BackfaceHidden: true}
	// turn is a face's turn about its upright axis: the front from 0 to 180 as the card turns over, the back from -180 to 0. It turns as the answer shows and comes back at once for the next card; under reduced motion the faces cross-fade instead.
	turn := func(back bool) core.Style {
		on := shown()
		st := core.Style{}
		if still {
			st.Opacity = core.Alpha(1)
			if on != back {
				st.Opacity = core.Alpha(0)
			}
			if on {
				st.Transition = core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionOpacity}
			}
			return st
		}
		deg := float32(0)
		switch {
		case !back && on:
			deg = 180
		case back && !on:
			deg = -180
		}
		st.Transform = core.Transform{RotateY: deg}
		if on {
			st.Transition = flip
		}
		return st
	}
	poster := core.Show{When: func() bool { src := posterOf(s, item()); return src.URI != "" || len(src.Data) > 0 }, Then: func(cx *core.Scope) core.Node {
		// The poster fades into the card from its middle down.
		fade := core.Gradient{Kind: core.GradientLinear, Angle: 180, Stops: []core.ColorStop{{Color: core.RGBA(0, 1), At: core.Pct(50)}, {Color: core.RGBA(0, 0), At: core.Pct(100)}}}
		return core.Image{TestID: "review-poster", SourceFunc: func() core.ImageSource { return posterOf(s, item()) }, ResizeMode: core.Cover, ElementsHidden: true,
			Style: core.Style{Width: core.Pct(100), Height: core.Pt(190), FlexShrink: 0, Mask: &fade}}
	}}
	front := core.View{TestID: "review-show", Role: core.RoleButton, LabelFunc: func() string { return locales.ReviewShow(cx) }, OnPress: func(core.PressEvent) { show() },
		Style: face.Merge(core.Style{Gap: 10, AlignItems: core.AlignCenter}),
		// Turned over, the front is behind the back for the screen reader too, which would otherwise read both faces.
		AccessibilityElementsHidden: shown,
		StyleFunc: func() core.Style {
			st := turn(false)
			st.Background = colors.Surface()
			if shown() {
				st.PointerEvents = core.PointerEventsNone
			}
			return st
		},
		Children: []core.Node{
			poster,
			core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 28, Right: 24, Left: 24}, AlignSelf: core.AlignStretch}, Children: []core.Node{
				core.Text{TestID: "review-text", ContentFunc: text(func(it learning.Item) string { return it.Text }), Style: core.Style{FontSize: 38, LineHeight: 44, FontWeight: look.Heavy, LetterSpacing: -0.5}.Merge(center), StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: has(func(it learning.Item) string { return it.Reading }), Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "review-reading", ContentFunc: text(func(it learning.Item) string { return it.Reading }), Style: look.Body.Merge(center), StyleFunc: ui.Colored(colors.Muted)}
				}},
				core.Show{When: has(func(it learning.Item) string { return it.Context }), Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "review-context", SpansFunc: func() []core.Span { return marked(item().Context, item().Text, look.Heavy) },
						Style: core.Style{FontSize: 17, LineHeight: 25, Margin: core.Edges{Top: 12}}.Merge(center), StyleFunc: ui.Colored(colors.Text)}
				}},
			}},
			core.View{Style: core.Style{FlexGrow: 1}},
			core.Text{TestID: "review-hint", ContentFunc: func() string { return locales.ReviewFlipHint(cx) }, Style: look.Caption.Merge(center).Merge(core.Style{Padding: core.Edges{Right: 24, Bottom: 24, Left: 24}}),
				StyleFunc: ui.Colored(func() core.Color { return theme.TabInactive })},
		}}
	line := func(id string, style core.Style, alpha float32, get func(learning.Item) string) core.Node {
		return core.Show{When: has(get), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: id, ContentFunc: text(get), Style: style.Merge(center), StyleFunc: func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), alpha)} }}
		}}
	}
	// The back is drawn in the dark palette, whatever the app's.
	back := core.View{TestID: "review-back", Style: face, AccessibilityElementsHidden: func() bool { return !shown() }, StyleFunc: func() core.Style { return turn(true) }, Children: []core.Node{
		core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
			theme.Dark(cx)
			colors := ui.ColorsOf(cx)
			return core.View{TestID: "review-answer", Style: core.Style{FlexGrow: 1, Gap: 12, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(30, 24)},
				StyleFunc: ui.Filled(colors.Surface),
				Children: []core.Node{
					core.Text{ContentFunc: text(func(it learning.Item) string { return it.Text }), Style: core.Style{FontSize: 30, LineHeight: 36, FontWeight: look.Heavy}.Merge(center), StyleFunc: ui.Colored(colors.Text)},
					line("review-meaning", core.Style{FontSize: 26, LineHeight: 34, FontWeight: core.FontWeightBold}, 1, func(it learning.Item) string { return it.Meaning }),
					line("review-context-translation", core.Style{FontSize: 15, LineHeight: 22}, 0.7, func(it learning.Item) string { return it.ContextTranslation }),
					line("review-note", core.Style{FontSize: 15, LineHeight: 22}, 0.7, func(it learning.Item) string { return it.Note }),
					core.Show{When: has(func(it learning.Item) string { return it.Title }), Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "review-film", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
							thumb(cx, s, item),
							core.Text{ContentFunc: text(func(it learning.Item) string { return it.Title }), MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexShrink: 1}),
								StyleFunc: func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.55)} }},
						}}
					}},
				}}
		}},
	}}
	return core.View{TestID: "review-card", Style: core.Style{FlexGrow: 1, MaxHeight: core.Pt(520), Perspective: 1200, CornerRadius: 26,
		Shadows: []core.BoxShadow{{Offset: core.Point{Y: 20}, Blur: 50, Color: theme.CardShadow}}}, Children: []core.Node{front, back}}
}

// finished is what the review says once every item is through: a check, how many it went through and how many times one was not known, and OK, which leaves.
func finished(cx *core.Scope, run func() reviewRun, leave func()) core.Node {
	colors := ui.ColorsOf(cx)
	center := core.Style{TextAlign: core.TextAlignCenter}
	ok := func() string { return locales.CommonOK(cx) }
	check := core.View{Style: core.Style{Width: core.Pt(96), Height: core.Pt(96), CornerRadius: 48, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Margin: core.Edges{Bottom: 16},
		Gradients: []core.Gradient{core.LinearGradient(135, theme.DoneFrom, theme.DoneTo)},
		Shadows:   []core.BoxShadow{{Offset: core.Point{Y: 16}, Blur: 36, Color: theme.DoneShadow}}},
		Children: []core.Node{ui.Icon(doneIcon, 44, colors.OnAccent)}}
	return core.View{TestID: "review-done", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(32, 32)}, Children: []core.Node{
		check,
		core.Text{ContentFunc: func() string { return locales.ReviewDoneTitle(cx) }, Role: core.RoleHeader, Style: core.Style{FontSize: 26, LineHeight: 32, FontWeight: look.Heavy}.Merge(center), StyleFunc: ui.Colored(colors.Text)},
		core.Text{TestID: "review-done-summary", ContentFunc: func() string { return locales.ReviewDoneSummary(cx, run().total, run().again) },
			Style: core.Style{FontSize: 15, LineHeight: 22, MaxWidth: core.Pt(340)}.Merge(center), StyleFunc: ui.Colored(colors.Muted)},
		core.View{Style: core.Style{Width: core.Pt(220), Margin: core.Edges{Top: 24}}, Children: []core.Node{
			// The design's OK is taller and rounder than look.Button, in heavier type.
			base.Button{TestID: "review-finish", LabelFunc: ok, OnPress: func(core.PressEvent) { leave() },
				Style: core.Style{Height: core.Pt(52), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Active: &core.Style{Opacity: core.Alpha(0.7)}},
				StyleFunc: func() core.Style {
					return core.Style{Background: colors.Accent(), Color: colors.OnAccent(), FocusVisible: ui.FocusRing(cx, -2)}
				},
				Children: []core.Node{core.Text{ContentFunc: ok, MaxLines: 1, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightBold}}}},
		}},
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
