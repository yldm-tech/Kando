package category

import (
	"kando/apps/kando/live"
)

// The rows of the sidebar's section other than a category's, which CategoryFilter and SetCategoryFilter name as they name a category by its ID: All shows the films whatever their categories, None those in no category. A category's ID is random hex, so it is never one of these.
const (
	All  = "all"
	None = "none"
)

// Row is a row of the sidebar's section: a category, or All or None, with how many films choosing it shows.
type Row struct {
	ID, Name string
	Count    int
}

// Naming is the dialog that names a category: a new one when ID is "", which the films of Paths are put in once it is made, or the category ID being renamed. Name is what the field holds, and Err why the name was refused, ErrNameEmpty or ErrNameTaken, nil while it was not.
type Naming struct {
	Open  bool
	ID    string
	Name  string
	Paths []string
	Err   error
}

// App is the library's categories as the sidebar's section, the films' menus and the dialogs read them, and what they report.
type App interface {
	live.State
	// CategoriesShown reports whether the section shows: while the open library holds done films.
	CategoriesShown() bool
	// CategoryRows is the section's categories, ordered by name, each with how many of the films the library page would show with every other filter as it is it holds (App.WatchCounts counts so too), then None while the library has a category. All has no row of its own here; the section draws it first.
	CategoryRows() []Row
	// CategoryFilter is the row the library page shows the films of while it is shown, All, None or a category's ID; SetCategoryFilter shows the library page filtered by id, keeping the other filters, and closes the open film.
	CategoryFilter() string
	SetCategoryFilter(id string)
	// Categories is the open library's categories, ordered by name; InCategory reports whether the film whose video is at path is in the category id.
	Categories() []Category
	InCategory(path, id string) bool
	// PutInCategory puts the films whose videos are at paths in the category id, or with in false takes them out; MoveCategory takes them out of from and puts them in to.
	PutInCategory(id string, in bool, paths ...string)
	MoveCategory(from, to string, paths ...string)
	// AskNewCategory opens the dialog that names a new category, which the films whose videos are at paths are put in once it is made; AskRenameCategory opens it on the category id; AskDeleteCategory asks first before the category id is deleted.
	AskNewCategory(paths ...string)
	AskRenameCategory(id string)
	AskDeleteCategory(id string)
	// CategoryNaming is the naming dialog's state, which the dialog changes as the field does; SubmitCategoryName makes or renames the category, closing the dialog unless the name is refused; CloseCategoryName closes it and changes nothing.
	CategoryNaming() *Naming
	SubmitCategoryName()
	CloseCategoryName()
	// CategoryDeleting is the category whose deletion is waiting to be confirmed, ok false while none is; DeleteCategory deletes it and CloseDeleteCategory keeps it.
	CategoryDeleting() (c Category, ok bool)
	DeleteCategory()
	CloseDeleteCategory()
}
