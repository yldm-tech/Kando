package films

import (
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// dropZone is drawn over the library while files are dragged over it: a dashed frame in the accent and a card saying what letting go does. It lets the drop through to the page under it.
type dropZone struct{}

// Render implements core.Node.
func (dropZone) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	// What the card says letting go does is in the theme's accent ink, the dark blue on the light page.
	accentInk := theme.Of(cx).AccentInk
	card := core.View{
		Style: core.Style{AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(18, 28), CornerRadius: 16,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: 10}, Blur: 30, Color: core.RGBA(0x0A3C8C, 0.18)}}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Background().Faded(0.92)} },
		Children: []core.Node{
			ui.Icon(svg(dropIcon), 34, colors.Accent),
			core.Text{ContentFunc: func() string { return locales.FilmsDropTitle(cx) }, Style: core.Style{FontSize: 15, FontWeight: ui.Bold}, StyleFunc: ui.Colored(accentInk)},
			core.Text{ContentFunc: func() string { return locales.FilmsDropHint(cx) }, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)},
		},
	}
	return core.View{
		TestID:        "films-drop-zone",
		PointerEvents: core.PointerEventsNone,
		Style: core.Style{Position: core.Absolute, Top: core.Pt(12), Left: core.Pt(12), Right: core.Pt(12), Bottom: core.Pt(12), CornerRadius: 18,
			BorderWidth: 2, BorderStyle: core.BorderDashed, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style {
			return core.Style{BorderColor: colors.Accent(), Background: colors.Accent().Faded(0.07)}
		},
		Children: []core.Node{card},
	}
}
