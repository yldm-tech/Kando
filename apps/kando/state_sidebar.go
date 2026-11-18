package main

import (
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
)

// sidebarUI is the state of the sidebar.
type sidebarUI struct {
	// filter is the source the sidebar shows: sidebar.SourceAll or one of the platforms of core.ParseLink.
	filter string
	// spoken is the language the sidebar shows the films of, sidebar.SpokenAll or whisper's code for one, together with filter; like filter it is not saved, and another library starts without it (SpokenFilter).
	spoken string
	// watch is how far the films the sidebar shows were watched, sidebar.WatchAll or another of its Watch values, together with filter and spoken; like them it is not saved, and another library starts without it. Unlike the language it stays chosen when no film is in that state any longer, as after the last film in progress was marked watched, so the page says there is none rather than showing every film by itself.
	watch string
	// query is what the search field under the switcher holds: the library page shows the done films whose names hold it (films.ByTitle). It is not saved, and another library starts without it, as with filter. queryFocus asks the field for the keyboard focus once, as ⌘F does (sidebarSearch).
	query      string
	queryFocus bool
	// ready is what the status line last learned about whether a job can start (readiness.go).
	ready readiness
	// libraryCounts is how many films each known library that is not open holds, by its cleaned path, read off the UI thread for the library switcher's menu (countLibraries).
	libraryCounts map[string]int
	// counting numbers the countLibraries runs, so the counts of a run that ends after a later one are dropped.
	counting int
}

func newSidebarUI() sidebarUI {
	return sidebarUI{filter: sidebar.SourceAll, spoken: sidebar.SpokenAll, watch: sidebar.WatchAll}
}

// savedSidebar is the sidebar's part of settings.json.
type savedSidebar struct{}

func (s *savedSidebar) clean() {}

// sidebarHooks keeps the status line and the switcher's counts up to date: both are read once the window exists, the status line again when the tools or the AI service change or 测试 reaches the model through the EveryAPI CLI's key, and the counts again whenever another library opens. After every change it lets go of a language chosen that the list no longer offers (sidebarDropLanguage).
func (a *app) sidebarHooks() areaHooks {
	return areaHooks{
		ready: func() {
			a.checkReadiness(true)
			a.countLibraries()
		},
		toolsChanged: a.checkReadiness,
		llmChanged:   func() { a.checkReadiness(false) },
		// Through the EveryAPI CLI's key, a passing 测试 also answers whether the CLI is signed in.
		llmTested: func(err error, credential bool) {
			if err == nil && credential {
				a.signedIn(nil)
			}
		},
		libraryChanged: a.sidebarLibraryChanged,
		afterChange:    a.sidebarDropLanguage,
	}
}

// sidebarDropLanguage lets go of the language chosen in the sidebar once the list no longer offers it (SpokenFilter), as after its last film was removed or the films were left heard in one language, so a film heard in it later does not narrow the library by itself. The page already shows every language then, so nothing a binding reads changes.
func (a *app) sidebarDropLanguage() bool {
	if a.side.spoken != sidebar.SpokenAll && a.SpokenFilter() == sidebar.SpokenAll {
		a.side.spoken = sidebar.SpokenAll
	}
	return false
}

// sidebarKeys are the sidebar's shortcuts.
func (a *app) sidebarKeys(*core.Scope) []shortcut { return nil }

// sidebarSearchable reports whether ⌘F goes to the sidebar's search field now: no film is open, as the player takes ⌘F while one is, and the open library has films to search, as sidebar.View shows the field only then.
func (a *app) sidebarSearchable() bool { return a.openKey == "" && a.FilmCount() > 0 }

// sidebarSearch is ⌘F (libraryKeys): the keyboard in the sidebar's search field. From the browser's page or a subscription's it goes to the library page first, where the films it finds show.
func (a *app) sidebarSearch() {
	if a.page == pageBrowse || a.page == pageSubscription {
		a.Select(pageFilms)
	}
	a.side.queryFocus = true
}
