package sidebar

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/chrome"
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	PageFilms   = ""
	PageYouTube = "youtube"
	PageBrowse  = "browse" // the built-in browser (浏览), a row of the sources' list that is not a source
	// PageSubscription is a subscription's page, which a row of the subscriptions' list (View.Subscriptions) shows.
	PageSubscription = "subscription"
	// PageMediaSources is the 媒体来源 page (package sourcebrowser), a row of the sources' list that is not a source, always shown, so a NAS or a media server can be added from it before there is one.
	PageMediaSources = "media-sources"
	PageGeneral      = "general"
	PageSpeech       = "speech"
	PageAI           = "ai"
	PageTranslate    = "translate"
	PageAccount      = "account"

	// Width is the sidebar's column: the card and the gutter around it on every side.
	Width        ns.Points = 244
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
	// LibraryHasPassword reports whether the library at dir has a password, and LibraryLocked whether it is locked now, so the switcher shows its name with a lock and no count. SetPassword, ChangePassword and RemovePassword open the password dialog for it, and LockNow locks the open library again.
	LibraryHasPassword(dir string) bool
	LibraryLocked(dir string) bool
	SetPassword(dir string)
	ChangePassword(dir string)
	RemovePassword(dir string)
	LockNow()
	// SourceFilter is the source the library page shows, SourceAll or another Source value.
	SourceFilter() string
	// SetSourceFilter shows the library page filtered by source and closes the open film.
	SetSourceFilter(source string)
	// SourceCount is how many films of the open library come from source.
	SourceCount(source string) int
	// SpokenCounts is how many done films of the open library were heard in each language, those whose language is not known left out. SpokenFilter is the language the library page shows the films of, SpokenAll or whisper's code for one of them, which applies together with SourceFilter; SetSpokenFilter shows the library page filtered by it, keeping the source, and closes the open film.
	SpokenCounts() []SpokenCount
	SpokenFilter() string
	SetSpokenFilter(code string)
	// WatchCounts is how many done films are in each watch state among those the library page would show with every other filter as it is, the source, the language and the search, so a row's count is how many films choosing it shows. WatchFilter is the watch state the library page shows the films in, WatchAll or another Watch value, which applies together with the others; SetWatchFilter shows the library page filtered by it, keeping the others, and closes the open film.
	WatchCounts() WatchCounts
	WatchFilter() string
	SetWatchFilter(state string)
	// Query is what the search field holds: the library page shows the done films whose names hold it. SetQuery is what the user typed into it, which shows the library page, where the films found are, when another page shows. QueryFocus asks the field for the keyboard focus once, as ⌘F does; the field sets it back to false when it has taken the focus, or when it is not shown. SubmitQuery is Return in the field.
	Query() *string
	SetQuery(text string)
	QueryFocus() *bool
	SubmitQuery()
	// OpenSettings shows the settings at section, one of the settings package's Section values; "" opens 通用.
	OpenSettings(section string)
	// Readiness is what the status line at the foot of the sidebar says.
	Readiness() Readiness
	// PaletteButton reports whether the status card carries a button that opens the command palette: where the app has no menu bar of its own (Windows, Linux), so the palette, and through it the commands of the menu bar alone and the shortcut cheat sheet, can be found without knowing its key. PaletteKey is that key in NSGo's form ("mod+k"), which the button's tooltip writes as the platform does, and TogglePalette opens the palette.
	PaletteButton() bool
	PaletteKey() string
	TogglePalette()
	// Jobs is how many of the queue's jobs are waiting or running and how many failed, which the status line counts, so a job that ends while another page is shown is seen there; ShowJobs shows them, in 处理中 on the library page.
	Jobs() Jobs
	ShowJobs()
}

// Jobs is how many jobs the queue holds that are waiting or running (Active) and that failed rather than being cancelled (Failed).
type Jobs struct {
	Active, Failed int
}

// The card the sidebar is drawn on, 8 points in from the window's edges and from the page on every side, so it stands apart from the page as an object of its own rather than a column beside it, its corners the platform's cards' (theme.RadiiOf). On macOS it floats over the page's own ground as Finder's sidebar does on macOS 26: in the light, Liquid Glass washed with white (glassWash), a frosted face before macOS 26, which has no glass; in the dark, the card's gray, a step lighter than the page, with no glass, since the glass takes on the colors of the desktop behind the window there and shows them as a blue-to-green gradient beside the page's neutral gray, which no tint over it can neutralize: NSGo draws the background under the glass. Elsewhere it is the prototype's white at 42% over the window's material, Mica on Windows, or over the painted gutter. Everywhere it has a white inner hairline, a dark outer one and a soft shadow.
const (
	cardMargin ns.Points = 8
	// inset is how far the switcher, the sources and the status line stand in from the card's sides.
	inset ns.Points = 8
	// listPad is how far ui.Sidebar stands its rows in from its own sides (its content's padding, nsgo ui/sidebar.go), so the switcher and the search field, which stand as far in, fill the same width as a source row; rowRadius and rowPad are those rows' corners and the room inside their ends.
	listPad   ns.Points = 10
	rowRadius ns.Points = 8
	rowPad    ns.Points = 8
	// LightsRow is the empty row at the top of the card that the traffic lights sit in.
	LightsRow ns.Points = 48
)

// The card's face and its rings are the theme's card color, translucent, and its shade, and its shadow a card's (theme.Of, theme.ShadowCard). On macOS the gutter around it is the page's ground, which the window paints (view.go); elsewhere it shows the window's material, and it stands in for it where the window shows none (a test, Linux, a window without one): the prototype's frosted strip.

// glassWash is how much white a view of the card's own lays over the glass in the light. The glass takes on the colors of the desktop behind the window, a cyan-to-green gradient over a blue wallpaper, and the card's background cannot tone it down, as NSGo draws it under the glass; at 70% the panel reads as the near-white of Finder's sidebar and keeps the glass's rim.
const glassWash = 0.7

// floats reports whether the sidebar floats over the page's ground as macOS 26 draws its sidebars: on macOS, where the window shows no material.
func floats(cx *ns.Scope) bool { return cx.Env().OS() == ns.OSMacOS && !nsdesktop.ShowsMaterial(cx) }

// View is the sidebar: a column Width wide holding the card with the library switcher, the search field, the sources, the spoken languages, the watch states, the categories, the subscriptions and the status line. Its top row clears the traffic lights of a window with a hidden title bar and drags the window.
type View struct {
	App App
	// Categories is the list of the library's categories (package category), under the watch states, and Subscriptions the list of the subscriptions (package subscribe), under them.
	Categories    ns.Node
	Subscriptions ns.Node
}

// Render implements core.Node.
func (v View) Render(cx *ns.Scope) ns.Node {
	a := v.App
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	// The hairlines are rings of half a point that take no room, as the prototype's box-shadows: the card's face just inside the edge, the shade just outside it.
	card := ns.View{
		Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0), AlignItems: ns.AlignStretch, CornerRadius: theme.RadiiOf(cx).Card},
		StyleFunc: func() ns.Style {
			s := ns.Style{Background: k.Card().Faded(0.42),
				Shadows: []ns.BoxShadow{{Spread: 0.5, Color: k.Card().Faded(0.85), Inset: true}, {Spread: 0.5, Color: k.Shade(0.06)}, theme.ShadowCard}}
			switch {
			case floats(cx) && cx.Dark():
				s.Background = k.Card()
			case floats(cx):
				s.Background, s.GlassEffect, s.BackdropFilters = k.Card().Faded(0.3), ns.GlassRegular, []ns.Filter{ns.Blur(30)}
			}
			return s
		},
		Children: []ns.Node{
			// The wash over the glass in the light, under the card's lists.
			ns.View{TestID: "sidebar-wash", Style: ns.Style{Position: ns.Absolute, Top: ns.Pt(0), Right: ns.Pt(0), Bottom: ns.Pt(0), Left: ns.Pt(0), CornerRadius: theme.RadiiOf(cx).Card},
				StyleFunc: func() ns.Style {
					if !floats(cx) || cx.Dark() {
						return ns.Style{}
					}
					return ns.Style{Background: k.Card().Faded(glassWash)}
				}},
			ns.View{
				Desktop: ns.DesktopViewProps{WindowDrag: ns.WindowDragOn},
				Style:   ns.Style{FlexShrink: 0},
				StyleFunc: func() ns.Style {
					top := ns.Points(10)
					if chrome.Covered(cx).Top > 0 {
						top = LightsRow
					}
					return ns.Style{Height: ns.Pt(top)}
				},
			},
			switcher{a},
			search{a},
			// The languages follow the sources, the watch states the languages, then the categories and the subscriptions, each list as tall as its rows, and the room left stays under them. When they are taller than the card, they scroll as one column, as Finder's sidebar does, rather than each list being cut short by the next.
			ns.Scroll{TestID: "sidebar-lists", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
				ns.View{Style: ns.Style{FlexShrink: 0}, Children: []ns.Node{sources{a}, languages{a}, watched{a}, v.Categories, v.subscriptions()}},
			}},
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
		Style:  ns.Style{Width: ns.Pt(Width), FlexShrink: 0, AlignItems: ns.AlignStretch, Padding: ns.All(cardMargin)},
		// On macOS the gutter around the card is the page's ground, which the window paints; with the window's material on, it shows the material; otherwise it is painted.
		StyleFunc: func() ns.Style {
			if floats(cx) || nsdesktop.ShowsMaterial(cx) {
				return ns.Style{}
			}
			return ns.Style{Background: k.Gutter()}
		},
		Children: []ns.Node{card},
	}
}

// subscriptions is the list of the subscriptions as far in from the card's edges as the other lists, nil when the view has none.
func (v View) subscriptions() ns.Node {
	if v.Subscriptions == nil {
		return nil
	}
	return ns.View{Style: ns.Style{FlexShrink: 1, MinHeight: ns.Pt(0), Padding: ns.Symmetric(0, inset)}, Children: []ns.Node{v.Subscriptions}}
}

// Menu is the library menu of the settings page's 切换 button, for nsgo's desktop.PopupMenu: the known libraries, new and open, reveal, and forgetting the open one. Its items do what they say through a's actions and tell a it changed.
func Menu(cx *ns.Scope, a App) []nsdesktop.MenuItem {
	currentDir := a.LibraryDir()
	var items []nsdesktop.MenuItem
	for _, dir := range a.KnownLibraries() {
		items = append(items, nsdesktop.MenuItem{Label: Title(dir), Radio: true, Checked: filepath.Clean(currentDir) == filepath.Clean(dir), OnSelect: live.Do(a, func() { a.Choose(dir) })})
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

// rowStyle is a part of the sidebar that stands as far in from the card's edges as the source rows' fills, inset and then listPad, 10 points above the next: the switcher and the search field.
var rowStyle = ns.Style{Direction: ns.Row, AlignItems: ns.AlignStretch, Padding: ns.Edges{Right: inset + listPad, Bottom: 10, Left: inset + listPad}}
