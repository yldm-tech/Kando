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
	// Film may follow the app's whole revision (live.Func), which changes on every keystroke and progress report; the memo lets the message be written again only when the film's name itself changes.
	film := core.NewMemo(cx, c.Film)
	cancel := func() {
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
	return core.Show{When: c.Open, Then: func(cx *core.Scope) core.Node {
		// Enter answers 取消, the default answer, wherever the keyboard focus is in the dialog; the binding goes when the dialog does.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		return ui.Dialog{
			TestID:                 "quit-confirm",
			Role:                   core.RoleAlertDialog,
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              func() string { return locales.QueueQuitWhileRunningTitle(cx) },
			DescriptionFunc:        func() string { return locales.QueueQuitWhileRunningMessage(cx, film.Get()) },
			OnOpenChange: func(open bool, reason base.Reason) {
				if !open {
					cancel()
				}
			},
			Footer: []core.Node{
				ui.Button{TextFunc: func() string { return locales.QueueQuitWhileRunningCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "quit-cancel", OnPress: cancel},
				ui.Button{TextFunc: func() string { return locales.QueueQuitWhileRunningQuit(cx) }, Color: colors.Danger, TestID: "quit-quit", OnPress: c.OnQuit},
			},
		}
	}}
}
