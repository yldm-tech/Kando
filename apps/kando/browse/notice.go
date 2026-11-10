package browse

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// notice is the message bar under the toolbar: what went wrong, or that something worked, with 打开设置 when a setting fixes what it says, and its close button.
func notice(c *ui.Context, a App) {
	n := a.BrowseNotice()
	if !n.Open {
		return
	}
	ui.Column(c).Shrink(0).AlignItems(ui.Stretch).Padding(10, 14, 0, 14).Children(func() {
		var actions func()
		if n.Section != "" {
			actions = func() {
				c.Key("browse-notice-settings")
				if breeze.Button(c, locales.BrowseOpenSettings()).Clicked() {
					a.OpenSettings(n.Section)
				}
			}
		}
		breeze.InfoBarActions(c, n.Kind, n.Text, "", &n.Open, actions)
	})
}
