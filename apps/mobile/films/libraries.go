package films

import (
	"slices"

	"kando/apps/mobile/brand"
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/shell"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// folderTiles are the colors the libraries' folder tiles take in turn, as the design draws one blue and the next orange.
var folderTiles = []core.Color{theme.TileBlue, theme.TileOrange, theme.TileGreen, theme.TileViolet, theme.TileTeal}

// switcher is the pill at the top of the Library's page that names the library the list shows and opens the sheet that picks another: the app's icon, the library's name and a chevron. It shows while there is a library to pick, or one is chosen whose computer did not answer, so the list can be taken back to every library; a tablet's sidebar does its work where it shows.
func (v *view) switcher(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	name := func() string { return LibraryName(cx, v.library) }
	return core.Show{When: func() bool {
		if shell.Sidebar(cx) || v.act.Libraries == nil {
			return false
		}
		return v.library.Get().Desktop != "" || len(catalog.Libraries(v.s.Computers(), v.entries.Get())) > 1
	}, Then: func(cx *core.Scope) core.Node {
		return base.Button{TestID: "films-library", LabelFunc: func() string { return locales.FilmsLibrary(cx, name()) }, OnPress: func(core.PressEvent) { v.act.Libraries() },
			HitSlop: core.Edges{Top: 6, Bottom: 6},
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(32), CornerRadius: 16, Padding: core.Edges{Right: 10, Left: 4}, FlexShrink: 1,
				Active: &core.Style{Opacity: core.Alpha(0.6)}},
			StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, 0)} },
			Children: []core.Node{
				core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true, Style: core.Style{Width: core.Pt(26), Height: core.Pt(26), CornerRadius: 6, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0}},
				core.Text{ContentFunc: name, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
				ui.Icon(lucide.ChevronDown, 9, colors.Muted),
			}}
	}}
}

// LibrarySheet is the sheet that picks the library the Library shows into library while open reports true, as the Library's switcher and a tablet's sidebar open it: every library, then each library of the paired computers and those only the phone still knows of, on a folder tile with its computer's name under it, the one shown checked. Picking one closes it. It belongs over the app, so it shows whatever section a tablet shows.
func LibrarySheet(cx *core.Scope, s *session.Session, library *core.Signal[catalog.Scope], open *core.Signal[bool]) core.Node {
	colors := ui.ColorsOf(cx)
	libs := core.NewMemoFunc(cx, func() []catalog.Scope { return catalog.Libraries(s.Computers(), s.Entries()) }, slices.Equal)
	pick := func(lib catalog.Scope) {
		library.Set(lib)
		open.Set(false)
	}
	semibold := core.Style{FontWeight: core.FontWeightSemibold}
	check := func(on func() bool) core.Node {
		return core.Show{When: on, Then: func(*core.Scope) core.Node { return ui.Icon(lucide.Check, 20, colors.Accent) }}
	}
	all := look.Row{TestID: "films-library-all", Index: func() int { return 0 }, TitleStyle: semibold, TitleLines: 1,
		Leading:  look.ColorTile(cx, lucide.Library, func() core.Color { return theme.TileGrey }),
		Title:    func() string { return locales.FilmsAllLibraries(cx) },
		Checked:  func() bool { return library.Get().Desktop == "" },
		Trailing: check(func() bool { return library.Get().Desktop == "" }),
		OnPress:  func() { pick(catalog.Scope{}) }}
	return look.Dialog{TestID: "films-libraries", Open: open.Get, OnClose: func() { open.Set(false) }, Title: func() string { return locales.FilmsLibraries(cx) },
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.FilmsLibrariesNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 4, Left: 4}, Margin: core.Edges{Top: -12}}), StyleFunc: ui.Colored(colors.Muted)},
			look.Group{Children: []core.Node{
				all,
				core.Each(libs.Get, func(l catalog.Scope) string { return l.Desktop + "/" + l.ID + "/" + l.Name }, func(cx *core.Scope, l func() catalog.Scope, i func() int) core.Node {
					chosen := func() bool { got := library.Get(); return got.Desktop == l().Desktop && got.ID == l().ID && got.Name == l().Name }
					return look.Row{TestID: "films-library-" + l().Desktop + "-" + l().ID, Index: func() int { return i() + 1 }, TitleStyle: semibold, TitleLines: 1,
						Leading: look.ColorTile(cx, lucide.Folder, func() core.Color { return folderTiles[i()%len(folderTiles)] }),
						Title:   func() string { return l().Name },
						Detail:  func() string { return l().Computer },
						Checked: chosen, Trailing: check(chosen),
						OnPress: func() { pick(l()) }}
				}),
			}},
		}}
}

// LibraryName is the library the Library shows, as the switcher names it, or all of them; read it in a binding.
func LibraryName(cx *core.Scope, library *core.Signal[catalog.Scope]) string {
	if lib := library.Get(); lib.Desktop != "" {
		return lib.Name
	}
	return locales.FilmsAllLibraries(cx)
}
