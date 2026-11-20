package settings

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ShortcutsPage lists the shortcuts of a hardware keyboard, each with its keys on a keycap, sections being the names of the shell's sections, which ⌘1 and on show.
func ShortcutsPage(cx *core.Scope, sections []func() string, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	var rows []core.Node
	for i, r := range shortcuts.List(cx, sections) {
		rows = append(rows, look.Row{TestID: "settings-shortcut-" + r.ID, Index: func() int { return i },
			Title: r.What,
			Trailing: core.View{Style: core.Style{Height: core.Pt(26), Padding: core.Symmetric(0, 9), CornerRadius: 7, BorderCurve: core.BorderCurveContinuous, JustifyContent: core.JustifyCenter, FlexShrink: 0},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
				Children: []core.Node{core.Text{TestID: "settings-shortcut-keys-" + r.ID, ContentFunc: r.Keys, MaxLines: 1,
					Style: look.Caption.Merge(core.Style{FontFamily: core.FontMonospace}), StyleFunc: ui.Colored(colors.Text)}}}})
	}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsShortcuts(cx) }, TestID: "settings-shortcuts-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return locales.SettingsShortcutsNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
					look.Group{TestID: "settings-shortcuts-list", Children: rows},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}
