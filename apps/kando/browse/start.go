package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// startWidth is how wide the start's text runs at most, so its two sentences wrap into a short paragraph under the title rather than across the whole page.
const startWidth core.Points = 460

// start is the page before a web page is open, and after home: the globe, what the page is for, and 清除登录信息… at its foot where the pages' sign-ins can be cleared.
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
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MaxWidth: core.Pt(startWidth), JustifyContent: core.JustifyCenter,
			AlignItems: core.AlignCenter, Padding: core.Symmetric(24, 24)},
		Children: []core.Node{
			core.View{Style: core.Style{Margin: core.Edges{Bottom: 16}}, Children: []core.Node{ui.Icon(svg(iconGlobeThin), 48, colors.Muted)}},
			title,
			description,
		},
	}
	clear := core.Show{When: func() bool { return state().Clearable }, Then: func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Direction: core.Row, FlexShrink: 0, JustifyContent: core.JustifyCenter, Padding: core.Edges{Bottom: 24}},
			// The button's words are set as it is built, so it is built again for another language.
			Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
				return ui.Button{Text: locales.BrowseClearData(cx), Variant: ui.ButtonSecondary, TestID: "browse-clear",
					Disabled: func() bool { return state().Clearing }, OnPress: live.Do(a, a.BrowseAskClear)}
			})},
		}
	}}
	return core.View{
		TestID:   "browse-start",
		Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignCenter},
		Children: []core.Node{status, clear},
	}
}
