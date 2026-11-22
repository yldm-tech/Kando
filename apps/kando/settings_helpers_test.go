package main

import "github.com/nsgo-dev/nsgo/nsgotest"

// settingsOpenAt opens the settings at section, one of the settings package's Section values, through app.OpenSettings, and returns the app the test clicks and asserts on there. Tests reach a settings page through it rather than through the main window, so only the settings area decides how the settings open and where they are shown. nsgotest's host opens the app's windows on a desktop, and its Surface acts in them, but with Press, Type, Key, Find, Query, Screenshot and Close alone, while the settings tests also check what shows (Exists, Texts), clear and focus fields, hover, right-click, drop files and resize the window, so newTestApp has the settings show over the main window (settingsUI.overMain, settingsOverlay) and the app returned is tt itself; under `nsgo test -target macos` too. TestSettingsInTheirOwnWindow tests the settings in their own window.
func settingsOpenAt(a *app, tt *nsgotest.App, section string) *nsgotest.App {
	a.OpenSettings(section)
	refresh(tt, a)
	return tt
}
