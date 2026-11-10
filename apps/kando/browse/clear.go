package browse

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// clearDialog confirms 清除登录信息… before the pages' cookies and storage go. 取消 is the default, so Enter keeps them.
func clearDialog(c *ui.Context, a App) {
	open := a.BrowseClearOpen()
	if !*open {
		return
	}
	if breeze.DestructiveAlertDialog(c, open, locales.BrowseClearTitle(), locales.BrowseClearMessage(), 0, locales.BrowseClearConfirm(), locales.BrowseCancel()) == 0 {
		a.BrowseClear()
	}
}
