package sidebar

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	PageFilms     = ""
	PageYouTube   = "youtube"
	PageBrowse    = "browse" // the built-in browser (浏览), a row of the sources' list that is not a source
	PageGeneral   = "general"
	PageSpeech    = "speech"
	PageAI        = "ai"
	PageTranslate = "translate"
	PageAccount   = "account"

	// Width is the sidebar's column: the card and the gutter of the window's material around it.
	Width        ns.Points = 236
	PagePadX     ns.Points = 32
	CardMinWidth ns.Points = 280
	CardGap      ns.Points = 18
)

// Sources the library can be filtered by. Apart from SourceAll they are the platforms core.ParseLink reads from a film's source; a film without a source it reads is a local file.
const (
	SourceAll      = "all"
	SourceYouTube  = string(core.PlatformYouTube)
	SourceBilibili = string(core.PlatformBilibili)
	SourceWeb      = string(core.PlatformWeb)
	SourceLocal    = string(core.PlatformLocal)
)

// App is the shelf and navigation the sidebar reads and reports. The sidebar reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	LibraryDir() string
	KnownLibraries() []string
	SaveErr() string
	Page() string
	Select(page string)
	Choose(dir string)
	BeginNew()
	PickLibrary()
	Reveal(path string)
	Forget(dir string)
	// FilmCount is how many films the open library holds.
	FilmCount() int
	// LibraryCount is how many films the library at dir holds, for a library that is not open; ok is false until the count has been read.
	LibraryCount(dir string) (n int, ok bool)
	// SourceFilter is the source the library page shows, SourceAll or another Source value.
	SourceFilter() string
	// SetSourceFilter shows the library page filtered by source and closes the open film.
	SetSourceFilter(source string)
	// SourceCount is how many films of the open library come from source.
	SourceCount(source string) int
	// Query is what the search field holds: the library page shows the done films whose names hold it. QueryFocus asks the field for the keyboard focus once, as ⌘F does; the field sets it back to false when it has taken the focus, or when it is not shown.
	Query() *string
	QueryFocus() *bool
	// OpenSettings shows the settings at section, one of the settings package's Section values; "" opens 通用.
	OpenSettings(section string)
	// Readiness is what the status line at the foot of the sidebar says.
	Readiness() Readiness
}

// The card the sidebar is drawn on, floating over the window's material: the prototype's white at 42% with a white inner hairline, a dark outer one and a soft shadow, 8 points in from the window's edges.
const (
	cardMargin ns.Points = 8
	cardRadius ns.Points = 14
	// insetStart and insetEnd are how far the switcher, the sources and the status line stand in from the card's edges. In the prototype the card is 231 wide and runs 3 points under the content, so they end 8 points before that hidden edge, 5 before the one that shows.
	insetStart ns.Points = 8
	insetEnd   ns.Points = 5
	// LightsRow is the empty row at the top of the card that the traffic lights sit in.
	LightsRow ns.Points = 48
)

var (
	cardFill      = ns.RGBA(0xFFFFFF, 0.42)
	cardHighlight = ns.RGBA(0xFFFFFF, 0.85)
	cardHairline  = ns.RGBA(0x000000, 0.06)
	cardShadow    = ns.RGBA(0x1E325A, 0.08)
	// gutterFill stands in for the window's material where the window shows none (a test, Linux, a window without one): the prototype's frosted strip over a white page.
	gutterFill = ns.RGB(0xF0F2F6)
)

// View is the sidebar: a column Width wide holding the card with the library switcher, the search field, the sources and the status line. Its top row clears the traffic lights of a window with a hidden title bar and drags the window.
type View struct {
	App App
}

// Render implements core.Node.
func (v View) Render(cx *ns.Scope) ns.Node {
	a := v.App
	colors := ui.ColorsOf(cx)
	// The hairlines are rings of half a point that take no room, as the prototype's box-shadows: white just inside the edge, dark just outside it.
	card := ns.View{
		Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), AlignItems: ns.AlignStretch, CornerRadius: cardRadius, Background: cardFill,
			Shadows: []ns.BoxShadow{{Spread: 0.5, Color: cardHighlight, Inset: true}, {Spread: 0.5, Color: cardHairline}, {Offset: ns.Point{Y: 4}, Blur: 14, Color: cardShadow}}},
		Children: []ns.Node{
			ns.View{
				Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOn},
				Style:   ns.Style{FlexShrink: 0},
				StyleFunc: func() ns.Style {
					top := ns.Points(10)
					if nsdesktop.TitleBar(cx).Top > 0 {
						top = LightsRow
					}
					return ns.Style{Height: ns.Pt(top)}
				},
			},
			switcher{a},
			search{a},
			sources{a},
			ns.Show{
				When: live.Func(a, func() bool { return a.SaveErr() != "" }),
				Then: func(cx *ns.Scope) ns.Node {
					return ns.Text{ContentFunc: live.Func(a, a.SaveErr), Style: ui.TypeBody.Merge(ns.Style{Padding: ns.Edges{Top: 8, Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Danger)}
				},
			},
			footer{a},
		},
	}
	return ns.View{
		TestID: "sidebar",
		Style:  ns.Style{Width: ns.Pt(Width), FlexShrink: 0, AlignItems: ns.AlignStretch, Padding: ns.Edges{Top: cardMargin, Bottom: cardMargin, Left: cardMargin}},
		// With the window's material on, the gutter around the card shows it; without it, the gutter is painted.
		StyleFunc: func() ns.Style {
			if nsdesktop.ShowsMaterial(cx) {
				return ns.Style{}
			}
			return ns.Style{Background: gutterFill}
		},
		Children: []ns.Node{card},
	}
}

// Menu is the library menu of the settings page's 切换 button, for nsgo's desktop.PopupMenu: the known libraries, new and open, reveal, and forgetting the open one. Its items do what they say through a's actions and tell a it changed.
func Menu(cx *ns.Scope, a App) []nsdesktop.MenuItem {
	currentDir := a.LibraryDir()
	var items []nsdesktop.MenuItem
	for _, dir := range a.KnownLibraries() {
		// TODO(nsgo#118): make the libraries radio items, one of which is chosen, as the menus of MyGo's Menu.Item(...).Radio were; NSGo's menu items are only checkable https://github.com/nsgo-dev/nsgo/issues/118
		items = append(items, nsdesktop.MenuItem{Label: Title(dir), Checkable: true, Checked: filepath.Clean(currentDir) == filepath.Clean(dir), OnSelect: live.Do(a, func() { a.Choose(dir) })})
	}
	items = append(items,
		nsdesktop.MenuItem{Label: locales.SidebarMenuNewLibrary(cx), OnSelect: live.Do(a, a.BeginNew)},
		nsdesktop.MenuItem{Label: locales.SidebarMenuOpenLibrary(cx), OnSelect: live.Do(a, a.PickLibrary)},
		nsdesktop.MenuItem{Separator: true},
		nsdesktop.MenuItem{Label: locales.SidebarMenuShowInFinder(cx, desktop.Platform(cx)), Disabled: currentDir == "", OnSelect: live.Do(a, func() { a.Reveal(currentDir) })},
	)
	if len(a.KnownLibraries()) > 1 {
		items = append(items, nsdesktop.MenuItem{Label: locales.SidebarMenuForget(cx), OnSelect: live.Do(a, func() { a.Forget(currentDir) })})
	}
	return items
}

// Title is the name of the library at dir: its folder's name without the .kando extension, or dir itself for a root.
func Title(dir string) string {
	base := filepath.Base(dir)
	base = strings.TrimSuffix(base, media.LibraryExt)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return dir
	}
	return base
}

// rowStyle is a part of the sidebar that stands insetStart and insetEnd in from the card's edges, 10 points above the next: the switcher and the search field.
var rowStyle = ns.Style{Direction: ns.Row, AlignItems: ns.AlignStretch, Padding: ns.Edges{Right: insetEnd, Bottom: 10, Left: insetStart}}
