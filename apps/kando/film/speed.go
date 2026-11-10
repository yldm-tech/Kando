package film

import (
	"strconv"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Rates are the speeds a film plays at, slowest first, as multiples of the speed it was recorded at. The speed menu lists them and ⇧, and ⇧. step through them.
var Rates = [...]float64{0.5, 0.75, 1, 1.25, 1.5, 2}

// RateText is how the player writes a speed, as "1.5×".
func RateText(rate float64) string {
	return locales.FilmSpeedRate(strconv.FormatFloat(rate, 'f', -1, 64))
}

// speedMenu is the transport's speed button, named for the speed the film plays at, and the menu it opens above it, as the subtitle button's: the speeds from slowest to fastest, the one playing checked.
func (s screen) speedMenu(c *ui.Context, t *ui.Theme) {
	c.Key("film-speed")
	rate := RateText(s.Rate())
	open := s.SpeedMenu()
	btn := breeze.FlatButton(c, "").Label(locales.FilmSpeedLabel(rate)).Tooltip(locales.FilmSpeed() + " " + breeze.ShortcutText(c, ui.Shift, ui.KeyComma) + " / " + breeze.ShortcutText(c, ui.Shift, ui.KeyPeriod))
	if btn.Clicked() {
		*open = !*open
	}
	if *open {
		btn.Background(ui.RGBA(255, 255, 255, 0.2))
	}
	// As wide as the widest speed, 1.25×, so the scrubber beside it keeps its length whatever the speed.
	btn.Height(30).MinWidth(50).Justify(ui.Center).Padding(0, 8).Radius(15).Gap(0).TextColor(onGlass).Children(func() {
		ui.Text(c, rate).FontSize(12).FontFeatures("tnum").SingleLine()
	})
	menu := breeze.Popover(c, btn, open, func() {
		ui.Column(c).Width(168).Children(func() {
			ui.Text(c, locales.FilmSpeed()).FontSize(11).FontWeight(700).Opacity(0.55).Padding(6, 10, 4, 10)
			for _, r := range Rates {
				s.speedRow(c, t, r)
			}
		})
	})
	// Placed as the subtitle menu is: its right edge 6 past the button's and its bottom 14 above the button's top.
	menu.AttachTo(btn, ui.AnchorTopEnd, ui.AnchorBottomEnd).Padding(6).Margin(0, 0, 14, 0).Right(-6)
}

// speedRow is a radio row of the speed menu: a check while the film plays at rate, the speed, and for 1× a note that it is the normal speed.
func (s screen) speedRow(c *ui.Context, t *ui.Theme, rate float64) {
	c.Key("film-speed-" + strconv.FormatFloat(rate, 'f', -1, 64))
	on := s.Rate() == rate
	row := ui.ButtonBase(c).Padding(6, 10).Gap(8).Radius(8).AlignItems(ui.Center).Label(RateText(rate)).Role(ui.RoleRadio).Checked(on)
	hoverFace(row, t)
	if row.Clicked() {
		s.SetRate(rate)
		*s.SpeedMenu() = false
	}
	row.Children(func() {
		check := ""
		if on {
			check = "✓"
		}
		ui.Text(c, check).FontSize(13).Width(14).Shrink(0)
		ui.Text(c, RateText(rate)).FontSize(13).FontFeatures("tnum").Grow(1)
		if rate == 1 {
			ui.Text(c, locales.FilmSpeedNormal()).FontSize(11).Opacity(0.5).Shrink(0)
		}
	})
}
