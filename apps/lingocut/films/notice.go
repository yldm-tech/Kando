package films

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// notice is the message bar under the top bar, with 打开设置 when a setting fixes what it says.
func notice(c *ui.Context, a App) {
	n := a.Notice()
	if !n.Open {
		return
	}
	var actions func()
	if n.Section != "" {
		actions = func() {
			c.Key("notice-settings")
			if breeze.Button(c, "打开设置").Shrink(0).Clicked() {
				a.OpenSettings(n.Section)
			}
		}
	}
	breeze.InfoBarActions(c, n.Kind, n.Text, "", &n.Open, actions)
}
