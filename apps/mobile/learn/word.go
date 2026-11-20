package learn

import (
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// wordSheet is the sheet of the word under key's key, from the word list: the word and how it is said, its meaning, the line it was looked up in with its translation and the film, the way back to that place in the film when it is on the phone, and its removal from the list.
func wordSheet(cx *core.Scope, s *session.Session, items func() []learning.Item, key *core.Signal[string], play func(string, time.Duration)) core.Node {
	colors := ui.ColorsOf(cx)
	item := func() (learning.Item, bool) {
		k := key.Get()
		for _, it := range items() {
			if it.Key() == k {
				return it, true
			}
		}
		return learning.Item{}, false
	}
	// last is the word the sheet shows: the one open, kept while the sheet closes.
	last := core.NewSignal(cx, learning.Item{})
	cx.Effect(func() {
		if i, ok := item(); ok {
			last.Set(i)
		}
	})
	it := last.Get
	open := func() bool { _, ok := item(); return ok }
	closeSheet := func() { key.Set("") }
	button := func(id string, text func() string, accent bool, press func()) core.Node {
		return base.Button{TestID: id, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
			Style: core.Style{Height: core.Pt(50), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0),
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 16), Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				if accent {
					return core.Style{Background: colors.Surface(), Color: colors.Accent(), FocusVisible: ui.FocusRing(cx, -2)}
				}
				return core.Style{Background: colors.Inset(), Color: colors.Text(), FocusVisible: ui.FocusRing(cx, -2)}
			},
			Children: []core.Node{core.Text{ContentFunc: text, MaxLines: 1, AdjustsFontSizeToFit: true, MinimumFontScale: 0.8, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightBold, FlexShrink: 1}}}}
	}
	buttons := []core.Node{
		// The way back to the film shows while the film is whole on the phone.
		core.Show{When: func() bool { return playable(s, it()) }, Then: func(cx *core.Scope) core.Node {
			return button("word-back-to-film", func() string { return locales.LearnBackToFilm(cx) }, true, func() {
				w := it()
				closeSheet()
				play(store.Key(w.Desktop, w.Film), w.Start)
			})
		}},
		button("word-remove", func() string { return locales.LearnRemoveFromList(cx) }, false, func() {
			confirmRemove(cx, s, it(), closeSheet)
		}),
	}
	// The speaker says the word in its film's language, where the platform speaks text.
	speaker := core.Show{When: func() bool { return cx.Env().CanSpeak() && it().Text != "" }, Then: func(cx *core.Scope) core.Node {
		return base.Button{TestID: "word-say", LabelFunc: func() string { return locales.LearnWordSay(cx) },
			// Speak refuses only a language that is no BCP 47 tag, as a film whose language was detected may have; the word is said in the platform's voice then.
			OnPress: func(core.PressEvent) {
				if cx.Speak(it().Text, core.Voice{Lang: it().Spoken}, nil) != nil {
					_ = cx.Speak(it().Text, core.Voice{}, nil)
				}
			},
			Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Active: &core.Style{Opacity: core.Alpha(0.7)}},
			StyleFunc: func() core.Style {
				return core.Style{Background: ui.Alpha(colors.Accent(), 0.1), FocusVisible: ui.FocusRing(cx, -2)}
			},
			Children: []core.Node{ui.Icon(lucide.Volume2, 20, colors.Accent)}}
	}}
	content := func() core.Node {
		line := core.Show{When: func() bool { return it().Context != "" }, Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				look.GroupHeader(cx, func() string { return locales.LearnInThisLine(cx) }),
				core.View{TestID: "word-line", Style: core.Style{Padding: core.Symmetric(14, 16), Gap: 8}, StyleFunc: func() core.Style { return look.Card(cx) }, Children: []core.Node{
					core.Text{SpansFunc: func() []core.Span { return marked(it().Context, it().Text, look.Heavy) }, Style: look.Body.Merge(core.Style{FontSize: 17, LineHeight: 24}), StyleFunc: ui.Colored(colors.Text)},
					core.Show{When: func() bool { return it().ContextTranslation != "" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return it().ContextTranslation }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)}
					}},
					core.Show{When: func() bool { return it().Title != "" }, Then: func(cx *core.Scope) core.Node {
						return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
							thumb(cx, s, it),
							core.Text{ContentFunc: func() string { return it().Title }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
						}}
					}},
				}},
			}}
		}}
		return core.View{TestID: "word-sheet", Style: core.Style{Gap: 16}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 12}, Children: []core.Node{
				core.View{Style: core.Style{Gap: 4, FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
					core.Text{TestID: "word-text", ContentFunc: func() string { return it().Text }, Role: core.RoleHeader, Style: core.Style{FontSize: 30, LineHeight: 36, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
					core.Show{When: func() bool { return it().Reading != "" }, Then: func(cx *core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return it().Reading }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)}
					}},
				}},
				speaker,
			}},
			core.Show{When: func() bool { return it().Meaning != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Padding: core.Symmetric(14, 16)}, StyleFunc: func() core.Style { return look.Card(cx) }, Children: []core.Node{
					core.Text{TestID: "word-meaning", ContentFunc: func() string { return it().Meaning }, Style: core.Style{FontSize: 18, LineHeight: 24, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
				}}
			}},
			line,
			// The buttons share a row, shrinking their text where it is long.
			core.View{Style: core.Style{Direction: core.Row, Gap: 12, Margin: core.Edges{Top: 4}}, Children: buttons},
		}}
	}
	return look.Dialog{TestID: "word-dialog", Open: open, OnClose: closeSheet, TitleHidden: true, Title: func() string { return it().Text }, Children: []core.Node{content()}}
}
