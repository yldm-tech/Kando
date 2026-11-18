package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Group is a card of rows, as a grouped list's section.
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
		Style:     core.Style{CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} }}
}

// IconTile is a row's leading icon on a small rounded tile: white on the accent, or danger's on its tint.
func IconTile(cx *core.Scope, icon func() *core.SVG, tone Tone) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	fill, tint := colors.Accent, colors.OnAccent
	if tone == ToneDanger {
		fill, tint = tints.Danger, colors.Danger
	}
	return core.View{Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 7, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: ui.Filled(fill),
		Children:  []core.Node{ui.IconOf(icon, 17, tint)}}
}

// SectionTitle heads a group of rows: a section of a page, named for the screen reader's headings too.
func SectionTitle(cx *core.Scope, text func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: text, Role: core.RoleHeader, MaxLines: 1, Style: Heading.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Text)}
}
