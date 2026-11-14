package desktop

import "github.com/egoist/mygo"

// The functions ending in MyGo are the package's MyGo backend, through MyGo's Shell and Power, for the root package while it still runs on MyGo. They need the running MyGo app, so tests must not call them.

// OpenURLMyGo opens url with the system's default app for it: a web page in the browser, or a pane of System Settings for an x-apple.systempreferences: URL.
func OpenURLMyGo(url string) error { return mygo.Shell.OpenExternal(url) }

// RevealMyGo shows path selected in the file manager: Finder selecting the item on macOS, File Explorer selecting it on Windows, and on Linux the file manager's org.freedesktop.FileManager1 ShowItems, which selects it too, falling back to opening its folder when no file manager answers. It reports no failure, as each system's reveal either shows the item or falls back on its own.
func RevealMyGo(path string) { mygo.Shell.ShowItemInFolder(path) }

// TrashMyGo moves a file or a folder to the Trash, the Recycle Bin on Windows. It fails, and leaves the item where it is, when the item cannot go there and would be deleted for good instead, as on a network share on Windows.
func TrashMyGo(path string) error { return mygo.Shell.TrashItem(path) }

// KeepAwakeMyGo keeps the computer from sleeping, and with display set the display from turning off too, until release is called; calling release again does nothing. The reason shows where the system lists what keeps it awake.
func KeepAwakeMyGo(reason string, display bool) (release func()) {
	return mygo.Power.KeepAwake(reason, display)
}
