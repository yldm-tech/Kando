package main

import "lingocut/apps/lingocut/sidebar"

// sidebarUI is the state of the sidebar.
type sidebarUI struct {
	// filter is the source the sidebar shows: sidebar.SourceAll or one of the platforms of core.ParseLink.
	filter string
}

func newSidebarUI() sidebarUI {
	return sidebarUI{filter: sidebar.SourceAll}
}

// savedSidebar is the sidebar's part of settings.json.
type savedSidebar struct{}

func (a *app) sidebarHooks() areaHooks { return areaHooks{} }

// sidebarKeys are the sidebar's shortcuts.
func (a *app) sidebarKeys() []shortcut { return nil }
