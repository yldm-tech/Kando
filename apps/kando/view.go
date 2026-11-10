package main

import (
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"

	"github.com/egoist/mygo/ui"
)

const (
	pageFilms     = sidebar.PageFilms
	pageYouTube   = sidebar.PageYouTube
	pageBrowse    = sidebar.PageBrowse
	pageGeneral   = sidebar.PageGeneral
	pageSpeech    = sidebar.PageSpeech
	pageAI        = sidebar.PageAI
	pageTranslate = sidebar.PageTranslate
	pageAccount   = sidebar.PageAccount

	sidebarWidth = sidebar.Width
	pagePadX     = sidebar.PagePadX
)

// appVersion is the version the app shows and reports: main sets it to the one mygo build gave the app, mygo.json's, which Release bumps. Without one, as in tests and go run, it stays this fixed one, which the tests and screenshots show.
var appVersion = "0.1.0"

// view connects the window's parts: the theme, the shortcut table, the sidebar, the page and the overlays above them. Each part keeps its own decisions in its own file: sidebarShown, sidebarColumn and contentBand in bind_sidebar.go, filmFillsWindow, filmOwnsTitleBar and filmPadding in bind_film.go, dropFiles in bind_films.go, browseColumn in bind_browse.go, the shortcut table in keys.go, overlays in state_overlay.go. What an area does once the frame is built is its areaHooks.afterFrame (events.go).
func (a *app) view(c *ui.Context) {
	t, dark := palette(c)
	a.services = c.Services()
	a.wireKeys(c)
	items := a.films()
	// With the window's material on, the sidebar shows it and the content stays opaque.
	vibrancy := c.Vibrancy()
	if vibrancy {
		c.Root().Background(ui.Transparent)
	}
	// The traffic lights sit at the top left. The band under the title bar is the drag region, above both the sidebar and the content.
	bar := c.TitleBar()
	showSidebar := a.sidebarShown()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if showSidebar {
			a.sidebarColumn(c, t, bar)
		}
		content := ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch)
		if vibrancy {
			content.Background(t.Background)
		}
		content.Children(func() {
			if band := a.contentBand(bar); band > 0 {
				left := bar.Left
				if showSidebar {
					left = 0
				}
				ui.Row(c).Height(band).Padding(0, bar.Right, 0, left).DragWindow()
			}
			a.mainColumn(c, t, dark, items)
		})
	})
	a.overlays(c, t, dark)
	a.areasAfterFrame()
}

func (a *app) mainColumn(c *ui.Context, t, dark *ui.Theme, items []films.Film) {
	padX, padY := pagePadX, float32(28)
	if a.filmFillsWindow() {
		padX, padY = a.filmPadding()
	} else if a.libraryOwnsTitleBar() || a.page == pageBrowse {
		// The library page's header bar spans the column from its top, and the page pads what is under it itself (films.Show); the browser's toolbar and page run to the column's edges (browseColumn).
		padX, padY = 0, 0
	}
	body := ui.Column(c).Grow(1).MinWidth(0).Padding(padY, padX).Gap(20)
	if files := body.DroppedFiles(); len(files) > 0 {
		a.dropFiles(files)
	}
	if body.FileDragOver() {
		body.Background(t.Surface)
	}
	body.Children(func() {
		if a.page == pageBrowse {
			a.browseColumn(c, t)
			return
		}
		films.Show(c, t, a, items, func(c *ui.Context, _ *ui.Theme, item films.Film) {
			film.Show(c, dark, a, item)
		})
	})
}
