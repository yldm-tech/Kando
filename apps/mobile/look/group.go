package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Group is a card of rows, as a grouped list's section: white (the palette's Surface) on the grey page, 16 round, with no rim or shadow.
// Its rows draw their own hairline.
// TODO(nsgo#1861): ui.FormSection.Title and Footer and ui.FormRow.Title, Detail and Label are strings only, with no TitleFunc, FooterFunc, DetailFunc or LabelFunc (ui/formsection.go:7, ui/formrow.go:36-45; NSGo's textFuncExceptions lists FormSection.Title, FormRow.Title and FormRow.Label as still awaiting bindings), so rows built on them would keep the language they were built in; a section's rows are Row, which draws its own hairline (Row.Index, Row.Inset) where FormSection would draw one 12 points in beside any row but a FormRow (ui/formsection.go:52-55, 74-90); make Group a FormSection on Surface, CardRadius round, once its rows can be FormRows https://github.com/nsgo-dev/nsgo/issues/1861
// TODO(nsgo#1841): ui.FormRow has no slot under its title and detail (Row.Below), no radio role (Row.Checked), no limit to its title's lines and no least height of its own (ui/formrow.go:27-47), which Row's callers use, so a section's rows stay Row even once FormRow takes a TitleFunc https://github.com/nsgo-dev/nsgo/issues/1841
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

// ColorTile is a row's leading icon, white on a tile of the row's own color, as the design gives each settings row one (theme.Tile…, or theme.Of(cx).TileInk, which follows the appearance); fill is read in a binding.
func ColorTile(cx *core.Scope, icon func() *core.SVG, fill func() core.Color) core.Node {
	return tile(icon, fill, ui.ColorsOf(cx).OnAccent)
}

// GlyphTile is a settings row's leading mark as the design draws it: a character or two, white at 13 points and 800, on a 30pt tile of the row's own color (theme.Tile…, or theme.Of(cx).TileInk); glyph and fill are read in bindings, as a glyph can follow the interface's language. The row's title names it, so assistive technology skips it.
func GlyphTile(cx *core.Scope, glyph func() string, fill func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{AccessibilityElementsHidden: func() bool { return true },
		Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(fill),
		Children: []core.Node{core.Text{ContentFunc: glyph, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 16, FontWeight: Heavy, TextAlign: core.TextAlignCenter},
			StyleFunc: ui.Colored(colors.OnAccent)}}}
}

// tile is a 30pt tile, 8 round, filled with fill, with icon in tint.
func tile(icon func() *core.SVG, fill, tint func() core.Color) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(fill),
		Children:  []core.Node{ui.IconOf(icon, 17, tint)}}
}

// SectionTitle heads a section of a page's content, as Continue Watching: SectionHeading, 22pt bold or Android's 20pt medium, named for the screen reader's headings too.
func SectionTitle(cx *core.Scope, text func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1, Style: SectionHeading(cx), StyleFunc: ui.Colored(colors.Text)}
}

// GroupHeader names the Group under it, as a grouped list's section header: a muted caption over the card, in line with the rows' text. It is a Text, so a caller can give it a TestID.
func GroupHeader(cx *core.Scope, text func() string) core.Text {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1, Style: Caption.Merge(core.Style{Padding: core.Edges{Left: 16, Right: 16}}),
		StyleFunc: ui.Colored(colors.Muted)}
}
