package main

import (
	"slices"

	"kando/apps/mobile/add"
	"kando/apps/mobile/browse"
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/film"
	"kando/apps/mobile/films"
	"kando/apps/mobile/jobs"
	"kando/apps/mobile/learn"
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
	exportPage
	conflictsPage
	shortcutsPage
	widgetsPage
	browsePage
)

// The tabs, in the order the tab bar shows them. Jobs is a section of its own on a tablet only; on a phone it opens from the Library. Browse is left out of store builds (browse.Available).
const (
	filmsTab = iota
	browseTab
	learnTab
	jobsTab
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
	if widget.Available() {
		widgets.Keep(cx, s, widget.Update)
	}
	outer := base.NewNav(cx, route{page: rootPage})
	navs := [tabs]*base.Nav[route]{base.NewNav(cx, route{page: filmsPage}), base.NewNav(cx, route{page: browsePage}), base.NewNav(cx, route{page: learnPage}), base.NewNav(cx, route{page: jobsPage}),
		base.NewNav(cx, route{page: settingsPage})}
	selected := core.NewSignal(cx, filmsTab)
	// shown are the tabs the shell shows, in order: the shell knows a tab by its place among them.
	shown := shownTabs(look.Tablet(cx))
	adding := core.NewSignal(cx, false)
	openAdd := func() { adding.Set(true) }
	// searches counts the times the user asked for the Library's search from the keyboard (shortcuts.Search), which the Library's page focuses each time.
	searches := core.NewSignal(cx, 0)
	// openJobs shows the Jobs page: a tablet's section, or a page of the Library's stack on a phone, which goes to the Library.
	openJobs := func() {
		if look.Tablet(cx) {
			selected.Set(jobsTab)
			return
		}
		selected.Set(filmsTab)
		if routes := navs[filmsTab].Routes(); routes[len(routes)-1].page != jobsPage {
			navs[filmsTab].Push(route{page: jobsPage})
		}
	}
	toaster := ui.NewToaster(cx)
	queueToasts(cx, s, toaster, func(desktop string, lib sync.Library, f sync.Film) {
		selected.Set(filmsTab)
		navs[filmsTab].Push(route{page: filmPage, desktop: desktop, library: lib, film: f})
	})
	openPair := func() { outer.Push(route{page: pairPage}) }
	// Paired, the phone shows the Library, where the computer's libraries are, from its first page.
	paired := func() {
		selected.Set(filmsTab)
		navs[filmsTab].PopToRoot()
		outer.Pop(1)
	}
	// Unpaired, a computer's films not on the phone go from the Library's stack, and from a tablet's Jobs, with the pages opened from them: with no client left they could not be downloaded. A film's page of one on the phone stays. Its page in Settings goes.
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
		// The computer's page in Settings goes too.
		routes := navs[settingsTab].Routes()
		if i := slices.IndexFunc(routes, func(r route) bool { return r.page == computerPage && r.desktop == id }); i >= 0 {
			navs[settingsTab].Pop(len(routes) - i)
		}
	}
	// The film playing shows over the tabs, or in the mini player over them.
	layer := player.NewLayer(cx, s)
	play := layer.Play
	// A film's page opens the player held where the film was left, with the recap of the part watched, as a poster does on the computer; Continue Watching and a line found play at once.
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
						Review:     func() { n.Push(route{page: reviewPage}) },
						Vocabulary: func() { n.Push(route{page: vocabularyPage}) },
						Lines:      func() { n.Push(route{page: linesPage}) },
						Export:     func() { n.Push(route{page: exportPage}) }})
				case reviewPage:
					return learn.ReviewPage(cx, s, back)
				case vocabularyPage:
					return learn.VocabularyPage(cx, s, back)
				case linesPage:
					return learn.LinesPage(cx, s, back)
				case exportPage:
					return learn.ExportPage(cx, s, back)
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
						Computer: func(id string) {
							n.Push(route{page: computerPage, desktop: id})
							s.SyncLearning()
						},
						Unpaired: unpaired})
				case computerPage:
					return settings.ComputerPage(cx, s, r.desktop, unpaired, back)
				case shortcutsPage:
					return settings.ShortcutsPage(cx, sectionNames(cx, shown), back)
				case widgetsPage:
					return settings.WidgetsPage(cx, s, back)
				case targetPage:
					return settings.TargetPage(cx, s, back)
				case conflictsPage:
					return settings.ConflictsPage(cx, s, back)
				case filmPage:
					return film.Page(cx, s, r.desktop, r.library.Name, r.film, playHeld, back)
				case browsePage:
					return browse.Page(cx, s, browse.Actions{Pair: openPair, Shown: func() bool { return selected.Get() == browseTab && outer.Depth() == 0 && !layer.Full() }})
				case jobsPage:
					act := jobs.Actions{Add: openAdd, Pair: openPair, Film: openFilm, Back: back}
					if depth() == 0 {
						act.Back = nil
					}
					return jobs.Page(cx, s, act)
				}
				return films.Page(cx, s, films.Actions{Pair: openPair, Play: play, PlayAt: layer.PlayAt, Add: openAdd, Jobs: openJobs, Searches: searches.Get,
					Film: func(e catalog.Entry) { openFilm(e.Desktop, e.Library, e.Film) }})
			})
			return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{stack, player.MiniBar(cx, layer)}}
		}
	}
	change := func(i int) {
		i = shown[i]
		// The tab shown, pressed again, goes back to its first page.
		if i == selected.Get() {
			navs[i].PopToRoot()
			return
		}
		selected.Set(i)
	}
	sh := func(cx *core.Scope) core.Node {
		all := [tabs]shell.Tab{
			filmsTab:  {Text: func() string { return locales.FilmsTitle(cx) }, Icon: lucide.SquarePlay, SystemIcon: "play.rectangle.on.rectangle", TestID: "tab-films", Content: tabStack(navs[filmsTab])},
			browseTab: {Text: func() string { return locales.BrowseTitle(cx) }, Icon: lucide.Globe, SystemIcon: "globe", TestID: "tab-browse", Content: tabStack(navs[browseTab])},
			learnTab:  {Text: func() string { return locales.LearnTitle(cx) }, Icon: lucide.GraduationCap, SystemIcon: "rectangle.stack", TestID: "tab-learn", Content: tabStack(navs[learnTab])},
			jobsTab: {Text: func() string { return locales.JobsTitle(cx) }, Icon: lucide.ListChecks, SystemIcon: "checklist", TestID: "tab-jobs", Content: tabStack(navs[jobsTab]),
				Badge:     func() int { active, failed := s.Unfinished(); return active + failed },
				BadgeWarn: func() bool { _, failed := s.Unfinished(); return failed > 0 }},
			settingsTab: {Text: func() string { return locales.SettingsTitle(cx) }, Icon: lucide.Settings, SystemIcon: "gearshape", TestID: "tab-settings", Content: tabStack(navs[settingsTab])},
		}
		list := make([]shell.Tab, len(shown))
		for i, t := range shown {
			list[i] = all[t]
		}
		shellKeys(cx, len(shown), layer.Full, change, openAdd, func() {
			selected.Set(filmsTab)
			navs[filmsTab].PopToRoot()
			searches.Update(func(n int) int { return n + 1 })
		})
		return shell.Shell{Selected: func() int { return slices.Index(shown, selected.Get()) }, OnChange: change,
			Footer: settings.Computers(cx, s, func() { change(slices.Index(shown, settingsTab)) }),
			Tabs:   list}
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
			ui.Toasts{Toaster: toaster, TestID: "toasts"},
			layer.Node(cx),
		}}
	})
}

// shownTabs are the tabs the shell shows, in order, on a tablet or a phone: Browse where the build has it, and Jobs on a tablet.
func shownTabs(tablet bool) []int {
	shown := []int{filmsTab}
	if browse.Available {
		shown = append(shown, browseTab)
	}
	shown = append(shown, learnTab)
	if tablet {
		shown = append(shown, jobsTab)
	}
	return append(shown, settingsTab)
}
