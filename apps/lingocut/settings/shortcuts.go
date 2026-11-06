package settings

import (
	"strings"

	"lingocut/apps/lingocut/settings/form"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// shortcuts is the 快捷键 page: the app's shortcut table, read only, grouped as the cheat sheet groups it and filtered by the page's own search field. Its blocks are 8 points apart, where other pages' are 6, and it starts 6 points lower than a page that opens with a section label, as the prototype's.
func (s screen) shortcuts(c *ui.Context, t *ui.Theme) {
	query := s.ShortcutQuery()
	ui.Column(c).Gap(8).AlignItems(ui.Stretch).Margin(6, 0, 0, 0).Children(func() {
		c.Key("shortcut-search")
		breeze.TextInput(c, query).Label("搜索命令或按键").Placeholder("搜索命令或按键").Height(26).FontSize(12)
		ui.Text(c, "单个按键只在没有输入框获得焦点时生效。").FontSize(11).FixedLineHeight(15).TextColor(form.Secondary).Wrap().Padding(0, 4)
		groups, order := map[string][]Shortcut{}, []string{}
		for _, row := range s.Shortcuts() {
			if !shortcutMatches(c, row, *query) {
				continue
			}
			if _, ok := groups[row.Group]; !ok {
				order = append(order, row.Group)
			}
			groups[row.Group] = append(groups[row.Group], row)
		}
		if len(order) == 0 {
			ui.Text(c, "没有匹配的快捷键").FontSize(12).TextColor(form.Secondary).Padding(0, 12)
			return
		}
		for _, group := range order {
			form.Label(c, group, 8, nil)
			form.Card(c, func() {
				for i, row := range groups[group] {
					if i > 0 {
						form.Separator(c, false)
					}
					ui.Row(c).Shrink(0).MinHeight(36).Padding(4, 14).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, row.Label).FontSize(13).TextColor(form.Text).Grow(1).Shrink(1).MinWidth(0).Wrap()
						keycap(c, breeze.ShortcutText(c, row.Mods, row.Key))
					})
				}
			})
		}
	})
}

// keycap is a shortcut's keys on a key, as the prototype's table draws them: 22 points high, light gray with a darker bottom edge.
func keycap(c *ui.Context, keys string) {
	ui.Row(c).Shrink(0).Height(22).Padding(0, 8).Radius(6).AlignItems(ui.Center).Background(ui.RGBA(0, 0, 0, 0.05)).InsetShadow(0, -1, 0, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
		ui.Text(c, keys).FontSize(12).TextColor(form.Text).SingleLine()
	})
}

// shortcutMatches reports whether the search query finds row by what it does, its group or its keys, ignoring case.
func shortcutMatches(c *ui.Context, row Shortcut, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(row.Group+" "+row.Label+" "+breeze.ShortcutText(c, row.Mods, row.Key)), q)
}
