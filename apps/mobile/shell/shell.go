// Package shell is the app's top-level sections and the way between them: on a phone the tab bar, the system's on iOS; on a tablet a sidebar while it is on its side and a tab bar while it is upright, the sections kept as it turns.
package shell

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
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
	// Badge, when set, is a count a tablet's sidebar and tab bar show on the section while it is not 0, in warning's color while BadgeWarn reports true.
	// TODO(nsgo): ui.ShellTab has no badge (ui/tabshell.go:10-20), so a phone's tab bar, iOS's system one too (UITabBarItem.badgeValue), shows no count; a phone shows Jobs' count on the Library's Jobs button instead.
	Badge     func() int
	BadgeWarn func() bool
}

// badge is t's count where it has one.
func (t Tab) badge(testID string, st core.Style) []core.Node {
	if t.Badge == nil {
		return nil
	}
	return []core.Node{look.Count{TestID: testID, Count: t.Badge, Warn: t.BadgeWarn, Style: st}}
}

// Shell is the sections and the bar or sidebar between them. Selected is the app's, set from OnChange, which a press on the section shown reports too. Footer ends a tablet's sidebar.
type Shell struct {
	Tabs     []Tab
	Selected func() int
	OnChange func(int)
	Footer   core.Node
}

// Sidebar reports whether the shell under cx shows its sidebar: a tablet on its side, wide enough for the sidebar and a page beside it. Read it in a binding.
func Sidebar(cx *core.Scope) bool {
	w := cx.Env().Window()
	return look.Tablet(cx) && w.Width > w.Height && w.Width >= 900
}

// OverTabBar reports whether a section's pages sit over the tab bar, not beside a sidebar, for look.InTab.
func OverTabBar(cx *core.Scope) func() bool { return func() bool { return !Sidebar(cx) } }

// Render implements core.Node.
func (s Shell) Render(cx *core.Scope) core.Node {
	if !look.Tablet(cx) {
		return s.phone(cx)
	}
	return s.tablet(cx)
}

// phone is NSGo's tab shell: the system's bar on iOS, a drawn one elsewhere.
func (s Shell) phone(cx *core.Scope) core.Node {
	// TODO(nsgo#484): Android's system tab bar takes drawable names only, with no way to bundle the app's own (core/tabbar.go:20-26, TabBar.java:154-170); drawn bar until it can show vector icons https://github.com/nsgo-dev/nsgo/issues/484
	ios := cx.Env().OS() == core.OSIOS
	tabs := make([]ui.ShellTab, len(s.Tabs))
	for i, t := range s.Tabs {
		tabs[i] = ui.ShellTab{Text: t.Text(), Icon: t.Icon, TestID: t.TestID, Content: t.Content}
		if ios {
			tabs[i].SystemIcon = t.SystemIcon
		}
	}
	// TODO(nsgo#145): ui.ShellTab.Text and TabShell.Label are strings only (ui/tabshell.go:10-36), so the tab titles keep the language they were built in; pass TextFunc/LabelFunc once ShellTab has them (mobile.TabBar already takes ItemsFunc) https://github.com/nsgo-dev/nsgo/issues/145
	return ui.TabShell{Label: locales.AppName(cx), Selected: s.Selected, OnChange: s.OnChange, Tabs: tabs}
}

// tablet lays the sections beside a sidebar on its side and over a tab bar upright, switching the one for the other in place, so the sections and their pages stay as the tablet turns.
func (s Shell) tablet(cx *core.Scope) core.Node {
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
	return core.View{TestID: "shell", Style: ui.FillRest,
		StyleFunc: func() core.Style {
			st := core.Style{Background: colors.Background(), Color: colors.Text(), Direction: core.Column}
			if side() {
				st.Direction = core.Row
			}
			return st
		},
		Children: []core.Node{
			core.View{Children: []core.Node{s.sidebar(cx)}, StyleFunc: func() core.Style {
				if side() {
					return core.Style{}
				}
				return core.Style{Display: core.DisplayNone}
			}},
			core.View{Style: ui.FillRest, Children: sections},
			core.View{Children: []core.Node{s.bar(cx)}, StyleFunc: func() core.Style {
				if side() || env.KeyboardHeight() > 0 {
					return core.Style{Display: core.DisplayNone}
				}
				return core.Style{}
			}},
		}}
}

// sidebar is the sections as a list down the tablet's side under the app's name, the selected one filled in the accent, and the footer at its foot.
func (s Shell) sidebar(cx *core.Scope) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
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
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(44), Padding: core.Symmetric(0, 12), CornerRadius: 10, BorderCurve: core.BorderCurveContinuous},
			StyleFunc: func() core.Style {
				st := core.Style{Color: colors.Text(), Active: &core.Style{Background: colors.Pressed()}, FocusVisible: ui.FocusRing(cx, -2)}
				if on() {
					st.Background, st.Color = colors.Accent(), colors.OnAccent()
				}
				return st
			},
			Children: append([]core.Node{
				ui.IconOf(tab.Icon, 20, ink),
				core.Text{ContentFunc: tab.Text, MaxLines: 1, Style: look.Body.Merge(core.Style{FontWeight: core.FontWeightSemibold, FlexShrink: 1, FlexGrow: 1})},
			}, tab.badge("sidebar-badge-"+tab.TestID, core.Style{})...)})
	}
	return core.View{TestID: "sidebar", Style: core.Style{Width: core.Pt(270), FlexGrow: 1, Gap: 16, Borders: core.Edges{Right: 0.5}, Padding: core.Edges{Top: 12, Right: 12, Bottom: 12, Left: 12},
		PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Right: colors.Line()}}
		},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 4, Left: 4, Bottom: 4}}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), CornerRadius: 9, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					StyleFunc: ui.Filled(tints.Accent), Children: []core.Node{ui.IconOf(s.Tabs[0].Icon, 20, colors.Accent)}},
				core.Text{ContentFunc: func() string { return locales.AppName(cx) }, Role: core.RoleHeader, MaxLines: 1, Style: look.Title.Merge(core.Style{FontWeight: core.FontWeightBold})},
			}},
			base.Tabs{TestID: "sidebar-tabs", Label: locales.AppName(cx), Style: core.Style{Gap: 2}, Children: rows},
			core.View{Style: core.Style{FlexGrow: 1}},
			s.Footer,
		}}
}

// bar is the tab bar of an upright tablet: the sections' icons over their names, the selected one in the accent.
func (s Shell) bar(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	var tabs []core.Node
	for i, tab := range s.Tabs {
		// TODO(nsgo#145): base.Tab.Label is a string only (base/tabs.go:13-28), so a tab's spoken name keeps the language it was built in; pass a LabelFunc once Tab has one https://github.com/nsgo-dev/nsgo/issues/145
		tabs = append(tabs, base.Tab{TestID: "bar-" + tab.TestID, Label: tab.Text(), Selected: func() bool { return s.Selected() == i },
			OnPress: func(core.PressEvent) { s.OnChange(i) },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 2,
				MinHeight: core.Pt(50), CornerRadius: 10},
			StyleFunc: func() core.Style {
				return core.Style{Color: colors.Muted(), Active: &core.Style{Background: colors.Pressed()}, Selected: &core.Style{Color: colors.Accent()}, FocusVisible: ui.FocusRing(cx, -2)}
			},
			Children: []core.Node{
				core.View{Children: append([]core.Node{ui.IconOf(tab.Icon, 24, ui.Inherited)},
					tab.badge("bar-badge-"+tab.TestID, core.Style{Position: core.Absolute, Top: core.Pt(-4), Left: core.Pt(16)})...)},
				core.Text{ContentFunc: tab.Text, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightMedium})},
			}})
	}
	return core.View{Style: core.Style{Direction: core.Row, Padding: core.Symmetric(4, 40), Borders: core.Edges{Top: 0.5},
		PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Top: colors.Line()}}
		}, Children: []core.Node{
			base.Tabs{Label: locales.AppName(cx), TestID: "tab-bar", Style: core.Style{Direction: core.Row, FlexGrow: 1, Gap: 4}, Children: tabs},
		}}
}
