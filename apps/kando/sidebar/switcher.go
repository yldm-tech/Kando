package sidebar

import (
	"path/filepath"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// The pill is the theme's card color, translucent, pressed in its shade, its hairline the shade (theme.Of).
var (
	// The folder tile's blue, from light at the top left to dark at the bottom right.
	tileFrom = core.RGB(0x2F8BFF)
	tileTo   = core.RGB(0x0A57C2)
	white    = core.RGB(0xFFFFFF)
)

// switcher is the library pill at the top of the sidebar: a folder tile, the caption 资料库 over the open library's name, and up and down arrows. A press opens the system's menu of the known libraries under it.
type switcher struct{ a App }

// Render implements core.Node.
func (s switcher) Render(cx *core.Scope) core.Node {
	a := s.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	name := live.Func(a, func() string {
		if dir := a.LibraryDir(); dir != "" {
			return Title(dir)
		}
		return locales.SidebarChooseLibrary(cx)
	})
	// open is set while the menu shows, which keeps the pill pressed.
	open := core.NewSignal(cx, false)
	// place is the row around the pill, which base.Button takes no Ref for, so the row's frame gives the pill's: the menu opens under the pill, at its leading edge, wherever it was pressed from, a key included.
	place := &core.Ref{}
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X + insetStart, Y: b.Origin.Y + b.Size.Height - 10 + 4}
		}
		open.Set(true)
		nsdesktop.PopupMenu(cx, switcherMenu(cx, a), at, func() { open.Set(false) })
	}
	tile := core.View{
		Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), FlexShrink: 0, CornerRadius: 6, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Gradients: []core.Gradient{core.LinearGradient(135, tileFrom, tileTo)}},
		Children: []core.Node{ui.Icon(svg(iconFolder), 14, ui.Fixed(white))},
	}
	// The prototype's two lines take 15 and 16 points, the browser's normal line heights for these sizes, which puts the caption's top 4.5 points below the pill's.
	caption := core.Text{ContentFunc: func() string { return locales.SidebarLibrary(cx) }, MaxLines: 1,
		Style: core.Style{FontSize: 11, LineHeight: 16.5, FontWeight: ui.Bold, LetterSpacing: 0.2}, StyleFunc: ui.Colored(colors.Muted)}
	title := core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
		Style: core.Style{FontSize: 13, LineHeight: 16, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}
	// The hairline is a ring inside the edge that takes no room, as the prototype's inset box-shadow.
	button := base.Button{
		TestID:    "sidebar-switcher",
		LabelFunc: func() string { return locales.SidebarSwitcherLabel(cx, name()) },
		Expanded:  open.Get,
		OnPress:   press,
		Style: core.Style{Direction: core.Row, FlexGrow: 1, Height: core.Pt(40), AlignItems: core.AlignCenter, Gap: 9, Padding: core.Symmetric(0, 10), CornerRadius: 10,
			Cursor: core.CursorDefault},
		StyleFunc: func() core.Style {
			hairline := []core.BoxShadow{{Spread: 0.5, Color: k.Shade(0.06), Inset: true}}
			if open.Get() {
				return core.Style{Background: k.Shade(0.07), Shadows: hairline}
			}
			return core.Style{Background: k.Card().Faded(0.6), Shadows: hairline, Active: &core.Style{Background: k.Shade(0.07)}}
		},
		Children: []core.Node{
			tile,
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{caption, title}},
			ui.Icon(svg(iconUpDown), 10, colors.Muted),
		},
	}
	return core.View{Ref: place, Style: rowStyle, Children: []core.Node{button}}
}

// switcherMenu lists the known libraries, the open one checked, each with how many films it holds; then 新建资料库…, 打开已有资料库… and 在访达中显示 the open library. The system's menu has no column for the counts, so each joins its library's name. It is built as the menu opens, so it says what is true then.
func switcherMenu(cx *core.Scope, a App) []nsdesktop.MenuItem {
	open := a.LibraryDir()
	libraries := a.KnownLibraries()
	var items []nsdesktop.MenuItem
	for _, dir := range libraries {
		current := open != "" && filepath.Clean(open) == filepath.Clean(dir)
		label := Title(dir)
		if current {
			label = locales.SidebarLibraryCount(cx, label, a.FilmCount())
		} else if n, ok := a.LibraryCount(dir); ok {
			label = locales.SidebarLibraryCount(cx, label, n)
		}
		// TODO(nsgo#118): make the libraries radio items, the open one chosen, as MyGo's Menu.Item(...).Radio drew them; NSGo's menu items are only checkable https://github.com/nsgo-dev/nsgo/issues/118
		items = append(items, nsdesktop.MenuItem{Label: label, Checkable: true, Checked: current, OnSelect: live.Do(a, func() { a.Choose(dir) })})
	}
	if len(libraries) > 0 {
		items = append(items, nsdesktop.MenuItem{Separator: true})
	}
	items = append(items,
		nsdesktop.MenuItem{Label: locales.SidebarNewLibrary(cx), OnSelect: live.Do(a, a.BeginNew)},
		nsdesktop.MenuItem{Label: locales.SidebarOpenLibrary(cx), OnSelect: live.Do(a, a.PickLibrary)},
	)
	if open != "" {
		items = append(items, nsdesktop.MenuItem{Label: locales.SidebarRevealLibrary(cx, Title(open), desktop.Platform(cx)), OnSelect: live.Do(a, func() { a.Reveal(open) })})
	}
	return items
}
