package films

import (
	"bytes"
	"cmp"
	"slices"

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

// SortLabel names an order in the language of m.
func SortLabel(m locales.Messages, order string) string {
	switch ValidSort(order) {
	case SortTitle:
		return m.FilmsSortTitle()
	case SortSource:
		return m.FilmsSortSource()
	default:
		return m.FilmsSortAdded()
	}
}

// sourceOrder is the sources in the order the sidebar lists them, which SortSource groups the films in.
var sourceOrder = []string{sidebar.SourceYouTube, sidebar.SourceBilibili, sidebar.SourceWeb, sidebar.SourceLocal}

// Sorted returns items in order, leaving items as they are. items are the films added last first, as List gives them, which SortAdded keeps. SortTitle orders them by name as tag, the interface's language, orders words (Chinese by pinyin, numbers by value, so "Lecture 2" comes before "Lecture 10"); SortSource groups them by source in the sidebar's order, other sites by their names. Films the order does not tell apart keep the order they were added in.
func Sorted(items []Film, order, tag string) []Film {
	order = ValidSort(order)
	if order == SortAdded || len(items) < 2 {
		return items
	}
	// The collator turns each film's name or source into a key once, rather than at every comparison.
	col := collate.New(language.Make(tag), collate.Numeric)
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
			e.key = col.KeyFromString(&buf, SourceLabel(locales.In(tag), item))
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

// sortButton is the pull-down beside the grid's heading, naming the order the grid shows and offering the others in the system's menu, the order shown checked. It opens under the button, at its leading edge.
type sortButton struct{ a App }

// Render implements core.Node.
func (s sortButton) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	open := core.NewSignal(cx, false)
	// place is the view around the button, which base.Button takes no Ref for, so its frame gives the button's.
	place := &core.Ref{}
	label := live.Func(a, func() string { return locales.FilmsSortButton(cx, SortLabel(locales.For(cx), a.LibrarySort())) })
	menu := func() []nsdesktop.MenuItem {
		current := ValidSort(a.LibrarySort())
		items := make([]nsdesktop.MenuItem, len(Sorts))
		for i, order := range Sorts {
			// TODO(nsgo#118): make the orders radio items, the one shown chosen, as MyGo's Menu.Item(...).Radio drew them; NSGo's menu items are only checkable https://github.com/nsgo-dev/nsgo/issues/118
			items[i] = nsdesktop.MenuItem{Label: SortLabel(locales.For(cx), order), Checkable: true, Checked: order == current, OnSelect: live.Do(a, func() { a.SetLibrarySort(order) })}
		}
		return items
	}
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
		}
		open.Set(true)
		nsdesktop.PopupMenu(cx, menu(), at, func() { open.Set(false) })
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
