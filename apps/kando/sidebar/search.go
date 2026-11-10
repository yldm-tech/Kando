package sidebar

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// search is the field under the library switcher that narrows the library page to the done films whose names hold what is typed, as `kando list [query]` does, beside the sources it narrows with. It shows while the open library has films. ⌘F puts the keyboard in it (QueryFocus), and its clear button or Escape empties it. Its look is the settings window's search field in its rail: 28 points high, its corners 8, gray on the card without a border.
func search(c *ui.Context, a App) {
	focus := a.QueryFocus()
	if a.FilmCount() == 0 {
		// Without the field a request for its focus has nothing to go to; it is dropped, so the field does not take the focus when it shows later.
		*focus = false
		return
	}
	ui.Row(c).Padding(0, insetEnd, 10, insetStart).AlignItems(ui.Stretch).Children(func() {
		c.Key("sidebar-search")
		field := breeze.SearchField(c, a.Query()).Placeholder(locales.SidebarSearchPlaceholder() + "  " + breeze.ShortcutText(c, ui.Cmd, ui.KeyF)).Label(locales.SidebarSearchLabel())
		field.Grow(1).MinWidth(0).Height(28).Radius(8).Border(0, ui.Transparent).Background(ui.RGBA(0, 0, 0, 0.06)).FontSize(13)
		if *focus {
			field.Focus()
			*focus = false
		}
	})
}
