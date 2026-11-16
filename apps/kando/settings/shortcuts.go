package settings

import (
	"slices"

	"kando/apps/kando/hotkey"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// shortcuts is the 快捷键 page: the app's shortcut table, read only, grouped as the cheat sheet groups it and filtered by the page's own search field. Its blocks are 8 points apart, where other pages' are 6, and it starts 6 points lower than a page that opens with a section label, as the prototype's.
type shortcuts struct{ s screen }

// Render implements core.Node.
func (p shortcuts) Render(cx *core.Scope) core.Node {
	a := p.s.a
	field := ui.Input{TestID: "settings-shortcut-search", Label: locales.SettingsShortcutsSearch(cx), Placeholder: locales.SettingsShortcutsSearch(cx),
		Value:    live.Func(a, func() string { return *a.ShortcutQuery() }),
		OnChange: live.On(a, func(text string) { *a.ShortcutQuery() = text })}
	note := core.Text{Content: locales.SettingsShortcutsNote(cx), Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 4)}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
	// found is the rows the search finds, so the list is built again only when they change, not after every change of the app's state, and not while the field keeps the focus.
	found := core.NewMemoFunc(cx, live.Func(a, func() []Shortcut {
		var rows []Shortcut
		for _, row := range a.Shortcuts(cx) {
			if shortcutMatches(cx, row, *a.ShortcutQuery()) {
				rows = append(rows, row)
			}
		}
		return rows
	}), func(x, y []Shortcut) bool { return slices.Equal(x, y) })
	list := core.Dynamic(func(cx *core.Scope) core.Node {
		rows := found.Get()
		return core.Func(func(cx *core.Scope) core.Node { return shortcutGroups(cx, rows) })
	})
	return core.View{Style: core.Style{Gap: 8, AlignItems: core.AlignStretch, Margin: core.Edges{Top: 6}}, Children: []core.Node{field, note, list}}
}

// shortcutGroups lists rows under their groups' labels, in the order the groups first come, or says that the search found none.
func shortcutGroups(cx *core.Scope, rows []Shortcut) core.Node {
	colors := ui.ColorsOf(cx)
	if len(rows) == 0 {
		return core.Text{Content: locales.SettingsShortcutsNone(cx), Style: core.Style{FontSize: 12, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Muted)}
	}
	groups, order := map[string][]Shortcut{}, []string{}
	for _, row := range rows {
		if _, ok := groups[row.Group]; !ok {
			order = append(order, row.Group)
		}
		groups[row.Group] = append(groups[row.Group], row)
	}
	var nodes []core.Node
	for _, group := range order {
		var lines []core.Node
		for i, row := range groups[group] {
			if i > 0 {
				lines = append(lines, form.Separator(false))
			}
			lines = append(lines, core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(36), Padding: core.Symmetric(4, 14), Gap: 10, AlignItems: core.AlignCenter},
				Children: []core.Node{
					core.Text{Content: row.Label, Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)},
					keycap(cx, hotkey.Label(cx, row.Key)),
				}})
		}
		nodes = append(nodes, form.Label(group, 8), form.Card(lines...))
	}
	return core.View{Style: core.Style{Gap: 8, AlignItems: core.AlignStretch}, Children: nodes}
}

// keycap is a shortcut's keys on a key, as the prototype's table draws them: 22 points high, light gray with a darker bottom edge.
func keycap(cx *core.Scope, keys string) core.Node {
	k := theme.Of(cx)
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, Height: core.Pt(22), Padding: core.Symmetric(0, 8), CornerRadius: 6, AlignItems: core.AlignCenter},
		StyleFunc: func() core.Style {
			return core.Style{Background: k.Shade(0.05), Shadows: []core.BoxShadow{{Offset: core.Point{Y: -1}, Color: k.Shade(0.08), Inset: true}}}
		},
		Children: []core.Node{core.Text{Content: keys, MaxLines: 1, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Text)}}}
}

// shortcutMatches reports whether the search query finds row, as the command palette finds a command (hotkey.Match): every word of it in what the row does, its group or its keys as the page writes them, ignoring case.
func shortcutMatches(cx *core.Scope, row Shortcut, query string) bool {
	_, ok := hotkey.Match(query, row.Label, row.Group, hotkey.Label(cx, row.Key))
	return ok
}
