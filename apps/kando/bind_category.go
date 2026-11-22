package main

import (
	"kando/apps/kando/category"
	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// The open library's categories as the sidebar's section, the films' menus, the library page and the dialogs read them (category.App, films.App).

// CategoriesShown reports whether the sidebar's 分类 section shows: while the open library holds done films, as the watch states' list does.
func (a *app) CategoriesShown() bool { return a.SourceCount(sidebar.SourceAll) > 0 }

// Categories is the open library's categories, ordered by name in the interface's language.
func (a *app) Categories() []category.Category {
	if _, ok := a.categoryBook(); !ok {
		return nil
	}
	return category.Sorted(a.cats.book.Categories, a.prefs.interfaceLanguage)
}

// CategoryRows is the section's rows: each category, ordered by name, and then 未分类 while there is one, each with how many of the films the library page shows for the source, the language, the watch state and the search chosen it holds, so a row's count is how many films choosing it shows.
func (a *app) CategoryRows() []category.Row {
	list := a.Categories()
	if len(list) == 0 {
		return nil
	}
	shown := films.Shown(a.films(), a, a.side.filter, a.SpokenFilter(), a.side.watch, a.side.query)
	rows := make([]category.Row, 0, len(list)+1)
	for _, c := range list {
		rows = append(rows, category.Row{ID: c.ID, Name: c.Name, Count: len(a.categoryKeep(shown, c.ID))})
	}
	return append(rows, category.Row{ID: category.None, Name: a.messages().CategoryNone(), Count: len(a.categoryKeep(shown, category.None))})
}

// CategoryFilter is the row the library page shows the films of, category.All while another page shows, as the sidebar chooses no row then.
func (a *app) CategoryFilter() string {
	if a.page != pageFilms {
		return ""
	}
	return a.cats.filter
}

// SetCategoryFilter goes to the library page, closing the open film, and shows the films of id there, among those from the source, in the language and the watch state chosen.
func (a *app) SetCategoryFilter(id string) {
	a.Select(pageFilms)
	a.cats.filter = id
}

// CategoryShown is the row the library page shows the films of while it shows: category.All, category.None or a category's ID (films.App).
func (a *app) CategoryShown() string { return a.cats.filter }

// CategoryName is the name of the category id, "" for one the library does not have (films.App).
func (a *app) CategoryName(id string) string {
	c, _ := a.cats.book.Find(id)
	return c.Name
}

// InCategories is those of items in the row the library page shows the films of (films.App).
func (a *app) InCategories(items []films.Film) []films.Film {
	return a.categoryKeep(items, a.cats.filter)
}

// categoryKeep is those of items in the row id: every one for category.All, those in no category for category.None, those in the category otherwise. A film with no video is in none.
func (a *app) categoryKeep(items []films.Film, id string) []films.Film {
	if id == category.All || id == "" {
		return items
	}
	if _, ok := a.categoryBook(); !ok {
		if id == category.None {
			return items
		}
		return nil
	}
	var kept []films.Film
	for _, item := range items {
		in := a.cats.book.Of(item.VideoPath)
		if (id == category.None && len(in) == 0) || (id != category.None && a.cats.book.In(item.VideoPath, id)) {
			kept = append(kept, item)
		}
	}
	return kept
}

// InCategory reports whether the film whose video is at path is in the category id.
func (a *app) InCategory(path, id string) bool { return a.cats.book.In(path, id) }

// PutInCategory puts the films whose videos are at paths in the category id, or with in false takes them out, and saves the library's categories.
func (a *app) PutInCategory(id string, in bool, paths ...string) {
	if book, ok := a.categoryBook(); ok && book.Put(id, in, paths...) {
		a.categoryWrite()
	}
}

// MoveCategory takes the films whose videos are at paths out of the category from and puts them in to, and saves the library's categories.
func (a *app) MoveCategory(from, to string, paths ...string) {
	if book, ok := a.categoryBook(); ok && book.Move(from, to, paths...) {
		a.categoryWrite()
	}
}

// CategoryMenu is the 分类 ▸ items of the menu of items, one film's or several selected (category.Menu).
func (a *app) CategoryMenu(cx *core.Scope, items ...films.Film) []nsdesktop.MenuItem {
	if _, ok := a.categoryBook(); !ok {
		return nil
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.VideoPath)
	}
	return category.Menu(cx, a, paths...)
}

// AskNewCategory opens the dialog that names a new category, which the films whose videos are at paths go into once it is made. It holds the window's modal flag until it closes.
func (a *app) AskNewCategory(paths ...string) {
	if _, ok := a.categoryBook(); !ok {
		return
	}
	a.cats.naming = category.Naming{Open: true, Paths: paths}
	if a.cats.nameRelease == nil {
		a.cats.nameRelease = a.holdModal()
	}
}

// AskRenameCategory opens the naming dialog on the category id, its name in the field.
func (a *app) AskRenameCategory(id string) {
	c, ok := a.cats.book.Find(id)
	if _, open := a.categoryBook(); !ok || !open {
		return
	}
	a.cats.naming = category.Naming{Open: true, ID: id, Name: c.Name}
	if a.cats.nameRelease == nil {
		a.cats.nameRelease = a.holdModal()
	}
}

func (a *app) CategoryNaming() *category.Naming { return &a.cats.naming }

// SubmitCategoryName makes the category the dialog names, putting the films it was opened for in it and showing it in the sidebar when the library page shows, or renames the one it was opened on, and saves the library's categories. A name refused leaves the dialog open, saying why.
func (a *app) SubmitCategoryName() {
	book, ok := a.categoryBook()
	naming := &a.cats.naming
	if !ok || !naming.Open {
		a.categoryCloseName()
		return
	}
	if naming.ID != "" {
		if err := book.Rename(naming.ID, naming.Name); err != nil {
			naming.Err = err
			return
		}
	} else {
		c, err := book.Add(naming.Name)
		if err != nil {
			naming.Err = err
			return
		}
		book.Put(c.ID, true, naming.Paths...)
	}
	a.categoryWrite()
	a.categoryCloseName()
}

// CloseCategoryName closes the naming dialog and changes nothing.
func (a *app) CloseCategoryName() { a.categoryCloseName() }

// AskDeleteCategory asks first before the category id is deleted, holding the window's modal flag until the confirmation closes.
func (a *app) AskDeleteCategory(id string) {
	if _, ok := a.cats.book.Find(id); !ok {
		return
	}
	a.cats.deleting = id
	if a.cats.deleteRelease == nil {
		a.cats.deleteRelease = a.holdModal()
	}
}

func (a *app) CategoryDeleting() (category.Category, bool) {
	if a.cats.deleting == "" {
		return category.Category{}, false
	}
	return a.cats.book.Find(a.cats.deleting)
}

// DeleteCategory deletes the category the confirmation is about, its films staying in the library, and saves the library's categories.
func (a *app) DeleteCategory() {
	if book, ok := a.categoryBook(); ok && a.cats.deleting != "" {
		book.Delete(a.cats.deleting)
		a.categoryWrite()
	}
	a.categoryCloseDelete()
}

// CloseDeleteCategory keeps the category and closes the confirmation.
func (a *app) CloseDeleteCategory() { a.categoryCloseDelete() }
