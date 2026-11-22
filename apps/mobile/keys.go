package main

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/shortcuts"

	"github.com/nsgo-dev/nsgo/core"
)

// shellKeys binds the shell's shortcuts on a hardware keyboard (shortcuts): ⌘1 and on show the sections, named sections in their order, through change, as a press on their tab does; Add opens the Add sheet with add, and Search goes to the Library's search with search. They rest while pairing or a sheet covers the shell, and while covered, as the player over the app, whose view is not a page over the shell's. Each has its name in the app's language under Go, which iPadOS lists while ⌘ is held and in its menu bar, while it would act.
func shellKeys(cx *core.Scope, sections []func() string, covered func() bool, change func(int), add, search func()) {
	group := func() string { return locales.ShortcutsGroupGo(cx) }
	bind := func(key string, title func() string, fn func()) {
		cx.OnKeyWith(key, core.KeyOptions{Group: group,
			Title: func() string {
				if covered() {
					return ""
				}
				return title()
			}}, func() {
			if !covered() {
				fn()
			}
		})
	}
	for i, name := range sections {
		bind(shortcuts.Section(i), name, func() { change(i) })
	}
	bind(shortcuts.Search, func() string { return locales.ShortcutsSearch(cx) }, search)
	bind(shortcuts.Add, func() string { return locales.ShortcutsAdd(cx) }, add)
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
