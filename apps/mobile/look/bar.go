package look

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// inTabKey marks the pages of a tab's stack while they sit over a tab bar rather than the screen's foot.
var inTabKey = core.NewKey[func() bool]("look.inTab", nil)

// InTab marks what cx builds as the pages of a tab's stack, over a tab bar while over reports true (a tablet's shell shows a sidebar in its place on its side); call it where the stack is built.
func InTab(cx *core.Scope, over func() bool) { inTabKey.Provide(cx, over) }

// ActionBar is the bar of a page's actions at its foot, the last of its column, where a thumb reaches them: over the software keyboard while it shows, else over the tab bar, or over the safe area on a page outside the tabs.
func ActionBar(cx *core.Scope, children ...core.Node) core.Node {
	over := inTabKey.Use(cx)
	inTab := func() bool { return over != nil && over() }
	return core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea}},
		// EnvEdges takes one inset an edge, so the bottom's switches between the keyboard's, the tab bar's and the safe area's.
		StyleFunc: func() core.Style {
			pad := core.Edges{Top: 12, Right: 16, Left: 16}
			env := core.EnvEdges{Right: core.EnvSafeArea, Left: core.EnvSafeArea}
			switch inset := ui.TabBarInset(cx); {
			case cx.Env().KeyboardHeight() > 0:
				pad.Bottom, env.Bottom = 8, core.EnvKeyboard
			case inset > 0 || inTab():
				// A drawn tab bar takes room of its own and keeps clear of the bottom safe area itself, so a page above it adds none.
				// TODO(nsgo#493): TabShell's drawn bar takes the bottom safe area but its sections' screens still report it (core/nav.go:140-155, ui/tabshell.go:139-140), so the app marks a tab's pages to leave it out; drop the mark once TabShell gives its sections insets without the bottom it covers https://github.com/nsgo-dev/nsgo/issues/493
				pad.Bottom = 12 + inset
			default:
				pad.Bottom, env.Bottom = 12, core.EnvSafeArea
			}
			return core.Style{Padding: pad, PaddingEnv: env}
		},
		Children: []core.Node{core.View{Style: core.Style{MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter, Gap: 8}, Children: children}}}
}
