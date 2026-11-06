package main

import (
	"lingocut/apps/lingocut/films"

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

func (a *app) SourceFilter() string { return a.lib.filter }

// SetSourceFilter goes to the library page, closing the open film, and shows the films from source there.
func (a *app) SetSourceFilter(source string) {
	a.Select(pageFilms)
	a.lib.filter = source
}

func (a *app) SourceCount(source string) int { return len(films.BySource(a.films(), source)) }

// sidebarShown reports whether the window shows the sidebar. A film opened from 全部影片 takes the whole window.
func (a *app) sidebarShown() bool { return !a.filmFillsWindow() }
