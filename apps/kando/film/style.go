package film

import (
	"fmt"
	"slices"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// stylePresets are the style view's presets, as the prototype has them (script 3272): outlined, on boxes, and outlined larger. presetLabel names them.
var stylePresets = []struct {
	id         string
	box        bool
	tran, orig float32
	// sample is the size of the 字幕 its card shows.
	sample float32
}{
	{"outline", false, 30, 22, 15},
	{"box", true, 30, 22, 15},
	{"large", false, 42, 30, 20},
}

// presetLabel is what a preset is called in the interface's language.
func presetLabel(id string) string {
	switch id {
	case "box":
		return locales.FilmPresetBox()
	case "large":
		return locales.FilmPresetLarge()
	}
	return locales.FilmPresetOutline()
}

// styleColors are the colors a line can take; colorName names them.
var styleColors = []string{"#FFFFFF", "#FFD60A", "#FF9F0A"}

// colorName is what a line's color is called in the interface's language.
func colorName(hex string) string {
	switch hex {
	case "#FFD60A":
		return locales.FilmColorYellow()
	case "#FF9F0A":
		return locales.FilmColorOrange()
	}
	return locales.FilmColorWhite()
}

// styleView is the subtitle configuration shown by the 样式 tab or the mode menu's 样式…: playback mode and delay, then the presets, font, order, position, size, color and weight of each line.
func (s screen) styleView(c *ui.Context) {
	look := s.Cue()
	changed := false
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).WidthPercent(100).Padding(0, 14, 16, 14).Gap(14).Children(func() {
			ui.Row(c).Height(28).AlignItems(ui.Center).Children(func() {
				panelHeading(c, locales.FilmStylePlayback())
				if breeze.Button(c, locales.FilmStyleReset()).Height(24).Padding(0, 8).AlignSelf(ui.Center).Clicked() {
					*look = DefaultLook()
					changed = true
				}
			})
			panelGroup(c, func() {
				styleRow(c, locales.FilmStyleShow(), false, func() {
					mode := styleModeIndex(s.SubMode())
					before := mode
					c.Key("film-style-mode")
					breeze.Segmented(c, &mode, ModeName(SubOff), ModeName(SubOriginal), ModeName(SubTranslation), ModeName(SubBilingual)).Label(locales.FilmStyleShow())
					if mode != before {
						s.SetSubMode([]string{SubOff, SubOriginal, SubTranslation, SubBilingual}[mode])
					}
				})
				styleRow(c, locales.FilmDelay(), true, func() {
					value := s.Delay().Seconds()
					c.Key("film-style-delay")
					if breeze.Stepper(c, &value, -30, 30, 0.1).Label(locales.FilmDelay()).Changed() {
						s.SetDelay(time.Duration(value * float64(time.Second)).Round(100 * time.Millisecond))
					}
				})
			})
			preset := 0
			for i, p := range stylePresets {
				if p.id == look.Preset {
					preset = i
				}
			}
			c.Key("film-style-preset")
			ui.Row(c).Gap(8).Role(ui.RoleRadioGroup).Label(locales.FilmStylePresets()).Children(func() {
				for i, p := range stylePresets {
					c.Key("film-style-preset-" + p.id)
					if presetCard(c, presetLabel(p.id), p.box, p.sample, i == preset).Clicked() && i != preset {
						look.Preset, look.Box, look.Shadow = p.id, p.box, !p.box
						look.Translation.Size, look.Original.Size = p.tran, p.orig
						changed = true
					}
				}
			})
			panelGroup(c, func() {
				styleRow(c, locales.FilmStyleFont(), false, func() {
					c.Key("film-style-font")
					// The menu lists the families the platform offers (styleFonts) by their names in the interface's language and the look keeps the saved value of the one chosen.
					fonts := styleFonts(breeze.PlatformOf(c))
					names := make([]string, len(fonts))
					for i, family := range fonts {
						names[i] = fontName(family)
					}
					shown := fontName(look.Translation.Family)
					chosen := shown
					breeze.Select(c, &chosen, names).Label(locales.FilmStyleFont())
					if i := slices.Index(names, chosen); chosen != shown && i >= 0 {
						look.Translation.Family, look.Original.Family = fonts[i], fonts[i]
						changed = true
					}
				})
				styleRow(c, locales.FilmStyleOrder(), true, func() {
					order := 0
					if s.SubOrder() == SubOriginalFirst {
						order = 1
					}
					c.Key("film-style-order")
					before := order
					breeze.Segmented(c, &order, subtitle.OrderName(SubTranslationFirst), subtitle.OrderName(SubOriginalFirst)).Label(locales.FilmStyleOrder())
					if order != before {
						s.SetSubOrder([]string{SubTranslationFirst, SubOriginalFirst}[order])
						changed = true
					}
				})
				styleRow(c, locales.FilmStylePosition(), true, func() {
					place := 1
					if look.High {
						place = 0
					}
					c.Key("film-style-place")
					before := place
					breeze.Segmented(c, &place, locales.FilmPositionHigh(), locales.FilmPositionBottom()).Label(locales.FilmStylePosition())
					if place != before {
						look.High = place == 0
						changed = true
					}
				})
			})
			changed = s.faceGroup(c, "translation", locales.FilmLineTranslation(), &look.Translation, 16, 48) || changed
			changed = s.faceGroup(c, "original", locales.FilmLineOriginal(), &look.Original, 12, 40) || changed
		})
	})
	if changed {
		s.LookChanged()
	}
}

func styleModeIndex(mode string) int {
	switch mode {
	case SubOriginal:
		return 1
	case SubTranslation:
		return 2
	case SubBilingual:
		return 3
	default:
		return 0
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
			line := styledLine{text: locales.FilmPresetSample(), face: Face{Size: sample, Color: "#FFFFFF", Bold: true}}
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

// faceGroup sets one line's size, between lo and hi, its color and whether it is bold. id keys its controls and name heads it. It reports whether any changed.
func (s screen) faceGroup(c *ui.Context, id, name string, face *Face, lo, hi float64) bool {
	changed := false
	panelHeading(c, name)
	panelGroup(c, func() {
		styleRow(c, locales.FilmStyleSize(), false, func() {
			size := float64(face.Size)
			c.Key("film-style-size-" + id)
			breeze.Slider(c, &size, lo, hi).Step(1).Grow(1).MinWidth(0).Label(locales.FilmFaceSize(name))
			if float32(size) != face.Size {
				face.Size = float32(size)
				changed = true
			}
			ui.Text(c, fmt.Sprintf("%.0f", face.Size)).FontFeatures("tnum").Opacity(0.7).Width(22).TextAlign(ui.End).Shrink(0)
		})
		styleRow(c, locales.FilmStyleColor(), true, func() {
			for _, hex := range styleColors {
				c.Key("film-style-color-" + id + hex)
				if colorDot(c, hex, locales.FilmFaceColor(name, colorName(hex)), face.Color == hex).Clicked() {
					face.Color = hex
					changed = true
				}
			}
			ui.Box(c).Size(1, 18).Background(ui.RGBA(255, 255, 255, 0.2)).Shrink(0)
			c.Key("film-style-bold-" + id)
			bold := face.Bold
			breeze.Toggle(c, &face.Bold, "B").Label(locales.FilmFaceBold(name))
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
