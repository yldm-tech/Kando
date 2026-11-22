package main

import (
	"kando/apps/kando/category"

	"github.com/nsgo-dev/nsgo/core"
)

// categoryUI is the state of the open library's categories (package category).
type categoryUI struct {
	// book is the categories of the library open now, read from its folder (categorySync); a book of no folder while none is read, as while the library is locked.
	book category.Book
	// filter is the row of the sidebar's section the library page shows the films of: category.All, category.None or a category's ID, together with the sidebar's other filters. Like them it is not saved, and another library starts without it.
	filter string
	// naming is the dialog that names a new category or renames one, and deleting the ID of the category whose deletion waits to be confirmed, "" while none does. nameRelease and deleteRelease give back the window's modal flag the dialogs hold while they are open (holdModal), nil while they are closed.
	naming        category.Naming
	deleting      string
	nameRelease   func()
	deleteRelease func()
}

func newCategoryUI() categoryUI { return categoryUI{filter: category.All} }

// categoryHooks reads the open library's categories as it opens and once its files are in its folder, as after it was unlocked, starts each library with every category shown, and keys its films by their videos' new paths as the shelf follows them.
func (a *app) categoryHooks() areaHooks {
	return areaHooks{
		libraryChanged: func() {
			a.cats.filter = category.All
			a.categorySync()
		},
		shelfChanged: a.categorySync,
		videosMoved: func(move func(string) (string, bool)) {
			a.categorySync()
			if a.cats.book.MoveVideos(move) {
				a.categoryWrite()
			}
		},
		afterChange: a.categoryDropGone,
	}
}

// categorySync reads the categories of the library open now when the book holds another's. A locked library's files are not in its folder (lockFilesAway): its categories are not read, and none can be changed, until they are, so a file in its image is never written over with an empty book.
func (a *app) categorySync() {
	dir := a.snap().LibraryDir
	if dir == "" || a.lockFilesAway(dir) {
		a.cats.book = category.Book{}
		return
	}
	if a.cats.book.Dir() == dir {
		return
	}
	book, err := category.Load(dir)
	a.cats.book = book
	if err != nil {
		a.saveErr = a.messages().CategoryReadFailed()
	}
}

// categoryBook is the open library's categories while they can be changed: read from its folder and not locked away. ok is false otherwise, and nothing is changed then.
func (a *app) categoryBook() (*category.Book, bool) {
	dir := a.snap().LibraryDir
	if dir == "" || a.cats.book.Dir() != dir || a.lockShut() {
		return nil, false
	}
	return &a.cats.book, true
}

// categoryWrite saves the open library's categories, saying on the message bar when that failed.
func (a *app) categoryWrite() {
	if err := a.cats.book.Save(); err != nil {
		if err == category.ErrUnread {
			a.saveErr = a.messages().CategoryReadFailed()
			return
		}
		a.saveErr = a.messages().CategorySaveFailed(err.Error())
	}
}

// categoryDropGone lets go of the category chosen in the sidebar once the library no longer has it, as after it was deleted, so the page shows every category rather than none.
func (a *app) categoryDropGone() bool {
	switch a.cats.filter {
	case category.All, category.None:
		return false
	}
	if _, ok := a.cats.book.Find(a.cats.filter); ok {
		return false
	}
	a.cats.filter = category.All
	return true
}

// categoryOverlay is the dialog that names a category and the confirmation that deletes one.
func (a *app) categoryOverlay(*core.Scope) core.Node { return category.Overlay(a) }

// categoryCloseName closes the naming dialog and gives back the modal flag it held.
func (a *app) categoryCloseName() {
	a.cats.naming = category.Naming{}
	if a.cats.nameRelease != nil {
		a.cats.nameRelease()
		a.cats.nameRelease = nil
	}
}

// categoryCloseDelete closes the confirmation of 删除分类… and gives back the modal flag it held.
func (a *app) categoryCloseDelete() {
	a.cats.deleting = ""
	if a.cats.deleteRelease != nil {
		a.cats.deleteRelease()
		a.cats.deleteRelease = nil
	}
}

var _ category.App = (*app)(nil)
