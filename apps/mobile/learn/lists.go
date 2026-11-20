package learn

import (
	"strings"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// VocabularyPage lists the words saved, the latest first: the word and how it is said, its meaning, the film it was looked up in and when it is next due; each can be removed, on every device, once the learner confirms.
func VocabularyPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	return listPage(cx, s, learning.Word, "vocabulary", func() string { return locales.LearnVocabulary(cx) }, func() string { return locales.VocabEmpty(cx) }, back)
}

// LinesPage lists the lines saved, the latest first: the line, its translation, the learner's note and the film it is from; each can be removed, on every device, once the learner confirms.
func LinesPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	return listPage(cx, s, learning.Line, "lines", func() string { return locales.LearnSavedLines(cx) }, func() string { return locales.LinesEmpty(cx) }, back)
}

func listPage(cx *core.Scope, s *session.Session, kind learning.Kind, id string, title, empty func() string, back func()) base.NavPage {
	list := core.NewMemoFunc(cx, func() []learning.Item {
		var out []learning.Item
		for _, it := range s.LearnItems() {
			if it.Kind == kind {
				out = append(out, it)
			}
		}
		return out
	}, nil)
	ref := &core.Ref{}
	return look.NavPage(cx, base.NavPage{TitleFunc: title, TestID: id + "-page", Children: []core.Node{
		core.Show{When: func() bool { return len(list.Get()) == 0 },
			Then: func(cx *core.Scope) core.Node {
				return look.Empty{TestID: id + "-empty", Icon: lucide.BookOpen, Title: empty}
			},
			Else: func(cx *core.Scope) core.Node {
				return core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
					core.View{StyleFunc: look.Column(cx), Children: []core.Node{
						look.Group{TestID: id + "-list", Children: []core.Node{
							core.Each(list.Get, func(it learning.Item) string { return it.Key() }, func(cx *core.Scope, it func() learning.Item, i func() int) core.Node {
								return itemRow(cx, s, it, i, id)
							}),
						}},
					}},
				}}
			}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// itemRow is a word's or a line's row, with its options, which remove it.
func itemRow(cx *core.Scope, s *session.Session, it func() learning.Item, i func() int, id string) core.Node {
	colors := ui.ColorsOf(cx)
	title := func() string {
		if r := it().Reading; r != "" && it().Kind == learning.Word {
			return it().Text + "  " + r
		}
		return it().Text
	}
	detail := func() string {
		if it().Note != "" {
			return it().Meaning + "\n" + it().Note
		}
		return it().Meaning
	}
	below := core.Text{ContentFunc: func() string { return facts(cx, it(), s.Now()) }, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Faint)}
	remove := func() { confirmRemove(cx, s, it(), nil) }
	return look.Row{TestID: id + "-item-" + it().ID, Index: i, TitleLines: 3,
		Title: title, Detail: detail, Below: below,
		Trailing: ui.IconButton{TestID: id + "-remove-" + it().ID, Icon: lucide.Ellipsis, Variant: ui.IconButtonGhost, Size: 44, IconSize: 20,
			LabelFunc: func() string { return locales.LearnItemOptions(cx, it().Text) }, OnPress: remove}}
}

// facts are an item's film and its schedule: due now, mastered, or due in so many days.
func facts(cx *core.Scope, it learning.Item, now time.Time) string {
	var parts []string
	if it.Title != "" {
		parts = append(parts, it.Title)
	}
	switch {
	case it.IsDue(now):
		parts = append(parts, locales.LearnDueNow(cx))
	case it.Mastered():
		parts = append(parts, locales.LearnMastered(cx))
	default:
		parts = append(parts, locales.LearnDueIn(cx, max(1, int((it.Due.Sub(now)+23*time.Hour)/(24*time.Hour)))))
	}
	return strings.Join(parts, " · ")
}

// confirmRemove asks whether to remove item, on every device, and removes it once the learner confirms, then calls done when set.
func confirmRemove(cx *core.Scope, s *session.Session, item learning.Item, done func()) {
	t, msg := locales.VocabRemoveTitle(cx, item.Text), locales.VocabRemoveMessage(cx)
	if item.Kind == learning.Line {
		t, msg = locales.LinesRemoveTitle(cx), locales.LinesRemoveMessage(cx)
	}
	// A tablet asks in a dialog in the middle of the screen, as its sheets show, rather than an action sheet with nothing to point at.
	cx.ShowAlert(core.AlertConfig{Title: t, Message: msg, Sheet: !look.Tablet(cx), Cancelable: true, Actions: []core.AlertAction{
		{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
		{Label: locales.LearnRemove(cx), Role: core.AlertDestructive},
	}}, func(i int) {
		if i == 1 {
			s.RemoveItem(item)
			if done != nil {
				done()
			}
		}
	})
}

// kept is the film it is from as kept on the phone, false when it is not.
func kept(s *session.Session, it learning.Item) (store.Film, bool) {
	if it.Film == "" {
		return store.Film{}, false
	}
	return s.Film(store.Key(it.Desktop, it.Film))
}

// playable reports whether the film it is from is whole on the phone, so its player can show the place: one still downloading, or waiting to, is not.
func playable(s *session.Session, it learning.Item) bool {
	f, ok := kept(s, it)
	return ok && f.Done()
}

// posterOf is the poster of the film it is from: the one kept on the phone, or one the Library fetched this session; none otherwise.
func posterOf(s *session.Session, it learning.Item) core.ImageSource {
	if f, ok := kept(s, it); ok && f.Poster != "" {
		return look.LocalPoster(s, f)
	}
	if data := s.KnownPoster(store.Key(it.Desktop, it.Film)); len(data) > 0 {
		return core.ImageSource{Data: data}
	}
	return core.ImageSource{}
}

// thumb is the small poster of the film it is from, 32 by 18, as a line's card and a word's sheet show it beside the film's title.
func thumb(cx *core.Scope, s *session.Session, it func() learning.Item) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(18), CornerRadius: 3, Overflow: core.Hidden, FlexShrink: 0}, StyleFunc: ui.Filled(colors.Inset),
		Children: []core.Node{core.Image{SourceFunc: func() core.ImageSource { return posterOf(s, it()) }, ResizeMode: core.Cover, ElementsHidden: true,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}}
}

// marked is text with each time word comes in it marked on a yellow band, in weight; text alone where word is not in it.
func marked(text, word string, weight core.FontWeight) []core.Span {
	var spans []core.Span
	for word != "" {
		i := strings.Index(text, word)
		if i < 0 {
			break
		}
		if i > 0 {
			spans = append(spans, core.Span{Content: text[:i]})
		}
		spans = append(spans, core.Span{Content: word, Style: core.Style{FontWeight: weight, Background: theme.Highlight, CornerRadius: 3}})
		text = text[i+len(word):]
	}
	if text != "" {
		spans = append(spans, core.Span{Content: text})
	}
	return spans
}

// lineCard is a saved line on its card: the line and its translation, the learner's note on its yellow box, and the film it is from with where in it, which plays it there when the film is on the phone. A long press, or the screen reader's action, removes it.
func lineCard(cx *core.Scope, s *session.Session, it func() learning.Item, play func(string, time.Duration)) core.Node {
	colors, own := ui.ColorsOf(cx), theme.Of(cx)
	remove := func() { confirmRemove(cx, s, it(), nil) }
	note := core.Show{When: func() bool { return it().Note != "" }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.Symmetric(8, 10), CornerRadius: 10, Background: theme.NoteBox}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return it().Note }, Style: look.Caption, StyleFunc: ui.Colored(own.NoteText)},
		}}
	}}
	footer := []core.Node{
		thumb(cx, s, it),
		core.Text{ContentFunc: func() string { return it().Title }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
		core.Text{ContentFunc: func() string { return look.Duration(it().Start) }, MaxLines: 1, Style: look.Footnote.Merge(core.Style{FontFamily: core.FontMonospace}), StyleFunc: ui.Colored(colors.Muted)},
		// The play button shows while the film is whole on the phone, following it as it finishes downloading or is deleted; it keeps a 28-point target over the footer's 18.
		core.Show{When: func() bool { return playable(s, it()) }, Then: func(cx *core.Scope) core.Node {
			return base.Button{TestID: "learn-line-play-" + it().ID, LabelFunc: func() string { return locales.LearnLinePlay(cx) },
				OnPress:  func(core.PressEvent) { play(store.Key(it().Desktop, it().Film), it().Start) },
				Style:    core.Style{Width: core.Pt(18), Height: core.Pt(18), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.6)}},
				HitSlop:  core.All(13),
				Children: []core.Node{ui.Icon(playIcon, 14, colors.Accent)}}
		}},
	}
	// TODO(nsgo): core.AccessibilityAction's Label is a string, with no LabelFunc, so the card that names its remove action is built again as the language changes.
	id := "learn-line-" + it().ID
	return core.Dynamic(func(cx *core.Scope) core.Node {
		return core.View{TestID: id, Style: core.Style{Padding: core.Symmetric(14, 16), Gap: 6}, StyleFunc: func() core.Style { return look.Card(cx) },
			OnLongPress:           func(core.PressEvent) { remove() },
			AccessibilityActions:  []core.AccessibilityAction{{Name: "remove", Label: locales.LearnRemove(cx)}},
			OnAccessibilityAction: func(string) { remove() },
			Children: []core.Node{
				core.Text{ContentFunc: func() string { return it().Text }, Style: core.Style{FontSize: 16, LineHeight: 22, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return it().Meaning != "" }, Then: func(cx *core.Scope) core.Node {
					// The translation is the ink a little lighter, as a word's meaning is, rather than the muted grey.
					return core.Text{ContentFunc: func() string { return it().Meaning }, Style: core.Style{FontSize: 14, LineHeight: 20}, StyleFunc: func() core.Style { return core.Style{Color: ui.Alpha(colors.Text(), 0.85)} }}
				}},
				note,
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Margin: core.Edges{Top: 6}}, Children: footer},
			}}
	})
}
