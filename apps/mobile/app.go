package main

import (
	"kando/apps/mobile/film"
	"kando/apps/mobile/films"
	"kando/apps/mobile/learn"
	"kando/apps/mobile/library"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/onboarding"
	"kando/apps/mobile/pair"
	"kando/apps/mobile/player"
	"kando/apps/mobile/session"
	"kando/apps/mobile/settings"
	"kando/apps/mobile/shell"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// page is a kind of page of the app's stacks.
type page uint8

const (
	rootPage page = iota
	filmsPage
	learnPage
	settingsPage
	targetPage
	pairPage
	introductionPage
	libraryPage
	filmPage
	playerPage
)

// The tabs, in the order the tab bar shows them.
const (
	filmsTab = iota
	learnTab
	settingsTab
	tabs
)

// route is a page of a stack and what it shows: the computer and library a library's or a film's page is of, the film, and the key of the film the player plays.
type route struct {
	page    page
	desktop string
	library sync.Library
	film    sync.Film
	key     string
}

// App is the app with the session cfg configures: the tabs of the Library, Learn and Settings, each a stack of the pages it opens, under a stack of what covers them, pairing, the introduction and the player; the introduction in the tabs' place until the user has been through it.
func App(cx *core.Scope, cfg session.Config) core.Node {
	theme.Apply(cx)
	colors := ui.ColorsOf(cx)
	s := session.New(cx, cfg)
	outer := base.NewNav(cx, route{page: rootPage})
	navs := [tabs]*base.Nav[route]{base.NewNav(cx, route{page: filmsPage}), base.NewNav(cx, route{page: learnPage}), base.NewNav(cx, route{page: settingsPage})}
	selected := core.NewSignal(cx, filmsTab)
	openPair := func() { outer.Push(route{page: pairPage}) }
	// Paired, the phone shows the Library, where the computer's libraries are, from its first page.
	paired := func() {
		selected.Set(filmsTab)
		navs[filmsTab].PopToRoot()
		outer.Pop(1)
	}
	play := func(key string) { outer.Push(route{page: playerPage, key: key}) }
	tabStack := func(n *base.Nav[route]) func(cx *core.Scope) core.Node {
		return func(cx *core.Scope) core.Node {
			look.InTab(cx, shell.OverTabBar(cx))
			return n.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
				back := func() { n.Pop(1) }
				openFilm := func(desktop string, lib sync.Library, f sync.Film) {
					n.Push(route{page: filmPage, desktop: desktop, library: lib, film: f})
				}
				switch r.page {
				case learnPage:
					return learn.Page(cx)
				case settingsPage:
					return settings.Page(cx, s, settings.Actions{Pair: openPair,
						Target:       func() { n.Push(route{page: targetPage}) },
						Introduction: func() { outer.Push(route{page: introductionPage}) }})
				case targetPage:
					return settings.TargetPage(cx, s, back)
				case libraryPage:
					return library.Page(cx, s, r.desktop, r.library, func(f sync.Film) { openFilm(r.desktop, r.library, f) }, back)
				case filmPage:
					return film.Page(cx, s, r.desktop, r.library.Name, r.film, play, back)
				}
				return films.Page(cx, s, films.Actions{Pair: openPair,
					Film: func(f store.Film) { openFilm(f.Desktop, sync.Library{Name: f.Library}, f.Film) },
					Library: func(desktop string, lib sync.Library) {
						n.Push(route{page: libraryPage, desktop: desktop, library: lib})
					}})
			})
		}
	}
	change := func(i int) {
		// The tab shown, pressed again, goes back to its first page.
		if i == selected.Get() {
			navs[i].PopToRoot()
			return
		}
		selected.Set(i)
	}
	sh := func(cx *core.Scope) core.Node {
		return shell.Shell{Selected: selected.Get, OnChange: change,
			Footer: settings.Computers(cx, s, func() { change(settingsTab) }),
			Tabs: []shell.Tab{
				{Text: func() string { return locales.FilmsTitle(cx) }, Icon: lucide.SquarePlay, SystemIcon: "play.rectangle.on.rectangle", TestID: "tab-films", Content: tabStack(navs[filmsTab])},
				{Text: func() string { return locales.LearnTitle(cx) }, Icon: lucide.GraduationCap, SystemIcon: "rectangle.stack", TestID: "tab-learn", Content: tabStack(navs[learnTab])},
				{Text: func() string { return locales.SettingsTitle(cx) }, Icon: lucide.Settings, SystemIcon: "gearshape", TestID: "tab-settings", Content: tabStack(navs[settingsTab])},
			}}
	}
	// Before the store is read the page stays blank: reading it is quick.
	root := core.Show{When: s.Loaded, Then: func(cx *core.Scope) core.Node {
		return core.Show{When: func() bool { return s.Prefs().Onboarded },
			Then: sh,
			Else: func(cx *core.Scope) core.Node { return onboarding.Page(cx, s, func() {}) }}
	}}
	return outer.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		switch r.page {
		case pairPage:
			return pair.Page(cx, s, paired, func() { outer.Pop(1) })
		case introductionPage:
			return base.NavPage{TestID: "introduction", HeaderHidden: true, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
				onboarding.Page(cx, s, func() { outer.Pop(1) }),
			}}
		case playerPage:
			return player.Page(cx, s, r.key, func() { outer.Pop(1) })
		}
		return base.NavPage{TestID: "root", HeaderHidden: true, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{root}}
	})
}
