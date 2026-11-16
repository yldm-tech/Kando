package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// clearDialog confirms 清除登录信息… before the pages' cookies and storage go. 取消 is the default, so Enter keeps them, as do Escape and closing the dialog.
type clearDialog struct{ a App }

// Render implements core.Node.
func (d clearDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	cancel := live.Do(a, a.BrowseCancelClear)
	return core.Show{When: live.Func(a, a.BrowseClearOpen), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		// Enter answers 取消, the default answer, wherever the keyboard focus is in the dialog; the binding goes when the dialog does.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		// The dialog's words are set as it is built, so it is built again for another language; it reads nothing of the app's.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Dialog{
				TestID:                 "browse-clear-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  locales.BrowseClearTitle(cx),
				Description:            locales.BrowseClearMessage(cx),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.BrowseCancel(cx), Variant: ui.ButtonSecondary, TestID: "browse-clear-cancel", OnPress: cancel},
					ui.Button{Text: locales.BrowseClearConfirm(cx), Color: colors.Danger, TestID: "browse-clear-confirm-button", OnPress: live.Do(a, a.BrowseClear)},
				},
			}
		})
	}}
}
