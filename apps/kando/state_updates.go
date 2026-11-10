package main

import (
	"kando/apps/kando/settings"

	"github.com/egoist/mygo/plugins/updater"
)

// updatesUI is how Kando updates itself, as Settings › 通用 shows it. The app reads it from MyGo's updater plugin (updates.go) once the window is ready, and again whenever the updater says it changed, so the page draws it without asking the updater, which reads its file the first time.
type updatesUI struct {
	shown settings.Updates
	// The updater plugin's functions, which only main uses the plugin for; tests replace them.
	read         func() settings.Updates
	check        func()
	setChecks    func(on bool)
	setDownloads func(on bool)
	follow       func(fn func())
}

func newUpdatesUI() updatesUI {
	return updatesUI{
		read:         updaterState,
		check:        updater.CheckForUpdates,
		setChecks:    updater.SetAutomaticChecks,
		setDownloads: updater.SetAutomaticDownloads,
		follow:       func(fn func()) { updater.OnChange(fn) },
	}
}

// updatesHooks: once the window is ready, the app reads the updater's state and follows its changes, after a check or an answer in the update window as well as after the switches of Settings › 通用.
func (a *app) updatesHooks() areaHooks {
	return areaHooks{ready: a.updatesFollow}
}

// updatesFollow reads the updater's state now and every time it changes.
func (a *app) updatesFollow() {
	a.updates.shown = a.updates.read()
	a.updates.follow(func() {
		a.update(func() { a.updates.shown = a.updates.read() })
	})
}
