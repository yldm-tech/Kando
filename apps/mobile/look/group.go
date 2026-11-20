package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Group is a card of rows, as a grouped list's section: white (the palette's Surface) on the grey page, 16 round, with no rim or shadow.
// ui.FormSection divides only its static Children (formsection.go:20-27); rows from core.Each draw their own hairline.
// TODO(nsgo#483): ui.FormSection divides only its static Children (formsection.go:20-27) and ui.FormRow is a desktop's (40pt, 13pt text, formsection.go:48, formrow.go:60-70) with no room for a poster; use them once a section divides rows from core.Each and a row takes a phone's size and a leading image https://github.com/nsgo-dev/nsgo/issues/483
type Group struct {
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (g Group) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: g.TestID, Children: g.Children,
		Style:     core.Style{CornerRadius: CardRadius, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
		StyleFunc: ui.Filled(colors.Surface)}
}

// CardRadius is the corner of a card: a Group, a stat tile, a page's own card.
const CardRadius core.Points = 16

// Card is the style of a card that is not a Group, as a stat tile or a page's own card: the palette's Surface, CardRadius round; read it in a StyleFunc.
func Card(cx *core.Scope) core.Style {
	return core.Style{Background: ui.ColorsOf(cx).Surface(), CornerRadius: CardRadius, BorderCurve: core.BorderCurveContinuous}
}

// IconTile is a row's leading icon on a small rounded tile: white on the accent or on the warning's color, or danger's on its tint.
func IconTile(cx *core.Scope, icon func() *core.SVG, tone Tone) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	fill, tint := colors.Accent, colors.OnAccent
	switch tone {
	case ToneDanger:
		fill, tint = tints.Danger, colors.Danger
	case ToneWarn:
		fill = colors.Warn
	}
	return tile(icon, fill, tint)
}

// ColorTile is a row's leading icon, white on a tile of the row's own color, as the design gives each settings row one (theme.Tile…).
func ColorTile(icon func() *core.SVG, fill core.Color) core.Node {
	return tile(icon, func() core.Color { return fill }, func() core.Color { return core.RGB(0xFFFFFF) })
}

// tile is a 30pt tile, 8 round, filled with fill, with icon in tint.
func tile(icon func() *core.SVG, fill, tint func() core.Color) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(fill),
		Children:  []core.Node{ui.IconOf(icon, 17, tint)}}
}

// SectionTitle heads a section of a page's content, as Continue Watching: 22pt bold, named for the screen reader's headings too.
func SectionTitle(cx *core.Scope, text func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1, Style: Heading, StyleFunc: ui.Colored(colors.Text)}
}

// GroupHeader names the Group under it, as a grouped list's section header: a muted caption over the card, in line with the rows' text.
func GroupHeader(cx *core.Scope, text func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1, Style: Caption.Merge(core.Style{Padding: core.Edges{Left: 16, Right: 16}}),
		StyleFunc: ui.Colored(colors.Muted)}
}
