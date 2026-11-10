package main

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
)

// Files the system opens with Kando: a video chosen with Finder's Open With or dropped on the Dock icon (the fileAssociations of mygo.json), or named on the command line, as in `kando ~/Movies/clip.mp4`, which cli.OpensWindow lets through to the window. On Windows and Linux a second launch with a file hands it to the running copy through the single-instance lock. MyGo reports each file to main's mygo.App.OnOpenFile on the main thread, which calls libraryOpenFile.

// libraryOpenFile takes a file the system asked Kando to open, to be added to the queue as a drop on the window adds it (dropFiles). A file that arrives before the window exists waits for it (libraryHooks' ready); the others are added through update, together with those that arrive before that report runs, so the files of one open make one message. A file already waiting here is not taken twice. Main thread only.
func (a *app) libraryOpenFile(path string) {
	if path == "" {
		return
	}
	path = filepath.Clean(path)
	if slices.Contains(a.lib.opened, path) {
		return
	}
	a.lib.opened = append(a.lib.opened, path)
	a.libraryPostOpened()
}

// libraryPostOpened asks update to add the files waiting in opened, unless there are none, they wait for the window, or a report that adds them is already on its way. Going through update also lets every area's ready run first at startup, so the queue has taken back the jobs of the last run before the opened videos join it.
func (a *app) libraryPostOpened() {
	if len(a.lib.opened) == 0 || a.lib.holdOpened || a.lib.openedPosted {
		return
	}
	a.lib.openedPosted = true
	a.update(a.libraryAddOpened)
}

// libraryAddOpened adds the files waiting in opened to the queue and brings the window forward, on the library, whose message bar and queue show what became of them. A video that already has a job waiting or at work in the open library is left out rather than queued again: macOS hands the app a video named on the command line twice, once as AppKit opens the launch's arguments and once as MyGo reads them, and a video opened again while its subtitles are still being made would only be made twice. UI thread only.
func (a *app) libraryAddOpened() {
	a.lib.openedPosted = false
	if len(a.lib.opened) == 0 {
		return
	}
	paths := slices.DeleteFunc(a.lib.opened, a.libraryInQueue)
	a.lib.opened = nil
	switch {
	case len(paths) > 0:
		a.dropFiles(paths)
	case a.page != pageFilms || a.openKey != "":
		a.Select(pageFilms)
	}
	a.remoteRaise()
}

// libraryInQueue reports whether the video at path has a job in the open library that is waiting or at work: one whose source is the file's URL, or names the same file by another path, as through a symbolic link.
func (a *app) libraryInQueue(path string) bool {
	dir := a.snap().LibraryDir
	info, statErr := os.Stat(path)
	for _, job := range a.library.Jobs {
		if job.Stage.Done() || !inQueueLibrary(job, dir) {
			continue
		}
		u, err := url.Parse(job.Source)
		if err != nil || u.Scheme != "file" || u.Path == "" {
			continue
		}
		if filepath.Clean(u.Path) == path {
			return true
		}
		if statErr != nil {
			continue
		}
		if other, err := os.Stat(u.Path); err == nil && os.SameFile(info, other) {
			return true
		}
	}
	return false
}
