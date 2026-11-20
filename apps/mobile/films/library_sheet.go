package films

import (
	"strconv"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// folderFills are the libraries' folder tiles in the sheet, a color each in turn.
var folderFills = []core.Color{theme.TileBlue, theme.TileOrange, theme.TileGreen, theme.TileViolet, theme.TileTeal}

// LibrarySheet is the sheet that picks the library the Library shows into library, or every library, while open reports true: the switcher's and a tablet's sidebar header's. Each library is a row with its folder, its name and its computer, the one shown checked; a row picked closes the sheet.
func LibrarySheet(cx *core.Scope, s *session.Session, library *core.Signal[catalog.Scope], open *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	libs := core.NewMemoFunc(cx, func() []catalog.Scope { return catalog.Libraries(s.Computers(), s.Entries()) }, nil)
	pick := func(lib catalog.Scope) {
		library.Set(lib)
		open.Set(false)
	}
	same := func(a, b catalog.Scope) bool { return a.Desktop == b.Desktop && a.ID == b.ID && a.Name == b.Name }
	check := func(on func() bool) core.Node {
		return core.Show{When: on, Then: func(cx *core.Scope) core.Node { return ui.Icon(lucide.Check, 20, colors.Accent) }}
	}
	all := look.Row{TestID: "films-library-option-0", Index: func() int { return 0 },
		Leading: look.ColorTile(cx, lucide.Library, colors.Accent),
		Title:   func() string { return locales.FilmsAllLibraries(cx) },
		Checked: func() bool { return library.Get().Desktop == "" }, Trailing: check(func() bool { return library.Get().Desktop == "" }),
		OnPress: func() { pick(catalog.Scope{}) }}
	return look.Dialog{TestID: "films-library-sheet", Open: open.Get, OnClose: func() { open.Set(false) }, Title: func() string { return locales.FilmsLibraries(cx) },
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.FilmsLibrariesNote(cx) }, Style: core.Style{FontSize: 13, LineHeight: 18, Margin: core.Edges{Top: -10}}, StyleFunc: ui.Colored(colors.Muted)},
			look.Group{Children: []core.Node{
				all,
				core.Each(libs.Get, func(l catalog.Scope) string { return l.Desktop + "/" + l.ID + "/" + l.Name }, func(cx *core.Scope, l func() catalog.Scope, i func() int) core.Node {
					on := func() bool { return same(library.Get(), l()) }
					return look.Row{TestID: "films-library-option-" + strconv.Itoa(i()+1), Index: func() int { return i() + 1 },
						Leading: look.ColorTile(cx, lucide.Folder, func() core.Color { return folderFills[i()%len(folderFills)] }),
						Title:   func() string { return l().Name }, TitleLines: 1,
						// sync.Library carries no folder path (packages/sync/protocol.go:80-84), so the row says the library's computer where the design shows its folder.
						Detail:  func() string { return l().Computer },
						Checked: on, Trailing: check(on),
						OnPress: func() { pick(l()) }}
				}),
			}},
		}}
}
