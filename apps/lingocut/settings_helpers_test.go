package main

import "github.com/egoist/mygo/ui"

// settingsOpenAt opens the settings at section, one of the settings package's Section values, through app.OpenSettings and returns a Tester on the settings window, drawn once, for the test to click and assert on. Tests reach a settings page through it rather than through the main window, so only the settings area decides how the settings open and where they are drawn. tt, the main window's Tester, is left alone and keeps showing the main window.
func settingsOpenAt(a *app, tt *ui.Tester, section string) *ui.Tester {
	return newSettingsTester(a, section)
}

// newSettingsTester opens the settings at section and draws the settings window's content on a Tester of its own, at the window's size.
func newSettingsTester(a *app, section string) *ui.Tester {
	a.OpenSettings(section)
	tt := ui.NewTester(a.settingsView, 880, 640)
	tt.Frame()
	return tt
}
