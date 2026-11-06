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
}{
	{"outline", "描边", false, 30, 22},
	{"box", "底框", true, 30, 22},
	{"large", "大字", false, 42, 30},
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
		if flatIconButton(c, iconChevronLeft, "返回侧栏").Clicked() {
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
			before := preset
			breeze.Segmented(c, &preset, "描边", "底框", "大字").Label("预设").AlignSelf(ui.Center)
			if preset != before {
				p := stylePresets[preset]
				look.Preset, look.Box, look.Shadow = p.id, p.box, !p.box
				look.Translation.Size, look.Original.Size = p.tran, p.orig
				changed = true
			}
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

// styleRow is a row of a style group: its label, then its controls at the end, under a hairline when divided.
func styleRow(c *ui.Context, label string, divided bool, controls func()) {
	row := ui.Row(c).Gap(10).MinHeight(40).Padding(6, 12).AlignItems(ui.Center)
	if divided {
		row.BorderWidth(0.5, 0, 0, 0).BorderColor(ui.RGBA(255, 255, 255, 0.12))
	}
	row.Children(func() {
		ui.Text(c, label).Width(56).Shrink(0)
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
