package films

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// notice is the message bar under the top bar, as the prototype draws it: amber with a warning triangle for what went wrong, green with a check mark for what worked, 打开设置 when a setting fixes what it says, 打开 when it names a film in the library, and × to close it.
func notice(c *ui.Context, a App) {
	n := a.Notice()
	if !n.Open {
		return
	}
	mark, fill, ring, ink := warningMark, ui.Hex("#FFF7E3"), ui.RGBA(180, 120, 0, 0.25), ui.Hex("#5A3A00")
	if n.Kind == "success" {
		mark, fill, ring, ink = okMark, ui.Hex("#EAF7EE"), ui.RGBA(30, 130, 60, 0.25), ui.Hex("#14532D")
	}
	// The bar is 42 points high with one line of text, its hairline included.
	bar := ui.Row(c).MinHeight(42).Margin(0, 0, 14, 0).Padding(8.5, 10, 8.5, 14).Gap(10).AlignItems(ui.Center).Radius(14).Background(fill).Border(0.5, ring)
	bar.Role(ui.RoleStatus).Label(n.Text).Children(func() {
		ui.Image(c, mark).Size(16, 16).Shrink(0)
		ui.Text(c, n.Text).FontSize(13).TextColor(ink).Grow(1).MinWidth(0).Wrap()
		if n.Section != "" {
			c.Key("notice-settings")
			if breeze.Button(c, locales.FilmsOpenSettings()).Height(26).Shrink(0).Clicked() {
				a.OpenSettings(n.Section)
			}
		}
		if n.Film != "" {
			c.Key("notice-film")
			if breeze.Button(c, locales.QueueAlreadyOpen()).Height(26).Shrink(0).Clicked() {
				if item, ok := a.JobFilm(n.Film); ok {
					n.Open = false
					a.Open(item)
				}
			}
		}
		c.Key("notice-close")
		close := ui.ButtonBase(c).Label(locales.FilmsCloseNotice()).Size(24, 24).Radius(12).Shrink(0).AlignItems(ui.Center).Justify(ui.Center)
		if close.Hovered() {
			close.Background(ui.RGBA(0, 0, 0, 0.06))
		}
		if close.Clicked() {
			n.Open = false
		}
		close.Children(func() {
			ui.Text(c, "×").FontSize(15).TextColor(ink)
		})
	})
}
