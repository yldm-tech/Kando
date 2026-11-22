package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/sites"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// startWidth is how wide the start's text and its sites run at most, so its sentence stays a short line under the title and its tiles three to a row.
const startWidth core.Points = 520

// startSites are the sites the start offers, by domain, in the order they show: the ones learners study from most.
var startSites = []string{"youtube.com", "bilibili.com", "ted.com", "nhk.or.jp", "nicovideo.jp", "bbc.co.uk"}

// startColumns is how many of startSites a row of the start holds.
const startColumns = 3

// start is the page before a web page is open, and after home: the globe, what the page is for, the sites learners study from most, which open in the browser, 支持的网站…, which lists the rest, the line of whose sign-in the pages and downloads borrow (signInLine), and 清除登录信息… at its foot where the pages' sign-ins can be cleared.
type start struct{ a App }

// Render implements core.Node.
func (s start) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	state := live.Memo(cx, a, a.BrowseState)
	heading := func() string { return locales.BrowseEmptyTitle(cx) }
	title := core.View{
		Role:               core.RoleHeader,
		AccessibilityLevel: func() int { return 1 },
		LabelFunc:          heading,
		Children: []core.Node{core.Text{ContentFunc: heading, Style: ui.TypeHeading.Merge(core.Style{TextAlign: core.TextAlignCenter}),
			StyleFunc: ui.Colored(colors.Text)}},
	}
	description := core.Text{
		ContentFunc: func() string { return locales.BrowseEmptyDescription(cx) },
		Style:       core.Style{FontSize: 14, LineHeight: 20, TextAlign: core.TextAlignCenter, Margin: core.Edges{Top: 8}},
		StyleFunc:   ui.Colored(colors.Muted),
	}
	status := core.View{
		Role:      core.RoleGroup,
		LabelFunc: heading,
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Width: core.Pct(100), MinWidth: core.Pt(0), MaxWidth: core.Pt(startWidth), JustifyContent: core.JustifyCenter,
			AlignItems: core.AlignCenter, Padding: core.Symmetric(24, 24)},
		Children: []core.Node{
			core.View{Style: core.Style{Margin: core.Edges{Bottom: 16}}, Children: []core.Node{ui.Icon(svg(iconGlobeThin), 48, colors.Muted)}},
			title,
			description,
			startTiles(cx, a),
			ui.Button{TextFunc: func() string { return locales.SitesMenuItem(cx) }, Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "browse-sites", OnPress: live.Do(a, a.OpenSites)},
			signInLine(cx, a, state),
		},
	}
	clear := core.Show{When: func() bool { return state().Clearable }, Then: func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{Direction: core.Row, FlexShrink: 0, JustifyContent: core.JustifyCenter, Padding: core.Edges{Bottom: 24}},
			Children: []core.Node{ui.Button{TextFunc: func() string { return locales.BrowseClearData(cx) }, Variant: ui.ButtonSecondary, TestID: "browse-clear",
				Disabled: func() bool { return state().Clearing }, OnPress: live.Do(a, a.BrowseAskClear)}},
		}
	}}
	return core.View{
		TestID:   "browse-start",
		Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignCenter},
		Children: []core.Node{status, clear},
	}
}

// startTiles are startSites as the sites sheet draws them, startColumns to a row across the start's width; pressing one opens it.
func startTiles(cx *core.Scope, a App) core.Node {
	var found []catalog.Site
	for _, domain := range startSites {
		for _, s := range catalog.Load().Sites {
			if s.Domain == domain {
				found = append(found, s)
				break
			}
		}
	}
	var rows []core.Node
	for i := 0; i < len(found); i += startColumns {
		var cells []core.Node
		for j := i; j < i+startColumns; j++ {
			cell := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignStretch}}
			if j < len(found) {
				site := found[j]
				cell.Children = []core.Node{sites.Tile(cx, a, func() catalog.Site { return site }, live.Do(a, func() { a.OpenSite(site) }))}
			}
			cells = append(cells, cell)
		}
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignStretch}, Children: cells})
	}
	return core.View{
		TestID:    "browse-sites-tiles",
		Role:      core.RoleGroup,
		LabelFunc: func() string { return locales.SitesGridLabel(cx) },
		Style:     core.Style{AlignSelf: core.AlignStretch, Gap: 8, Margin: core.Edges{Top: 24, Bottom: 8}},
		Children:  rows,
	}
}

// signInLine is the start's line of the sign-in the pages and downloads borrow: the browser's, with 刷新登录, which reads it again, and 更改…, or none, with 选择浏览器…; 更改… and 选择浏览器… open Settings › 通用, where it is chosen.
func signInLine(cx *core.Scope, a App, state func() State) core.Node {
	colors := ui.ColorsOf(cx)
	on := core.NewMemo(cx, func() bool { return state().SignIn != "" })
	text := core.Text{
		ContentFunc: func() string {
			if name := state().SignIn; name != "" {
				return locales.BrowseSignInOn(cx, name)
			}
			return locales.BrowseSignInOff(cx)
		},
		Style:     core.Style{FontSize: 12, LineHeight: 16, FlexShrink: 1, TextAlign: core.TextAlignCenter},
		StyleFunc: ui.Colored(colors.Muted),
	}
	settings := live.Do(a, func() { a.OpenSettings(sectionid.General) })
	// The buttons are built again once a browser is chosen or none is.
	buttons := core.Show{
		When: on.Get,
		Then: func(cx *core.Scope) core.Node {
			return core.Group([]core.Node{
				ui.Button{TextFunc: func() string { return locales.BrowseSignInRefresh(cx) }, Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "browse-signin-refresh",
					Disabled: func() bool { return state().SigningIn }, OnPress: live.Do(a, a.BrowseRefreshSignIn)},
				ui.Button{TextFunc: func() string { return locales.BrowseSignInChange(cx) }, Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "browse-signin", OnPress: settings},
			})
		},
		Else: func(cx *core.Scope) core.Node {
			return ui.Button{TextFunc: func() string { return locales.BrowseSignInChoose(cx) }, Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "browse-signin", OnPress: settings}
		},
	}
	return core.View{
		TestID:   "browse-signin-line",
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, Wrap: core.Wrap, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 2, Margin: core.Edges{Top: 4}},
		Children: []core.Node{text, buttons},
	}
}
