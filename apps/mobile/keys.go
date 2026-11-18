package main

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/core"
)

// shellKeys binds the shell's shortcuts on a hardware keyboard (shortcuts): ⌘1 and on show the sections, of which there are sections, through change, as a press on their tab does; Add opens the Add sheet with add, and Search goes to the Library's search with search. They rest while the player, pairing or a sheet covers the shell.
func shellKeys(cx *core.Scope, sections int, change func(int), add, search func()) {
	for i := range sections {
		cx.OnKey(shortcuts.Section(i), func() { change(i) })
	}
	cx.OnKey(shortcuts.Add, add)
	cx.OnKey(shortcuts.Search, search)
}

// sectionNames are the names of the tabs shown, in their order, as Settings › Keyboard Shortcuts lists the sections ⌘1 and on show.
func sectionNames(cx *core.Scope, shown []int) []func() string {
	names := [tabs]func() string{
		filmsTab:    func() string { return locales.FilmsTitle(cx) },
		learnTab:    func() string { return locales.LearnTitle(cx) },
		jobsTab:     func() string { return locales.JobsTitle(cx) },
		settingsTab: func() string { return locales.SettingsTitle(cx) },
	}
	out := make([]func() string, len(shown))
	for i, t := range shown {
		out[i] = names[t]
	}
	return out
}
