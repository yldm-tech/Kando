package category

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// Menu is the films' 分类 ▸ items, for the films whose videos are at paths, one film's or several selected: 分类 ▸ with each category, checked when every one of them is in it, which puts them all in it or, when it is checked, takes them all out, then 新建分类…, which puts them in the category it makes. While the library page shows a category's films, 移到 ▸ takes them out of it into another, as from 电影 to 动漫, and 从「…」中移出 takes them out of it. A film with no video has none.
// TODO(nsgo#1823): let a poster, or the films selected, be dragged onto a category's row in the sidebar to put them in it, as Finder and Music do; for now this menu is the only way https://github.com/nsgo-dev/nsgo/issues/1823
func Menu(cx *core.Scope, a App, paths ...string) []nsdesktop.MenuItem {
	var kept []string
	for _, path := range paths {
		if path != "" {
			kept = append(kept, path)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	paths = kept
	categories := a.Categories()
	var into []nsdesktop.MenuItem
	for _, c := range categories {
		all := true
		for _, path := range paths {
			if !a.InCategory(path, c.ID) {
				all = false
				break
			}
		}
		id := c.ID
		into = append(into, nsdesktop.MenuItem{Label: c.Name, Checkable: true, Checked: all, OnSelect: live.Do(a, func() { a.PutInCategory(id, !all, paths...) })})
	}
	if len(into) > 0 {
		into = append(into, nsdesktop.MenuItem{Separator: true})
	}
	into = append(into, nsdesktop.MenuItem{Label: locales.CategoryNewEllipsis(cx), OnSelect: live.Do(a, func() { a.AskNewCategory(paths...) })})
	out := []nsdesktop.MenuItem{{Label: locales.CategoryMenu(cx), Items: into}}

	current := a.CategoryFilter()
	from, ok := find(categories, current)
	if !ok {
		return out
	}
	var to []nsdesktop.MenuItem
	for _, c := range categories {
		if c.ID == from.ID {
			continue
		}
		id := c.ID
		to = append(to, nsdesktop.MenuItem{Label: c.Name, OnSelect: live.Do(a, func() { a.MoveCategory(from.ID, id, paths...) })})
	}
	if len(to) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.CategoryMoveTo(cx), Items: to})
	}
	return append(out, nsdesktop.MenuItem{Label: locales.CategoryTakeOut(cx, from.Name), OnSelect: live.Do(a, func() { a.PutInCategory(from.ID, false, paths...) })})
}

// find is the category id among categories.
func find(categories []Category, id string) (Category, bool) {
	for _, c := range categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}
