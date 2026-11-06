package main

import (
	"lingocut/apps/lingocut/films"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

// The library page's reads and clicks (films.App).

func (a *app) MakingLibrary() bool { return a.snap().MakingLibrary }

func (a *app) OpenKey() string { return a.openKey }

func (a *app) SetOpenKey(key string) { a.openKey = key }

func (a *app) Source() *string { return &a.queue.source }

func (a *app) Status() string { return a.queue.status }

func (a *app) Running() bool { return a.running() }

func (a *app) HasJob() bool { return a.queue.job != nil }

func (a *app) PlaybackOpen() bool {
	return a.player.play != nil || a.player.playPath != "" || a.player.playErr != ""
}

func (a *app) StopPlayback() { a.stopPlayback() }

func (a *app) CancelMaking() {
	a.edit(func(s *session) { s.MakingLibrary = false })
}

func (a *app) LibraryPlace() string { return a.libraryPlace() }

func (a *app) PickLibraryParent() { a.pickLibraryParent() }

func (a *app) CreateLibrary() { a.createLibrary() }

func (a *app) BindName(edit func(*string) bool) error {
	name, err := nodra.BindField(a.store, func(s session) string { return s.LibraryName }, func(s *session, value string) error {
		s.LibraryName = value
		return nil
	})
	if err != nil {
		return err
	}
	_ = name.Edit(edit)
	return nil
}

func (a *app) Start() { a.start() }

func (a *app) Stop() { a.stop() }

func (a *app) PickVideos() { a.pickVideos() }

func (a *app) Poster(path string) ui.ImageSource { return a.posterOf(path) }

func (a *app) EnsurePoster(path string) { a.ensurePoster(path) }

func (a *app) Open(item films.Film) {
	a.openKey = item.Key
	a.loadSubtitles(item.Key, item.SubtitlePath)
	a.startPlayback(item.VideoPath)
}

func (a *app) Remove(id string) { a.remove(id) }

func (a *app) Retry(id string) {
	if job, ok := a.library.Find(id); ok {
		a.retry(job)
	}
}

func (a *app) JobFilm(id string) (films.Film, bool) {
	job, ok := a.library.Find(id)
	if !ok {
		return films.Film{}, false
	}
	return films.FromJob(job, jobName(job), statusText(job)), true
}

func (a *app) films() []films.Film {
	shot := a.snap()
	return films.List(shot.LibraryDir, a.library.Jobs, shot.Videos, jobName, statusText)
}

// dropFiles opens the first video among files dropped on the page, ready to process.
func (a *app) dropFiles(paths []string) {
	for _, path := range paths {
		if media.IsVideoFile(path) {
			a.importLocal(path)
			return
		}
	}
}

// libraryKeys are the library's shortcuts.
func (a *app) libraryKeys() []shortcut {
	return []shortcut{
		{Group: "文件", Label: "打开本地视频…", Mods: ui.Cmd, Key: ui.KeyO, Run: a.pickVideos},
	}
}
