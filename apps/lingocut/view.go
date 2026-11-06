package main

import (
	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"

	"github.com/egoist/mygo/ui"
)

const (
	pageFilms     = sidebar.PageFilms
	pageYouTube   = sidebar.PageYouTube
	pageGeneral   = sidebar.PageGeneral
	pageSpeech    = sidebar.PageSpeech
	pageAI        = sidebar.PageAI
	pageTranslate = sidebar.PageTranslate
	pageAccount   = sidebar.PageAccount

	appVersion = "0.1.0"

	sidebarWidth = sidebar.Width
	pagePadX     = sidebar.PagePadX
)

// view connects the window's parts: the theme, the shortcut table, the sidebar, the page and the overlays above them. Each part keeps its own decisions in its own file: sidebarShown and sidebarColumn in bind_sidebar.go, filmFillsWindow in bind_film.go, dropFiles in bind_films.go, settingsPage in bind_settings.go, the shortcut table in keys.go, overlays in state_overlay.go. What an area does once the frame is built is its areaHooks.afterFrame (events.go).
func (a *app) view(c *ui.Context) {
	t, dark := palette(c)
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
			if bar.Height > 0 {
				left := bar.Left
				if showSidebar {
					left = 0
				}
				ui.Row(c).Height(bar.Height).Padding(0, bar.Right, 0, left).DragWindow()
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
		padX, padY = 16, 12
	}
	body := ui.Column(c).Grow(1).MinWidth(0).Padding(padY, padX).Gap(20)
	if files := body.DroppedFiles(); len(files) > 0 {
		a.dropFiles(files)
	}
	if body.FileDragOver() {
		body.Background(t.Surface)
	}
	body.Children(func() {
		if a.settingsPage(c, t) {
			return
		}
		films.Show(c, t, a, items, func(c *ui.Context, _ *ui.Theme, item films.Film) {
			film.Show(c, dark, a, item)
		})
	})
}
