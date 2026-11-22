package subtitle

import (
	"cmp"

	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The choices that change how a look draws its lines, and the controls for them that the player's style view and Settings › 字幕外观 share. Each side lays the controls out in its own rows: the player on its dark glass, the settings in their cards.

// Preset is a preset of the look, as the prototype has them (script 3272): ID names it in the look (Look.Preset), Box draws the lines on boxes rather than outlined, Translation and Original are the sizes it gives the lines, and Sample the size the player's card for it draws its sample at.
type Preset struct {
	ID                    string
	Box                   bool
	Translation, Original float32
	Sample                core.Points
}

// Presets are the look's presets: outlined, on boxes, and outlined larger.
var Presets = []Preset{
	{"outline", false, 30, 22, 15},
	{"box", true, 30, 22, 15},
	{"large", false, 42, 30, 20},
}

// Apply gives l the preset: its outline or boxes and its sizes.
func (p Preset) Apply(l *Look) {
	l.Preset, l.Box, l.Shadow = p.ID, p.Box, !p.Box
	l.Translation.Size, l.Original.Size = p.Translation, p.Original
}

// PresetOf is the ID of the preset l was given last, the first preset for a look given none.
func PresetOf(l Look) string { return cmp.Or(l.Preset, Presets[0].ID) }

// PresetName is what a preset is called in the language the interface shows under cx.
func PresetName(cx *core.Scope, id string) string {
	switch id {
	case "box":
		return locales.FilmPresetBox(cx)
	case "large":
		return locales.FilmPresetLarge(cx)
	}
	return locales.FilmPresetOutline(cx)
}

// Colors are the colors a line can take, as the look keeps them; ColorName names them.
var Colors = []string{"#FFFFFF", "#FFD60A", "#FF9F0A"}

// ColorName is what a line's color is called in the language the interface shows under cx.
func ColorName(cx *core.Scope, hex string) string {
	switch hex {
	case "#FFD60A":
		return locales.FilmColorYellow(cx)
	case "#FF9F0A":
		return locales.FilmColorOrange(cx)
	}
	return locales.FilmColorWhite(cx)
}

// Line is one line of a cue as the style controls change it: ID keys its controls, Min and Max bound its size, and Face picks its face out of a look.
type Line struct {
	ID       string
	Min, Max float64
	Face     func(l *Look) *Face
	name     func(cx *core.Scope) string
}

// Name is what the line is called in the language the interface shows under cx.
func (l Line) Name(cx *core.Scope) string { return l.name(cx) }

// The lines of a cue, the translation and the original, as the style controls change them.
var (
	TranslationLine = Line{ID: "translation", Min: 16, Max: 48, Face: func(l *Look) *Face { return &l.Translation }, name: locales.FilmLineTranslation}
	OriginalLine    = Line{ID: "original", Min: 12, Max: 40, Face: func(l *Look) *Face { return &l.Original }, name: locales.FilmLineOriginal}
)

// Edit is how a style control changes the look: it hands change the look to change, and keeps it once changed.
type Edit func(change func(l *Look))

// FontSelect is the drop-down of the font both lines are drawn in, the families the platform offers (Fonts) by their names in the interface's language; the look keeps the saved value of the one chosen. id is its test ID; look reads the look.
func FontSelect(cx *core.Scope, id string, look func() Look, edit Edit) core.Node {
	fonts := Fonts(cx.Env().OS())
	opts := make([]ui.Option[string], len(fonts))
	for i, family := range fonts {
		opts[i] = ui.Option[string]{Value: family, LabelFunc: func() string { return FontName(cx, family) }}
	}
	return ui.Select[string]{TestID: id, LabelFunc: func() string { return locales.FilmStyleFont(cx) }, Options: opts,
		Value: func() string { return look().Translation.Family },
		OnChange: func(family string) {
			edit(func(l *Look) { l.Translation.Family, l.Original.Family = family, family })
		},
		Style: core.Style{Width: core.Pt(150)}}
}

// SizeSlider is the slider of line's size, between its Min and Max. id is its test ID; look reads the look.
func SizeSlider(cx *core.Scope, id string, line Line, look func() Look, edit Edit) ui.Slider {
	return ui.Slider{TestID: id, LabelFunc: func() string { return locales.FilmFaceSize(cx, line.Name(cx)) }, Min: line.Min, Max: line.Max, Step: 1,
		Value:    func() float64 { l := look(); return float64(line.Face(&l).Size) },
		OnChange: func(v float64) { edit(func(l *Look) { line.Face(l).Size = float32(v) }) },
		Style:    core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}}
}
