package main

import (
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

const (
	pageFilms     = ""
	pageYouTube   = "youtube"
	pageGeneral   = "general"
	pageSpeech    = "speech"
	pageAI        = "ai"
	pageTranslate = "translate"

	appVersion = "0.1.0"

	sidebarWidth float32 = 248
	pagePadX     float32 = 32
	cardMinWidth float32 = 280
	cardGap      float32 = 18
)

func (a *app) view(c *ui.Context) {
	theme.Apply(c)
	t := c.Theme()
	films := a.films()
	// The traffic lights sit on the dark content. The band under them is the drag region.
	bar := c.TitleBar()
	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if bar.Height > 0 {
			ui.Row(c).Height(bar.Height).Padding(0, bar.Right, 0, bar.Left).DragWindow()
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if a.page != pageFilms || a.openKey == "" {
				a.sidebar(c, t, len(films))
			}
			a.mainColumn(c, t, films)
		})
	})
}

func (a *app) mainColumn(c *ui.Context, t *ui.Theme, films []film) {
	padX, padY := pagePadX, float32(28)
	if a.page == pageFilms && a.openKey != "" {
		padX, padY = 16, 12
	}
	body := ui.Column(c).Grow(1).MinWidth(0).Padding(padY, padX).Gap(20)
	if files := body.DroppedFiles(); len(files) > 0 {
		for _, file := range files {
			if media.IsVideoFile(file) {
				a.importLocal(file)
				break
			}
		}
	}
	if body.FileDragOver() {
		body.Background(t.Surface)
	}
	body.Children(func() {
		switch a.page {
		case pageGeneral, pageSpeech, pageAI, pageTranslate:
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).WidthPercent(100).MaxWidth(920).Gap(22).Children(func() {
					a.settings(c, t)
				})
			})
		default:
			a.libraryPage(c, t, films)
		}
	})
}

// shellBody is the main column under Onevra's floating header. Opening a film replaces the sidebar page; closing it restores the sidebar.
func (a *app) shellBody(c *ui.Context) {
	film := a.page == pageFilms && a.openKey != ""
	if a.shell != nil && film != a.shellFilm {
		a.shellFilm = film
		if film {
			a.shell.SetPage(func(c *ui.Context) { a.shellBody(c) })
		} else {
			a.shell.SetPage(nil)
		}
	}
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		a.mainColumn(c, c.Theme(), a.films())
	})
}
