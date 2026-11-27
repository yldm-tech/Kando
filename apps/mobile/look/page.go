package look

import (
	"slices"

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
	// TODO(nsgo#1842): Android's system header is now Material 3's top app bar, whose HeaderLargeTitle is a 152pt bar that collapses as the page scrolls (nsgo#485), but its large title is a TextView whose size alone is set (Header.java:85-86), in the platform's regular weight where the design's is 28pt medium (DisplayAndroid), with the buttons only in the bar's row over it, where Jobs has its plus level with its title (TabRootTitleRow) and a tablet's header is one row (Head.Inline); and a desktop's is ui.NavHeader's 44pt bar (navheader.go:50-64); so a tab's root page draws its title in its content; give an Android root page the system's large title once its type can be set https://github.com/nsgo-dev/nsgo/issues/1842
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

// NavPage is ui.NavPage, a page a tab's page opens, under header: on Android the system's toolbar with its back arrow, titled in Material's title large (SubPageTitle) left of the bar on the bar's frosted fill, as the design's top app bar, with header's Leading and Trailing at its ends (its Symbols are iOS's only); elsewhere ui.NavPage's own.
// TODO(nsgo#1842): Android's toolbar is now Material 3's top app bar, 64 tall with a 22sp title (nsgo#485), but the title's size alone is set (Header.java:237-239), so it keeps the platform toolbar's title appearance; the app gives the toolbar its title as the header's view (base.NavPage.Header, which ui.NavPage makes the title view) in the design's 22pt regular; give the toolbar the title itself once the header takes the title's type https://github.com/nsgo-dev/nsgo/issues/1842
func NavPage(cx *core.Scope, p base.NavPage, header ui.NavHeader) base.NavPage {
	if cx.Env().OS() == core.OSIOS && mobile.IsAvailable(cx) {
		return ui.NavPage(cx, iosSubPage(cx, p, header), header)
	}
	if cx.Env().OS() != core.OSAndroid || !mobile.IsAvailable(cx) {
		return ui.NavPage(cx, p, header)
	}
	colors := ui.ColorsOf(cx)
	if p.Header == nil {
		if title := subPageTitle(p, header); title != nil {
			// ui.NavPage keeps only the header's Scroll once the page has a Header of its own, and keeps the page's left and right views, so the header's buttons move there.
			if len(header.Leading) > 0 && p.HeaderLeft == nil {
				p.HeaderLeft = headerItems(header.Leading)
			}
			if len(header.Trailing) > 0 && p.HeaderRight == nil {
				p.HeaderRight = headerItems(header.Trailing)
			}
			ends := len(header.Leading) + len(header.Trailing)
			// The toolbar is the page's, as wide as the page, which beside a tablet's sidebar is narrower than the window: the title is bounded by the page's width once it is laid out, by the window's before.
			width := core.NewSignal(cx, core.Points(0))
			p.Children = append(slices.Clip(p.Children), core.View{PointerEvents: core.PointerEventsNone,
				Style:    core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Left: core.Pt(0), Height: core.Pt(0)},
				OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) }})
			p.Header = core.Text{ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, Style: SubPageTitle, StyleFunc: func() core.Style {
				page := width.Get()
				if page <= 0 {
					page = cx.Env().Window().Width
				}
				return core.Style{Color: colors.Text(), MaxWidth: core.Pt(SubPageTitleWidth(page, ends))}
			}}
		}
	}
	p = ui.NavPage(cx, p, header)
	bar := theme.Of(cx).Bar
	p.HeaderStyleFunc = func() core.Style {
		return core.Style{Background: bar(), Color: colors.Text(), TintColor: colors.Text()}
	}
	return p
}

// SubPageTitleWidth is the most a sub-page's title can take of Android's toolbar on a page page wide with buttons buttons in its ends: the toolbar is laid out at the title's own size (ui.NavPage), so without a bound a long title, as a computer's name, would run under the buttons rather than end in an ellipsis. The toolbar starts a title 72 after its back arrow's edge (Material's inset with a navigation icon) and holds each button in a 48pt target, the end's 12 from the edge (Header.java); 16 is left after the title. At least 48.
func SubPageTitleWidth(page core.Points, buttons int) core.Points {
	w := page - 72 - 16
	if buttons > 0 {
		w -= core.Points(48*buttons + 12)
	}
	return max(w, 48)
}

// iosSubPage is p as iOS's system header shows it with its title as the header's view (base.NavPage.Header, which ui.NavPage makes the bar's title view) in the system title's 17pt semibold (Title), rather than as the page's title, so that the page itself takes no accessibility label. The page keeps its title for the back button of a page pushed over it (HeaderBackTitle), and the header's buttons move to the page's ends, as ui.NavPage keeps only the header's Scroll once the page has a Header of its own.
// TODO(nsgo#2700): a titled screen's view is labelled with its title (core/nav.go:83-88), and iOS makes every labelled view an accessibility element (internal/platform/ios/access.go:294), so VoiceOver and UI automation reach the page as one element and none of the controls on it; give iOS's sub pages their title back (p.Title/TitleFunc) once a screen's title no longer makes it an element. HeaderBackTitle is a string with no Func twin, so the back title keeps the language the page was built in. https://github.com/nsgo-dev/nsgo/issues/2700
func iosSubPage(cx *core.Scope, p base.NavPage, header ui.NavHeader) base.NavPage {
	title := subPageTitle(p, header)
	if p.Header != nil || title == nil {
		return p
	}
	if p.HeaderBackTitle == "" {
		p.HeaderBackTitle = core.Untracked(cx, title)
	}
	if len(header.Leading) > 0 && p.HeaderLeft == nil {
		p.HeaderLeft = headerItems(header.Leading)
	}
	if len(header.Trailing) > 0 && p.HeaderRight == nil {
		p.HeaderRight = headerItems(header.Trailing)
		p.IOS.HeaderRightSymbols = header.Symbols
	}
	p.Title, p.TitleFunc = "", nil
	p.Header = core.Text{TestID: "nav-title", ContentFunc: title, Role: core.RoleHeader, MaxLines: 1, Style: Title, StyleFunc: ui.Colored(ui.ColorsOf(cx).Text)}
	return p
}

// headerItems is the buttons of a header's end in a row, as ui.NavPage puts them in the system's header.
func headerItems(children []core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: children}
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
