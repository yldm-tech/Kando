// Package quit draws the confirmation of quitting Kando while a job is running.
package quit

import (
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Confirm asks, while Open returns true, whether to quit although the job making Film is still running. 退出 runs OnQuit; 取消, Enter, Escape and closing the dialog run OnCancel, which keep Kando running and the job with it: 取消 is the default answer, as it was MyGo's alert's default button. The message names the film Film returns as it changes, as the job running may be the next one by the time the user answers.
type Confirm struct {
	Open     func() bool
	Film     func() string
	OnQuit   func()
	OnCancel func()
}

// Render implements core.Node.
func (c Confirm) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	// Film may follow the app's whole revision (live.Func), which changes on every keystroke and progress report; the memo lets the dialog below be built again only when the film's name itself changes.
	film := core.NewMemo(cx, c.Film)
	cancel := func() {
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
	return core.Show{When: c.Open, Then: func(cx *core.Scope) core.Node {
		// Enter answers 取消, the default answer, wherever the keyboard focus is in the dialog; the binding goes when the dialog does.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		// The dialog's words are set as it is built, so it is built again for another film or language.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the film and the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Dialog{
				TestID:                 "quit-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  locales.QueueQuitWhileRunningTitle(cx),
				Description:            locales.QueueQuitWhileRunningMessage(cx, film.Get()),
				OnOpenChange: func(open bool, reason base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.QueueQuitWhileRunningCancel(cx), Variant: ui.ButtonSecondary, TestID: "quit-cancel", OnPress: cancel},
					ui.Button{Text: locales.QueueQuitWhileRunningQuit(cx), Color: colors.Danger, TestID: "quit-quit", OnPress: c.OnQuit},
				},
			}
		})
	}}
}
