package films

import (
	"bytes"
	"cmp"
	"slices"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// The orders the done films can be shown in, as settings.json keeps them. SortAdded is the films added last first, the order the shelf keeps them in (List), and the one the page shows until another is chosen. SortWatched is the films played last first, SortDuration the longest first, and SortSpoken groups them by the language heard in them.
const (
	SortAdded    = "added"
	SortWatched  = "watched"
	SortTitle    = "title"
	SortDuration = "duration"
	SortSource   = "source"
	SortSpoken   = "spoken"
)

// Sorts are the orders the sort menu offers, in the order it lists them.
var Sorts = []string{SortAdded, SortWatched, SortTitle, SortDuration, SortSource, SortSpoken}

// ValidSort is order when it is one of Sorts and SortAdded otherwise, as for an empty value or one another version saved.
func ValidSort(order string) string {
	if slices.Contains(Sorts, order) {
		return order
	}
	return SortAdded
}

// Order is how the done films are listed: by one of Sorts, in its own direction (Descends), or the other way round while Reversed.
type Order struct {
	By       string
	Reversed bool
}

// Valid is o with By one of Sorts.
func (o Order) Valid() Order { return Order{By: ValidSort(o.By), Reversed: o.Reversed} }

// Descends reports whether order lists the films from the most to the least, the newest, the last played or the longest first, as Finder lists dates and sizes; the others go from A to Z.
func Descends(order string) bool {
	switch ValidSort(order) {
	case SortAdded, SortWatched, SortDuration:
		return true
	}
	return false
}

// Ascending reports whether o lists the films from the least to the most: A to Z, the oldest or the shortest first.
func (o Order) Ascending() bool { return Descends(o.By) == o.Reversed }

// Toward is o by the same order, ascending or not.
func (o Order) Toward(ascending bool) Order {
	return Order{By: o.By, Reversed: Descends(o.By) == ascending}
}

// Then is the order a click on the heading of a column that sorts by order gives, as Finder's list does: the column already sorting turns around, another sorts in its own direction.
func (o Order) Then(order string) Order {
	if ValidSort(order) == ValidSort(o.By) {
		return Order{By: o.By, Reversed: !o.Reversed}
	}
	return Order{By: order}
}

// SortLabel names an order in the language of m.
func SortLabel(m locales.Messages, order string) string {
	switch ValidSort(order) {
	case SortTitle:
		return m.FilmsSortTitle()
	case SortDuration:
		return m.FilmsSortDuration()
	case SortSource:
		return m.FilmsSortSource()
	case SortWatched:
		return m.FilmsSortWatched()
	case SortSpoken:
		return m.FilmsSortSpoken()
	default:
		return m.FilmsSortAdded()
	}
}

// sourceOrder is the sources in the order the sidebar lists them, which SortSource groups the films in.
var sourceOrder = []string{sidebar.SourceYouTube, sidebar.SourceBilibili, sidebar.SourceWeb, sidebar.SourceLocal}

// Sorted returns items in o, leaving items as they are. items are the films added last first, as List gives them, which SortAdded keeps. SortWatched puts the films played last first, by played, when the video at a path last played (App.PlayedAt); SortDuration the longest first, by length (App.Duration); SortTitle orders them by name as tag, the interface's language, orders words (Chinese by pinyin, numbers by value, so "Lecture 2" comes before "Lecture 10"); SortSource groups them by source in the sidebar's order, other sites by their names; SortSpoken groups them by the language heard in them (Heard), by its name in tag's language. A Reversed order turns that around, but only for the films the order can place: those it knows nothing of stay last either way, as Finder keeps its empty fields last, being a film never played, one whose length is not known yet, one whose language is not known, and for SortAdded a video found in the folder, which Kando did not add, rather than one added without a job (Film.Added). Films the order does not tell apart keep the order they were added in.
func Sorted(items []Film, o Order, tag string, played func(path string) time.Time, length func(path string) time.Duration) []Film {
	o = o.Valid()
	if len(items) < 2 || o == (Order{By: SortAdded}) {
		return items
	}
	type entry struct {
		item Film
		// unknown is a film the order cannot place, which goes last.
		unknown bool
		rank    int
		num     int64
		key     []byte
	}
	// The collator turns each film's name or source into a key once, rather than at every comparison.
	col := collate.New(language.Make(tag), collate.Numeric)
	var buf collate.Buffer
	entries := make([]entry, len(items))
	for i, item := range items {
		e := entry{item: item}
		switch o.By {
		case SortAdded:
			// The shelf lists the films added last first: the later in items, the earlier added.
			e.num, e.unknown = int64(-i), item.JobID == "" && item.Added.IsZero()
		case SortWatched:
			at := played(item.VideoPath)
			e.num, e.unknown = at.UnixNano(), at.IsZero()
		case SortDuration:
			d := length(item.VideoPath)
			e.num, e.unknown = int64(d), d <= 0
		case SortTitle:
			e.key = col.KeyFromString(&buf, filmName(item))
		case SortSpoken:
			if heard := Heard(item); heard == "" {
				e.unknown = true
			} else {
				e.key = col.KeyFromString(&buf, subtitle.SpokenLanguageNameIn(locales.In(tag), heard))
			}
		default:
			e.rank = slices.Index(sourceOrder, SourceOf(item))
			e.key = col.KeyFromString(&buf, SourceLabel(locales.In(tag), item))
		}
		entries[i] = e
	}
	// ascending is the films from the least to the most: A to Z, the earliest, the shortest.
	ascending := o.Ascending()
	slices.SortStableFunc(entries, func(x, y entry) int {
		if x.unknown != y.unknown {
			if x.unknown {
				return 1
			}
			return -1
		}
		if x.unknown {
			return 0
		}
		d := cmp.Compare(x.rank, y.rank)
		if d == 0 {
			d = cmp.Compare(x.num, y.num)
		}
		if d == 0 {
			d = bytes.Compare(x.key, y.key)
		}
		if !ascending {
			d = -d
		}
		return d
	})
	out := make([]Film, len(entries))
	for i, e := range entries {
		out[i] = e.item
	}
	return out
}

// SortMenu is the orders in the system's menu, the one shown checked, then 升序 and 降序, the direction shown checked: the menu of the sort button and of View › 排序方式.
func SortMenu(cx *core.Scope, a App) []nsdesktop.MenuItem {
	current := a.LibrarySort().Valid()
	m := locales.For(cx)
	items := make([]nsdesktop.MenuItem, 0, len(Sorts)+3)
	for _, order := range Sorts {
		items = append(items, nsdesktop.MenuItem{Label: SortLabel(m, order), Radio: true, Checked: order == current.By, OnSelect: live.Do(a, func() { a.SetLibrarySort(Order{By: order}) })})
	}
	ascending := current.Ascending()
	return append(items, nsdesktop.MenuItem{Separator: true},
		nsdesktop.MenuItem{Label: m.FilmsSortAscending(), Radio: true, Checked: ascending, OnSelect: live.Do(a, func() { a.SetLibrarySort(current.Toward(true)) })},
		nsdesktop.MenuItem{Label: m.FilmsSortDescending(), Radio: true, Checked: !ascending, OnSelect: live.Do(a, func() { a.SetLibrarySort(current.Toward(false)) })},
	)
}

// sortButton is the pull-down in the done films' heading, naming the order they show in and offering the others and the two directions in the system's menu (SortMenu). It opens under the button, at its leading edge.
type sortButton struct{ a App }

// Render implements core.Node.
func (s sortButton) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	open := core.NewSignal(cx, false)
	// place is the view around the button, which base.Button takes no Ref for, so its frame gives the button's.
	place := &core.Ref{}
	label := live.Func(a, func() string { return locales.FilmsSortButton(cx, SortLabel(locales.For(cx), a.LibrarySort().By)) })
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
		}
		open.Set(true)
		nsdesktop.PopupMenu(cx, SortMenu(cx, a), at, func() { open.Set(false) })
	}
	// The button is the platform's pull-down: the order's name and a chevron, a quiet fill under the pointer and while its menu is open.
	button := base.Button{
		TestID:    "films-sort",
		LabelFunc: label,
		Expanded:  open.Get,
		OnPress:   press,
		Style: core.Style{Direction: core.Row, Height: core.Pt(26), Padding: core.Symmetric(0, 10), Gap: 6, CornerRadius: ui.RadiusControl, AlignItems: core.AlignCenter,
			Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() core.Style {
			s := core.Style{Hover: &core.Style{Background: colors.Hover()}, Active: &core.Style{Background: colors.Pressed()}}
			if open.Get() {
				s.Background = colors.Pressed()
			}
			return s
		},
		Children: []core.Node{
			core.Text{ContentFunc: label, MaxLines: 1, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Text)},
			ui.Icon(svg(chevronIcon), 10, colors.Muted),
		},
	}
	return core.View{Ref: place, Style: core.Style{FlexShrink: 0}, Children: []core.Node{button}}
}
