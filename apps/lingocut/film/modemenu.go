package film

import (
	"time"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// ModeName is what the player calls a subtitle mode, on its button, in its menu and on the OSD.
func ModeName(mode string) string {
	switch mode {
	case SubOff:
		return "关闭"
	case SubOriginal:
		return "仅原文"
	case SubTranslation:
		return "仅译文"
	default:
		return "双语"
	}
}

// modeChoices are the mode menu's rows, in the prototype's order (template 619-636), with the line under each and the key that reaches it.
var modeChoices = []struct {
	mode, note string
	key        ui.Key
}{
	{SubOff, "只隐藏字幕", ui.KeyV},
	{SubOriginal, "", ui.KeyS},
	{SubTranslation, "译文没好时显示弱化的原文", ui.KeyS},
	{SubBilingual, "", ui.KeyS},
}

// modeMenu is the transport's subtitle button, named for the mode, and the menu it opens above it: the four modes, the subtitle delay and 样式…, which opens the style view in the side panel.
func (s screen) modeMenu(c *ui.Context, t *ui.Theme) {
	c.Key("film-mode")
	name := ModeName(s.SubMode())
	open := s.ModeMenu()
	btn := breeze.IconLabelButton(c, iconSubtitles, name).Label("字幕：" + name).Tooltip("字幕 " + breeze.ShortcutText(c, 0, ui.KeyS) + " / " + breeze.ShortcutText(c, 0, ui.KeyV))
	if btn.Clicked() {
		*open = !*open
	}
	breeze.Popover(c, btn, open, func() {
		ui.Column(c).Width(258).Gap(2).Children(func() {
			breeze.Text(c, "显示", breeze.TextCaption|breeze.TextSecondary).Padding(4, 8, 2, 8)
			for _, choice := range modeChoices {
				s.modeRow(c, t, choice.mode, choice.note, choice.key)
			}
			breeze.Divider(c).Margin(4, 0)
			s.delayRow(c)
			c.Key("film-style")
			row := ui.ButtonBase(c).Padding(6, 8, 6, 30).Radius(6).Justify(ui.Start).Label("样式…")
			hoverFace(row, t)
			if row.Clicked() {
				*open = false
				s.SetStyleView(true)
			}
			row.Children(func() { ui.Text(c, "样式…") })
		})
	})
}

// modeRow is a radio row of the mode menu: a check while chosen, the mode's name with a line under it, and its key at the end.
func (s screen) modeRow(c *ui.Context, t *ui.Theme, mode, note string, key ui.Key) {
	c.Key("film-mode-" + mode)
	on := s.SubMode() == mode
	row := ui.ButtonBase(c).Padding(6, 8).Gap(8).Radius(6).AlignItems(ui.Start).Label(ModeName(mode)).Role(ui.RoleRadio).Checked(on)
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
		ui.Text(c, check).Width(14).Shrink(0)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, ModeName(mode))
			if note != "" {
				breeze.Text(c, note, breeze.TextCaption|breeze.TextSecondary).Wrap()
			}
		})
		breeze.Text(c, breeze.ShortcutText(c, 0, key), breeze.TextCaption|breeze.TextSecondary).Shrink(0)
	})
}

// delayRow moves the subtitles against the picture by a tenth of a second, as Z and X do.
func (s screen) delayRow(c *ui.Context) {
	ui.Row(c).Padding(4, 8, 4, 30).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "字幕延迟").Grow(1)
		breeze.Text(c, DelayText(s.Delay()), breeze.TextSecondary).FontFeatures("tnum").Shrink(0)
		c.Key("film-delay")
		v := s.Delay().Seconds()
		if breeze.Stepper(c, &v, -30, 30, 0.1).Label("字幕延迟").Changed() {
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
