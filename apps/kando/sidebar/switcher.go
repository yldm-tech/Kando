package sidebar

import (
	"path/filepath"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

var (
	pillFill     = ui.RGBA(255, 255, 255, 0.6)
	pillOpenFill = ui.RGBA(0, 0, 0, 0.07)
	pillHairline = ui.RGBA(0, 0, 0, 0.06)
	// The folder tile's blue, from light at the top left to dark at the bottom right.
	tileFrom = ui.Hex("#2F8BFF")
	tileTo   = ui.Hex("#0A57C2")
	white    = ui.Hex("#FFFFFF")
)

// switcher is the library pill at the top of the sidebar: a folder tile, the caption 资料库 over the open library's name, and up and down arrows. A click opens the system menu of the known libraries.
func switcher(c *ui.Context, t *ui.Theme, a App) {
	name := locales.SidebarChooseLibrary()
	if dir := a.LibraryDir(); dir != "" {
		name = Title(dir)
	}
	ui.Row(c).Padding(0, insetEnd, 10, insetStart).AlignItems(ui.Stretch).Children(func() {
		// The hairline is a ring inside the edge that takes no room, as the prototype's inset box-shadow.
		pill := ui.ButtonBase(c).Grow(1).Height(40).Padding(0, 10).Gap(9).Radius(10).InsetShadow(0, 0, 0, 0.5, pillHairline).Label(locales.SidebarSwitcherLabel(name))
		if pill.Pressed() {
			pill.Background(pillOpenFill)
		} else {
			pill.Background(pillFill)
		}
		pill.Menu(func(m *ui.Menu) { switcherMenu(m, a) })
		pill.Children(func() {
			tile := ui.Box(c).Size(24, 24).Shrink(0).Radius(6).AlignItems(ui.Center).Justify(ui.Center)
			tile.LinearGradient(ui.LinearGradient{From: tileFrom, To: tileTo, Angle: 135})
			tile.Children(func() {
				ui.Icon(c, iconFolder).Size(14, 14).TextColor(white)
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				// The prototype's two lines take 15 and 16 points, the browser's normal line heights for these sizes, which puts the caption's top 4.5 points below the pill's.
				breeze.Text(c, locales.SidebarLibrary(), breeze.TextCaption, breeze.TextSecondary).FontWeight(700).LetterSpacing(0.2).LineHeight(1.5).SingleLine()
				breeze.Text(c, name, breeze.TextHeading).FontWeight(600).LineHeight(16.0 / 13).SingleLine().Ellipsis("…")
			})
			ui.Icon(c, iconUpDown).Size(10, 10).TextColor(t.TextMuted).Shrink(0)
		})
	})
}

// switcherMenu lists the known libraries, the open one checked, each with how many films it holds; then 新建资料库…, 打开已有资料库… and 在访达中显示 the open library. The system menu has no column for the counts, so each joins its library's name.
func switcherMenu(m *ui.Menu, a App) {
	open := a.LibraryDir()
	libraries := a.KnownLibraries()
	for _, dir := range libraries {
		current := open != "" && filepath.Clean(open) == filepath.Clean(dir)
		label := Title(dir)
		if current {
			label = locales.SidebarLibraryCount(label, a.FilmCount())
		} else if n, ok := a.LibraryCount(dir); ok {
			label = locales.SidebarLibraryCount(label, n)
		}
		if m.Item(label).Radio(current).Chosen() {
			a.Choose(dir)
		}
	}
	if len(libraries) > 0 {
		m.Separator()
	}
	if m.Item(locales.SidebarNewLibrary()).Chosen() {
		a.BeginNew()
	}
	if m.Item(locales.SidebarOpenLibrary()).Chosen() {
		a.PickLibrary()
	}
	if open != "" {
		if m.Item(locales.SidebarRevealLibrary(Title(open))).Chosen() {
			a.Reveal(open)
		}
	}
}
