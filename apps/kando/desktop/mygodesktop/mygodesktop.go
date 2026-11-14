// Package mygodesktop is the desktop package's MyGo backend, through MyGo's Shell and Power, for the root package while it still runs on MyGo: a package of its own, so the areas ported to NSGo that import desktop link no MyGo. The root package's last step deletes it.
package mygodesktop

import "github.com/egoist/mygo"

// The functions need the running MyGo app, so tests must not call them.

// OpenURL opens url with the system's default app for it: a web page in the browser, or a pane of System Settings for an x-apple.systempreferences: URL.
func OpenURL(url string) error { return mygo.Shell.OpenExternal(url) }

// Reveal shows path selected in the file manager: Finder selecting the item on macOS, File Explorer selecting it on Windows, and on Linux the file manager's org.freedesktop.FileManager1 ShowItems, which selects it too, falling back to opening its folder when no file manager answers. It reports no failure, as each system's reveal either shows the item or falls back on its own.
func Reveal(path string) { mygo.Shell.ShowItemInFolder(path) }

// Trash moves a file or a folder to the Trash, the Recycle Bin on Windows. It fails, and leaves the item where it is, when the item cannot go there and would be deleted for good instead, as on a network share on Windows.
func Trash(path string) error { return mygo.Shell.TrashItem(path) }

// KeepAwake keeps the computer from sleeping, and with display set the display from turning off too, until release is called; calling release again does nothing. The reason shows where the system lists what keeps it awake.
func KeepAwake(reason string, display bool) (release func()) {
	return mygo.Power.KeepAwake(reason, display)
}
