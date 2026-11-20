package look

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// TabRoot is p as the root page of a tab's stack, titled title, with trailing in the header's end and p.HeaderLeft, when set, at its start: on iOS the system's large title, which collapses as the page scrolls, with symbols as the system's own buttons for trailing; elsewhere a large title of its own at the top of the page (PageTitle), the header's views in a row of their own over it.
func TabRoot(cx *core.Scope, p base.NavPage, title func() string, trailing []core.Node, symbols func() []core.HeaderSymbol) base.NavPage {
	colors := ui.ColorsOf(cx)
	p.StyleFunc = ui.Filled(colors.Background)
	if cx.Env().OS() == core.OSIOS {
		p.TitleFunc = title
		p.HeaderLargeTitle = true
		p.HeaderStyleFunc = func() core.Style {
			return core.Style{Background: colors.Background(), Color: colors.Text(), TintColor: colors.Accent()}
		}
		if len(trailing) > 0 {
			p.HeaderRight = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: trailing}
			p.IOS.HeaderRightSymbols = symbols
		}
		return p
	}
	// TODO(nsgo#485): ui.NavPage draws a 44pt desktop header on Android (navpage.go:44-50, navheader.go:50-64), with no large title or Material top app bar, so a tab's root page draws its title in its content; give it the platform's top app bar once NSGo has one https://github.com/nsgo-dev/nsgo/issues/485
	p.HeaderHidden = true
	head := core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, Style: PageTitle(cx).Merge(core.Style{Padding: core.Edges{Top: 2, Right: 16, Bottom: 4, Left: 16}}),
		StyleFunc: ui.Colored(colors.Text)}
	bar := core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: []core.Node{head}}
	if p.HeaderLeft != nil {
		// The header's start, as the Library's switcher, leads the row of its buttons, which the room between keeps at the end.
		trailing = append([]core.Node{p.HeaderLeft, core.View{Style: core.Style{FlexGrow: 1}}}, trailing...)
		p.HeaderLeft = nil
	}
	if len(trailing) == 0 {
		// A page without buttons starts its title 56 points down, as the design's Settings does.
		bar.Style.Padding.Top, bar.Style.PaddingEnv.Top = 54, core.EnvSafeArea
		p.Children = append([]core.Node{bar}, p.Children...)
		return p
	}
	// The header's buttons sit in a row of their own over the title, at its end, as the system's do over a large title: a 44-point row 16 down, its targets 10 from the edge, so a 34-point circle in one ends 15 from it and the title starts 62 down.
	actions := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyEnd, Gap: 8, MinHeight: core.Pt(44), Padding: core.Edges{Top: 16, Right: 10, Left: 16},
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Right: core.EnvSafeArea, Left: core.EnvSafeArea}}, Children: trailing}
	p.Children = append([]core.Node{actions, bar}, p.Children...)
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
