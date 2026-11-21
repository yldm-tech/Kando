package study

import (
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/saved"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ReviewApp is what the review reads of the line it shows and the answers it reports. It reads it inside its bindings through live.Func and calls its actions through live.Do, so what they change shows at once (package live).
type ReviewApp interface {
	live.State
	// ReviewCard is the line the review shows now, or that it is done.
	ReviewCard() Card
	// ReviewReveal shows the line's translation; ReviewGrade tells how the learner knew the line, good or again, which schedules it and goes on to the next; ReviewReplay plays its sound again from its start, and ReviewClipEnded is told when the sound has played to its end.
	ReviewReveal()
	ReviewGrade(good bool)
	ReviewReplay()
	ReviewClipEnded()
	// CloseReview ends the review.
	CloseReview()
}

// Card is what the review shows: a saved line, its film, where in it it starts, its original and its translation, with the learner's Note on it, or with Word set a word of the vocabulary, the word as Original, its Reading, its meaning as Translation and the line it was looked up in, Context, with that line's ContextTranslation; whether the translation is shown, and the line's sound, the file Clip, "" while it is cut, which plays while Playing; Plays counts the times it was started, so each one plays it from its start. NoAudio is set for a line whose sound cannot be had. Left is how many are left, this one with them, and Reviewed how many were answered; Done is set once none is left.
type Card struct {
	Done                        bool
	Reviewed, Left              int
	Word                        bool
	Film, Original, Translation string
	Reading, Note               string
	Context, ContextTranslation string
	Start                       time.Duration
	Revealed                    bool
	Clip                        string
	NoAudio, Playing            bool
	Plays                       int
}

// Review is the review of the saved lines and the words due, one at a time over the window: the line's sound plays and its original shows, or the word with its reading over the line it was looked up in, its translation, the note on the line or the word's meaning hidden until Show Translation or Space; then Again or Good, or 1 and 2, tell how the learner knew it, which goes on to the next. Play Again or R plays its sound again. Once none is left it says how many were reviewed. Done, Escape and closing it run CloseReview.
type Review struct {
	App ReviewApp
}

// Render implements core.Node.
func (r Review) Render(cx *core.Scope) core.Node {
	a := r.App
	colors := ui.ColorsOf(cx)
	card := live.Func(a, a.ReviewCard)
	done := live.Memo(cx, a, func() bool { return card().Done })
	revealed := live.Memo(cx, a, func() bool { return card().Revealed })
	title := func() string { return locales.SavedReviewTitle(cx) }
	// The keys go with the dialog: Space shows the translation, 1 and 2 answer once it shows, R plays the sound again.
	cx.OnKeyWith("space", core.KeyOptions{When: core.KeyAlways, OverModals: true}, live.Do(a, func() {
		if c := a.ReviewCard(); !c.Done && !c.Revealed {
			a.ReviewReveal()
		}
	}))
	grade := func(good bool) func() {
		return live.Do(a, func() {
			if c := a.ReviewCard(); !c.Done && c.Revealed {
				a.ReviewGrade(good)
			}
		})
	}
	cx.OnKeyWith("1", core.KeyOptions{When: core.KeyAlways, OverModals: true}, grade(false))
	cx.OnKeyWith("2", core.KeyOptions{When: core.KeyAlways, OverModals: true}, grade(true))
	cx.OnKeyWith("r", core.KeyOptions{When: core.KeyAlways, OverModals: true}, live.Do(a, func() {
		if !a.ReviewCard().Done {
			a.ReviewReplay()
		}
	}))
	head := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1}, Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, LabelFunc: title, Children: []core.Node{
			core.Text{ContentFunc: title, Style: ui.TypeTitle, StyleFunc: ui.Colored(colors.Text)},
		}},
		core.Show{When: func() bool { return !done() }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "review-left", ContentFunc: func() string { return locales.SavedReviewLeft(cx, card().Left) }, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
		}},
	}}
	body := core.Show{When: done,
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Text{TestID: "review-done", ContentFunc: func() string {
					if n := card().Reviewed; n > 0 {
						return locales.SavedReviewDone(cx, n)
					}
					return locales.SavedReviewNothingDue(cx)
				}, Style: ui.TypeBody.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
			}}
		},
		Else: func(cx *core.Scope) core.Node { return lineCard(cx, a, card, revealed) },
	}
	footer := core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd, FlexShrink: 0}, Children: []core.Node{
		core.Show{When: func() bool { return !done() && revealed() }, Then: func(cx *core.Scope) core.Node {
			return core.Group([]core.Node{
				textButton(locales.SavedReviewAgain, ui.Button{Variant: ui.ButtonSecondary, TestID: "review-again", OnPress: grade(false)}),
				textButton(locales.SavedReviewGood, ui.Button{TestID: "review-good", OnPress: grade(true)}),
			})
		}},
		core.Show{When: func() bool { return !done() && !revealed() }, Then: func(cx *core.Scope) core.Node {
			return textButton(locales.SavedReviewShow, ui.Button{TestID: "review-show", OnPress: live.Do(a, a.ReviewReveal)})
		}},
		core.Show{When: done, Then: func(cx *core.Scope) core.Node {
			return textButton(locales.SavedDone, ui.Button{TestID: "review-close", OnPress: live.Do(a, a.CloseReview)})
		}},
	}}
	// TODO(nsgo#145): give base.Dialog (ui.Dialog) LabelFunc and AccessibilityDescriptionFunc twins, so the review's accessible name follows the interface language as it changes while it is open; it is read as the review opens https://github.com/nsgo-dev/nsgo/issues/145
	return ui.Dialog{
		TestID: "review",
		Label:  locales.SavedReviewTitle(cx),
		Open:   func() bool { return true },
		Bare:   true,
		Style:  core.Style{Width: core.Pt(520), Height: core.Pt(400)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseReview()
				a.Changed()
			}
		},
		Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 16, Padding: core.All(20)}, Children: []core.Node{head, body, footer, clipPlayer(cx, a, card)}}},
	}
}

// lineCard is the line or the word under review: its film and time, Play Again, its original, or the word with its reading over the line it was looked up in, and once revealed its translation and the learner's note on it, or the word's meaning and the line's translation.
func lineCard(cx *core.Scope, a ReviewApp, card func() Card, revealed func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	where := func() string { c := card(); return locales.SavedWhere(cx, c.Film, saved.Clock(c.Start)) }
	replay := func() string { return locales.SavedReviewReplay(cx) }
	shown := func(text func() string) func() bool { return func() bool { return text() != "" } }
	reading := func() string { return card().Reading }
	context := func() string { return card().Context }
	note := func() string { return card().Note }
	contextTranslation := func() string { return card().ContextTranslation }
	return core.View{Style: core.Style{FlexGrow: 1, Gap: 12}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.Text{ContentFunc: where, MaxLines: 1, Style: ui.TypeCaption.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
			core.Show{When: func() bool { return card().NoAudio }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "review-no-audio", ContentFunc: func() string { return locales.SavedReviewNoAudio(cx) }, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
			}, Else: func(cx *core.Scope) core.Node {
				return textButton(func(*core.Scope) string { return replay() }, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "review-replay", OnPress: live.Do(a, a.ReviewReplay),
					Disabled: func() bool { return card().Clip == "" }})
			}},
		}},
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignBaseline, Gap: 10}, Children: []core.Node{
			core.Text{TestID: "review-original", ContentFunc: func() string { return card().Original }, Style: core.Style{FontSize: 22, LineHeight: 30, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
			core.Show{When: shown(reading), Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "review-reading", ContentFunc: reading, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}},
		core.Show{When: shown(context), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "review-context", ContentFunc: context, Style: core.Style{FontSize: 15, LineHeight: 21, FontStyle: core.FontStyleItalic}, StyleFunc: ui.Colored(colors.Text)}
		}},
		core.Show{When: revealed, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				core.Text{TestID: "review-translation", ContentFunc: func() string { return card().Translation }, Style: core.Style{FontSize: 16, LineHeight: 22}, StyleFunc: ui.Colored(colors.Muted)},
				core.Show{When: shown(contextTranslation), Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "review-context-translation", ContentFunc: contextTranslation, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: ui.Colored(colors.Muted)}
				}},
				core.Show{When: shown(note), Then: func(cx *core.Scope) core.Node {
					k := theme.Of(cx)
					return core.View{TestID: "review-note", Role: core.RoleGroup, LabelFunc: func() string { return locales.SavedNote(cx) },
						Style:     core.Style{Padding: core.Symmetric(6, 8), CornerRadius: 6, BorderWidth: 1},
						StyleFunc: func() core.Style { return core.Style{Background: k.WarnFill(), BorderColor: k.WarnRing()} },
						Children:  []core.Node{core.Text{ContentFunc: note, Style: ui.TypeBody, StyleFunc: ui.Colored(k.WarnText)}}}
				}},
			}}
		}},
	}}
}

// clipPlayer plays the line's sound, Card.Clip, from its start each time Card.Plays counts another play, and tells ReviewClipEnded when it has played to its end. It is the system's video player with nothing to show, kept out of sight.
// TODO(nsgo#1050): play the clip with audio.Player once every desktop plays audio (core.Env.CanPlayAudio): macOS does now, but Windows and Linux still have no audio backend and report it does not; a core.Video without a picture plays it meanwhile https://github.com/nsgo-dev/nsgo/issues/1050
func clipPlayer(cx *core.Scope, a ReviewApp, card func() Card) core.Node {
	ref := &core.Ref{}
	clip := live.Memo(cx, a, func() string { return card().Clip })
	plays := live.Memo(cx, a, func() int { return card().Plays })
	playing := live.Memo(cx, a, func() bool { return card().Playing })
	played := core.Untracked(cx, plays)
	cx.Effect(func() {
		n := plays()
		if n != played && clip() != "" {
			played = n
			ref.Seek(0)
		}
	})
	return core.Video{TestID: "review-clip", Ref: ref, SourceFunc: clip, PausedFunc: func() bool { return !playing() || clip() == "" },
		OnEnd: live.Do(a, a.ReviewClipEnded),
		Style: core.Style{Position: core.Absolute, Width: core.Pt(1), Height: core.Pt(1), Opacity: core.Alpha(0)}}
}
