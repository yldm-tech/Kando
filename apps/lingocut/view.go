package main

import (
	"lingocut/apps/lingocut/account"
	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/settings"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/media"

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

func (a *app) view(c *ui.Context) {
	t := palette(c)
	items := a.films()
	// With the window's material on, the sidebar shows it and the content stays opaque.
	vibrancy := c.Vibrancy()
	if vibrancy {
		c.Root().Background(ui.Transparent)
	}
	// The traffic lights sit at the top left. The band under the title bar is the drag region, above both the sidebar and the content.
	bar := c.TitleBar()
	showSidebar := a.page != pageFilms || a.openKey == ""
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if showSidebar {
			ui.Column(c).Shrink(0).AlignItems(ui.Stretch).Children(func() {
				if bar.Height > 0 {
					ui.Row(c).Height(bar.Height).Padding(0, 0, 0, bar.Left).DragWindow()
				}
				sidebar.Show(c, t, len(items), appLogo, a)
			})
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
			a.mainColumn(c, t, items)
		})
	})
}

func (a *app) mainColumn(c *ui.Context, t *ui.Theme, items []films.Film) {
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
					settings.Show(c, t, appVersion, a)
				})
			})
		case pageAccount:
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).WidthPercent(100).MaxWidth(920).Gap(22).Children(func() {
					account.Show(c, t, a)
				})
			})
		default:
			films.Show(c, t, a, items, func(c *ui.Context, t *ui.Theme, item films.Film) {
				film.Show(c, t, a, item)
			})
		}
	})
}
