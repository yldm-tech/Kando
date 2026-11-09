package film

import (
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ModeName is what the player calls a subtitle mode, on its button, in its menu and on the OSD, in the interface's language.
func ModeName(mode string) string { return subtitle.ModeName(mode) }

// modeChoices are the mode menu's rows, in the prototype's order (template 619-636), with the key that reaches each; modeNote is the line under it.
var modeChoices = []struct {
	mode string
	key  ui.Key
}{
	{SubOff, ui.KeyV},
	{SubOriginal, ui.KeyS},
	{SubTranslation, ui.KeyS},
	{SubBilingual, ui.KeyS},
}

// modeNote is the line under a row of the mode menu, or "" for a row without one.
func modeNote(mode string) string {
	switch mode {
	case SubOff:
		return locales.FilmModeOffNote()
	case SubTranslation:
		return locales.FilmModeTranslationNote()
	}
	return ""
}

// modeMenu is the transport's subtitle button, named for the mode, and the menu it opens above it: the four modes, the subtitle delay and 样式…, which opens the style view in the side panel.
func (s screen) modeMenu(c *ui.Context, t *ui.Theme) {
	c.Key("film-mode")
	name := ModeName(s.SubMode())
	open := s.ModeMenu()
	// The prototype's mode button: 30 high, its glyph and the mode's name, on white at 20% while its menu is open.
	btn := breeze.FlatButton(c, "").Label(locales.FilmSubtitlesMode(name)).Tooltip(locales.FilmSubtitles() + " " + breeze.ShortcutText(c, 0, ui.KeyS) + " / " + breeze.ShortcutText(c, 0, ui.KeyV))
	if btn.Clicked() {
		*open = !*open
	}
	if *open {
		btn.Background(ui.RGBA(255, 255, 255, 0.2))
	}
	btn.Height(30).Padding(0, 8).Radius(15).Gap(0).TextColor(onGlass).Children(func() {
		ui.Icon(c, iconSubtitles).Size(20, 15).Shrink(0).Margin(0, 6, 0, 0)
		ui.Text(c, name).FontSize(12).SingleLine()
	})
	menu := breeze.Popover(c, btn, open, func() {
		ui.Column(c).Width(258).Children(func() {
			ui.Text(c, locales.FilmModeHeading()).FontSize(11).FontWeight(700).Opacity(0.55).Padding(6, 10, 4, 10)
			for _, choice := range modeChoices {
				s.modeRow(c, t, choice.mode, modeNote(choice.mode), choice.key)
			}
			ui.Box(c).Height(0.5).Margin(5, 10).Background(ui.RGBA(255, 255, 255, 0.15))
			s.delayRow(c)
			c.Key("film-style")
			row := ui.ButtonBase(c).Padding(7, 10, 7, 32).Radius(8).Justify(ui.Start).Label(locales.FilmStyleMenuItem())
			hoverFace(row, t)
			if row.Clicked() {
				*open = false
				s.SetStyleView(true)
			}
			row.Children(func() { ui.Text(c, locales.FilmStyleMenuItem()).FontSize(13) })
		})
	})
	// The prototype opens the menu above the button, its right edge 6 past the button's and its bottom 14 above the button's top, over the picture rather than the panel.
	menu.AttachTo(btn, ui.AnchorTopEnd, ui.AnchorBottomEnd).Padding(6).Margin(0, 0, 14, 0).Right(-6)
}

// modeRow is a radio row of the mode menu: a check while chosen, the mode's name with a line under it, and its key at the end.
func (s screen) modeRow(c *ui.Context, t *ui.Theme, mode, note string, key ui.Key) {
	c.Key("film-mode-" + mode)
	on := s.SubMode() == mode
	row := ui.ButtonBase(c).Padding(6, 10).Gap(8).Radius(8).AlignItems(ui.Start).Label(ModeName(mode)).Role(ui.RoleRadio).Checked(on)
	hoverFace(row, t)
	if row.Clicked() {
		s.SetSubMode(mode)
		*s.ModeMenu() = false
	}
	row.Children(func() {
		check := ""
		if on {
			check = "✓"
		}
		ui.Text(c, check).FontSize(13).Width(14).Shrink(0)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, ModeName(mode)).FontSize(13)
			if note != "" {
				ui.Text(c, note).FontSize(11).Opacity(0.6).Wrap()
			}
		})
		ui.Text(c, breeze.ShortcutText(c, 0, key)).FontSize(11).Opacity(0.5).Shrink(0)
	})
}

// delayRow moves the subtitles against the picture by a tenth of a second, as Z and X do.
func (s screen) delayRow(c *ui.Context) {
	ui.Row(c).Padding(4, 10, 4, 32).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.FilmDelay()).FontSize(13).Grow(1)
		ui.Text(c, DelayText(s.Delay())).FontSize(12).FontFeatures("tnum").MinWidth(46).TextAlign(ui.Center).Shrink(0)
		c.Key("film-delay")
		v := s.Delay().Seconds()
		if breeze.Stepper(c, &v, -30, 30, 0.1).Label(locales.FilmDelay()).Changed() {
			s.SetDelay(seconds(v).Round(100 * time.Millisecond))
		}
	})
}

// hoverFace lights a menu row while the pointer is over it, as a menu's rows do.
func hoverFace(row ui.Element, t *ui.Theme) {
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
}
