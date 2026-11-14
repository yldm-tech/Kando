// Package quit draws the confirmation of quitting Kando while a job is running.
package quit

import (
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Confirm asks, while Open returns true, whether to quit although the job making Film is still running. 退出 runs OnQuit; 取消, Escape and closing the dialog run OnCancel, which keep Kando running and the job with it. The message names the film Film returns as it changes, as the job running may be the next one by the time the user answers.
type Confirm struct {
	Open     func() bool
	Film     func() string
	OnQuit   func()
	OnCancel func()
}

// Render implements core.Node.
func (c Confirm) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: c.Open, Then: func(cx *core.Scope) core.Node {
		// The dialog's words are set as it is built, so it is built again for another film or language.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			film := c.Film()
			return ui.AlertDialog{
				TestID:      "quit-confirm",
				Open:        func() bool { return true },
				Title:       locales.QueueQuitWhileRunningTitle(cx),
				Description: locales.QueueQuitWhileRunningMessage(cx, film),
				OnOpenChange: func(open bool, reason base.Reason) {
					if !open && reason != base.ReasonSelect && c.OnCancel != nil {
						c.OnCancel()
					}
				},
				Actions: []ui.AlertAction{
					{Text: locales.QueueQuitWhileRunningCancel(cx), Variant: ui.ButtonSecondary, TestID: "quit-cancel", OnPress: c.OnCancel},
					{Text: locales.QueueQuitWhileRunningQuit(cx), Color: colors.Danger, TestID: "quit-quit", OnPress: c.OnQuit},
				},
			}
		})
	}}
}
