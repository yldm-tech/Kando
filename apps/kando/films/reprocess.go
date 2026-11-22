package films

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// reprocessLook is what the confirmation of 重新处理 says: the film's name and the target language it would be translated into, or, for several films, how many they are (count) and the language they would all be translated into, "" when each keeps its own.
type reprocessLook struct {
	name, language string
	count          int
}

// ReprocessDialog confirms 重新处理 before a done film, or each of several selected, is transcribed and translated again, which costs whisper's time and the AI service's translation. 取消, Escape and closing the dialog keep the film as it is. The app draws it over the window, so the film page's 重新处理整部影片 asks too.
type ReprocessDialog struct{ App App }

// Render implements core.Node.
func (r ReprocessDialog) Render(cx *core.Scope) core.Node {
	a := r.App
	open := live.Func(a, func() bool {
		open, _, _ := a.Reprocessing()
		return *open
	})
	// What the confirmation says is worked out again only when the films it is about change, not on every change of the app's state.
	look := live.Memo(cx, a, func() reprocessLook {
		_, items, language := a.Reprocessing()
		name := ""
		if len(items) == 1 {
			name = filmName(items[0])
		}
		return reprocessLook{name: name, language: language, count: len(items)}
	})
	cancel := live.Do(a, a.CloseReprocess)
	reprocess := live.Do(a, func() {
		open, items, _ := a.Reprocessing()
		if *open {
			a.CloseReprocess()
			a.ReprocessFilms(items...)
		}
	})
	return core.Show{When: open, Then: func(cx *core.Scope) core.Node {
		title := func() string {
			if l := look(); l.count > 1 {
				return locales.FilmsReprocessConfirmMany(cx, l.count)
			}
			return locales.FilmsReprocessConfirm(cx, look().name)
		}
		message := func() string {
			l := look()
			switch {
			case l.count > 1 && l.language == "":
				return locales.FilmsReprocessManyOwnMessage(cx)
			case l.count > 1:
				return locales.FilmsReprocessManyMessage(cx, subtitle.TargetLanguageName(cx, l.language))
			}
			return locales.FilmsReprocessConfirmMessage(cx, subtitle.TargetLanguageName(cx, l.language))
		}
		return ui.Dialog{
			TestID:                 "reprocess-confirm",
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
				ui.Button{TextFunc: func() string { return locales.FilmsCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "reprocess-cancel", OnPress: cancel},
				ui.Button{TextFunc: func() string { return locales.FilmsReprocess(cx) }, TestID: "reprocess-confirm-run", OnPress: reprocess},
			},
		}
	}}
}
