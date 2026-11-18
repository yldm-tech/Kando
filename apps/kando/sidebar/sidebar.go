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
	PageGeneral      = "general"
	PageSpeech       = "speech"
	PageAI           = "ai"
	PageTranslate    = "translate"
	PageAccount      = "account"

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

// The card the sidebar is drawn on, floating over the window's material: the prototype's white at 42% with a white inner hairline, a dark outer one and a soft shadow, 8 points in from the window's edges, its corners the platform's cards' (theme.RadiiOf).
const (
	cardMargin ns.Points = 8
	// insetStart and insetEnd are how far the switcher, the sources and the status line stand in from the card's edges. In the prototype the card is 231 wide and runs 3 points under the content, so they end 8 points before that hidden edge, 5 before the one that shows.
	insetStart ns.Points = 8
	insetEnd   ns.Points = 5
	// listPad is how far ui.Sidebar stands its rows in from its own sides (its content's padding, nsgo ui/sidebar.go), so the switcher and the search field, which stand as far in, fill the same width as a source row; rowRadius and rowPad are those rows' corners and the room inside their ends.
	listPad   ns.Points = 10
	rowRadius ns.Points = 8
	rowPad    ns.Points = 8
	// LightsRow is the empty row at the top of the card that the traffic lights sit in.
	LightsRow ns.Points = 48
)

// cardShadow is the soft shadow under the card. Its face and its rings are the theme's card color, translucent, and its shade (theme.Of), and the gutter around it stands in for the window's material where the window shows none (a test, Linux, a window without one): the prototype's frosted strip over the page.
var cardShadow = ns.RGBA(0x1E325A, 0.08)

// View is the sidebar: a column Width wide holding the card with the library switcher, the search field, the sources, the spoken languages, the watch states and the status line. Its top row clears the traffic lights of a window with a hidden title bar and drags the window.
type View struct {
	App App
	// Subscriptions is the list of the subscriptions (package subscribe), under the watch states.
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
			return ns.Style{Background: k.Card().Faded(0.42),
				Shadows: []ns.BoxShadow{{Spread: 0.5, Color: k.Card().Faded(0.85), Inset: true}, {Spread: 0.5, Color: k.Shade(0.06)}, {Offset: ns.Point{Y: 4}, Blur: 14, Color: cardShadow}}}
		},
		Children: []ns.Node{
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
			// The languages follow the sources and the watch states the languages, each list as tall as its rows, and the room left stays under them.
			ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{sources{a}, languages{a}, watched{a}, v.subscriptions()}},
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
	return ns.View{Style: ns.Style{FlexShrink: 1, MinHeight: ns.Pt(0), Padding: ns.Edges{Right: insetEnd, Left: insetStart}}, Children: []ns.Node{v.Subscriptions}}
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

// rowStyle is a part of the sidebar that stands as far in from the card's edges as the source rows' fills, insetStart and insetEnd and then listPad, 10 points above the next: the switcher and the search field.
var rowStyle = ns.Style{Direction: ns.Row, AlignItems: ns.AlignStretch, Padding: ns.Edges{Right: insetEnd + listPad, Bottom: 10, Left: insetStart + listPad}}
