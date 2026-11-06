package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// shortcuts is the 快捷键 page: the app's shortcut table, read only, grouped as the cheat sheet groups it and filtered by the page's own search field.
func (s screen) shortcuts(c *ui.Context, t *ui.Theme) {
	query := s.ShortcutQuery()
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		c.Key("shortcut-search")
		breeze.SearchField(c, query).Label("搜索命令或按键").Placeholder("搜索命令或按键")
		breeze.Text(c, "单个按键只在没有输入框获得焦点时生效。", breeze.TextCaption, breeze.TextSecondary).Padding(0, 10).Wrap()
	})
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
		breeze.Text(c, "没有匹配的快捷键", breeze.TextSecondary).Padding(0, 10)
		return
	}
	for _, group := range order {
		breeze.PreferencesGroup(c, group, "", nil, func() {
			for _, row := range groups[group] {
				breeze.ActionRow(c, row.Label, "", func() {
					breeze.ShortcutLabel(c, row.Mods, row.Key)
				})
			}
		})
	}
}

// shortcutMatches reports whether the search query finds row by what it does, its group or its keys, ignoring case.
func shortcutMatches(c *ui.Context, row Shortcut, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(row.Group+" "+row.Label+" "+breeze.ShortcutText(c, row.Mods, row.Key)), q)
}
