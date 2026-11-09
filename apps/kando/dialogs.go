package main

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo"
)

// The system file dialogs. They run off the UI thread and report back through update. Each holds the window's modal flag (holdModal) until it reports.

func (a *app) pickVideos() {
	win := a.win
	update := a.update
	release := a.holdModal()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       locales.LibraryPickVideosTitle(),
			ButtonLabel: locales.LibraryPickVideosButton(),
			Multiple:    true,
			Filters:     []mygo.FileFilter{{Name: locales.LibraryVideoFilter(), Extensions: []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv"}}},
		})
		update(func() {
			release()
			if err != nil {
				a.warn(err.Error(), "")
				return
			}
			if len(paths) > 0 {
				a.dropFiles(paths)
			}
		})
	}()
}

func (a *app) pickFolder(title, button, current string, done func(string)) {
	win := a.win
	update := a.update
	release := a.holdModal()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            win,
			Title:             title,
			ButtonLabel:       button,
			Directory:         true,
			CreateDirectories: true,
			DefaultPath:       current,
		})
		update(func() {
			release()
			if err != nil {
				a.saveErr = err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			done(paths[0])
		})
	}()
}
