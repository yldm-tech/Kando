package sidebar

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// search is the field under the library switcher that narrows the library page to the done films whose names hold what is typed, as `kando list [query]` does, beside the sources it narrows with. It shows while the open library has films. ⌘F puts the keyboard in it (QueryFocus), and its clear button or Escape empties it. Its look is the settings window's search field in its rail: 28 points high, its corners 8, gray on the card without a border, a magnifying glass before the text.
type search struct{ a App }

// Render implements core.Node.
func (s search) Render(cx *core.Scope) core.Node {
	a := s.a
	field := &core.Ref{}
	// A request for the focus goes to the field once it shows; without the field it has nothing to go to and is dropped, so the field does not take the focus when it shows later.
	cx.Effect(func() {
		a.Track()
		focus := a.QueryFocus()
		if !*focus {
			return
		}
		*focus = false
		if a.FilmCount() > 0 {
			field.Focus()
		}
	})
	return core.Show{
		When: live.Func(a, func() bool { return a.FilmCount() > 0 }),
		Then: func(cx *core.Scope) core.Node { return searchField(cx, a, field) },
	}
}

// searchField is the field itself, built while the library has films.
func searchField(cx *core.Scope, a App, field *core.Ref) core.Node {
	colors := ui.ColorsOf(cx)
	query := live.Func(a, func() string { return *a.Query() })
	clear := live.Do(a, func() { *a.Query() = "" })
	// Escape empties the field while it has the keyboard focus and holds something; an empty field leaves Escape to the window.
	focused := core.NewSignal(cx, false)
	escape := core.Show{When: func() bool { return focused.Get() && query() != "" }, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, clear)
		return nil
	}}
	input := base.Input{
		Ref: field, TestID: "sidebar-search", Role: core.RoleSearchField,
		LabelFunc:       func() string { return locales.SidebarSearchLabel(cx) },
		Value:           query,
		PlaceholderFunc: func() string { return locales.SidebarSearchPlaceholder(cx) + "  " + cx.KeyLabel("mod+f") },
		OnChange:        live.On(a, a.SetQuery),
		OnSubmit:        live.On(a, func(string) { a.SubmitQuery() }),
		OnFocus:         func() { focused.Set(true) },
		OnBlur:          func() { focused.Set(false) },
		Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Height: core.Pt(28), CornerRadius: 8, FontSize: 13,
			Padding: core.Edges{Left: 28, Right: 28}},
		// The field's gray on the card, which has no border, is the theme's shade.
		StyleFunc: func() core.Style {
			return core.Style{Background: theme.Of(cx).Shade(0.06), Color: colors.Text(), PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
		},
	}
	clearButton := core.Show{When: func() bool { return query() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(4), Right: core.Pt(4)}, Children: []core.Node{
			ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.SidebarSearchClear(cx) }, Size: 20, OnPress: clear, TestID: "sidebar-search-clear"},
		}}
	}}
	glass := core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(7), Left: core.Pt(9)},
		Children: []core.Node{ui.Icon(lucide.Search, 14, colors.Muted)}}
	return core.View{Style: rowStyle, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{input, glass, clearButton, escape}},
	}}
}
