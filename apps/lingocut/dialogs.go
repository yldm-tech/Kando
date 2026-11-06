package main

import "github.com/egoist/mygo"

// The system file dialogs. They run off the UI thread and report back through update.

func (a *app) pickVideos() {
	win := a.win
	update := a.update
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       "选择视频",
			ButtonLabel: "打开",
			Multiple:    true,
			Filters:     []mygo.FileFilter{{Name: "视频", Extensions: []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv"}}},
		})
		update(func() {
			if err != nil {
				a.queue.status = err.Error()
				return
			}
			if len(paths) > 0 {
				a.importLocal(paths[0])
			}
		})
	}()
}

func (a *app) pickFolder(title, button, current string, done func(string)) {
	win := a.win
	update := a.update
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
