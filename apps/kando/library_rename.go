package main

import (
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
)

// 重命名… gives a film the title Kando shows for it on the shelf, in the player and over the socket. The title is kept in library.json by the video's path (shelf.Library.Names) and follows the video when it moves, so a folder renamed in Finder keeps it; files on disk are never renamed, and processing the film again keeps it over the title its site gives.

// AskRename opens the rename dialog for a film, with the title it shows now in the field.
func (a *app) AskRename(item films.Film) {
	if item.VideoPath == "" {
		return
	}
	a.lib.renaming = item
	a.lib.renameName = item.Name
	if a.lib.renameName == "" {
		a.lib.renameName = item.Title
	}
	a.lib.renameOriginal = a.libraryOriginalName(item)
	a.lib.renameOpen = true
	if a.lib.renameRelease == nil {
		a.lib.renameRelease = a.holdModal()
	}
}

func (a *app) Renaming() (*bool, *string, string) {
	return &a.lib.renameOpen, &a.lib.renameName, a.lib.renameOriginal
}

// RenameFilm keeps the title typed into the rename dialog for its film and closes the dialog. An empty title, or the title the film has without one given in Kando, drops the one given before.
func (a *app) RenameFilm() {
	a.lib.renameOpen = false
	item := a.lib.renaming
	// The library watcher can move a job's video while the dialog is open, so its path is looked up again.
	if job, ok := a.library.Find(item.JobID); item.JobID != "" && ok && job.VideoPath != "" {
		item.VideoPath = job.VideoPath
	}
	name := strings.TrimSpace(a.lib.renameName)
	if name == a.lib.renameOriginal {
		name = ""
	}
	if !a.library.Rename(item.VideoPath, name) {
		return
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = locales.LibraryShelfSaveFailed(err.Error())
	}
}

// libraryOriginalName is the title a film has without one given in Kando: its job's (jobName), or for a video no job has processed, the name of its file, or of its folder for a video.<ext> in a folder of its own, as the shelf names it (media.ListVideos).
func (a *app) libraryOriginalName(item films.Film) string {
	if item.JobID != "" {
		if job, ok := a.library.Find(item.JobID); ok {
			return jobName(job)
		}
	}
	return displayVideoName(item.VideoPath)
}
