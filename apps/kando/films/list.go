package films

import (
	"slices"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base/datatable"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The ways the done films can show, as settings.json keeps them: ViewGrid is the posters under the hero, the page's look until another is chosen; ViewList a table of one row a film with its details side by side, as Finder's list view is, which fills the page under its heading and scrolls itself.
const (
	ViewGrid = "grid"
	ViewList = "list"
)

// ValidView is view when it is ViewGrid or ViewList and ViewGrid otherwise, as for an empty value or one another version saved.
func ValidView(view string) string {
	if view == ViewList {
		return ViewList
	}
	return ViewGrid
}

// viewSwitch is the segmented control at the end of the done films' heading that shows them as posters or as a list, as Finder's toolbar does.
type viewSwitch struct{ a App }

// Render implements core.Node.
func (v viewSwitch) Render(cx *core.Scope) core.Node {
	a := v.a
	icon := func(svg func() *core.SVG) core.Node { return ui.Icon(svg, 14, ui.Inherited) }
	return ui.Tabs[string]{
		Options: []ui.Option[string]{
			{Value: ViewGrid, LabelFunc: func() string { return locales.FilmsViewGrid(cx) }, Content: icon(lucide.LayoutGrid), TestID: "films-view-grid"},
			{Value: ViewList, LabelFunc: func() string { return locales.FilmsViewList(cx) }, Content: icon(lucide.List), TestID: "films-view-list"},
		},
		Value:     live.Func(a, a.LibraryView),
		OnChange:  live.On(a, a.SetLibraryView),
		Size:      ui.SizeSmall,
		LabelFunc: func() string { return locales.FilmsView(cx) },
		TestID:    "films-view",
	}
}

// The list's columns, by key. Each sortable one sorts by the order of the same name as the sort menu does (listColumnOrder).
const (
	colName      = "name"
	colDuration  = "duration"
	colProgress  = "progress"
	colSubtitles = "subtitles"
	colSpoken    = "spoken"
	colSource    = "source"
	colWatched   = "watched"
)

// listColumnOrder is the order a column's heading sorts by, "" for one that does not sort.
var listColumnOrder = map[string]string{colName: SortTitle, colDuration: SortDuration, colSpoken: SortSpoken, colSource: SortSource, colWatched: SortWatched}

// listColumnKey is the column that sorts by order, "" for SortAdded, which no column shows.
func listColumnKey(order string) string {
	for key, o := range listColumnOrder {
		if o == order {
			return key
		}
	}
	return ""
}

// listColumn is a column of the list after the name: its key and width. listColumns are in the order they show; listFits drops them, the last of listPriority first, when the table is too narrow for them beside a name listNameMin wide.
type listColumn struct {
	key   string
	width core.Points
	align datatable.Align
}

var listColumns = []listColumn{
	{colDuration, 72, datatable.AlignEndColumn},
	{colProgress, 116, datatable.AlignStartColumn},
	{colSubtitles, 132, datatable.AlignStartColumn},
	{colSpoken, 108, datatable.AlignStartColumn},
	{colSource, 108, datatable.AlignStartColumn},
	{colWatched, 156, datatable.AlignStartColumn},
}

// listPriority is the columns after the name from the one kept longest as the table narrows: how long a film is and how far it was watched, then what its subtitles are in, when it was last watched, the language heard and where it came from.
var listPriority = []string{colDuration, colProgress, colSubtitles, colWatched, colSpoken, colSource}

const (
	listNameMin   core.Points = 240
	listRowHeight core.Points = 44
	listThumbW    core.Points = 48
)

// listFits is the keys of the columns that show in a table width wide, the name first, then those of listColumns that fit beside a name at least listNameMin wide, in listColumns' order.
func listFits(width core.Points) []string {
	room := width - listNameMin
	keep := map[string]bool{}
	for _, key := range listPriority {
		i := slices.IndexFunc(listColumns, func(c listColumn) bool { return c.key == key })
		if listColumns[i].width > room {
			break
		}
		room -= listColumns[i].width
		keep[key] = true
	}
	out := []string{colName}
	for _, c := range listColumns {
		if keep[c.key] {
			out = append(out, c.key)
		}
	}
	return out
}

// listTitle names a column in the language of m.
func listTitle(m locales.Messages, key string) string {
	switch key {
	case colDuration:
		return m.FilmsSortDuration()
	case colProgress:
		return m.FilmsListProgress()
	case colSubtitles:
		return m.FilmsListSubtitles()
	case colSpoken:
		return m.FilmsSortSpoken()
	case colSource:
		return m.FilmsSortSource()
	case colWatched:
		return m.FilmsListWatched()
	}
	return m.FilmsSortTitle()
}

// listSubtitles is what a film's subtitles are, in the language of m, for the list: the language they were translated into, that the translation left some lines or all of them untranslated, or that there are none yet.
func listSubtitles(m locales.Messages, item Film) string {
	switch {
	case item.SubtitlePath == "":
		return m.FilmsNoSubtitles()
	case item.TranslationErr != "" || item.Untranslated > 0:
		return untranslatedNote(m, item)
	case item.Language == "":
		return ""
	}
	return LanguageName(m, item.Language)
}

// listProgress is how far a film was watched, in the language of m: watched to its end, the share played, or "" for one never played.
func listProgress(m locales.Messages, seen watched) string {
	switch {
	case seen.end:
		return m.FilmsWatched()
	case seen.part > 0:
		return m.FilmsListPercent(max(1, int(seen.part*100)))
	}
	return ""
}

// listDate is when a film last played, in the language of m, as Finder's list writes it: today and yesterday by name with the time, other days by their date and time, all in now's time zone and with the hour cycle the user chose (m's). now is the time it is now; the zero time is "".
func listDate(m locales.Messages, at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	// library.json keeps the offset the film played at, which is not now's after a move to another time zone or across a change of daylight saving time; the day and the time read in now's.
	at = at.In(now.Location())
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	switch {
	case !at.Before(today):
		return m.FilmsListToday(at)
	case !at.Before(today.AddDate(0, 0, -1)):
		return m.FilmsListYesterday(at)
	}
	return m.FilmsListDate(at)
}

// filmList is the done films as a table, Finder's list view: one row a film, a small poster and the title, then how long it is, how far it was watched, its subtitles, the language heard, where it came from and when it last played, the columns that do not fit left out. A click selects a row, a ⌘-click adds it to the selection or leaves it out, a ⇧-click selects the rows from the one selected last to it, and ⇧↑ and ⇧↓ grow or shrink the rows selected from it, as in Finder (Selection, which the app keeps and the menu bar acts on too); a double click or Return opens a film, and a right click offers the selection's menu on one of several selected, else the film's own (menuOf). The headings of the columns that sort sort by them, as the sort menu does, and a heading pressed again turns the order around. The selection follows its films when the order changes. done is in the order chosen already.
type filmList struct {
	a    App
	done func() []Film
}

// Render implements core.Node.
func (l filmList) Render(cx *core.Scope) core.Node {
	a, done := l.a, l.done
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	width := core.NewSignal(cx, core.Points(0))
	shown := core.NewMemoFunc(cx, func() []string { return listFits(width.Get()) }, slices.Equal[[]string]).Get
	// The table selects several rows (MultiSelect), clicks, ⌘-clicks, ⇧-clicks and ⇧-arrows as Finder's list does; Kando keeps the films selected by key, so the selection stays with them as they move, and the table's rows follow it (SelectedRows).
	sel := a.Selection()
	ref := &datatable.Ref{}
	itemAt := func(row int) (Film, bool) {
		items := done()
		if row < 0 || row >= len(items) {
			return Film{}, false
		}
		return items[row], true
	}
	keyOf := func(column int) string {
		keys := shown()
		if column < 0 || column >= len(keys) {
			return ""
		}
		return keys[column]
	}
	columns := func() []datatable.Column {
		m := locales.For(cx)
		keys := shown()
		out := make([]datatable.Column, 0, len(keys))
		rest := core.Points(0)
		for _, c := range listColumns {
			if slices.Contains(keys, c.key) {
				out = append(out, datatable.Column{Key: c.key, Title: listTitle(m, c.key), Width: c.width, Align: c.align, Sortable: listColumnOrder[c.key] != "", NoSelect: true})
				rest += c.width
			}
		}
		name := datatable.Column{Key: colName, Title: listTitle(m, colName), Width: max(listNameMin, width.Get()-rest), Sortable: true, NoSelect: true}
		return append([]datatable.Column{name}, out...)
	}
	text := func(row, column int) string {
		a.Track()
		item, ok := itemAt(row)
		if !ok {
			return ""
		}
		m := locales.For(cx)
		switch keyOf(column) {
		case colName:
			return filmName(item)
		case colDuration:
			if d := a.Duration(item.VideoPath); d > 0 {
				return Clock(d)
			}
		case colProgress:
			return listProgress(m, watchedOf(a, item))
		case colSubtitles:
			return listSubtitles(m, item)
		case colSpoken:
			if heard := Heard(item); heard != "" {
				return subtitle.SpokenLanguageNameIn(m, heard)
			}
		case colSource:
			return SourceLabel(m, item)
		case colWatched:
			return listDate(m, a.PlayedAt(item.VideoPath), time.Now())
		}
		return ""
	}
	// listTip is a cell's tooltip: the name's says what the mark of a video on a storage location means, under the film's name, as the poster's tooltip does (storedTip); the other cells and the name of a film without the mark have none.
	listTip := func(row, column int) string {
		a.Track()
		item, ok := itemAt(row)
		if !ok || keyOf(column) != colName {
			return ""
		}
		m := locales.For(cx)
		if l := storedLookOf(a, m, item); l.on {
			return filmName(item) + "\n" + l.label(m)
		}
		return ""
	}
	sort := func() datatable.Sort {
		o := live.Func(a, a.LibrarySort)().Valid()
		key := listColumnKey(o.By)
		if key == "" {
			return datatable.Sort{}
		}
		return datatable.Sort{Key: key, Descending: !o.Ascending()}
	}
	// The table cycles a heading through ascending, descending and unsorted; the list has no unsorted, so a press of the sorted heading turns its order around, and one of another sorts by it in its own direction (Order.Then). The cycle's unsorted step names no column: it is the sorted one pressed again.
	resort := live.On(a, func(s datatable.Sort) {
		current := a.LibrarySort().Valid()
		key := s.Key
		if key == "" {
			key = listColumnKey(current.By)
		}
		if order := listColumnOrder[key]; order != "" {
			a.SetLibrarySort(current.Then(order))
		}
	})
	rowOf := func(key string) int { return slices.IndexFunc(done(), func(item Film) bool { return item.Key == key }) }
	selected := live.Func(a, func() []string { return sel.Keys })
	selectedRows := func() []int {
		var rows []int
		for _, key := range selected() {
			if row := rowOf(key); row >= 0 {
				rows = append(rows, row)
			}
		}
		slices.Sort(rows)
		return rows
	}
	// What the user selects in the table, by click or by key, becomes the selection, an empty one after Escape.
	selectRows := live.On(a, func(rows []int) {
		items := done()
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			if row >= 0 && row < len(items) {
				keys = append(keys, items[row].Key)
			}
		}
		sel.Keys = keys
	})
	// The film Return opens is the selected one the table's keyboard is on, else the one selected last.
	open := live.Do(a, func() {
		items := core.Untracked(cx, done)
		key := sel.Lead()
		if row := ref.Selection().Row; row >= 0 && row < len(items) && sel.Has(items[row].Key) {
			key = items[row].Key
		}
		if item, ok := Find(items, key); ok && key != "" {
			a.Open(item)
		}
	})
	// Return opens the film selected last, as it does in Finder's list; a field or a button with the keyboard keeps its own Return.
	cx.OnKey("enter", open)
	styles := func() datatable.Styles {
		chevron := core.Style{Color: colors.Faint(), Selected: &core.Style{Color: colors.Text()}}
		return datatable.Styles{
			Header:         core.Style{Borders: core.Edges{Bottom: 0.5}, BorderColor: colors.Border(), Color: colors.Muted(), FontSize: 12, FontWeight: ui.Medium},
			HeaderCell:     core.Style{Padding: core.Symmetric(0, 8)},
			HeaderContent:  core.Style{Gap: 4},
			SortIndicator:  core.Style{CornerRadius: 4, Color: colors.Muted()},
			SortAscending:  chevron,
			SortDescending: chevron,
			Row:            core.Style{CornerRadius: 6},
			RowOdd:         core.Style{Background: k.Shade(0.03)},
			RowHover:       core.Style{Background: colors.Hover()},
			RowSelected:    core.Style{Background: ui.Alpha(colors.Accent(), 0.16)},
			RowActive:      core.Style{Background: colors.Pressed()},
			Cell:           core.Style{Padding: core.Symmetric(0, 8), Color: colors.Muted(), FontSize: 12},
		}
	}
	cell := func(cx *core.Scope, cell func() datatable.Cell) core.Node {
		key := core.NewMemo(cx, func() string { return cell().Key }).Get
		item := func() Film { f, _ := itemAt(cell().Row); return f }
		return core.Dynamic(func(cx *core.Scope) core.Node {
			switch key() {
			case colName:
				return listNameCell(cx, a, item, func() string { return cell().Text })
			case colProgress:
				return listProgressCell(cx, a, item, func() string { return cell().Text })
			}
			style := core.Style{FontSize: 12}
			if key() == colDuration {
				style.FontVariant = core.FontTabularNums
			}
			return core.Text{ContentFunc: func() string { return cell().Text }, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: style, StyleFunc: ui.Colored(colors.Muted)}
		})
	}
	return datatable.DataTable{
		TestID:               "films-list",
		Ref:                  ref,
		LabelFunc:            func() string { return locales.FilmsViewList(cx) },
		Columns:              columns,
		RowCount:             func() int { return len(done()) },
		Text:                 text,
		CellTooltip:          listTip,
		RowHeight:            listRowHeight,
		HeaderHeight:         28,
		FontSize:             12,
		Sort:                 sort,
		OnSortChange:         resort,
		MultiSelect:          true,
		SelectedRows:         selectedRows,
		OnSelectedRowsChange: selectRows,
		NoColumnSelect:       true,
		NoResizeColumns:      true,
		NoMoveColumns:        true,
		NoBounce:             true,
		Stripe:               func() bool { return true },
		StylesFunc:           styles,
		RenderCell:           cell,
		OnDoublePress: func(row, _ int) {
			if item, ok := itemAt(row); ok {
				live.Do(a, func() { a.Open(item) })()
			}
		},
		OnContextMenu: func(c datatable.ContextMenu) {
			item, ok := itemAt(c.Row)
			if !ok {
				return
			}
			nsdesktop.PopupMenu(cx, menuOf(cx, a, item, core.Untracked(cx, done), true), c.At, nil)
		},
		Style:    core.Style{FlexGrow: 1, MinHeight: core.Pt(listRowHeight * 2)},
		OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
	}
}

// listNameCell is a row's first cell: the film's poster, small, then its name on one line, and the mark of a video on a storage location after it (storedIcon).
func listNameCell(cx *core.Scope, a App, item func() Film, name func() string) core.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	thumb := core.View{
		Style:     core.Style{Width: core.Pt(listThumbW), AspectRatio: 16.0 / 9, CornerRadius: 4, BorderWidth: 0.5, FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: k.Thumb(), BorderColor: k.Shade(0.1)} },
		Children:  []core.Node{clip(4, cover(live.Func(a, func() core.ImageSource { return a.Poster(item().VideoPath) })))},
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Height: core.Pct(100), MinWidth: core.Pt(0)}, Children: []core.Node{
		thumb,
		core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		storedIcon(cx, storedMemo(cx, a, item)),
	}}
}

// listProgressCell is how far a film was watched: a check for one watched to its end, a short bar as far as it played for one stopped partway, then the cell's words (listProgress); nothing for one never played.
func listProgressCell(cx *core.Scope, a App, item func() Film, words func() string) core.Node {
	colors := ui.ColorsOf(cx)
	seen := live.Memo(cx, a, func() watched { return watchedOf(a, item()) })
	check := core.Show{When: func() bool { return seen().end }, Then: func(*core.Scope) core.Node {
		return core.Icon{SVG: lucide.Check(), Style: core.Style{Width: core.Pt(12), Height: core.Pt(12)}, StyleFunc: ui.Colored(theme.Of(cx).Glyph)}
	}}
	bar := core.Show{When: func() bool { s := seen(); return s.part > 0 && !s.end }, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(4), CornerRadius: 2, FlexShrink: 0}, StyleFunc: func() core.Style { return core.Style{Background: colors.Track()} },
			Children: []core.Node{core.View{Style: core.Style{Height: core.Pct(100), CornerRadius: 2}, StyleFunc: func() core.Style {
				return core.Style{Width: core.Pct(float32(seen().part * 100)), Background: colors.Accent()}
			}}}}
	}}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pct(100)}, Children: []core.Node{
		check,
		bar,
		core.Text{ContentFunc: words, MaxLines: 1, Style: core.Style{FontSize: 12, FontVariant: core.FontTabularNums}, StyleFunc: ui.Colored(colors.Muted)},
	}}
}
