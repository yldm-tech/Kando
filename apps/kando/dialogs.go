package main

import (
	ns "github.com/nsgo-dev/nsgo/core"
)

// The system file dialogs. NSGo shows them and calls back on the engine's goroutine with what the user picked; each holds the window's modal flag (holdModal) until it answers. Without the app's scope, as in a test that starts no app, no dialog shows.

// libraryVideoExtensions are the extensions, without the dot, of the videos 本地视频 lets the user pick: those media.IsVideoFile takes. The fileAssociations of mygo.json and the file_types of nsgo.yaml, which make Kando one of the apps that open these files, list the same ones (library_test.go checks).
var libraryVideoExtensions = []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv"}

// pickVideos asks for local videos and adds those picked to the queue (dropFiles).
func (a *app) pickVideos() {
	if a.cx == nil {
		return
	}
	types := make([]string, len(libraryVideoExtensions))
	for i, ext := range libraryVideoExtensions {
		types[i] = "." + ext
	}
	release := a.holdModal()
	// TODO(nsgo): give core.PickFilesOptions a title, a button label and a name for the filter, so the panel says 选择本地视频 and 添加 over 视频 as MyGo's open dialog did; it shows the system's own words
	// The answer belongs to the app, not to the button that asked, which may be gone by then (core.Scope.Run).
	a.cx.Run(func() {
		a.cx.PickFiles(ns.PickFilesOptions{Multiple: true, Types: types}, func(files []ns.File, err error) {
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

// pickFolder asks for a folder and passes the one picked to done.
func (a *app) pickFolder(done func(string)) {
	if a.cx == nil {
		return
	}
	release := a.holdModal()
	// TODO(nsgo): give core.PickDirectoriesOptions a title, a button label, the folder to start in and a way to make a new folder, so the panels of 打开已有资料库… and 更改 say their titles and buttons (libraryImportTitle, libraryPlaceTitle and their buttons in the catalogs), start in the folder shown now and offer 新建文件夹 as MyGo's open dialog did; they show the system's own words and start where the system chooses
	a.cx.Run(func() {
		a.cx.PickDirectories(ns.PickDirectoriesOptions{}, func(dirs []ns.Directory, err error) {
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
