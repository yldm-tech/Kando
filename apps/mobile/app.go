package main

import (
	"slices"

	"kando/apps/mobile/add"
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/film"
	"kando/apps/mobile/films"
	"kando/apps/mobile/jobs"
	"kando/apps/mobile/learn"
	"kando/apps/mobile/learning"
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
	"kando/apps/mobile/widgets"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/nsgo-dev/nsgo/widget"
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
	filmPage
	reviewPage
	vocabularyPage
	linesPage
	stylePage
	goalPage
	revokedPage
	jobsPage
	computerPage
	conflictsPage
	shortcutsPage
	widgetsPage
	languagePage
	offlinePage
)

// The tabs, in the order the tab bar and a tablet's sidebar show them.
const (
	filmsTab = iota
	jobsTab
	learnTab
	settingsTab
	tabs
)

// route is a page of a stack and what it shows: the computer and library a film's page is of, and the film.
type route struct {
	page    page
	desktop string
	library sync.Library
	film    sync.Film
}

// App is the app with the session cfg configures: the tabs of the Library, Learn and Settings, each a stack of the pages it opens, under a stack of what covers them, pairing, the introduction and the page that says a computer no longer takes the phone; the film playing over the tabs, or minimized in a bar over them; the introduction in the tabs' place until the user has been through it.
func App(cx *core.Scope, cfg session.Config) core.Node {
	theme.Apply(cx)
	colors := ui.ColorsOf(cx)
	s := session.New(cx, cfg)
	// The app's look is the one the user chose in Settings, or the system's.
	cx.Effect(func() {
		switch s.Prefs().Appearance {
		case store.AppearanceLight:
			cx.SetAppearance(core.AppearanceLight)
		case store.AppearanceDark:
			cx.SetAppearance(core.AppearanceDark)
		default:
			cx.SetAppearance(core.AppearanceSystem)
		}
	})
	// The interface's language is the one the user chose in Settings, or the system's ("" follows it again).
	cx.Effect(func() { cx.SetLocale(s.Prefs().Language) })
	if widget.Available() {
		widgets.Keep(cx, s, widget.Update)
	}
	outer := base.NewNav(cx, route{page: rootPage})
	navs := [tabs]*base.Nav[route]{base.NewNav(cx, route{page: filmsPage}), base.NewNav(cx, route{page: jobsPage}), base.NewNav(cx, route{page: learnPage}),
		base.NewNav(cx, route{page: settingsPage})}
	selected := core.NewSignal(cx, filmsTab)
	adding := core.NewSignal(cx, false)
	openAdd := func() { adding.Set(true) }
	// subscription is the subscription whose sheet shows over the app, by its key, "" for none: the Library's row and a tablet's sidebar open it.
	subscription := core.NewSignal(cx, "")
	// searches counts the times the user asked for the Library's search from the keyboard (shortcuts.Search), which the Library's page focuses each time.
	searches := core.NewSignal(cx, 0)
	// library is the library the Library shows, none for all, which a tablet's sidebar names and picks too.
	library := core.NewSignal(cx, catalog.Scope{})
	// picking is set while the sheet that picks it shows, which the Library's switcher and a tablet's sidebar open.
	picking := core.NewSignal(cx, false)
	pickLibrary := func() { picking.Set(true) }
	// openJobs shows the Jobs tab.
	openJobs := func() { selected.Set(jobsTab) }
	toaster := ui.NewToaster(cx)
	queueToasts(cx, s, toaster, func(desktop string, lib sync.Library, f sync.Film) {
		selected.Set(filmsTab)
		navs[filmsTab].Push(route{page: filmPage, desktop: desktop, library: lib, film: f})
	})
	openPair := func() { outer.Push(route{page: pairPage}) }
	// openSettings shows Settings at its first page.
	openSettings := func() {
		selected.Set(settingsTab)
		navs[settingsTab].PopToRoot()
	}
	// Paired, the phone shows the Library, where the computer's libraries are, from its first page.
	paired := func() {
		selected.Set(filmsTab)
		navs[filmsTab].PopToRoot()
		outer.Pop(1)
	}
	// Unpaired, a computer's films not on the phone go from the Library's stack, and from Jobs', with the pages opened from them: with no client left they could not be downloaded. A film's page of one on the phone stays. The computer page in Settings goes with the last computer.
	unpaired := func(id string) {
		for _, n := range []*base.Nav[route]{navs[filmsTab], navs[jobsTab]} {
			routes := n.Routes()
			if i := slices.IndexFunc(routes, func(r route) bool {
				_, kept := s.Film(store.Key(r.desktop, r.film.ID))
				return r.page == filmPage && r.desktop == id && !kept
			}); i >= 0 {
				n.Pop(len(routes) - i)
			}
		}
		// The computer's page in Settings shows whichever computer processes new links, so it goes once no computer is left to show.
		if len(s.Desktops()) > 0 {
			return
		}
		routes := navs[settingsTab].Routes()
		if i := slices.IndexFunc(routes, func(r route) bool { return r.page == computerPage }); i >= 0 {
			navs[settingsTab].Pop(len(routes) - i)
		}
	}
	// The film playing shows over the tabs, or in the mini player over them.
	layer := player.NewLayer(cx, s)
	play := layer.Play
	// A film's page and a film's row in the Library open the player held where the film was left, with the recap of the part watched, as a poster does on the computer; Continue Watching and a line found play at once.
	playHeld := layer.PlayHeld
	// A computer that no longer takes the phone shows the page that says so over everything, one computer at a time; revoked is the one it shows.
	revoked := ""
	cx.Effect(func() {
		ids := s.Revoked()
		if len(ids) == 0 || revoked != "" {
			return
		}
		revoked = ids[0]
		id := revoked
		cx.Post(func() { outer.Push(route{page: revokedPage, desktop: id}) })
	})
	tabStack := func(n *base.Nav[route]) func(cx *core.Scope) core.Node {
		return func(cx *core.Scope) core.Node {
			look.InTab(cx, shell.OverTabBar(cx))
			look.ProvideMini(cx, layer.Room)
			stack := n.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, depth func() int) base.NavPage {
				back := func() { n.Pop(1) }
				openFilm := func(desktop string, lib sync.Library, f sync.Film) {
					n.Push(route{page: filmPage, desktop: desktop, library: lib, film: f})
				}
				switch r.page {
				case learnPage:
					return learn.Page(cx, s, learn.Actions{Pair: openPair,
						// The review covers the whole app, tab bar and all.
						Review:     func() { outer.Push(route{page: reviewPage}) },
						Vocabulary: func() { n.Push(route{page: vocabularyPage}) },
						Lines:      func() { n.Push(route{page: linesPage}) },
						Settings:   openSettings,
						Play:       layer.PlayAt})
				case vocabularyPage:
					return learn.VocabularyPage(cx, s, back)
				case linesPage:
					return learn.LinesPage(cx, s, back)
				case stylePage:
					return settings.StylePage(cx, s, back)
				case goalPage:
					return settings.GoalPage(cx, s, back)
				case settingsPage:
					return settings.Page(cx, s, settings.Actions{Pair: openPair,
						Target: func() { n.Push(route{page: targetPage}) },
						Style:  func() { n.Push(route{page: stylePage}) },
						Goal:   func() { n.Push(route{page: goalPage}) },
						Conflicts: func() {
							s.OpenConflicts()
							n.Push(route{page: conflictsPage})
						},
						Introduction: func() { outer.Push(route{page: introductionPage}) },
						Shortcuts:    func() { n.Push(route{page: shortcutsPage}) },
						Widgets:      func() { n.Push(route{page: widgetsPage}) },
						Language:     func() { n.Push(route{page: languagePage}) },
						Offline:      func() { n.Push(route{page: offlinePage}) },
						Computer: func(id string) {
							n.Push(route{page: computerPage, desktop: id})
							s.SyncLearning()
						},
						Unpaired: unpaired})
				case computerPage:
					return settings.ComputerPage(cx, s, openPair, unpaired, back)
				case languagePage:
					return settings.LanguagePage(cx, s, back)
				case offlinePage:
					return settings.OfflinePage(cx, s, back)
				case shortcutsPage:
					return settings.ShortcutsPage(cx, sectionNames(cx), back)
				case widgetsPage:
					return settings.WidgetsPage(cx, s, back)
				case targetPage:
					return settings.TargetPage(cx, s, back)
				case conflictsPage:
					return settings.ConflictsPage(cx, s, back)
				case filmPage:
					return film.Page(cx, s, r.desktop, r.library.Name, r.film, playHeld, back)
				case jobsPage:
					act := jobs.Actions{Add: openAdd, Pair: openPair, Film: openFilm, Back: back}
					if depth() == 0 {
						act.Back = nil
					}
					return jobs.Page(cx, s, act)
				}
				// Where the tab bar is the system's, which shows no count, the Library's header carries the Jobs tab's.
				var jobsButton func()
				if shell.SystemBar(cx) {
					jobsButton = openJobs
				}
				return films.Page(cx, s, films.Actions{Pair: openPair, Play: play, PlayHeld: playHeld, PlayAt: layer.PlayAt, Add: openAdd, Jobs: jobsButton, OpenJobs: openJobs, PickLibrary: pickLibrary, Searches: searches.Get, Library: library,
					Subscription: subscription.Set, Film: func(e catalog.Entry) { openFilm(e.Desktop, e.Library, e.Film) }})
			})
			return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{stack, player.MiniBar(cx, layer)}}
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
	// unfinished is the jobs the computers have yet to finish, and failed those of them that need the user: the Jobs tab's badge, in the badges' orange while any failed.
	unfinished := func() int { active, failed := s.Unfinished(); return active + failed }
	failed := func() bool { _, failed := s.Unfinished(); return failed > 0 }
	// due is how many saved words and lines are due for review: the Learn tab's badge. The session's clock is read again as the items change, as the Learn page reads it.
	items := core.NewMemoFunc(cx, s.LearnItems, nil)
	due := core.NewMemo(cx, func() int { return len(learning.Due(items.Get(), s.Now())) })
	sh := func(cx *core.Scope) core.Node {
		list := []shell.Tab{
			filmsTab: {Text: func() string { return locales.FilmsTitle(cx) }, Icon: lucide.SquarePlay, SystemIcon: "play.rectangle", TestID: "tab-films", Content: tabStack(navs[filmsTab])},
			jobsTab: {Text: func() string { return locales.JobsTitle(cx) }, Icon: lucide.Download, SystemIcon: "arrow.down.circle", TestID: "tab-jobs", Content: tabStack(navs[jobsTab]),
				Badge: unfinished, BadgeWarn: failed},
			learnTab: {Text: func() string { return locales.LearnTitle(cx) }, Icon: learn.Icon, SystemIcon: "rectangle.stack", TestID: "tab-learn", Content: tabStack(navs[learnTab]),
				Badge: due.Get},
			settingsTab: {Text: func() string { return locales.SettingsTitle(cx) }, Icon: lucide.Sun, SystemIcon: "gearshape", TestID: "tab-settings", Content: tabStack(navs[settingsTab])},
		}
		shellKeys(cx, len(list), layer.Full, change, openAdd, func() {
			selected.Set(filmsTab)
			navs[filmsTab].PopToRoot()
			searches.Update(func(n int) int { return n + 1 })
		})
		return shell.Shell{Selected: selected.Get, OnChange: change,
			Library:   func() string { return films.LibraryName(cx, library) },
			OnLibrary: pickLibrary,
			Aside:     films.Sidebar(cx, s, subscription.Set, openAdd),
			Footer:    settings.Computers(cx, s, openJobs),
			Tabs:      list}
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
		case reviewPage:
			return learn.ReviewPage(cx, s, func() { outer.Pop(1) })
		case revokedPage:
			return settings.RevokedPage(cx, s, r.desktop, openPair, unpaired, func() {
				revoked = ""
				outer.Pop(1)
			})
		}
		return base.NavPage{TestID: "root", HeaderHidden: true, StyleFunc: ui.Filled(colors.Background), StatusBarFunc: layer.StatusBar, Children: []core.Node{
			// Under the player, the tabs are out of assistive technology's reach.
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, AccessibilityElementsHidden: layer.Full, Children: []core.Node{root}},
			add.Sheet(cx, s, adding.Get, func() { adding.Set(false) }, openJobs),
			films.SubscriptionSheet(cx, s, subscription),
			films.LibrarySheet(cx, s, library, picking),
			ui.Toasts{Toaster: toaster, TestID: "toasts"},
			layer.Node(cx),
		}}
	})
}
