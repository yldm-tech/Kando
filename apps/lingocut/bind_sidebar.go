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

// contentBand is the height of the drag band over the content, under the title bar: the title bar's own, or none while the open film draws its own header bar there (filmOwnsTitleBar).
func (a *app) contentBand(bar ui.TitleBar) float32 {
	if a.filmOwnsTitleBar() {
		return 0
	}
	return bar.Height
}

// trafficLights is where main puts the window's traffic lights, over the sidebar's top left corner.
func trafficLights() *mygo.Point { return &mygo.Point{X: 16, Y: 16} }

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
