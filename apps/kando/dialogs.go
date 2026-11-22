package main

import (
	ns "github.com/nsgo-dev/nsgo/core"

	"kando/packages/media"
)

// The system file dialogs. NSGo shows them and calls back on the engine's goroutine with what the user picked; each holds the window's modal flag (holdModal) until it answers. Without the app's scope, as in a test that starts no app, no dialog shows.

// pickVideos asks for local videos and sounds, the files the library takes (media.MediaExtensions), and adds those picked to the queue (dropFiles). The file_types of nsgo.yaml, which make Kando one of the apps that open these files, list the same ones (library_test.go checks).
func (a *app) pickVideos() {
	if a.cx == nil {
		return
	}
	var types []string
	for _, ext := range media.MediaExtensions() {
		types = append(types, "."+ext)
	}
	release := a.holdModal()
	m := a.messages()
	// The answer belongs to the app, not to the button that asked, which may be gone by then (core.Scope.Run).
	a.cx.Run(func() {
		a.cx.PickFiles(ns.PickFilesOptions{Multiple: true, Types: types, TypesName: m.LibraryVideoFilter(), Title: m.LibraryPickVideosTitle(), Button: m.LibraryPickVideosButton()}, func(files []ns.File, err error) {
			release()
			defer a.Changed()
			if err != nil {
				a.warn(err.Error(), "")
				return
			}
			var paths []string
			for _, f := range files {
				if f.Path != "" {
					paths = append(paths, f.Path)
				}
			}
			if len(paths) > 0 {
				a.dropFiles(paths)
			}
		})
	})
}

// pickVideoFolders asks for folders of videos and sounds, as 文件 › 添加文件夹… does, and adds what they hold, however deep, as dropping them does (dropFiles). A panel of its own, as Windows' picker cannot offer files and folders at once.
func (a *app) pickVideoFolders() {
	if a.cx == nil {
		return
	}
	release := a.holdModal()
	m := a.messages()
	a.cx.Run(func() {
		a.cx.PickDirectories(ns.PickDirectoriesOptions{Multiple: true, Title: m.LibraryPickFoldersTitle(), Button: m.LibraryPickFoldersButton()}, func(dirs []ns.Directory, err error) {
			release()
			defer a.Changed()
			if err != nil {
				a.warn(err.Error(), "")
				return
			}
			var paths []string
			for _, d := range dirs {
				if d.Path != "" {
					paths = append(paths, d.Path)
				}
			}
			if len(paths) > 0 {
				a.dropFiles(paths)
			}
		})
	})
}

// pickFolder asks for a folder in a panel with the title and button of o, starting in its Start when it has one, and passes the one picked to done.
func (a *app) pickFolder(o ns.PickDirectoriesOptions, done func(string)) {
	if a.cx == nil {
		return
	}
	release := a.holdModal()
	a.cx.Run(func() {
		a.cx.PickDirectories(o, func(dirs []ns.Directory, err error) {
			release()
			defer a.Changed()
			if err != nil {
				a.saveErr = err.Error()
				return
			}
			if len(dirs) == 0 || dirs[0].Path == "" {
				return
			}
			done(dirs[0].Path)
		})
	})
}
