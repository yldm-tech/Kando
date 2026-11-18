package main

import (
	"kando/apps/kando/desktop"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// chromeCommands are the commands the macOS menu bar carries as its own items (menu.go's menuBar) where the app has no menu bar, as on Windows and Linux, whose title bar (package chrome) shows none: rows without a key that the command palette adds to the shortcut table's (paletteCommands), so Ctrl+K reaches every command of the menu. They are not rows of the table, which the menu bar is built from, as they are its own items there. 退出看懂 asks first while a job runs (requestQuit), and 关闭窗口 closes the window as its close button does, through windowClosing. The Edit menu's text commands (undo, cut, copy, paste, select all) stay with the text field's own keys, and macOS's About, Hide and Show All have no counterpart there.
func (a *app) chromeCommands(cx *core.Scope) []shortcut {
	if appMenuBar {
		return nil
	}
	maximized := func() bool { return a.window.cx != nil && nsdesktop.Maximized(a.window.cx) }
	return []shortcut{
		{Group: groupApp, Label: locales.MenuQuit(cx), Run: a.requestQuit},
		{Group: groupFile, Label: locales.MenuSwitchLibrary(cx), When: func() bool { return len(a.knownLibraries()) > 0 }, Items: func() []nsdesktop.MenuItem { return a.menuLibraries(cx) }},
		{Group: groupFile, Label: locales.MenuRevealLibrary(cx, desktop.Platform(cx)), When: func() bool { return a.snap().LibraryDir != "" }, Run: func() { a.reveal(a.snap().LibraryDir) }},
		{Group: groupFile, Label: locales.MenuCloseWindow(cx), Run: a.chromeWindow(nsdesktop.CloseWindow)},
		{Group: groupView, Label: locales.MenuToggleFullScreen(cx), Run: a.chromeWindow(nsdesktop.ToggleFullScreen)},
		{Group: groupWindow, Label: locales.MenuMinimize(cx), Run: a.chromeWindow(nsdesktop.MinimizeWindow)},
		{Group: groupWindow, Label: chromeMaximizeLabel(cx, maximized()), Run: a.chromeWindow(nsdesktop.ToggleMaximize)},
	}
}

// chromeMaximizeLabel is the palette's row that maximizes the main window, or takes it back to its size while it is maximized, as Windows names the two.
func chromeMaximizeLabel(cx *core.Scope, maximized bool) string {
	if maximized {
		return locales.ChromeRestore(cx)
	}
	return locales.ChromeMaximize(cx)
}

// chromeWindow is a command that acts on the main window through its view's scope, which windowFollow keeps; before the window was built there is none to act on.
func (a *app) chromeWindow(act func(*core.Scope)) func() {
	return func() {
		if a.window.cx != nil {
			act(a.window.cx)
		}
	}
}
