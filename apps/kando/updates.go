package main

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// Kando updates itself from the signed releases of yldm-tech/Kando (updates in mygo.json), through MyGo's updater plugin: it checks in the background 10 seconds after launch and then once a day, and its window shows the release notes and offers to install the new version, skip it or remind the user later. The window is native UI, as Kando shows no web page, and speaks the interface's language, as Settings › 通用 sets App.Locale. Builds that cannot update themselves, such as development builds and the Debian package, never check in the background, and the window says why when the user checks.

// useUpdater registers the updater plugin. main calls it before App.Run.
func useUpdater() {
	mygo.Use(native.Plugin)
}

// updatesKeys is the app menu's 检查更新… row, which checks at once and shows the window whatever it finds, a version the user skipped included. It has no key, so on Linux and Windows, which have no menu bar, the command palette is the way in.
func (a *app) updatesKeys() []shortcut {
	return []shortcut{
		{Group: groupApp, Label: locales.UpdatesCheck(), Run: updater.CheckForUpdates},
	}
}
