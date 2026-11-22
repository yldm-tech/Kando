package main

import (
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"
)

// Films added to the open library without a job (shelf.Library.Added): videos kept outside its folder, as those of a large folder dropped, which show where they are as films without subtitles, as a video found in the folder does, until 生成字幕 makes them. A video's file is never moved, renamed or written; taking such a film off the shelf forgets it and leaves its file as it was (libraryForget), and once its job completes the job holds the video (show).

// libraryAddOnly adds videos to the open library without a job, those it does not have yet and that are not in its folder, whose own videos it lists already, and says in the message bar how many it added. It reports whether the drop went in, none being new included; a locked library, or one whose shelf could not be read, takes nothing.
func (a *app) libraryAddOnly(videos []string) bool {
	if a.lockShut() {
		msg, section := a.precheckReady()
		a.warn(msg, section)
		return false
	}
	if a.library.Unread() != nil {
		a.warn(a.messages().LibraryShelfNotSaved(), "")
		return false
	}
	root := a.snap().LibraryDir
	var outside []string
	fresh, _ := a.dropCount(videos)
	for _, video := range fresh {
		if !films.InDir(root, video) {
			outside = append(outside, video)
		}
	}
	m := a.messages()
	n := a.library.AddVideos(root, time.Now(), outside...)
	if n == 0 {
		a.tell(m.QueueAlreadyInLibrary())
		return true
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, m.LibraryShelfSaveFailed)
	}
	a.refreshLibrary()
	a.tell(m.LibraryAddedOnly(n))
	return true
}

// libraryGenerateAdded adds videos to the open library as libraryAddOnly does, then makes the subtitles of each that has none, one after another behind the jobs waiting, as its 生成字幕 would (queueFromFolder): one that fails or is stopped goes back to being a film without subtitles rather than a failed card, and one with subtitles beside it is not paid for again. The message bar says how many were queued, and why the first that was not was not.
func (a *app) libraryGenerateAdded(videos []string) bool {
	if !a.libraryAddOnly(videos) {
		return false
	}
	byPath := map[string]films.Film{}
	for _, item := range a.films() {
		if item.JobID == "" && item.SubtitlePath == "" && !item.Missing {
			byPath[item.VideoPath] = item
		}
	}
	var todo []films.Film
	var sources []string
	for _, video := range videos {
		if item, ok := byPath[video]; ok {
			todo = append(todo, item)
			sources = append(sources, core.FileURL(video))
		}
	}
	if len(todo) == 0 {
		return true
	}
	m := a.messages()
	if msg, section := a.precheck(sources); msg != "" {
		a.warn(msg, section)
		return true
	}
	// The queue saves the shelf once for all of them rather than once a film.
	a.queue.batching = true
	queued, reason := 0, ""
	for _, item := range todo {
		if _, err := a.queueFromFolder(item); err != nil {
			if reason == "" {
				reason = sourceError(m, err)
			}
			continue
		}
		queued++
	}
	a.queue.batching = false
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, m.QueueSaveFailed)
	}
	a.queueTellSystem()
	if queued > 0 {
		a.sidebarShowQueued(sources)
	}
	a.librarySay(len(todo), queued, reason, "", m.LibraryAddedStarting, m.LibraryMakingSome)
	return true
}
