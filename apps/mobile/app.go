package main

import (
	"kando/apps/mobile/desktops"
	"kando/apps/mobile/film"
	"kando/apps/mobile/home"
	"kando/apps/mobile/library"
	"kando/apps/mobile/pair"
	"kando/apps/mobile/player"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// page is a kind of page of the app's stack.
type page uint8

const (
	homePage page = iota
	pairPage
	desktopsPage
	libraryPage
	filmPage
	playerPage
)

// route is a page of the stack and what it shows: the computer and library a library's or a film's page is of, the film, and the key of the film the player plays.
type route struct {
	page    page
	desktop string
	library sync.Library
	film    sync.Film
	key     string
}

// App is the app with the session cfg configures, a stack of pages over the home page.
func App(cx *core.Scope, cfg session.Config) core.Node {
	theme.Apply(cx)
	s := session.New(cx, cfg)
	nav := base.NewNav(cx, route{page: homePage})
	back := func() { nav.Pop(1) }
	paired := func() {
		// Pairing goes back where it was opened from: the home page, or the computers' page.
		nav.Pop(1)
	}
	return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		switch r.page {
		case pairPage:
			return pair.Page(cx, s, paired, back)
		case desktopsPage:
			return desktops.Page(cx, s, func() { nav.Push(route{page: pairPage}) }, back)
		case libraryPage:
			return library.Page(cx, s, r.desktop, r.library, func(f sync.Film) {
				nav.Push(route{page: filmPage, desktop: r.desktop, library: r.library, film: f})
			}, back)
		case filmPage:
			return film.Page(cx, s, r.desktop, r.library.Name, r.film, func(key string) { nav.Push(route{page: playerPage, key: key}) }, back)
		case playerPage:
			return player.Page(cx, s, r.key, back)
		}
		return home.Page(cx, s, home.Actions{
			Pair:     func() { nav.Push(route{page: pairPage}) },
			Desktops: func() { nav.Push(route{page: desktopsPage}) },
			Library: func(desktop string, lib sync.Library) {
				nav.Push(route{page: libraryPage, desktop: desktop, library: lib})
			},
			Film: func(f store.Film) {
				nav.Push(route{page: filmPage, desktop: f.Desktop, library: sync.Library{Name: f.Library}, film: f.Film})
			},
		})
	})
}
