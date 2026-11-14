// Package desktop opens URLs, reveals and trashes files, and keeps the computer awake for Kando. Notifications, the Dock badge and the progress bar are not here: they stay with the queue in queue_desktop.go. Each function takes the scope of the component or handler that calls it and runs on the engine's goroutine, as NSGo's services do; the root package, while it still runs on MyGo, uses the same functions of desktop/mygodesktop.
package desktop

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/core"
)

// OpenURL opens url with the system's default app for it: a web page in the browser, or a pane of System Settings for an x-apple.systempreferences: URL. done, when not nil, learns whether it opened: false for a URL NSGo refuses to open (core.CheckURL) and for one no app handles.
func OpenURL(cx *core.Scope, url string, done func(opened bool)) { cx.OpenURL(url, done) }

// errFolderNotOpened is Reveal's error when the system opened no file manager for the folder.
var errFolderNotOpened = errors.New("no app opened the folder")

// Reveal shows path in the file manager by opening the folder that holds it. done, when not nil, learns why that failed, or nil once the folder is open.
func Reveal(cx *core.Scope, path string, done func(err error)) {
	// TODO(nsgo#117): select path in Finder, File Explorer or the FileManager1 file manager rather than only opening its folder https://github.com/nsgo-dev/nsgo/issues/117
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

// Trash would move a file or a folder to the Trash, the Recycle Bin on Windows. NSGo cannot do that yet, so it fails with an error wrapping errors.ErrUnsupported and leaves the item where it is: it never deletes for good.
func Trash(cx *core.Scope, path string) error {
	// TODO(nsgo#117): move path to the Trash or the Recycle Bin, failing and leaving it where it is when it would be deleted for good, as on a network share on Windows https://github.com/nsgo-dev/nsgo/issues/117
	return fmt.Errorf("moving %s to the Trash: %w", filepath.Base(path), errors.ErrUnsupported)
}

// KeepAwake would keep the computer from sleeping, and with display set the display from turning off too, until release is called; calling release again does nothing. The reason is what the system would list as keeping it awake. NSGo cannot hold the computer awake yet, so the computer may sleep while a job runs or a film plays.
func KeepAwake(cx *core.Scope, reason string, display bool) (release func()) {
	// TODO(nsgo#115): hold a power assertion for reason (the display too when display is set) until release, as IOPMAssertion, SetThreadExecutionState and the login1 or ScreenSaver inhibitors do https://github.com/nsgo-dev/nsgo/issues/115
	return func() {}
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

// DroppedPaths are the files of a drop, as core.View's OnFileDrop reports them as file: URLs, as paths on this system; what is no file: URL is left out.
func DroppedPaths(uris []string) []string {
	var paths []string
	for _, uri := range uris {
		u, err := url.Parse(uri)
		if err != nil || u.Scheme != "file" || u.Path == "" {
			continue
		}
		path := u.Path
		// A Windows path comes as file:///C:/dir/file, its drive after the slash.
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		paths = append(paths, filepath.FromSlash(path))
	}
	return paths
}
