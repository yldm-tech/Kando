package category

import (
	"slices"
	"strconv"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// List is the sidebar's 分类 section: its title with the + button that names a new category, then 全部, a row for each category with how many films it holds, and 未分类 while the library has a category. Choosing a row shows the library page's films in it, among those of the source, the language, the watch state and the search chosen, as all apply. A category's row has a menu to rename it or delete it. While the library has no category, a line says what categories are for. It shows while the open library holds done films. The sidebar (package sidebar) places it under the watch states.
type List struct{ App App }

// Render implements core.Node.
func (l List) Render(cx *core.Scope) core.Node {
	a := l.App
	rows := core.NewMemoFunc(cx, live.Func(a, a.CategoryRows), slices.Equal[[]Row])
	current := live.Func(a, a.CategoryFilter)
	colors := ui.ColorsOf(cx)
	add := ui.IconButton{Icon: lucide.Plus, LabelFunc: func() string { return locales.CategoryNew(cx) }, Size: 22, TestID: "category-add", OnPress: live.Do(a, func() { a.AskNewCategory() })}
	actions := ui.Tooltip{TextFunc: func() string { return locales.CategoryNew(cx) }, Trigger: add, Style: core.Style{FlexShrink: 0}}
	empty := core.Text{TestID: "category-empty", ContentFunc: func() string { return locales.CategoryEmpty(cx) }, Style: core.Style{FontSize: 11, LineHeight: 16, Padding: core.Edges{Top: 2, Right: 10, Left: 10}}, StyleFunc: ui.Colored(colors.Muted)}
	return core.Show{When: live.Func(a, a.CategoriesShown), Then: func(cx *core.Scope) core.Node {
		return core.View{
			// The list takes the room its rows need and scrolls when the sidebar is too short for it.
			Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0), Padding: core.Symmetric(0, 8)},
			Children: []core.Node{ui.Sidebar{
				TestID: "sidebar-categories",
				SectionsFunc: func() []ui.SidebarSection {
					list := rows.Get()
					all := locales.CategoryAll(cx)
					items := []ui.SidebarItem{{ID: All, Text: all, TestID: "category-" + All, Content: rowContent(cx, Row{ID: All, Name: all}, false)}}
					for _, r := range list {
						item := ui.SidebarItem{ID: r.ID, Text: rowName(cx, r), TestID: "category-" + r.ID, Content: rowContent(cx, r, true)}
						if r.ID != None {
							item.Menu = rowMenu(cx, a, r.ID)
						}
						items = append(items, item)
					}
					section := ui.SidebarSection{Title: locales.CategorySection(cx), Items: items, Actions: actions}
					if len(list) == 0 {
						section.Note = empty
					}
					return []ui.SidebarSection{section}
				},
				Selected: current,
				OnSelect: live.On(a, func(id string) {
					if id != current() {
						a.SetCategoryFilter(id)
					}
				}),
			}},
		}
	}}
}

// rowName is a row's accessible name: its name, with how many films it holds, which the row draws as a number.
func rowName(cx *core.Scope, r Row) string {
	if r.ID == All {
		return r.Name
	}
	return locales.CategoryRowCount(cx, r.Name, r.Count)
}

// rowContent is a row's mark, name and, when counted, how many films it holds: a stack of tags for 全部, a tag for a category and a dashed circle for 未分类.
func rowContent(cx *core.Scope, r Row, counted bool) core.Node {
	colors := ui.ColorsOf(cx)
	icon := lucide.Tag
	switch r.ID {
	case All:
		icon = lucide.Tags
	case None:
		icon = lucide.CircleDashed
	}
	name := ui.Text(r.Name, 13, ui.Regular, colors.Text)
	name.MaxLines, name.Style.FlexGrow, name.Style.FlexShrink = 1, 1, 1
	children := []core.Node{ui.Icon(icon, 16, theme.Of(cx).Glyph), name}
	if counted {
		count := ui.Tabular(ui.Text(strconv.Itoa(r.Count), 12, ui.Regular, colors.Muted))
		count.TestID, count.Style.FlexShrink = "category-count-"+r.ID, 0
		children = append(children, count)
	}
	return core.Group(children)
}

// rowMenu is a category row's menu: 重命名… and 删除分类…. The section builds it in its SectionsFunc, so its words follow the interface language with the rows.
func rowMenu(cx *core.Scope, a App, id string) []core.Node {
	return []core.Node{
		ui.MenuItem{Text: locales.CategoryRename(cx), TestID: "category-rename-" + id, OnSelect: live.Do(a, func() { a.AskRenameCategory(id) })},
		ui.MenuItem{Text: locales.CategoryDelete(cx), Variant: ui.MenuItemDanger, TestID: "category-delete-" + id, OnSelect: live.Do(a, func() { a.AskDeleteCategory(id) })},
	}
}
