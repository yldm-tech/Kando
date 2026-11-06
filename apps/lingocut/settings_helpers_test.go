package main

import "github.com/egoist/mygo/ui"

// settingsOpenAt opens the settings at section, one of the settings package's Section values, through app.OpenSettings, draws a frame and returns the Tester that shows them, for the test to click and assert on. Tests reach a settings page through it rather than by clicking the sidebar, so only the settings area decides how the settings open and where they are drawn. While the settings are sidebar pages they show in the main window, so that is tt, the main window's Tester; once they have a window of their own this returns a Tester on that window and tt keeps showing the main window.
func settingsOpenAt(a *app, tt *ui.Tester, section string) *ui.Tester {
	a.OpenSettings(section)
	tt.Frame()
	return tt
}
