package main

import (
	"path/filepath"
	"runtime"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The sidebar's reads and clicks (sidebar.App). Several of these also serve films.App, film.App and settings.App.

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

func (a *app) Logo() *ui.Bitmap { return appLogo }

func (a *app) SourceFilter() string { return a.side.filter }

// SetSourceFilter goes to the library page, closing the open film, and shows the films from source there.
func (a *app) SetSourceFilter(source string) {
	a.Select(pageFilms)
	a.side.filter = source
}

func (a *app) SourceCount(source string) int { return len(films.BySource(a.films(), source)) }

// LibraryCount is the count countLibraries last read for the library at dir.
func (a *app) LibraryCount(dir string) (int, bool) {
	n, ok := a.side.libraryCounts[filepath.Clean(dir)]
	return n, ok
}

func (a *app) Readiness() sidebar.Readiness { return a.readiness() }

// sidebarLibraryChanged runs after the open library changed (libraryChanged in sidebarHooks): the sidebar shows all of the new library's films, and the switcher's counts are read again, the library just left among them.
func (a *app) sidebarLibraryChanged() {
	a.side.filter = sidebar.SourceAll
	a.countLibraries()
}

// sidebarShown reports whether the window shows the sidebar. A film opened from 全部影片 takes the whole window.
func (a *app) sidebarShown() bool { return !a.filmFillsWindow() }

// sidebarColumn is the sidebar at the window's left. The top row of its card holds the traffic lights and drags the window, so it needs no drag band over it.
func (a *app) sidebarColumn(c *ui.Context, t *ui.Theme, _ ui.TitleBar) {
	sidebar.Show(c, t, a)
}

// contentBand is the height of the drag band over the content, under the title bar: none while the page there draws its own header bar (pageOwnsTitleBar), otherwise the title bar's own. On macOS mygo's title bar leaves as much room under the traffic lights as above them, 64 points once trafficLights moves them down into the sidebar card's top row, so the content keeps a band only as tall as that row, which still clears the traffic lights when an open film hides the sidebar. Elsewhere the window controls fill the title bar, and the band is the title bar.
func (a *app) contentBand(bar ui.TitleBar) float32 {
	if a.pageOwnsTitleBar() {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return min(bar.Height, sidebar.LightsRow)
	}
	return bar.Height
}

// pageOwnsTitleBar reports whether the page in the content column draws its own header bar at its top (breeze.HeaderBar). Such a bar already keeps clear of the window controls, drags the window and is at least as tall as the title bar, so a drag band over it would leave the title bar's room twice. Each page says so itself: the open film in filmOwnsTitleBar (bind_film.go), the library page in libraryOwnsTitleBar (bind_films.go).
func (a *app) pageOwnsTitleBar() bool { return a.filmOwnsTitleBar() || a.libraryOwnsTitleBar() }

// trafficLights is where main puts the window's traffic lights: in the empty top row of the sidebar's card (sidebar.Show), 8 points in from the window's corner, that is 16 from the card's edge and centered in its 48-point row. That makes the title bar mygo reports 64 points tall; contentBand keeps the content's drag band at the card row's 48.
func trafficLights() *mygo.Point { return &mygo.Point{X: 24, Y: 25} }

// sidebarMaterial is the window's material, which the sidebar shows: AppKit's source list on macOS and Mica on Windows, whose NavigationView pane is transparent over it, as Onevra's shell chose.
func sidebarMaterial() mygo.Vibrancy {
	vibrancy := mygo.VibrancyNone
	switch runtime.GOOS {
	case "darwin":
		vibrancy = mygo.VibrancySidebar
	case "windows":
		vibrancy = mygo.VibrancyMica
	}
	return vibrancy
}
