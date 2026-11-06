package main

import (
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

// sidebarShown reports whether the window shows the sidebar. A film opened from 全部影片 takes the whole window.
func (a *app) sidebarShown() bool { return !a.filmFillsWindow() }

// sidebarColumn is the sidebar at the window's left, under the drag band over it.
func (a *app) sidebarColumn(c *ui.Context, t *ui.Theme, bar ui.TitleBar) {
	ui.Column(c).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		if bar.Height > 0 {
			ui.Row(c).Height(bar.Height).Padding(0, 0, 0, bar.Left).DragWindow()
		}
		sidebar.Show(c, t, a)
	})
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
