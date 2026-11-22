package film

import (
	"math"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Layers is how far the open film's subtitles have come, layer by layer, for the progress badge at the top right of the picture: Show is set while its job waits or runs, or while a done film has lines the translation left untranslated (Untranslated); Percent is how much of the film is translated; Rows are the layers in the order a job makes them.
type Layers struct {
	Show         bool
	Running      bool
	Percent      int
	Untranslated int
	Rows         []Layer
}

// Layer is one layer of the open film's subtitles: Kind is LayerSpeech, LayerTranslation or LayerReview, Engine what makes it, as a model's name, "" when not known, and Done how much of it is made, from 0 to 1.
type Layer struct {
	Kind   string
	Engine string
	Done   float64
}

// The layers of a film's subtitles.
const (
	LayerSpeech      = "speech"
	LayerTranslation = "translation"
	LayerReview      = "review"
)

// Progresser is what the progress badge reads of the open film (Layers).
type Progresser interface {
	// SubtitleLayers is how far the open film's subtitles have come.
	SubtitleLayers() Layers
}

// layerName is what the badge calls a layer in the language the interface shows under cx.
func layerName(cx *core.Scope, kind string) string {
	switch kind {
	case LayerSpeech:
		return locales.FilmLayerSpeech(cx)
	case LayerReview:
		return locales.FilmLayerReview(cx)
	}
	return locales.FilmLayerTranslation(cx)
}

// badgeGlass is the glass of the badge and its panel.
var (
	badgeGlass = glassSpec{theme.GlassBar, 19}
	badgePanel = glassSpec{theme.GlassCard, 18}
)

// badgeSize is the badge's diameter.
const badgeSize core.Points = 38

// progressBadge is the round badge at the top right of the picture while the open film's subtitles are being made, or a done film has lines left untranslated (prototype template 682-710): a ring of how much is translated, an amber ring with ! when lines need the user. A press opens a panel saying how far the film's subtitles are, each layer with what makes it and a bar of how much is made, the untranslated lines with the button that translates them, and Settings…, which opens Settings › 翻译.
func (s *screen) progressBadge() core.Node {
	a := s.a
	layers := live.Func(a, a.SubtitleLayers)
	shown := live.Memo(s.cx, a, func() bool { return a.SubtitleLayers().Show })
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, false)
		attention := func() bool { l := layers(); return !l.Running && l.Untranslated > 0 }
		label := func() string {
			l := layers()
			if attention() {
				return locales.FilmBadgeAttention(cx, l.Percent, l.Untranslated)
			}
			return locales.FilmBadgeRunning(cx, l.Percent)
		}
		ring := ui.ProgressRing{TestID: "film-badge-ring", Size: 28, Value: func() float32 { return float32(layers().Percent) },
			Tint: func() core.Color {
				if attention() {
					return theme.Of(cx).WarnInk()
				}
				return onGlass
			}}
		mark := core.Show{When: attention, Then: func(cx *core.Scope) core.Node {
			return core.Text{Content: "!", Role: core.RoleNone, PointerEvents: core.PointerEventsNone,
				Style:     core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0), TextAlign: core.TextAlignCenter, AlignSelf: core.AlignCenter, FontSize: 14, LineHeight: badgeSize, FontWeight: core.FontWeightBold},
				StyleFunc: func() core.Style { return core.Style{Color: theme.Of(cx).WarnInk()} }}
		}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Top: core.Pt(62)},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 20), Transition: roomTransition}
			},
			Children: []core.Node{base.Popover{TestID: "film-badge", LabelFunc: label, Tooltip: tip(cx, label),
				Open:         open.Get,
				OnOpenChange: func(now bool, _ base.Reason) { open.Set(now) },
				Placement:    core.PlaceBottom, Align: core.AlignEnd, Offset: 8,
				Style: badgeGlass.style().Merge(core.Style{Width: core.Pt(badgeSize), Height: core.Pt(badgeSize), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.2)}, FocusVisible: ui.FocusRing(cx, -2)}),
				Trigger:    core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Width: core.Pt(badgeSize), Height: core.Pt(badgeSize), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{ring, mark}},
				PopupStyle: badgePanel.floating().Merge(core.Style{Width: core.Pt(310), Padding: core.All(14), Gap: 10}),
				Children:   s.badgePanel(cx, layers, attention, func() { open.Set(false) }),
			}},
		}
	}}
}

// badgePanel is what the badge opens: how much of the film is translated and what is being done, the layers, the untranslated lines with the button that translates them, and Settings….
func (s *screen) badgePanel(cx *core.Scope, layers func() Layers, attention func() bool, close func()) []core.Node {
	a := s.a
	summary := live.Func(a, func() string {
		l := layers()
		if !l.Running && l.Untranslated > 0 {
			return locales.FilmBadgeUntranslated(cx, l.Untranslated)
		}
		return a.JobStatus(cx)
	})
	rows := core.Each(func() []Layer { return layers().Rows }, func(l Layer) string { return l.Kind }, func(cx *core.Scope, layer func() Layer, _ func() int) core.Node {
		pct := func() string { return locales.FilmBadgePercent(cx, int(math.Round(layer().Done*100))) }
		return core.View{TestID: "film-badge-layer", Role: core.RoleGroup, LabelFunc: func() string { return layerName(cx, layer().Kind) + " " + pct() }, Style: core.Style{Gap: 4}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return layerName(cx, layer().Kind) }, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, LineHeight: 16, Color: onGlass}},
				core.Text{ContentFunc: func() string { return layer().Engine }, MaxLines: 1, Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: core.Alpha(0.7)}},
				core.Text{ContentFunc: pct, MaxLines: 1, Style: core.Style{Width: core.Pt(40), TextAlign: core.TextAlignRight, FontSize: 12, LineHeight: 16, FontVariant: core.FontTabularNums, Color: onGlass}},
			}},
			core.View{Style: core.Style{Height: core.Pt(4), CornerRadius: 2, Background: core.RGBA(0xFFFFFF, 0.16), Overflow: core.Hidden}, Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(4)}, StyleFunc: func() core.Style {
					return core.Style{Width: core.Pct(float32(min(max(layer().Done, 0), 1) * 100)), Background: ui.Alpha(theme.Of(cx).AccentInk(), 0.9)}
				}},
			}},
		}}
	})
	fix := core.Show{When: attention, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.FilmBadgeUntranslatedNote(cx, layers().Untranslated) }, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, LineHeight: 16},
				StyleFunc: func() core.Style { return core.Style{Color: ui.Mix(theme.Of(cx).WarnInk(), onGlass, 0.3)} }},
			pillButton(cx, pill{id: "film-badge-translate", label: func() string { return locales.FilmBadgeTranslateMissing(cx) }, color: func() core.Color { return core.RGBA(0xFFFFFF, 0.16) },
				press: live.Do(a, func() {
					close()
					a.TranslateFilmMissing()
				})}),
		}}
	}}
	settings := func() string { return locales.FilmBadgeSettings(cx) }
	return []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
			core.Text{TestID: "film-badge-percent", ContentFunc: func() string { return locales.FilmBadgeSubtitles(cx, layers().Percent) }, MaxLines: 1,
				Style: core.Style{FontSize: 20, LineHeight: 24, FontWeight: core.FontWeightBold, FontVariant: core.FontTabularNums, Color: onGlass}},
			core.Text{TestID: "film-badge-summary", ContentFunc: summary, Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, LineHeight: 16, Color: onGlass, Opacity: core.Alpha(0.75)}},
		}},
		core.View{Style: core.Style{Gap: 8, Padding: core.Symmetric(10, 12), CornerRadius: 12, Background: core.RGBA(0xFFFFFF, 0.07)}, Children: []core.Node{rows, fix}},
		menuRow(cx, "film-badge-settings", core.RoleButton, settings, nil, live.Do(a, func() {
			close()
			a.OpenSettings(sectionid.Translate)
		}), core.Style{}, core.Text{ContentFunc: settings, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass}}),
	}
}
