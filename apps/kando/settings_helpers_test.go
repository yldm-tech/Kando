package main

import "github.com/nsgo-dev/nsgo/nsgotest"

// settingsOpenAt opens the settings at section, one of the settings package's Section values, through app.OpenSettings, and returns the app the test clicks and asserts on there. Tests reach a settings page through it rather than through the main window, so only the settings area decides how the settings open and where they are shown. nsgotest's host opens no windows of the app's, so the settings show over the main window (settingsOverlay) and the app returned is tt itself; under `nsgo test -target macos` too, until the test host opens windows.
func settingsOpenAt(a *app, tt *nsgotest.App, section string) *nsgotest.App {
	a.OpenSettings(section)
	refresh(tt, a)
	return tt
}
