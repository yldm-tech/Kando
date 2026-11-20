package main

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/core"
)

// shellKeys binds the shell's shortcuts on a hardware keyboard (shortcuts): ⌘1 and on show the sections, of which there are sections, through change, as a press on their tab does; Add opens the Add sheet with add, and Search goes to the Library's search with search. They rest while pairing or a sheet covers the shell, and while covered, as the player over the app, whose view is not a page over the shell's.
func shellKeys(cx *core.Scope, sections int, covered func() bool, change func(int), add, search func()) {
	on := func(fn func()) func() {
		return func() {
			if !covered() {
				fn()
			}
		}
	}
	for i := range sections {
		cx.OnKey(shortcuts.Section(i), on(func() { change(i) }))
	}
	cx.OnKey(shortcuts.Add, on(add))
	cx.OnKey(shortcuts.Search, on(search))
}

// sectionNames are the names of the tabs, in their order, as Settings › Keyboard Shortcuts lists the sections ⌘1 and on show.
func sectionNames(cx *core.Scope) []func() string {
	return []func() string{
		filmsTab:    func() string { return locales.FilmsTitle(cx) },
		jobsTab:     func() string { return locales.JobsTitle(cx) },
		learnTab:    func() string { return locales.LearnTitle(cx) },
		settingsTab: func() string { return locales.SettingsTitle(cx) },
	}
}
