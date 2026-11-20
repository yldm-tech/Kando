// Package shell is the app's top-level sections and the way between them: on a phone the tab bar, the system's on iOS and Android; on a tablet a sidebar while it is on its side and a tab bar while it is upright, the sections kept as it turns.
package shell

import (
	"kando/apps/mobile/brand"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/ui"
)

// Tab is a top-level section.
type Tab struct {
	Text func() string
	Icon func() *core.SVG
	// SystemIcon is the SF Symbol of the system's tab bar on an iPhone.
	SystemIcon string
	// Content is the section, a stack of pages, built when first shown and kept while others show.
	Content func(cx *core.Scope) core.Node
	TestID  string
	// Badge, when set, is a count the drawn tab bar and a tablet's sidebar show on the section while it is not 0, in the badges' orange while BadgeWarn reports true, red otherwise.
	// The count is not in the section's spoken name yet: base.Tab.Label is a fixed string (nsgo#145, above), so a screen reader does not hear it.
	// TODO(nsgo#741): ui.ShellTab and mobile.TabBarItem have no badge (ui/tabshell.go:10-20, core/tabbar.go:20-28), so the system's tab bar of an iPhone or an Android phone (UITabBarItem.badgeValue, BottomNavigationView's badge) shows no count, and the app shows the jobs' count on the Library's header there instead (SystemBar); pass Badge through once they take one, and drop that button. https://github.com/nsgo-dev/nsgo/issues/741
	Badge     func() int
	BadgeWarn func() bool
}

// badge is t's count where it has one, size tall, on the accent's fill while onAccent reports true.
func (t Tab) badge(testID string, size core.Points, onAccent func() bool, st core.Style) []core.Node {
	if t.Badge == nil {
		return nil
	}
	return []core.Node{look.Count{TestID: testID, Count: t.Badge, Warn: t.BadgeWarn, Size: size, OnAccent: onAccent, Style: st}}
}

// Shell is the sections and the bar or sidebar between them. Selected is the app's, set from OnChange, which a press on the section shown reports too. A tablet's sidebar is headed by the app's icon and name over Library, the library the Library shows, which pressed runs OnLibrary; Aside follows the sections down it, taking the room left, as the subscriptions do, and Footer ends it.
type Shell struct {
	Tabs      []Tab
	Selected  func() int
	OnChange  func(int)
	Library   func() string
	OnLibrary func()
	Aside     core.Node
	Footer    core.Node
}

// Sidebar reports whether the shell under cx shows its sidebar: a tablet on its side, wide enough for the sidebar and a page beside it. Read it in a binding.
func Sidebar(cx *core.Scope) bool {
	w := cx.Env().Window()
	return look.Tablet(cx) && w.Width > w.Height && w.Width >= 900
}

// OverTabBar reports whether a section's pages sit over the tab bar, not beside a sidebar, for look.InTab.
func OverTabBar(cx *core.Scope) func() bool { return func() bool { return !Sidebar(cx) } }

// SystemBar reports whether the shell under cx is the system's tab bar, an iPhone's or an Android phone's, which shows no Badge.
func SystemBar(cx *core.Scope) bool { return !look.Tablet(cx) && mobile.IsAvailable(cx) }

// Render implements core.Node.
func (s Shell) Render(cx *core.Scope) core.Node {
	if SystemBar(cx) {
		return s.phone(cx)
	}
	return s.drawn(cx)
}

// phone is NSGo's tab shell with the system's bar, an iPhone's or an Android phone's.
func (s Shell) phone(cx *core.Scope) core.Node {
	ios := cx.Env().OS() == core.OSIOS
	tabs := make([]ui.ShellTab, len(s.Tabs))
	for i, t := range s.Tabs {
		tabs[i] = ui.ShellTab{Text: t.Text(), Icon: t.Icon, TestID: t.TestID, Content: t.Content}
		if ios {
			tabs[i].SystemIcon = t.SystemIcon
		}
	}
	// TODO(nsgo#145): ui.ShellTab.Text and TabShell.Label are strings only (ui/tabshell.go:10-36), so the tab titles keep the language they were built in; pass TextFunc/LabelFunc once ShellTab has them (mobile.TabBar already takes ItemsFunc) https://github.com/nsgo-dev/nsgo/issues/145
	// TODO(nsgo): Android's navigation bar takes only the accent (core/tabbar.go tint) and fixes its fill, its idle color and its indicator's opacity (TabBar.java:62, 173-195), so it shows Material's lavender bar with the accent for the selected tab; give it the card's fill (colors.Surface), theme.TabInactive idle, the accent at theme.M3NavIndicator behind the selected tab and theme.M3NavActive (the accent in the dark) for it once the bar takes them.
	return ui.TabShell{Label: locales.AppName(cx), Selected: s.Selected, OnChange: s.OnChange, Tabs: tabs}
}

// drawn is the shell the app draws itself, a tablet's and, where the system has no tab bar, a phone's: the sections beside a sidebar while a tablet is on its side and over a tab bar otherwise, switching the one for the other in place, so the sections and their pages stay as the tablet turns.
func (s Shell) drawn(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	env := cx.Env()
	side := func() bool { return Sidebar(cx) }
	var sections []core.Node
	for i, tab := range s.Tabs {
		// A section is built when first shown, then hidden while others show, keeping its state.
		seen := false
		sections = append(sections, core.View{TestID: tab.TestID, Children: []core.Node{core.Show{
			When: func() bool { seen = seen || s.Selected() == i; return seen },
			Then: tab.Content}},
			StyleFunc: func() core.Style {
				st := ui.FillRest
				if s.Selected() != i {
					st.Display = core.DisplayNone
				}
				return st
			}})
	}
	var bar core.Node
	if look.Tablet(cx) {
		bar = s.wideBar(cx)
	} else {
		bar = s.bar(cx)
	}
	children := []core.Node{core.View{Style: ui.FillRest, Children: sections},
		core.View{Children: []core.Node{bar}, StyleFunc: func() core.Style {
			if side() || env.KeyboardHeight() > 0 {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}}}
	if look.Tablet(cx) {
		children = append([]core.Node{core.View{Children: []core.Node{s.sidebar(cx)}, StyleFunc: func() core.Style {
			if side() {
				return core.Style{}
			}
			return core.Style{Display: core.DisplayNone}
		}}}, children...)
	}
	return core.View{TestID: "shell", Style: ui.FillRest,
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Background(), Color: colors.Text(), Direction: core.Column}
			if side() {
				st.Direction = core.Row
			}
			return st
		},
		Children: children}
}

// sidebar is the sections as a list down the tablet's side under the app's icon and name, the selected one filled in the accent, and the footer at its foot.
func (s Shell) sidebar(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	var rows []core.Node
	for i, tab := range s.Tabs {
		on := func() bool { return s.Selected() == i }
		ink := func() core.Color {
			if on() {
				return colors.OnAccent()
			}
			return colors.Accent()
		}
		// TODO(nsgo#145): base.Tab.Label is a string only (base/tabs.go:13-28), so a tab's spoken name keeps the language it was built in; pass a LabelFunc once Tab has one https://github.com/nsgo-dev/nsgo/issues/145
		rows = append(rows, base.Tab{TestID: "sidebar-" + tab.TestID, Label: tab.Text(), Selected: on, OnPress: func(core.PressEvent) { s.OnChange(i) },
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(44), Padding: core.Symmetric(0, 12), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
			StyleFunc: func() core.Style {
				st := core.Style{Color: colors.Text(), Active: &core.Style{Background: colors.Pressed()}, FocusVisible: ui.FocusRing(cx, -2)}
				if on() {
					st.Background, st.Color = colors.Accent(), colors.OnAccent()
				}
				return st
			},
			Children: append([]core.Node{
				ui.IconOf(tab.Icon, 22, ink),
				core.Text{ContentFunc: tab.Text, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1, FlexGrow: 1})},
			}, tab.badge("sidebar-badge-"+tab.TestID, 22, on, core.Style{})...)})
	}
	return core.View{TestID: "sidebar", Style: core.Style{Width: core.Pt(270), FlexGrow: 1, Borders: core.Edges{Right: 0.5}, Padding: core.Edges{Top: 20, Right: 14, Bottom: 14, Left: 14},
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Right: colors.Line()}}
		},
		Children: []core.Node{
			s.header(cx),
			base.Tabs{TestID: "sidebar-tabs", Label: locales.AppName(cx), Style: core.Style{Gap: 4, Margin: core.Edges{Top: 18}}, Children: rows},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{s.aside()}},
			s.Footer,
		}}
}

// header heads the sidebar: the app's icon and name over the library the Library shows, a button that picks another where the shell has OnLibrary.
func (s Shell) header(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	name := func() string { return locales.AppName(cx) }
	texts := []core.Node{core.Text{ContentFunc: name, Role: core.RoleHeader, MaxLines: 1, Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)}}
	if s.Library != nil {
		texts = append(texts, core.Text{TestID: "sidebar-library", ContentFunc: s.Library, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)})
	}
	children := []core.Node{
		core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 9, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: texts},
	}
	st := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(52), Padding: core.Symmetric(6, 8), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous}
	if s.OnLibrary == nil {
		return core.View{TestID: "sidebar-header", Style: st, Children: children}
	}
	children = append(children, ui.Icon(lucide.ChevronDown, 10, colors.Muted))
	label := func() string { return locales.FilmsLibrary(cx, s.Library()) }
	return base.Button{TestID: "sidebar-header", LabelFunc: label, OnPress: func(core.PressEvent) { s.OnLibrary() }, Style: st,
		StyleFunc: func() core.Style {
			return core.Style{Active: &core.Style{Background: colors.Pressed()}, FocusVisible: ui.FocusRing(cx, -2)}
		},
		Children: children}
}

// aside is what follows the sections down the sidebar, nothing where the shell has none.
func (s Shell) aside() core.Node {
	if s.Aside == nil {
		return core.View{}
	}
	return s.Aside
}

// barStyle is the drawn tab bars' frosted fill.
func barStyle(cx *core.Scope) func() core.Style {
	return func() core.Style {
		return core.Style{Background: theme.Of(cx).Bar(), BackdropFilters: theme.BarBlur}
	}
}

// barLine is the hairline over a drawn tab bar: a view of its own, as a border on the bar's frosted view does not show on macOS.
// TODO(nsgo#935): on Apple a border on some sides only is a shape sublayer under the view's children (internal/platform/apple/border.go:12-16), and a view with BackdropFilters adds its blur and the tint that draws its Background as a subview (internal/platform/macos/material.go:191-221), which covers it, so the bar draws its hairline as a child view; use Borders on the bar once the border draws over the backdrop. https://github.com/nsgo-dev/nsgo/issues/935
func barLine(cx *core.Scope) core.Node {
	return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(0.5), PointerEvents: core.PointerEventsNone},
		StyleFunc: ui.Filled(theme.Of(cx).BarLine)}
}

// tabStyle is a drawn tab's color: the accent while selected, the tab bar's grey otherwise.
func tabStyle(cx *core.Scope) func() core.Style {
	colors := ui.ColorsOf(cx)
	return func() core.Style {
		return core.Style{Color: theme.TabInactive, Active: &core.Style{Opacity: core.Alpha(0.6)}, Selected: &core.Style{Color: colors.Accent()}, FocusVisible: ui.FocusRing(cx, -2)}
	}
}

// bar is a phone's drawn tab bar, where the system has none: the sections' icons over their names, the selected one in the accent, each with its badge at the icon's corner.
func (s Shell) bar(cx *core.Scope) core.Node {
	var tabs []core.Node
	for i, tab := range s.Tabs {
		// TODO(nsgo#145): base.Tab.Label is a string only (base/tabs.go:13-28), so a tab's spoken name keeps the language it was built in; pass a LabelFunc once Tab has one https://github.com/nsgo-dev/nsgo/issues/145
		tabs = append(tabs, base.Tab{TestID: "bar-" + tab.TestID, Label: tab.Text(), Selected: func() bool { return s.Selected() == i },
			OnPress: func(core.PressEvent) { s.OnChange(i) },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 3,
				MinHeight: core.Pt(54), CornerRadius: 10},
			StyleFunc: tabStyle(cx),
			Children: []core.Node{
				core.View{Children: append([]core.Node{ui.IconOf(tab.Icon, 25, ui.Inherited)},
					tab.badge("bar-badge-"+tab.TestID, 18, nil, core.Style{Position: core.Absolute, Top: core.Pt(-3), Left: core.Pt(19)})...)},
				core.Text{ContentFunc: tab.Text, MaxLines: 1, Style: core.Style{FontSize: 10, LineHeight: 12, FontWeight: core.FontWeightSemibold}},
			}})
	}
	// Where the screen has no safe area at its foot, as a window on a Mac, the bar keeps the design's 84 points, the home indicator's room included.
	env := cx.Env()
	fill := barStyle(cx)
	return core.View{Style: core.Style{Direction: core.Row, PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: func() core.Style {
			st := fill()
			st.Padding = core.Edges{Top: 4, Right: 8, Left: 8, Bottom: 4}
			if env.SafeArea().Bottom == 0 {
				st.Padding.Bottom = 26
			}
			return st
		},
		Children: []core.Node{
			barLine(cx),
			base.Tabs{Label: locales.AppName(cx), TestID: "tab-bar", Style: core.Style{Direction: core.Row, FlexGrow: 1}, Children: tabs},
		}}
}

// wideBar is the tab bar of an upright tablet: each section's icon beside its name, the selected one in the accent, with its badge after the name.
func (s Shell) wideBar(cx *core.Scope) core.Node {
	var tabs []core.Node
	for i, tab := range s.Tabs {
		// TODO(nsgo#145): base.Tab.Label is a string only (base/tabs.go:13-28), so a tab's spoken name keeps the language it was built in; pass a LabelFunc once Tab has one https://github.com/nsgo-dev/nsgo/issues/145
		tabs = append(tabs, base.Tab{TestID: "bar-" + tab.TestID, Label: tab.Text(), Selected: func() bool { return s.Selected() == i },
			OnPress: func(core.PressEvent) { s.OnChange(i) },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 8,
				MinHeight: core.Pt(60), CornerRadius: 10},
			StyleFunc: tabStyle(cx),
			Children: append([]core.Node{
				ui.IconOf(tab.Icon, 24, ui.Inherited),
				core.Text{ContentFunc: tab.Text, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightSemibold, FlexShrink: 1}},
			}, tab.badge("bar-badge-"+tab.TestID, 20, nil, core.Style{})...)})
	}
	return core.View{Style: core.Style{Direction: core.Row, Padding: core.Edges{Right: 40, Bottom: 14, Left: 40},
		PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: barStyle(cx), Children: []core.Node{
			barLine(cx),
			base.Tabs{Label: locales.AppName(cx), TestID: "tab-bar", Style: core.Style{Direction: core.Row, FlexGrow: 1, Gap: 4}, Children: tabs},
		}}
}
