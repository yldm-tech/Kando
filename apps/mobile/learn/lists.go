package learn

import (
	"strings"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

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
	return ui.NavPage(cx, base.NavPage{TitleFunc: title, TestID: id + "-page", Children: []core.Node{
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
	remove := func() {
		item := it()
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
			}
		})
	}
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
