package film

import (
	"fmt"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// styleModes are the modes the style view's 显示字幕 offers, in its order.
var styleModes = []string{SubOff, SubOriginal, SubTranslation, SubBilingual}

// styleView is the subtitle configuration shown by the 样式 tab or the mode menu's 样式…: playback mode and delay, then the presets, font, order, position, size, color and weight of each line. Each control changes the look in place and has the app keep it (LookChanged). The choices of the look and the font and size controls are package subtitle's, which Settings › 字幕外观 shares.
func (s *screen) styleView(cx *core.Scope) core.Node {
	a := s.a
	look := func() Look { a.Track(); return *a.Cue() }
	edit := func(fn func(l *Look)) func() {
		return live.Do(a, func() {
			fn(a.Cue())
			a.LookChanged()
		})
	}
	change := func(fn func(l *Look)) { edit(fn)() }
	opts := make([]ui.Option[string], len(styleModes))
	for i, m := range styleModes {
		opts[i] = ui.Option[string]{Value: m, LabelFunc: func() string { return ModeName(cx, m) }}
	}
	modes := ui.Tabs[string]{TestID: "film-style-mode", LabelFunc: func() string { return locales.FilmStyleShow(cx) }, Size: ui.SizeSmall, Options: opts,
		Value: live.Func(a, func() string { return modeOrOff(a.SubMode()) }), OnChange: live.On(a, a.SetSubMode)}
	order := ui.Tabs[string]{TestID: "film-style-order", LabelFunc: func() string { return locales.FilmStyleOrder(cx) }, Size: ui.SizeSmall,
		Options: []ui.Option[string]{
			{Value: SubTranslationFirst, LabelFunc: func() string { return subtitle.OrderName(cx, SubTranslationFirst) }},
			{Value: SubOriginalFirst, LabelFunc: func() string { return subtitle.OrderName(cx, SubOriginalFirst) }},
		},
		Value: live.Func(a, a.SubOrder),
		OnChange: live.On(a, func(order string) {
			a.SetSubOrder(order)
			a.LookChanged()
		})}
	place := ui.Tabs[bool]{TestID: "film-style-place", LabelFunc: func() string { return locales.FilmStylePosition(cx) }, Size: ui.SizeSmall,
		Options: []ui.Option[bool]{
			{Value: true, LabelFunc: func() string { return locales.FilmPositionHigh(cx) }},
			{Value: false, LabelFunc: func() string { return locales.FilmPositionBottom(cx) }},
		},
		Value: func() bool { return look().High },
		// Picking a place also puts the open film's subtitles, if they were moved away from it (SubLift), back there.
		OnChange: func(high bool) {
			edit(func(l *Look) {
				l.High = high
				a.SetSubLift(0)
			})()
		}}
	font := subtitle.FontSelect(cx, "film-style-font", look, change)
	presets := make([]core.Node, len(subtitle.Presets))
	for i, p := range subtitle.Presets {
		on := func() bool { return subtitle.PresetOf(look()) == p.ID }
		presets[i] = presetCard(cx, "film-style-preset-"+p.ID, func() string { return subtitle.PresetName(cx, p.ID) }, p.Box, p.Sample, on, func() {
			if on() {
				return
			}
			change(p.Apply)
		})
	}
	presetRow := core.View{TestID: "film-style-preset", Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.FilmStylePresets(cx) },
		Style: core.Style{Direction: core.Row, Gap: 8}, Children: presets}
	reset := ui.Button{TestID: "film-style-reset", TextFunc: func() string { return locales.FilmStyleReset(cx) }, Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
		OnPress: edit(func(l *Look) {
			*l = DefaultLook()
			a.SetSubLift(0)
		})}
	content := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(28), AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			heading(func() string { return locales.FilmStylePlayback(cx) }), reset,
		}},
		styleGroup(
			styleRow(func() string { return locales.FilmStyleShow(cx) }, false, modes),
			styleRow(func() string { return locales.FilmDelay(cx) }, true, delayInput(cx, a, "film-style-delay")),
		),
		presetRow,
		styleGroup(
			styleRow(func() string { return locales.FilmStyleFont(cx) }, false, font),
			styleRow(func() string { return locales.FilmStyleOrder(cx) }, true, order),
			styleRow(func() string { return locales.FilmStylePosition(cx) }, true, place),
		),
	}
	content = append(content, s.faceGroup(cx, subtitle.TranslationLine, look, edit)...)
	content = append(content, s.faceGroup(cx, subtitle.OriginalLine, look, edit)...)
	return core.Scroll{TestID: "film-style", Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Right: 14, Bottom: 16, Left: 14}, Gap: 14}, Children: content},
	}}
}

// modeOrOff is mode as the style view's 显示字幕 offers it, an unknown one counting as bilingual.
func modeOrOff(mode string) string {
	switch mode {
	case SubOff, SubOriginal, SubTranslation:
		return mode
	}
	return SubBilingual
}

// heading is the heading over a group of the style view.
func heading(text func() string) core.Node {
	return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: onGlass, Opacity: core.Alpha(0.6), Padding: core.Symmetric(0, 4)}}
}

// styleGroup is a card of the style view, holding its rows.
func styleGroup(rows ...core.Node) core.Node { return panelGroup(rows...) }

// presetCard is a preset of the style view, a card of the prototype's (template 652-657): 字幕 drawn as the preset draws it, outlined or on a box, over its name, the chosen card lit and ringed in white.
func presetCard(cx *core.Scope, id string, label func() string, box bool, sample core.Points, on func() bool, press func()) core.Node {
	face := Face{Size: float32(sample), Color: "#FFFFFF", Bold: true}
	line := styledLine{face: face}
	var shown core.Node
	if box {
		shown = core.View{Style: core.Style{Padding: core.Symmetric(0, 8), CornerRadius: 5, Background: core.RGBA(0x000000, 0.7)}, Children: []core.Node{
			lineText(func() string { return locales.FilmPresetSample(cx) }, line, core.Style{LineHeight: 26, Color: onGlass}),
		}}
	} else {
		shown = subtitleLine(func() string { return locales.FilmPresetSample(cx) }, line, false, true, nil)
	}
	return base.Button{TestID: id, Role: core.RoleRadio, LabelFunc: label, Selected: on,
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Gap: 6, Padding: core.Edges{Top: 10, Right: 4, Bottom: 8, Left: 4}, CornerRadius: 12, AlignItems: core.AlignCenter,
			FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: core.RGBA(0xFFFFFF, 0.16), Shadows: []core.BoxShadow{{Spread: 1.5, Color: onGlass, Inset: true}}}
			}
			return core.Style{Background: core.RGBA(0xFFFFFF, 0.06), Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0xFFFFFF, 0.15), Inset: true}}}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Height: core.Pt(26), JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}, Children: []core.Node{shown}},
			core.Text{ContentFunc: label, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}},
		}}
}

// faceGroup sets one line's size, its color and whether it is bold, under the line's name.
func (s *screen) faceGroup(cx *core.Scope, line subtitle.Line, look func() Look, edit func(func(l *Look)) func()) []core.Node {
	id, of := line.ID, line.Face
	name := func() string { return line.Name(cx) }
	face := func() Face { l := look(); return *of(&l) }
	size := subtitle.SizeSlider(cx, "film-style-size-"+id, line, look, func(fn func(l *Look)) { edit(fn)() })
	sizeRow := core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
		size,
		core.Text{ContentFunc: func() string { return fmt.Sprintf("%.0f", face().Size) }, Style: core.Style{FontVariant: core.FontTabularNums, FontSize: 13, LineHeight: 18, Width: core.Pt(22), TextAlign: core.TextAlignEnd, FlexShrink: 0, Color: onGlass, Opacity: core.Alpha(0.7)}},
	}}
	colors := make([]core.Node, 0, len(subtitle.Colors)+2)
	for _, hex := range subtitle.Colors {
		on := func() bool { return face().Color == hex }
		colors = append(colors, colorDot(cx, "film-style-color-"+id+hex, hex, func() string { return locales.FilmFaceColor(cx, name(), subtitle.ColorName(cx, hex)) }, on,
			edit(func(l *Look) { of(l).Color = hex })))
	}
	colors = append(colors,
		core.View{Style: core.Style{Width: core.Pt(1), Height: core.Pt(18), FlexShrink: 0, Background: core.RGBA(0xFFFFFF, 0.2)}},
		glyphToggleText(cx, "film-style-bold-"+id, "B", func() string { return locales.FilmFaceBold(cx, name()) }, func() bool { return face().Bold },
			edit(func(l *Look) { of(l).Bold = !of(l).Bold })),
	)
	return []core.Node{
		heading(name),
		styleGroup(
			styleRow(func() string { return locales.FilmStyleSize(cx) }, false, sizeRow),
			styleRow(func() string { return locales.FilmStyleColor(cx) }, true, core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter}, Children: colors}),
		),
	}
}

// glyphToggleText is a small toggle showing text, lit while on, as the style view's bold.
func glyphToggleText(cx *core.Scope, id, text string, label func() string, on func() bool, press func()) core.Node {
	return base.Button{TestID: id, LabelFunc: label, Selected: on, Role: core.RoleButton,
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Width: core.Pt(28), Height: core.Pt(24), CornerRadius: 6, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0,
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.12)}, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: core.RGBA(0xFFFFFF, 0.24)}
			}
			return core.Style{}
		},
		Children: []core.Node{core.Text{Content: text, Style: core.Style{FontSize: 13, LineHeight: 16, FontWeight: core.FontWeightBold, Color: onGlass}}}}
}

// styleRow is a row of a style group: its label, then its controls at the end, under a hairline from 12 in when divided, as the prototype's grouped lists.
func styleRow(label func() string, divided bool, controls core.Node) core.Node {
	st := core.Style{Direction: core.Row, Gap: 10, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 12), AlignItems: core.AlignCenter}
	children := []core.Node{
		core.Text{ContentFunc: label, Style: core.Style{FontSize: 13, LineHeight: 18, MinWidth: core.Pt(40), FlexShrink: 0, Color: onGlass}},
		core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyEnd}, Children: []core.Node{controls}},
	}
	if divided {
		children = append(children, core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(12), Right: core.Pt(0), Height: core.Pt(0.5), Background: core.RGBA(0xFFFFFF, 0.12)}})
	}
	return core.View{Style: st, Children: children}
}

// colorDot is a swatch of a line's color, ringed while chosen.
func colorDot(cx *core.Scope, id, hex string, label func() string, on func() bool, press func()) core.Node {
	c, _ := core.ParseColor(hex)
	return base.Button{TestID: id, LabelFunc: label, Selected: on, Role: core.RoleRadio,
		OnPress: func(core.PressEvent) { press() },
		Style:   core.Style{Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, FlexShrink: 0, Background: c, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{BorderWidth: 1.5, BorderColor: onGlass, Shadows: []core.BoxShadow{{Spread: 2, Color: core.RGB(0x1C1C20)}}}
			}
			return core.Style{Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0x000000, 0.3), Inset: true}}}
		}}
}
