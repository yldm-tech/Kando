package browse

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// toolbarHeight is the toolbar's height on macOS, where it lies under the window's hidden title bar beside the sidebar's card and drags the window, as a unified toolbar does.
const toolbarHeight float32 = 52

// toolbar is the row along the top of the page: back, forward, reload (stop while the page loads) and home, the address field growing between them and 下载视频 at the end. On macOS it is the window's title bar beside the sidebar and drags the window; elsewhere the content's drag band lies above it.
func toolbar(c *ui.Context, t *ui.Theme, a App, s State) {
	mac := breeze.PlatformOf(c) == breeze.Mac
	bar := ui.Row(c).Shrink(0).FillWidth().AlignItems(ui.Center).Gap(10).Padding(9, 14, 9, 14).Background(t.Background)
	if mac {
		bar.MinHeight(toolbarHeight).DragWindow()
	}
	line := ui.RGBA(0, 0, 0, 0.1)
	bar.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 0.5, W: r.W, H: 0.5}, line, 0)
	})
	bar.Children(func() {
		ui.Row(c).Shrink(0).Gap(2).AlignItems(ui.Center).Children(func() {
			c.Key("browse-back")
			if breeze.FlatIconButton(c, iconBack, locales.BrowseBack()).Disabled(!s.CanGoBack).Clicked() {
				a.BrowseBack()
			}
			c.Key("browse-forward")
			if breeze.FlatIconButton(c, iconForward, locales.BrowseForward()).Disabled(!s.CanGoForward).Clicked() {
				a.BrowseForward()
			}
			if s.Loading {
				c.Key("browse-stop")
				if breeze.FlatIconButton(c, iconStop, locales.BrowseStop()).Clicked() {
					a.BrowseStop()
				}
			} else {
				c.Key("browse-reload")
				if breeze.FlatIconButton(c, iconReload, locales.BrowseReload()).Disabled(!s.Page).Clicked() {
					a.BrowseReload()
				}
			}
			c.Key("browse-home")
			if breeze.FlatIconButton(c, iconHome, locales.BrowseHome()).Disabled(!s.Page).Clicked() {
				a.BrowseHome()
			}
		})
		addressField(c, t, a)
		tools := breeze.Toolbar(c, func() {
			c.Key("browse-download")
			// The library's toolbar pills are 34 points high; the platform draws their faces.
			download := breeze.PrimaryButton(c, locales.BrowseDownload()).Height(34).Radius(17).PaddingX(18).Tooltip(locales.BrowseDownloadTooltip()).Disabled(!s.Page)
			if download.Clicked() {
				a.BrowseDownload()
			}
		}).Shrink(0)
		tools.Padding(0).MinHeight(0)
	})
}

// addressField is the pill the address is typed into, with the globe at its start and the library's focus ring around it while it has the keyboard. Enter opens what it holds.
func addressField(c *ui.Context, t *ui.Theme, a App) {
	field := ui.Row(c).Grow(1).MinWidth(160).Height(34).Radius(17).Padding(0, 12, 0, 12).Gap(8).AlignItems(ui.Center)
	field.Background(ui.RGBA(255, 255, 255, 0.75)).Border(0.5, ui.RGBA(0, 0, 0, 0.12)).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.06))
	focused := false
	field.Children(func() {
		ui.Icon(c, iconGlobe).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
		c.Key("browse-address")
		in := ui.TextInputBase(c, a.BrowseAddress()).Placeholder(locales.BrowseAddressPlaceholder()).Label(locales.BrowseAddressLabel()).Grow(1).MinWidth(0).FontSize(13).FocusRing(false)
		if focus := a.BrowseAddressFocus(); *focus {
			in.Focus()
			*focus = false
		}
		if in.Submitted() {
			a.BrowseGo()
		}
		focused = in.Focused()
	})
	a.BrowseAddressFocused(focused)
	if focused {
		ring := t.Accent.Alpha(0.5)
		field.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Stroke(ui.Rect{X: r.X - 2.5, Y: r.Y - 2.5, W: r.W + 5, H: r.H + 5}, ring, 19.5, 3)
		})
	}
}
