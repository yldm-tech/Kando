package films

import (
	"bytes"
	"cmp"
	"slices"

	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// The orders the done grid can be shown in, as settings.json keeps them. SortAdded is the films added last first, the order the shelf keeps them in (List), and the one the grid shows until another is chosen.
const (
	SortAdded  = "added"
	SortTitle  = "title"
	SortSource = "source"
)

// Sorts are the orders the grid's sort menu offers, in the order it lists them.
var Sorts = []string{SortAdded, SortTitle, SortSource}

// ValidSort is order when it is one of Sorts and SortAdded otherwise, as for an empty value or one another version saved.
func ValidSort(order string) string {
	if slices.Contains(Sorts, order) {
		return order
	}
	return SortAdded
}

// SortLabel names an order in the interface's language.
func SortLabel(order string) string {
	switch ValidSort(order) {
	case SortTitle:
		return locales.FilmsSortTitle()
	case SortSource:
		return locales.FilmsSortSource()
	default:
		return locales.FilmsSortAdded()
	}
}

// sourceOrder is the sources in the order the sidebar lists them, which SortSource groups the films in.
var sourceOrder = []string{sidebar.SourceYouTube, sidebar.SourceBilibili, sidebar.SourceWeb, sidebar.SourceLocal}

// Sorted returns items in order, leaving items as they are. items are the films added last first, as List gives them, which SortAdded keeps. SortTitle orders them by name as the interface's language orders words (Chinese by pinyin, numbers by value, so "Lecture 2" comes before "Lecture 10"); SortSource groups them by source in the sidebar's order, other sites by their names. Films the order does not tell apart keep the order they were added in.
func Sorted(items []Film, order string) []Film {
	order = ValidSort(order)
	if order == SortAdded || len(items) < 2 {
		return items
	}
	// The collator turns each film's name or source into a key once, rather than at every comparison.
	col := collate.New(language.Make(i18n.Locale()), collate.Numeric)
	var buf collate.Buffer
	type entry struct {
		item Film
		rank int
		key  []byte
	}
	entries := make([]entry, len(items))
	for i, item := range items {
		e := entry{item: item}
		if order == SortTitle {
			e.key = col.KeyFromString(&buf, filmName(item))
		} else {
			e.rank = slices.Index(sourceOrder, SourceOf(item))
			e.key = col.KeyFromString(&buf, SourceLabel(item))
		}
		entries[i] = e
	}
	slices.SortStableFunc(entries, func(x, y entry) int {
		if d := cmp.Compare(x.rank, y.rank); d != 0 {
			return d
		}
		return bytes.Compare(x.key, y.key)
	})
	out := make([]Film, len(entries))
	for i, e := range entries {
		out[i] = e.item
	}
	return out
}

// sortButton is the pull-down beside the grid's heading, naming the order the grid shows and offering the others in the system's menu.
func sortButton(c *ui.Context, a App) {
	current := ValidSort(a.LibrarySort())
	c.Key("films-sort")
	breeze.MenuButton(c, locales.FilmsSortButton(SortLabel(current)), func(m *ui.Menu) {
		for _, order := range Sorts {
			if m.Item(SortLabel(order)).Radio(order == current).Chosen() {
				a.SetLibrarySort(order)
			}
		}
	}).Shrink(0)
}
