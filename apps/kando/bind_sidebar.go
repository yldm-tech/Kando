package main

import (
	"path/filepath"
	"runtime"

	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// The sidebar's reads and clicks (sidebar.App). Several of these also serve films.App, film.App and settings.App. The sidebar reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

func (a *app) LibraryDir() string { return a.snap().LibraryDir }

func (a *app) KnownLibraries() []string { return a.knownLibraries() }

func (a *app) SaveErr() string { return a.saveErr }

func (a *app) Page() string { return a.page }

func (a *app) Select(page string) {
	a.page = page
	a.openKey = ""
	a.stopPlayback()
}

func (a *app) Choose(dir string) {
	a.edit(func(s *session) { s.MakingLibrary = false })
	a.page = pageFilms
	a.switchLibrary(dir)
}

func (a *app) BeginNew() { a.beginNewLibrary() }

func (a *app) PickLibrary() { a.pickLibrary() }

func (a *app) Reveal(path string) { a.reveal(path) }

func (a *app) Forget(dir string) { a.forgetLibrary(dir) }

func (a *app) FilmCount() int { return len(a.films()) }

// Logo is Kando's icon as an image source, for a core.Image.
func (a *app) Logo() core.ImageSource { return core.ImageSource{Data: appIconPNG} }

func (a *app) SourceFilter() string { return a.side.filter }

// SetSourceFilter goes to the library page, closing the open film, and shows the films from source there.
func (a *app) SetSourceFilter(source string) {
	a.Select(pageFilms)
	a.side.filter = source
}

// SourceCount is how many done films of the open library come from source, the films the grid shows for it; a job still in 处理中 is not one yet.
func (a *app) SourceCount(source string) int {
	n := 0
	for _, item := range films.BySource(a.films(), source) {
		if !films.InQueue(item) {
			n++
		}
	}
	return n
}

func (a *app) Query() *string { return &a.side.query }

func (a *app) QueryFocus() *bool { return &a.side.queryFocus }

// LibraryCount is the count countLibraries last read for the library at dir.
func (a *app) LibraryCount(dir string) (int, bool) {
	n, ok := a.side.libraryCounts[filepath.Clean(dir)]
	return n, ok
}

func (a *app) Readiness() sidebar.Readiness { return a.readiness() }

// sidebarLibraryChanged runs after the open library changed (libraryChanged in sidebarHooks): the sidebar shows all of the new library's films, with no search, and the switcher's counts are read again, the library just left among them.
func (a *app) sidebarLibraryChanged() {
	a.side.filter = sidebar.SourceAll
	a.side.query = ""
	a.countLibraries()
}

// sidebarShown reports whether the window shows the sidebar. A film opened from 全部影片 takes the whole window.
func (a *app) sidebarShown() bool { return !a.filmFillsWindow() }

// sidebarColumn is the sidebar at the window's left. The top row of its card holds the traffic lights and drags the window, so it needs no drag band over it.
func (a *app) sidebarColumn() core.Node { return sidebar.View{App: a} }

// contentBand is the height of the drag band over the content, under the title bar: none while the page there draws its own header bar (pageOwnsTitleBar), otherwise the title bar's own, titleBar being its height as desktop.TitleBar reports it. On macOS the traffic lights move down into the sidebar card's top row (trafficLights), which makes the title bar as tall as the room under them is above them, so the content keeps a band only as tall as that row, which still clears the traffic lights when an open film hides the sidebar. Elsewhere the window controls fill the title bar, and the band is the title bar.
func (a *app) contentBand(titleBar core.Points) core.Points {
	if a.pageOwnsTitleBar() {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return min(titleBar, sidebar.LightsRow)
	}
	return titleBar
}

// pageOwnsTitleBar reports whether the page in the content column draws its own header bar at its top. Such a bar already keeps clear of the window controls, drags the window and is at least as tall as the title bar, so a drag band over it would leave the title bar's room twice. Each page says so itself: the open film in filmOwnsTitleBar (bind_film.go), the library page in libraryOwnsTitleBar (bind_films.go), the browser in browseOwnsTitleBar (bind_browse.go).
func (a *app) pageOwnsTitleBar() bool {
	return a.filmOwnsTitleBar() || a.libraryOwnsTitleBar() || a.browseOwnsTitleBar()
}

// trafficLights is where view puts the window's traffic lights: in the empty top row of the sidebar's card (sidebar.View), 8 points in from the window's corner, that is 16 from the card's edge and centered in its 48-point row.
func trafficLights() desktop.WindowControls {
	return desktop.WindowControls{TrafficLights: core.Point{X: 24, Y: 25}}
}

// sidebarMaterial is the window's material, which the sidebar shows: AppKit's source list on macOS and Mica on Windows, whose navigation pane is transparent over it, as Onevra's shell chose. Linux shows none, and the sidebar paints its gutter.
func sidebarMaterial() core.WindowMaterial {
	switch runtime.GOOS {
	case "darwin":
		return desktop.WindowMaterialSidebar
	case "windows":
		return desktop.WindowMaterialMica
	}
	return desktop.WindowMaterialNone
}
