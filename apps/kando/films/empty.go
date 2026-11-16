package films

import (
	"kando/apps/kando/desktop"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// noLibrary is the page before there is a library: what a library is, 新建资料库… and 打开已有资料库….
type noLibrary struct{ a App }

// Render implements core.Node.
func (n noLibrary) Render(cx *core.Scope) core.Node {
	a := n.a
	return statusPage(cx, core.SVGView{SVG: libraryArt, Style: core.Style{FlexShrink: 0}}, locales.FilmsNoLibraryTitle, func(cx *core.Scope) string { return locales.FilmsNoLibraryDescription(cx, desktop.Platform(cx)) },
		textButton(locales.FilmsNewLibraryEllipsis, ui.Button{TestID: "empty-new", OnPress: live.Do(a, a.BeginNew)}),
		textButton(locales.FilmsOpenLibraryEllipsis, ui.Button{Variant: ui.ButtonSecondary, TestID: "empty-open", OnPress: live.Do(a, a.PickLibrary)}),
	)
}

// emptyLibrary is the page of a library that holds no film yet: how to add one, 选择本地视频…, 打开已有资料库… and 支持的网站, which lists the sites a link can come from.
type emptyLibrary struct{ a App }

// Render implements core.Node.
func (e emptyLibrary) Render(cx *core.Scope) core.Node {
	a := e.a
	return statusPage(cx, startTile(cx), locales.FilmsEmptyTitle, locales.FilmsEmptyDescription,
		textButton(locales.FilmsPickVideosEllipsis, ui.Button{TestID: "empty-pick", OnPress: live.Do(a, a.PickVideos)}),
		textButton(locales.FilmsOpenLibraryEllipsis, ui.Button{Variant: ui.ButtonSecondary, TestID: "empty-open", OnPress: live.Do(a, a.PickLibrary)}),
		textButton(locales.FilmsSupportedSites, ui.Button{Variant: ui.ButtonSecondary, TestID: "empty-sites", OnPress: live.Do(a, a.OpenSites)}),
	)
}

// emptySource is what a source of the sidebar shows when no film of the library comes from it.
type emptySource struct{}

// Render implements core.Node.
func (emptySource) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, Padding: core.Symmetric(80, 28)}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.FilmsSourceEmpty(cx) }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// emptySpoken is what the grid shows when none of the done films of the source chosen in the sidebar was heard in the language chosen under it, code being whisper's code for it.
type emptySpoken struct{ code func() string }

// Render implements core.Node.
func (e emptySpoken) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, Padding: core.Symmetric(80, 28)}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.FilmsSpokenEmpty(cx, subtitle.SpokenLanguageName(cx, e.code())) }, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// noMatch is what the grid shows when no done film's name holds the search field's query, with 清除搜索, which empties the field and brings the whole shelf back.
type noMatch struct {
	a     App
	query func() string
}

// Render implements core.Node.
func (n noMatch) Render(cx *core.Scope) core.Node {
	a := n.a
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(80, 28)}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.FilmsSearchNoMatch(cx, n.query()) }, Style: ui.TypeBody.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
		textButton(locales.FilmsSearchClear, ui.Button{Variant: ui.ButtonSecondary, TestID: "films-search-clear", OnPress: live.Do(a, func() { *a.Query() = "" })}),
	}}
}

// startTile is the play tile over a library that holds no film yet: the theme's gray tile with a hairline, and the play mark in the accent on it.
func startTile(cx *core.Scope) core.Node {
	k := theme.Of(cx)
	return core.View{TestID: "films-start-art",
		Style:     core.Style{Width: core.Pt(88), Height: core.Pt(88), FlexShrink: 0, CornerRadius: 24, BorderWidth: 0.5, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		StyleFunc: func() core.Style { return core.Style{Background: k.Tile(), BorderColor: k.Shade(0.06)} },
		Children: []core.Node{
			core.Icon{SVG: startGlyph, TestID: "films-start-glyph", Style: core.Style{Width: core.Pt(44), Height: core.Pt(44)}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Accent)},
		}}
}

// statusPage is an empty page as the prototype draws it: the illustration, the title at 20 points, the description centered in gray, and the actions in a row, in a column 420 wide, 90 points under the message bar.
func statusPage(cx *core.Scope, art core.Node, title, description func(*core.Scope) string, actions ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Symmetric(90, 28)}, Children: []core.Node{
		core.View{Style: core.Style{MaxWidth: core.Pt(420), AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			art,
			heading(2, core.Text{ContentFunc: func() string { return title(cx) }, Style: core.Style{FontSize: 20, FontWeight: ui.Bold, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)}),
			core.Text{ContentFunc: func() string { return description(cx) }, Style: core.Style{FontSize: 13, LineHeight: 20, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{Direction: core.Row, Gap: 10, Wrap: core.Wrap, JustifyContent: core.JustifyCenter, Margin: core.Edges{Top: 8}}, Children: actions},
		}},
	}}
}
