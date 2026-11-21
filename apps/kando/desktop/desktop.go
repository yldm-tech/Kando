// Package desktop opens URLs, reveals and trashes files, and keeps the computer awake for Kando. Notifications, the Dock badge and the progress bar are not here: they stay with the queue in queue_desktop.go. Each function takes the scope of the component or handler that calls it and runs on the engine's goroutine, as NSGo's services do.
package desktop

import (
	"errors"
	"log"
	"net/url"
	"path/filepath"
	"sync/atomic"

	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
)

// OpenURL opens url with the system's default app for it: a web page in the browser, or a pane of System Settings for an x-apple.systempreferences: URL. done, when not nil, learns whether it opened: false for a URL NSGo refuses to open (core.CheckURL) and for one no app handles.
func OpenURL(cx *core.Scope, url string, done func(opened bool)) { cx.OpenURL(url, done) }

// errFolderNotOpened is Reveal's error when the system opened no file manager for the folder.
var errFolderNotOpened = errors.New("no app opened the folder")

// Reveal shows path selected in the file manager (core.Scope.RevealFile): Finder on macOS, File Explorer on Windows, and on Linux the file manager GTK's file launcher opens. Where that cannot be done, as on Linux without such a file manager, it opens the folder that holds path instead. done, when not nil, learns why neither worked, or nil once the file manager shows the item or its folder; it runs on the engine's goroutine, unless cx is gone by then.
func Reveal(cx *core.Scope, path string, done func(err error)) {
	if headless.Load() {
		openFolder(cx, path, done)
		return
	}
	cx.RevealFile(path, func(err error) {
		if err == nil {
			if done != nil {
				done(nil)
			}
			return
		}
		log.Printf("desktop: the file manager did not show %s, opening its folder: %v", filepath.Base(path), err)
		openFolder(cx, path, done)
	})
}

// openFolder opens the folder that holds path in the file manager, Reveal's way when the file manager cannot select path.
func openFolder(cx *core.Scope, path string, done func(err error)) {
	cx.OpenFile(core.SharedFile{Path: filepath.Dir(path)}, func(opened bool, err error) {
		if done == nil {
			return
		}
		if err == nil && !opened {
			err = errFolderNotOpened
		}
		done(err)
	})
}

// ErrNotTrashed is Trash's error when the item could not go to the Trash and is left where it is: its volume has no Trash, it is not there, the system refused, or on Windows the Recycle Bin cannot take it and it would be deleted for good. What the system said is in the log, not in the error, so a caller says it in a sentence of its own.
var ErrNotTrashed = errors.New("the item was not moved to the Trash")

// Trash moves a file or a folder to the Trash, the Recycle Bin on Windows (core.Scope.TrashFile), and calls done on the engine's goroutine once it is there, with nil, or with ErrNotTrashed when the item cannot go there and is left where it is; it never deletes an item for good, as Windows would one the Recycle Bin cannot take. After Headless it fails with errors.ErrUnsupported and leaves the item where it is. What the system said is logged.
func Trash(cx *core.Scope, path string, done func(err error)) {
	if headless.Load() {
		done(errors.ErrUnsupported)
		return
	}
	cx.TrashFile(path, func(_ string, err error) {
		if err != nil {
			log.Printf("desktop: moving %s to the Trash: %v", filepath.Base(path), err)
			err = ErrNotTrashed
		}
		done(err)
	})
}

// headless is set by Headless.
var headless atomic.Bool

// Headless keeps Reveal and Trash off the system's file manager and Trash for the rest of the process, for tests, whose app runs on the machine of whoever runs them: Reveal opens the item's folder through the scope, which nsgotest records rather than opens, and Trash fails with errors.ErrUnsupported and leaves the item where it is.
func Headless() { headless.Store(true) }

// KeepAwake keeps the computer from sleeping, and with display set the display from turning off too, until release is called or cx is gone (core.Scope.KeepAwake); calling release again does nothing. The reason is what the system lists as keeping it awake.
func KeepAwake(cx *core.Scope, reason string, display bool) (release func()) {
	return cx.KeepAwake(reason, core.KeepAwakeOptions{Display: display})
}

// Platform names the desktop the app runs on as the catalogs' select messages take it, to name the system's file manager: "macOS", "Windows" or "Linux", and "other" elsewhere. A binding that reads it follows the environment.
func Platform(cx *core.Scope) string {
	switch cx.Env().OS() {
	case core.OSMacOS:
		return "macOS"
	case core.OSWindows:
		return "Windows"
	case core.OSLinux:
		return "Linux"
	}
	return "other"
}

// DroppedPaths are the files of a drop, as core.View's OnFileDrop reports them as file: URLs, as paths on this system (kcore.FilePath): on Windows file:///C:/dir/file is C:\dir\file, and file://server/share/file, a file of a network folder, \\server\share\file. What is no file: URL is left out.
func DroppedPaths(uris []string) []string {
	var paths []string
	for _, uri := range uris {
		u, err := url.Parse(uri)
		if err != nil {
			continue
		}
		if path, ok := kcore.FilePath(u); ok {
			paths = append(paths, path)
		}
	}
	return paths
}
