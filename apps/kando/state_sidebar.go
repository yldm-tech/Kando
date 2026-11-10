package main

import "kando/apps/kando/sidebar"

// sidebarUI is the state of the sidebar.
type sidebarUI struct {
	// filter is the source the sidebar shows: sidebar.SourceAll or one of the platforms of core.ParseLink.
	filter string
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
	return sidebarUI{filter: sidebar.SourceAll}
}

// savedSidebar is the sidebar's part of settings.json.
type savedSidebar struct{}

func (s *savedSidebar) clean() {}

// sidebarHooks keeps the status line and the switcher's counts up to date: both are read once the window exists, the status line again when the tools or the AI service change or 测试 reaches the model through the EveryAPI CLI's key, and the counts again whenever another library opens.
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
	}
}

// sidebarKeys are the sidebar's shortcuts.
func (a *app) sidebarKeys() []shortcut { return nil }

// sidebarSearchable reports whether ⌘F goes to the sidebar's search field now: no film is open, as the player takes ⌘F while one is, and the open library has films to search, as sidebar.Show draws the field only then.
func (a *app) sidebarSearchable() bool { return a.openKey == "" && a.FilmCount() > 0 }

// sidebarSearch is ⌘F on the library page (libraryKeys): the keyboard in the sidebar's search field.
func (a *app) sidebarSearch() { a.side.queryFocus = true }
