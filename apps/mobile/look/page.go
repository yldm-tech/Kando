package look

import (
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/ui"
)

// TabRoot is p as the root page of a tab's stack, titled title, with trailing in the header's end: on iOS the system's large title, which collapses as the page scrolls, with symbols as the system's own buttons for trailing; elsewhere a large title of its own at the top of the page (PageTitle), trailing in a row of its own over it.
func TabRoot(cx *core.Scope, p base.NavPage, title func() string, trailing []core.Node, symbols func() []core.HeaderSymbol) base.NavPage {
	return TabRootWith(cx, p, Head{Title: title, Trailing: trailing, Symbols: symbols})
}

// Head is what heads a tab's root page (TabRootWith): its large title, the view at the start of the row of buttons over it (the Library's switcher), the buttons at its end, the system's buttons for them on iOS, and whether the buttons share the title's row instead, as on a tablet, whose header is one row.
type Head struct {
	Title    func() string
	Leading  core.Node
	Trailing []core.Node
	Symbols  func() []core.HeaderSymbol
	Inline   bool
}

// TabRootWith is TabRoot with h's leading view and layout: on iOS Leading is the system header's start (HeaderLeft); elsewhere it starts the row of buttons over the title, or with Inline the title's row is the leading view, the title, then the buttons at its end.
func TabRootWith(cx *core.Scope, p base.NavPage, h Head) base.NavPage {
	colors := ui.ColorsOf(cx)
	p.StyleFunc = ui.Filled(colors.Background)
	if cx.Env().OS() == core.OSIOS {
		p.TitleFunc = h.Title
		p.HeaderLargeTitle = true
		p.HeaderStyleFunc = func() core.Style {
			return core.Style{Background: colors.Background(), Color: colors.Text(), TintColor: colors.Accent()}
		}
		if h.Leading != nil {
			p.HeaderLeft = h.Leading
		}
		if len(h.Trailing) > 0 {
			p.HeaderRight = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: h.Trailing}
			p.IOS.HeaderRightSymbols = h.Symbols
		}
		return p
	}
	// TODO(nsgo#485): Android's system header is a toolbar with no large title (Header.java), and a desktop's is ui.NavHeader's 44pt bar (navheader.go:50-64), so a tab's root page draws its title in its content; give it the platform's top app bar once NSGo has one https://github.com/nsgo-dev/nsgo/issues/485
	p.HeaderHidden = true
	head := core.Text{ContentFunc: h.Title, Role: core.RoleHeader, MaxLines: 1, Style: PageTitle(cx).Merge(core.Style{Padding: core.Edges{Top: 2, Right: 16, Bottom: 4, Left: 16}}),
		StyleFunc: ui.Colored(colors.Text)}
	if h.Inline {
		// One row: the leading view, the title, then the buttons at its end, the design's tablet header (padding 28 32 over the content's 32).
		head.Style.Padding = core.Edges{}
		head.Style.FlexShrink, head.Style.MinWidth = 1, core.Pt(0)
		var row []core.Node
		if h.Leading != nil {
			row = append(row, h.Leading)
		}
		row = append(row, head, core.View{Style: core.Style{FlexGrow: 1}})
		row = append(row, h.Trailing...)
		bar := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 28, Right: 32, Bottom: 4, Left: 32},
			PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: row}
		p.Children = append([]core.Node{bar}, p.Children...)
		return p
	}
	bar := core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: []core.Node{head}}
	if len(h.Trailing) == 0 && h.Leading == nil {
		// A page without buttons starts its title 56 points down, as the design's Settings does.
		bar.Style.Padding.Top, bar.Style.PaddingEnv.Top = 54, core.EnvSafeArea
		p.Children = append([]core.Node{bar}, p.Children...)
		return p
	}
	// The header's buttons sit in a row of their own over the title, at its end, as the system's do over a large title: a 44-point row 16 down, its targets 10 from the edge, so a 34-point circle in one ends 15 from it and the title starts 62 down. The leading view starts the row, 16 from the edge.
	var row []core.Node
	if h.Leading != nil {
		row = append(row, h.Leading)
	}
	row = append(row, core.View{Style: core.Style{FlexGrow: 1}})
	row = append(row, h.Trailing...)
	actions := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(44), Padding: core.Edges{Top: 16, Right: 10, Left: 16},
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: row}
	p.Children = append([]core.Node{actions, bar}, p.Children...)
	return p
}

// TabRootTitleRow is TabRoot with trailing at the end of the large title's own row rather than in a row over it, as the design's Jobs draws its plus level with its title; on iOS, where the system draws the header, it is TabRoot.
func TabRootTitleRow(cx *core.Scope, p base.NavPage, title func() string, trailing []core.Node, symbols func() []core.HeaderSymbol) base.NavPage {
	if cx.Env().OS() == core.OSIOS || len(trailing) == 0 {
		return TabRoot(cx, p, title, trailing, symbols)
	}
	colors := ui.ColorsOf(cx)
	p.StyleFunc = ui.Filled(colors.Background)
	p.HeaderHidden = true
	head := core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, Style: PageTitle(cx).Merge(core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.Edges{Top: 2, Bottom: 4, Left: 16}}),
		StyleFunc: ui.Colored(colors.Text)}
	// The title starts 56 down as on a page without buttons, and its buttons' 44-point targets end 10 from the edge, so a 34-point circle in one ends 15 from it.
	row := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 54, Right: 10}, PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea}},
		Children: append([]core.Node{head}, trailing...)}
	p.Children = append([]core.Node{row}, p.Children...)
	return p
}

// ContentStyle is the column of a tab's page and the pages it opens: inset from the screen's sides and their safe area, no wider than a reading column on a tablet.
func ContentStyle(*core.Scope) core.Style {
	return core.Style{Padding: core.Edges{Top: 8, Right: 16, Left: 16}, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea},
		MaxWidth: core.Pt(720), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 28}
}

// BottomPad is the room under a scrolled list's end, clear of iOS's tab bar floating over it and of the mini player; read it in a StyleFunc.
func BottomPad(cx *core.Scope) core.Points { return 24 + ui.TabBarInset(cx) + MiniRoom(cx) }

// Column is ContentStyle with BottomPad under it, as a StyleFunc.
func Column(cx *core.Scope) func() core.Style {
	return func() core.Style {
		s := ContentStyle(cx)
		s.Padding.Bottom = BottomPad(cx)
		return s
	}
}

// Tablet reports whether the app runs on a tablet, whose shell shows a sidebar on its side and whose sheets show in the middle of the screen.
func Tablet(cx *core.Scope) bool { return cx.Env().Idiom() == core.IdiomTablet }

// NavPage is ui.NavPage, a page a tab's page opens, under header: on Android the system's toolbar with its back arrow, titled in Material's title large (SubPageTitle) left of the bar on the bar's frosted fill, as the design's top app bar; elsewhere ui.NavPage's own.
// TODO(nsgo#485): Android's toolbar takes only a title, its color, the arrow's tint and the bar's background (Header.java:55-99), and titles in the platform's toolbar style, so the app gives it its title as the header's view (base.NavPage.Header, which ui.NavPage makes the title view) in the design's 22pt regular; set the title's style once NSGo has Material's top app bar https://github.com/nsgo-dev/nsgo/issues/485
func NavPage(cx *core.Scope, p base.NavPage, header ui.NavHeader) base.NavPage {
	if cx.Env().OS() != core.OSAndroid || !mobile.IsAvailable(cx) {
		return ui.NavPage(cx, p, header)
	}
	colors := ui.ColorsOf(cx)
	if p.Header == nil {
		if title := subPageTitle(p, header); title != nil {
			p.Header = core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, Style: SubPageTitle, StyleFunc: ui.Colored(colors.Text)}
		}
	}
	p = ui.NavPage(cx, p, header)
	bar := theme.Of(cx).Bar
	p.HeaderStyleFunc = func() core.Style {
		return core.Style{Background: bar(), Color: colors.Text(), TintColor: colors.Text()}
	}
	return p
}

// subPageTitle is the title a page and its header give it, the header's first, nil for a page without one.
func subPageTitle(p base.NavPage, header ui.NavHeader) func() string {
	switch {
	case header.TitleFunc != nil:
		return header.TitleFunc
	case header.Title != "":
		return func() string { return header.Title }
	case p.TitleFunc != nil:
		return p.TitleFunc
	case p.Title != "":
		return func() string { return p.Title }
	}
	return nil
}
