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
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The switcher's own colors; its row is shaded from the theme (theme.Of).
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
	// A locked library's tile shows a lock in place of the folder, and the switcher's name says so.
	locked := live.Func(a, func() bool { return a.LibraryDir() != "" && a.LibraryLocked(a.LibraryDir()) })
	glyph := func() *core.SVG {
		if locked() {
			return lucide.LockKeyhole()
		}
		return iconFolder
	}
	// open is set while the menu shows, which keeps the pill pressed.
	open := core.NewSignal(cx, false)
	// place is the row around the pill, which base.Button takes no Ref for, so the row's frame gives the pill's: the menu opens under the pill, at its leading edge, wherever it was pressed from, a key included.
	place := &core.Ref{}
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X + insetStart + listPad, Y: b.Origin.Y + b.Size.Height - 10 + 4}
		}
		open.Set(true)
		nsdesktop.PopupMenu(cx, switcherMenu(cx, a), at, func() { open.Set(false) })
	}
	tile := core.View{
		Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), FlexShrink: 0, CornerRadius: 6, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Gradients: []core.Gradient{core.LinearGradient(135, tileFrom, tileTo)}},
		Children: []core.Node{ui.IconOf(glyph, 14, ui.Fixed(white))},
	}
	// The prototype's two lines take 15 and 16 points, the browser's normal line heights for these sizes, which puts the caption's top 4.5 points below the pill's.
	caption := core.Text{ContentFunc: func() string { return locales.SidebarLibrary(cx) }, MaxLines: 1,
		Style: core.Style{FontSize: 11, LineHeight: 16.5, FontWeight: ui.Bold, LetterSpacing: 0.2}, StyleFunc: ui.Colored(colors.Muted)}
	title := core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
		Style: core.Style{FontSize: 13, LineHeight: 16, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}
	label := func() string {
		if locked() {
			return locales.SidebarSwitcherLabel(cx, locales.LockLibraryLocked(cx, name()))
		}
		return locales.SidebarSwitcherLabel(cx, name())
	}
	// A plain row on the sidebar, as the source rows under it: no card of its own, as wide as their fills with their corners, its tile over their icons, shaded only under the pointer and while its menu is open, and ringed while the keyboard is on it, as they are.
	button := base.Button{
		TestID:    "sidebar-switcher",
		LabelFunc: label,
		Expanded:  open.Get,
		OnPress:   press,
		Style: core.Style{Direction: core.Row, FlexGrow: 1, Height: core.Pt(40), AlignItems: core.AlignCenter, Gap: 9, Padding: core.Symmetric(0, rowPad), CornerRadius: rowRadius,
			Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			if open.Get() {
				return core.Style{Background: k.Shade(0.07)}
			}
			return core.Style{Hover: &core.Style{Background: k.Shade(0.05)}, Active: &core.Style{Background: k.Shade(0.07)}}
		},
		Children: []core.Node{
			tile,
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{caption, title}},
			ui.Icon(svg(iconUpDown), 10, colors.Muted),
		},
	}
	return core.View{Ref: place, Style: rowStyle, Children: []core.Node{button}}
}

// switcherMenu lists the known libraries, the open one checked, each with how many films it holds, or a locked one marked so without its count; then 新建资料库…, 打开已有资料库…, 在访达中显示 the open library, and its password's items: 设置密码…, or 更改密码…, 移除密码… and, while it is unlocked, 立即锁定. The system's menu has no column for the counts, so each joins its library's name. It is built as the menu opens, so it says what is true then.
func switcherMenu(cx *core.Scope, a App) []nsdesktop.MenuItem {
	open := a.LibraryDir()
	libraries := a.KnownLibraries()
	var items []nsdesktop.MenuItem
	for _, dir := range libraries {
		current := open != "" && filepath.Clean(open) == filepath.Clean(dir)
		label := Title(dir)
		if a.LibraryLocked(dir) {
			label = locales.LockLibraryLocked(cx, label)
		} else if current {
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
		items = append(items, nsdesktop.MenuItem{Separator: true})
		if !a.LibraryHasPassword(open) {
			items = append(items, nsdesktop.MenuItem{Label: locales.LockMenuSet(cx), OnSelect: live.Do(a, func() { a.SetPassword(open) })})
		} else {
			items = append(items,
				nsdesktop.MenuItem{Label: locales.LockMenuChange(cx), OnSelect: live.Do(a, func() { a.ChangePassword(open) })},
				nsdesktop.MenuItem{Label: locales.LockMenuRemove(cx), OnSelect: live.Do(a, func() { a.RemovePassword(open) })},
			)
			if !a.LibraryLocked(open) {
				items = append(items, nsdesktop.MenuItem{Label: locales.LockNow(cx), OnSelect: live.Do(a, a.LockNow)})
			}
		}
	}
	return items
}
