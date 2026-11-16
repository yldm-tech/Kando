package films

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// retranslateLook is what the confirmation of 重新翻译为 says: the film's name, the language it would be translated into and the one its translation is in now.
type retranslateLook struct {
	name, language, current string
}

// RetranslateDialog confirms translating a done film into another language than its subtitles are in, which replaces their translation and uses the AI service. 取消, Escape and closing the dialog keep the film as it is. The app draws it over the window, so the film page's 重新翻译为 asks too.
type RetranslateDialog struct{ App App }

// Render implements core.Node.
func (r RetranslateDialog) Render(cx *core.Scope) core.Node {
	a := r.App
	open := live.Func(a, func() bool {
		open, _, _ := a.Retranslating()
		return *open
	})
	look := live.Memo(cx, a, func() retranslateLook {
		_, item, language := a.Retranslating()
		return retranslateLook{name: filmName(item), language: language, current: item.Language}
	})
	cancel := live.Do(a, a.CloseRetranslate)
	translate := live.Do(a, a.Retranslate)
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the film and the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			l := look()
			return ui.Dialog{
				TestID:                 "retranslate-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  locales.FilmsRetranslateConfirm(cx, l.name, subtitle.TargetLanguageName(cx, l.language)),
				Description:            locales.FilmsRetranslateConfirmMessage(cx, subtitle.TargetLanguageName(cx, l.current)),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.FilmsCancel(cx), Variant: ui.ButtonSecondary, TestID: "retranslate-cancel", OnPress: cancel},
					ui.Button{Text: locales.FilmsTranslate(cx), TestID: "retranslate-confirm-run", OnPress: translate},
				},
			}
		})
	}}
}
