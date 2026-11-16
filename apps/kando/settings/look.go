package settings

import (
	"fmt"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// look is the 字幕外观 page: a preview of the subtitles in the player's look, how the player shows them, and the look itself: its preset, its font, and each line's size, color and weight, with the controls the player's style view uses where they fit a card (package subtitle). The preview is drawn in the player's dark theme. Every change is the player's at once, and saved.
type look struct{ s screen }

// Render implements core.Node.
func (p look) Render(cx *core.Scope) core.Node {
	a := p.s.a
	yield := form.SwitchRow{Title: locales.SettingsLookYield(cx), Subtitle: locales.SettingsLookYieldHint(cx), TestID: "settings-look-yield",
		On: live.Func(a, a.YieldSubs),
		OnChange: live.On(a, func(on bool) {
			a.SetYieldSubs(on)
			a.Save()
		})}
	look := live.Func(a, func() subtitle.Look { return *a.Cue() })
	edit := subtitle.Edit(func(change func(l *subtitle.Look)) {
		live.Do(a, func() {
			change(a.Cue())
			a.Save()
		})()
	})
	presets := make([]ui.Option[string], len(subtitle.Presets))
	for i, preset := range subtitle.Presets {
		presets[i] = ui.Option[string]{Value: preset.ID, Label: subtitle.PresetName(cx, preset.ID)}
	}
	return form.Page(
		lookPreview{a: a},
		form.Card(
			choice{a: a, id: "settings-look-mode", title: locales.SettingsLookMode(cx), get: a.SubMode, set: a.SetSubMode, options: []ui.Option[string]{
				{Value: subtitle.Original, Label: locales.SettingsLookModeOriginal(cx)}, {Value: subtitle.Translation, Label: locales.SettingsLookModeTranslation(cx)}, {Value: subtitle.Bilingual, Label: locales.SettingsLookModeBilingual(cx)},
			}},
			form.Separator(true),
			choice{a: a, id: "settings-look-order", title: locales.SettingsLookOrder(cx), get: a.SubOrder, set: a.SetSubOrder, options: []ui.Option[string]{
				{Value: subtitle.TranslationFirst, Label: locales.SettingsLookOrderTranslationFirst(cx)}, {Value: subtitle.OriginalFirst, Label: locales.SettingsLookOrderOriginalFirst(cx)},
			}},
			form.Separator(true),
			choice{a: a, id: "settings-look-untranslated", title: locales.SettingsLookUntranslated(cx), subtitle: locales.SettingsLookUntranslatedHint(cx), get: a.Untranslated, set: a.SetUntranslated, options: []ui.Option[string]{
				{Value: subtitle.UntranslatedDim, Label: locales.SettingsLookUntranslatedDim(cx)}, {Value: subtitle.UntranslatedBlank, Label: locales.SettingsLookUntranslatedBlank(cx)},
			}},
			form.Separator(true),
			yield,
		),
		form.Label(locales.SettingsLookStyle(cx), 12),
		form.Card(
			form.Row{Title: locales.FilmStylePresets(cx), Trailing: []core.Node{ui.Tabs[string]{TestID: "settings-look-preset", Label: locales.FilmStylePresets(cx), Size: ui.SizeSmall, Options: presets,
				Value: func() string { return subtitle.PresetOf(look()) },
				OnChange: func(id string) {
					for _, preset := range subtitle.Presets {
						if preset.ID == id {
							edit(preset.Apply)
						}
					}
				}}}},
			form.Separator(true),
			form.Row{Title: locales.FilmStyleFont(cx), Trailing: []core.Node{subtitle.FontSelect(cx, "settings-look-font", look, edit)}},
		),
		faceCard(cx, subtitle.TranslationLine, look, edit),
		faceCard(cx, subtitle.OriginalLine, look, edit),
	)
}

// faceCard sets one line's size, color and weight, under the line's name.
func faceCard(cx *core.Scope, line subtitle.Line, look func() subtitle.Look, edit subtitle.Edit) core.Node {
	face := func() subtitle.Face { l := look(); return *line.Face(&l) }
	size := subtitle.SizeSlider(cx, "settings-look-size-"+line.ID, line, look, edit)
	size.Style = core.Style{Width: core.Pt(180)}
	colors := make([]ui.Option[string], len(subtitle.Colors))
	for i, hex := range subtitle.Colors {
		colors[i] = ui.Option[string]{Value: hex, Label: subtitle.ColorName(cx, hex)}
	}
	return core.Group{
		form.Label(line.Name(cx), 12),
		form.Card(
			form.Row{Title: locales.FilmStyleSize(cx), Trailing: []core.Node{size, form.Value(func() string { return fmt.Sprintf("%.0f", face().Size) }, 12)}},
			form.Separator(true),
			form.Row{Title: locales.FilmStyleColor(cx), Trailing: []core.Node{ui.Tabs[string]{TestID: "settings-look-color-" + line.ID, Label: locales.SettingsLookColorOf(cx, line.Name(cx)), Size: ui.SizeSmall, Options: colors,
				Value:    func() string { return face().Color },
				OnChange: func(hex string) { edit(func(l *subtitle.Look) { line.Face(l).Color = hex }) }}}},
			form.Separator(true),
			form.Row{Title: locales.SettingsLookBold(cx), Trailing: []core.Node{ui.Switch{TestID: "settings-look-bold-" + line.ID, Label: locales.FilmFaceBold(cx, line.Name(cx)),
				Checked:  func() bool { return face().Bold },
				OnChange: func(on bool) { edit(func(l *subtitle.Look) { line.Face(l).Bold = on }) }}}},
		),
	}
}

// choice is a row with a segmented control choosing among options; a change is set and saved. A value none of the options holds, as the player's 关闭 for the display mode, leaves every segment unchosen.
type choice struct {
	a               App
	id              string
	title, subtitle string
	get             func() string
	set             func(string)
	options         []ui.Option[string]
}

// Render implements core.Node.
func (c choice) Render(*core.Scope) core.Node {
	a := c.a
	tabs := ui.Tabs[string]{Options: c.options, Label: c.title, Size: ui.SizeSmall, TestID: c.id,
		Value: live.Func(a, c.get),
		OnChange: live.On(a, func(value string) {
			c.set(value)
			a.Save()
		})}
	return form.Row{Title: c.title, Subtitle: c.subtitle, Trailing: []core.Node{tabs}}
}

// previewScene is the prototype's preview picture: sky, mountains and grass on a 16:7 tile.
var previewScene = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="140" viewBox="0 0 320 140"><rect width="320" height="140" fill="#7FA8C2"/><path d="M0 90 L60 55 L110 75 L170 40 L230 70 L280 50 L320 66 L320 140 L0 140Z" fill="#5F8496"/><path d="M0 115 Q80 92 160 108 T320 100 L320 140 L0 140Z" fill="#3E6A5B"/></svg>`)

// previewState is what the preview draws: the look and which lines show in which order, comparable so the lines are built again only when one of them changes.
type previewState struct {
	look        subtitle.Look
	mode, order string
}

// lookPreview draws the sample cue over the prototype's landscape as the player would: the lines the display mode shows, in its order, with the faces of the look, in its font, as high above the bottom as the player puts them.
type lookPreview struct{ a App }

// Render implements core.Node.
func (p lookPreview) Render(cx *core.Scope) core.Node {
	a := p.a
	theme.Dark(cx)
	state := core.NewMemo(cx, live.Func(a, func() previewState { return previewState{look: *a.Cue(), mode: a.SubMode(), order: a.SubOrder()} }))
	cue := core.Dynamic(func(cx *core.Scope) core.Node {
		st := state.Get()
		lines := previewLines(cx, st.look, st.mode, st.order)
		inset := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(core.Points(st.look.PreviewInset())), AlignItems: core.AlignCenter}
		if len(lines) == 0 {
			return core.View{Style: inset, Children: []core.Node{
				core.Text{Content: locales.SettingsLookPreviewOff(cx), Style: core.Style{FontSize: 13, Color: core.RGBA(0xFFFFFF, 0.7)}},
			}}
		}
		// The same block the player draws its subtitles in (film's playCue): a dark box behind the lines, or a shadow under each.
		block := core.Style{Gap: 2, CornerRadius: 8, AlignItems: core.AlignCenter}
		if st.look.Box {
			block.Padding, block.Background = core.Symmetric(8, 14), core.RGBA(0x000000, 0.55)
		}
		texts := make([]core.Node, len(lines))
		for i, line := range lines {
			style := core.Style{FontSize: core.Points(line.face.Size), LineHeightMultiple: 1.3, TextAlign: core.TextAlignCenter, Color: faceColor(line.face.Color), FontFamily: subtitle.FontFamily(line.face.Family)}
			if line.face.Bold {
				style.FontWeight = ui.Bold
			}
			if st.look.Shadow {
				style.TextShadowColor, style.TextShadowOffset, style.TextShadowRadius = core.RGBA(0x000000, 0.95), core.Point{Y: 1}, 3
			}
			texts[i] = core.Text{Content: line.text, Style: style}
		}
		return core.View{Style: inset, Children: []core.Node{core.View{Style: block, Children: texts}}}
	})
	scene := core.SVGView{SVG: previewScene, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Width: core.Pct(100), Height: core.Pct(100), PointerEvents: core.PointerEventsNone}}
	tag := core.Text{Content: locales.SettingsLookPreviewTag(cx), Role: core.RoleNone,
		Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(10), FontSize: 10, FontWeight: ui.Bold, Color: core.RGB(0xFFFFFF),
			Background: core.RGBA(0x000000, 0.4), CornerRadius: 6, Padding: core.Symmetric(2, 7)}}
	return core.View{TestID: "settings-look-preview", Role: core.RoleGroup, Label: locales.SettingsLookPreview(cx),
		Style: core.Style{FlexShrink: 0, AspectRatio: 16.0 / 7.0, Margin: core.Edges{Top: 6}, Background: core.RGB(0x2B3A3F), CornerRadius: 12, Overflow: core.Hidden,
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0x000000, 0.1)}}},
		Children: []core.Node{scene, tag, cue}}
}

// faceColor is a face's color, "#RRGGBB" as the look keeps it, white when it is not one.
func faceColor(hex string) core.Color {
	if c, err := core.ParseColor(hex); err == nil {
		return c
	}
	return core.RGB(0xFFFFFF)
}

// previewLine is a line of the preview and the face it is drawn in.
type previewLine struct {
	text string
	face subtitle.Face
}

// previewLines are the sample cue's lines the display mode shows, the one on top first, in the language the app shows under cx. The sample is one bilingual line as a translated film shows it: the translation in the interface's language, the original in another (English, or French in the English interface).
func previewLines(cx *core.Scope, look subtitle.Look, mode, order string) []previewLine {
	translation := previewLine{locales.SettingsLookPreviewTranslation(cx), look.Translation}
	original := previewLine{locales.SettingsLookPreviewOriginal(cx), look.Original}
	switch mode {
	case subtitle.Off:
		return nil
	case subtitle.Translation:
		return []previewLine{translation}
	case subtitle.Original:
		return []previewLine{original}
	}
	if order == subtitle.OriginalFirst {
		return []previewLine{original, translation}
	}
	return []previewLine{translation, original}
}
