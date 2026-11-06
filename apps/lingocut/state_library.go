package main

import "lingocut/apps/lingocut/sidebar"

// libraryUI is the state of the library page that is not the shelf itself (the shelf is the nodra session in state.go).
type libraryUI struct {
	// filter is the source the sidebar shows: sidebar.SourceAll or one of the platforms of core.ParseLink.
	filter string
}

func newLibraryUI() libraryUI {
	return libraryUI{filter: sidebar.SourceAll}
}
