package main

import "lingocut/apps/lingocut/sidebar"

// sidebarUI is the state of the sidebar.
type sidebarUI struct {
	// filter is the source the sidebar shows: sidebar.SourceAll or one of the platforms of core.ParseLink.
	filter string
	// ready is what the status line last learned about whether a job can start (readiness.go).
	ready readiness
	// libraryCounts is how many films each known library that is not open holds, by its cleaned path, read off the UI thread for the library switcher's menu (countLibraries).
	libraryCounts map[string]int
}

func newSidebarUI() sidebarUI {
	return sidebarUI{filter: sidebar.SourceAll}
}

// savedSidebar is the sidebar's part of settings.json.
type savedSidebar struct{}

func (s *savedSidebar) clean() {}

// sidebarHooks keeps the status line and the switcher's counts up to date: both are read once the window exists, the status line again when the tools or the AI service change, and the counts again whenever another library opens.
func (a *app) sidebarHooks() areaHooks {
	return areaHooks{
		ready: func() {
			a.checkReadiness(true)
			a.countLibraries()
		},
		toolsChanged:   a.checkReadiness,
		llmChanged:     func() { a.checkReadiness(false) },
		libraryChanged: a.sidebarLibraryChanged,
	}
}

// sidebarKeys are the sidebar's shortcuts.
func (a *app) sidebarKeys() []shortcut { return nil }
