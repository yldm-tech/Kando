package film

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// stylePresets are the style view's presets, as the prototype has them (script 3272): outlined, on boxes, and outlined larger.
var stylePresets = []struct {
	id, label  string
	box        bool
	tran, orig float32
	// sample is the size of the 字幕 its card shows.
	sample float32
}{
	{"outline", "描边", false, 30, 22, 15},
	{"box", "底框", true, 30, 22, 15},
	{"large", "大字", false, 42, 30, 20},
}

// styleColors are the colors a line can take.
var styleColors = []struct{ hex, name string }{{"#FFFFFF", "白色"}, {"#FFD60A", "黄色"}, {"#FF9F0A", "橙色"}}

var styleFonts = []string{"默认", "苹方", "冬青黑体"}

// styleView is the subtitle style, in the side panel in place of its tabs, reached from the mode menu's 样式…: the presets, the font, the order of the two lines and their place, then the size, color and weight of each line.
func (s screen) styleView(c *ui.Context) {
	look := s.Cue()
	changed := false
	ui.Row(c).Gap(6).Padding(12, 12, 8, 12).AlignItems(ui.Center).Children(func() {
		c.Key("film-style-back")
		if glyphButton(c, iconChevronSmall, "返回侧栏", 28, 28, 7, 11).Clicked() {
			s.SetStyleView(false)
		}
		ui.Text(c, "字幕样式").FontSize(14).Bold()
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).WidthPercent(100).Padding(0, 14, 16, 14).Gap(14).Children(func() {
			preset := 0
			for i, p := range stylePresets {
				if p.id == look.Preset {
					preset = i
				}
			}
			c.Key("film-style-preset")
			ui.Row(c).Gap(8).Role(ui.RoleRadioGroup).Label("预设").Children(func() {
				for i, p := range stylePresets {
					c.Key("film-style-preset-" + p.id)
					if presetCard(c, p.label, p.box, p.sample, i == preset).Clicked() && i != preset {
						look.Preset, look.Box, look.Shadow = p.id, p.box, !p.box
						look.Translation.Size, look.Original.Size = p.tran, p.orig
						changed = true
					}
				}
			})
			panelGroup(c, func() {
				styleRow(c, "字体", false, func() {
					c.Key("film-style-font")
					family := look.Translation.Family
					breeze.Select(c, &family, styleFonts).Label("字体")
					if family != look.Translation.Family {
						look.Translation.Family, look.Original.Family = family, family
						changed = true
					}
				})
				styleRow(c, "双语顺序", true, func() {
					order := 0
					if s.SubOrder() == SubOriginalFirst {
						order = 1
					}
					c.Key("film-style-order")
					before := order
					breeze.Segmented(c, &order, "译文在上", "原文在上").Label("双语顺序")
					if order != before {
						s.SetSubOrder([]string{SubTranslationFirst, SubOriginalFirst}[order])
						changed = true
					}
				})
				styleRow(c, "位置", true, func() {
					place := 1
					if look.High {
						place = 0
					}
					c.Key("film-style-place")
					before := place
					breeze.Segmented(c, &place, "偏高", "底部").Label("位置")
					if place != before {
						look.High = place == 0
						changed = true
					}
				})
			})
			changed = s.faceGroup(c, "译文", &look.Translation, 16, 48) || changed
			changed = s.faceGroup(c, "原文", &look.Original, 12, 40) || changed
			c.Key("film-style-reset")
			if breeze.Button(c, "恢复默认样式").AlignSelf(ui.Center).Clicked() {
				*look = DefaultLook()
				changed = true
			}
		})
	})
	if changed {
		s.LookChanged()
	}
}

// presetCard is a preset of the style view, a card of the prototype's (template 652-657): 字幕 drawn as the preset draws it, outlined or on a box, over its name, the chosen card lit and ringed in white.
func presetCard(c *ui.Context, label string, box bool, sample float32, on bool) ui.Element {
	card := ui.ButtonBase(c).Column().Grow(1).Basis(0).MinWidth(0).Gap(6).Padding(10, 4, 8, 4).Radius(12).AlignItems(ui.Center).Label(label).Role(ui.RoleRadio).Checked(on)
	if on {
		card.Background(ui.RGBA(255, 255, 255, 0.16)).InsetShadow(0, 0, 0, 1.5, onGlass)
	} else {
		card.Background(ui.RGBA(255, 255, 255, 0.06)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.15))
	}
	card.Children(func() {
		ui.Row(c).Height(26).AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
			line := styledLine{text: "字幕", face: Face{Size: sample, Color: "#FFFFFF", Bold: true}}
			if box {
				ui.Box(c).Padding(0, 8).Radius(5).Background(ui.RGBA(0, 0, 0, 0.7)).Children(func() { lineText(c, line).LineHeight(26.0 / sample).TextColor(onGlass) })
				return
			}
			subtitleLine(c, line, false, true)
		})
		ui.Text(c, label).FontSize(12)
	})
	return card
}

// faceGroup sets one line's size, between lo and hi, its color and whether it is bold. It reports whether any changed.
func (s screen) faceGroup(c *ui.Context, name string, face *Face, lo, hi float64) bool {
	changed := false
	panelHeading(c, name)
	panelGroup(c, func() {
		styleRow(c, "字号", false, func() {
			size := float64(face.Size)
			c.Key("film-style-size-" + name)
			breeze.Slider(c, &size, lo, hi).Step(1).Grow(1).MinWidth(0).Label(name + "字号")
			if float32(size) != face.Size {
				face.Size = float32(size)
				changed = true
			}
			ui.Text(c, fmt.Sprintf("%.0f", face.Size)).FontFeatures("tnum").Opacity(0.7).Width(22).TextAlign(ui.End).Shrink(0)
		})
		styleRow(c, "颜色", true, func() {
			for _, color := range styleColors {
				c.Key("film-style-color-" + name + color.hex)
				if colorDot(c, color.hex, name+color.name, face.Color == color.hex).Clicked() {
					face.Color = color.hex
					changed = true
				}
			}
			ui.Box(c).Size(1, 18).Background(ui.RGBA(255, 255, 255, 0.2)).Shrink(0)
			c.Key("film-style-bold-" + name)
			bold := face.Bold
			breeze.Toggle(c, &face.Bold, "B").Label(name + "加粗")
			if face.Bold != bold {
				changed = true
			}
		})
	})
	return changed
}

// styleRow is a row of a style group: its label, then its controls at the end, under a hairline from 12 in when divided, as the prototype's grouped lists.
func styleRow(c *ui.Context, label string, divided bool, controls func()) {
	row := ui.Row(c).Gap(10).MinHeight(40).Padding(6, 12).AlignItems(ui.Center)
	if divided {
		row.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + 12, Y: r.Y, W: r.W - 12, H: 0.5}, ui.RGBA(255, 255, 255, 0.12), 0)
		})
	}
	row.Children(func() {
		ui.Text(c, label).FontSize(13).MinWidth(40).Shrink(0)
		ui.Row(c).Grow(1).Gap(10).AlignItems(ui.Center).Justify(ui.End).Children(controls)
	})
}

// colorDot is a swatch of a line's color, ringed while chosen.
func colorDot(c *ui.Context, hex, label string, on bool) ui.Element {
	dot := ui.ButtonBase(c).Size(22, 22).Radius(11).Shrink(0).Background(ui.Hex(hex)).Label(label).Checked(on)
	if on {
		dot.Shadow(0, 0, 0, 2, ui.RGBA(28, 28, 32, 1)).Border(1.5, onGlass)
	} else {
		dot.InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.3))
	}
	return dot
}
