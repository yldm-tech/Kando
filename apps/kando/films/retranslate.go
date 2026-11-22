package films

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// retranslateLook is what the confirmation of 重新翻译为 says: the film's name, the language it would be translated into and the one its translation is in now; or, for several films, how many they are (count) and how many of their translations it replaces (replaced).
type retranslateLook struct {
	name, language, current string
	count, replaced         int
}

// RetranslateDialog confirms translating a done film, or several selected, into another language than its subtitles are in, which replaces their translation and uses the AI service. 取消, Escape and closing the dialog keep the film as it is. The app draws it over the window, so the film page's 重新翻译为 asks too.
type RetranslateDialog struct{ App App }

// Render implements core.Node.
func (r RetranslateDialog) Render(cx *core.Scope) core.Node {
	a := r.App
	open := live.Func(a, func() bool {
		open, _, _ := a.Retranslating()
		return *open
	})
	look := live.Memo(cx, a, func() retranslateLook {
		_, items, language := a.Retranslating()
		if len(items) == 1 {
			return retranslateLook{name: filmName(items[0]), language: language, current: items[0].Language, count: 1}
		}
		return retranslateLook{language: language, count: len(items), replaced: len(Replacing(items, language))}
	})
	cancel := live.Do(a, a.CloseRetranslate)
	translate := live.Do(a, a.Retranslate)
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		title := func() string {
			l := look()
			if l.count > 1 {
				return locales.FilmsRetranslateConfirmMany(cx, l.count, subtitle.TargetLanguageName(cx, l.language))
			}
			return locales.FilmsRetranslateConfirm(cx, l.name, subtitle.TargetLanguageName(cx, l.language))
		}
		message := func() string {
			l := look()
			if l.count > 1 {
				return locales.FilmsRetranslateManyMessage(cx, l.replaced)
			}
			return locales.FilmsRetranslateConfirmMessage(cx, subtitle.TargetLanguageName(cx, l.current))
		}
		return ui.Dialog{
			TestID:                 "retranslate-confirm",
			Role:                   core.RoleAlertDialog,
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              title,
			DescriptionFunc:        message,
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Footer: []core.Node{
				ui.Button{TextFunc: func() string { return locales.FilmsCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "retranslate-cancel", OnPress: cancel},
				ui.Button{TextFunc: func() string { return locales.FilmsTranslate(cx) }, TestID: "retranslate-confirm-run", OnPress: translate},
			},
		}
	}}
}

// Replacing is the films of items whose translation translating them into language replaces with one in another language, which is what the confirmation of 重新翻译为 asks about: those translated into another language. A film that was not translated at all has no translation to replace, whatever language its job was asked for.
func Replacing(items []Film, language string) []Film {
	var out []Film
	for _, item := range items {
		if current := translatedInto(item); current != "" && current != language {
			out = append(out, item)
		}
	}
	return out
}
