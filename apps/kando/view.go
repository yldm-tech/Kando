package main

import (
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
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

// appVersion is the version the app shows and reports: main sets it to the one nsgo build gave the app, nsgo.yaml's, which Release bumps. Without one, as in tests and go run, it stays this fixed one, which the tests and screenshots show.
var appVersion = "0.1.0"

// view is the main window's content, built once: the theme, the title bar, the shortcut table, the sidebar, the page and the overlays above them. Each part keeps its own decisions in its own file: sidebarShown, sidebarColumn, contentBand and trafficLights in bind_sidebar.go, filmFillsWindow, filmOwnsTitleBar and filmPadding in bind_film.go, libraryColumn and dropFiles in bind_films.go, browseColumn in bind_browse.go, the shortcut table in keys.go, the window's frame and close button in state_window.go, overlays in state_overlay.go. What changes later reaches the parts through their bindings (package live), so nothing here is built again.
func (a *app) view(cx *core.Scope) core.Node {
	theme.Apply(cx)
	// The title bar is the app's own: the traffic lights sit in the sidebar card's top row, and the band under the title bar drags the window.
	nsdesktop.SetTitleBar(cx, nsdesktop.TitleBarHidden, 0)
	nsdesktop.SetWindowControls(cx, trafficLights())
	// The window's title follows the interface's language as Settings › 通用 changes it.
	cx.Effect(func() { nsdesktop.SetTitle(cx, locales.LibraryWindowTitle(cx)) })
	a.windowFollow(cx)
	a.areasFollow(cx)
	colors := ui.ColorsOf(cx)
	showSidebar := live.Memo(cx, a, a.sidebarShown)
	band := core.View{
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		StyleFunc: live.Func(a, func() core.Style {
			bar := nsdesktop.TitleBar(cx)
			left := bar.Left
			if showSidebar() {
				left = 0
			}
			return core.Style{Height: core.Pt(a.contentBand(bar.Top)), Padding: core.Edges{Right: bar.Right, Left: left}, FlexShrink: 0}
		}),
	}
	// With the window's material on, the sidebar shows it and the content stays opaque.
	content := core.View{
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), AlignItems: core.AlignStretch},
		StyleFunc: ui.Filled(colors.Background),
		Children:  []core.Node{band, a.mainColumn(cx)},
	}
	return core.View{
		Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, AlignItems: core.AlignStretch},
		// The window's text is in the platform's font of the design (theme.FontOf), which every text under it inherits, in the interface's language as it changes.
		StyleFunc: func() core.Style {
			s := core.Style{FontFamily: theme.FontOf(cx)}
			if !nsdesktop.ShowsMaterial(cx) {
				s.Background = colors.Background()
			}
			return s
		},
		Children: []core.Node{
			core.Show{When: showSidebar, Then: func(*core.Scope) core.Node { return a.sidebarColumn() }},
			content,
			a.wireKeys(cx),
			a.overlays(cx),
		},
	}
}

// mainColumn is the page beside the sidebar: the built-in browser, or the library with the film open in it, padded as the page wants, and the drop target for videos dragged in from the file manager, which it lights while they are over it.
func (a *app) mainColumn(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	dragOver := core.NewSignal(cx, false)
	return core.View{
		TestID: "main-column",
		Style:  core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 20},
		StyleFunc: live.Func(a, func() core.Style {
			padX, padY := pagePadX, core.Points(28)
			if a.filmFillsWindow() {
				padX, padY = a.filmPadding()
			} else if a.libraryOwnsTitleBar() || a.page == pageBrowse {
				// The library page's header bar spans the column from its top, and the page pads what is under it itself (films.View); the browser's toolbar and page run to the column's edges (browseColumn).
				padX, padY = 0, 0
			}
			s := core.Style{Padding: core.Symmetric(padY, padX)}
			if dragOver.Get() {
				s.Background = colors.Hover()
			}
			return s
		}),
		OnFileDragEnter: func() { dragOver.Set(true) },
		OnFileDragLeave: func() { dragOver.Set(false) },
		OnFileDrop: live.On(a, func(uris []string) {
			dragOver.Set(false)
			if paths := desktop.DroppedPaths(uris); len(paths) > 0 {
				a.dropFiles(paths)
			}
		}),
		// The browser column stays mounted while another page shows, hidden (browseColumn), so its web view keeps its page, history and video position as MyGo's did; it makes the web view only once a page is first asked for.
		Children: []core.Node{a.browseColumn(cx), core.Show{
			When: live.Func(a, func() bool { return a.page != pageBrowse }),
			Then: a.libraryColumn,
		}},
	}
}
