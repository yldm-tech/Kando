package films

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// reprocessLook is what the confirmation of 重新处理 says: the film's name and the target language it would be translated into.
type reprocessLook struct {
	name, language string
}

// ReprocessDialog confirms 重新处理 before a done film is transcribed and translated again, which costs whisper's time and the AI service's translation. 取消, Escape and closing the dialog keep the film as it is. The app draws it over the window, so the film page's 重新处理整部影片 asks too.
type ReprocessDialog struct{ App App }

// Render implements core.Node.
func (r ReprocessDialog) Render(cx *core.Scope) core.Node {
	a := r.App
	open := live.Func(a, func() bool {
		open, _, _ := a.Reprocessing()
		return *open
	})
	// The confirmation is built again only when what it says changes, not on every change of the app's state.
	look := live.Memo(cx, a, func() reprocessLook {
		_, item, language := a.Reprocessing()
		return reprocessLook{name: filmName(item), language: language}
	})
	cancel := live.Do(a, a.CloseReprocess)
	reprocess := live.Do(a, func() {
		open, item, _ := a.Reprocessing()
		if *open {
			a.CloseReprocess()
			a.Reprocess(item)
		}
	})
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the film and the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			l := look()
			return ui.Dialog{
				TestID:                 "reprocess-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  locales.FilmsReprocessConfirm(cx, l.name),
				Description:            locales.FilmsReprocessConfirmMessage(cx, subtitle.TargetLanguageName(cx, l.language)),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.FilmsCancel(cx), Variant: ui.ButtonSecondary, TestID: "reprocess-cancel", OnPress: cancel},
					ui.Button{Text: locales.FilmsReprocess(cx), TestID: "reprocess-confirm-run", OnPress: reprocess},
				},
			}
		})
	}}
}
