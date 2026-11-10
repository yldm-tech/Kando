package browse

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// startWidth is how wide the start's text runs at most, so its two sentences wrap into a short paragraph under the title rather than across the whole page.
const startWidth float32 = 460

// start is the page before a web page is open, and after home: the globe, what the page is for, and 清除登录信息… at its foot.
func start(c *ui.Context, a App, s State) {
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Width(startWidth).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			breeze.StatusPage(c, iconGlobeThin, locales.BrowseEmptyTitle(), locales.BrowseEmptyDescription(), nil)
		})
		ui.Row(c).Shrink(0).Justify(ui.Center).Padding(0, 0, 24, 0).Children(func() {
			c.Key("browse-clear")
			if breeze.Button(c, locales.BrowseClearData()).Disabled(s.Clearing).Clicked() {
				a.BrowseAskClear()
			}
		})
	})
}
