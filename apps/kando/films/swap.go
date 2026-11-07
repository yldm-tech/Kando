package films

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// swapSheet is 换个链接: a field for another link to the same video, for a card whose link cannot be downloaded, and 取消 or 换成这个链接 to run the card again from it. Escape and 取消 close it.
func swapSheet(c *ui.Context, t *ui.Theme, a App) {
	open, item, link, refused := a.Swapping()
	if !*open {
		return
	}
	breeze.Sheet(c, open, func() {
		ui.Column(c).Width(440).MaxWidth(440).Gap(14).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				breeze.Text(c, "换个链接", breeze.TextHeading).FontSize(15).Role(ui.RoleHeading).Level(1)
				breeze.Text(c, "「"+filmName(item)+"」从这个链接下载不了。别处有这个视频的话，把它的链接贴在这里，会在这张卡片上重新开始。", breeze.TextCaption, breeze.TextSecondary).FontSize(12).Wrap()
			})
			breeze.Card(c, func() {
				c.Key("swap-link")
				breeze.EntryRow(c, "链接", link)
			})
			if refused != "" {
				ui.Text(c, refused).FontSize(12).TextColor(t.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				c.Key("swap-cancel")
				if breeze.Button(c, "取消").Clicked() {
					*open = false
				}
				c.Key("swap-go")
				if breeze.PrimaryButton(c, "换成这个链接").Clicked() {
					a.SwapLink()
				}
			})
		})
	})
}
