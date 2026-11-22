package main

import (
	"path/filepath"

	"kando/apps/kando/dropsheet"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// dropUI is the sheet a large drop opens (package dropsheet): whether it is open, and release, which gives back the window's modal flag it holds (holdModal); paths, what was dropped, and found, the films they hold (films.Dropped); generate, whether Add queues the films to have their subtitles made rather than only adding them to the library, off until the user chooses it; category, whether Add puts them in a category named after the one folder dropped, on until the user turns it off.
type dropUI struct {
	open     bool
	release  func()
	paths    []string
	found    films.Drop
	generate bool
	category bool
}

func newDropUI() dropUI { return dropUI{} }

// dropHooks close the sheet as another library opens, whose films the drop was not counted against.
func (a *app) dropHooks() areaHooks {
	return areaHooks{libraryChanged: a.dropClose}
}

// dropAsk opens the sheet for d, the films paths hold, which are more than the window adds without asking (films.ConfirmAbove).
func (a *app) dropAsk(paths []string, d films.Drop) {
	a.dropClose()
	if a.lib.notice.Text == a.messages().LibraryDropReading() {
		a.lib.notice = films.Notice{}
	}
	a.drop = dropUI{open: true, release: a.holdModal(), paths: paths, found: d, category: true}
}

// dropClose closes the sheet and gives back the window's modal flag. Cancel, Escape and Add close it.
func (a *app) dropClose() {
	if a.drop.release != nil {
		a.drop.release()
	}
	a.drop = dropUI{}
}

// dropFolder is the name of the one folder dropped, "" when several things were or the one is a file.
func (a *app) dropFolder() string {
	if len(a.drop.paths) != 1 || !dropIsFolder(a.drop.paths[0]) {
		return ""
	}
	return filepath.Base(filepath.Clean(a.drop.paths[0]))
}

// dropLook is what the sheet shows of the drop, as the open library has its films now.
func (a *app) dropLook() dropsheet.Look {
	d := a.drop.found
	fresh, unmade := a.dropCount(d.Videos)
	l := dropsheet.Look{Folder: a.dropFolder(), Videos: len(d.Videos), New: len(fresh), Unmade: unmade, Size: d.Size, Skipped: d.Skipped, Disguised: len(d.Disguised), Cut: d.Cut}
	for i := 0; i < len(d.Disguised) && i < len(l.Named); i++ {
		l.Named[i] = filepath.Base(d.Disguised[i])
	}
	_, books := a.categoryBook()
	l.Categories = books && l.Folder != ""
	return l
}

// dropCount sorts videos as the open library has them: fresh, those it does not have, and unmade, how many of the others it has as films without subtitles that no job makes, as a video of its folder, or one added without a job, it has not watched yet. The others it has done, or waiting or being made, and adds no job for.
func (a *app) dropCount(videos []string) (fresh []string, unmade int) {
	bare, have := map[string]bool{}, map[string]bool{}
	for _, item := range a.films() {
		if item.VideoPath == "" {
			continue
		}
		have[item.VideoPath] = true
		if item.JobID == "" && item.SubtitlePath == "" {
			bare[item.VideoPath] = true
		}
	}
	root := a.snap().LibraryDir
	for _, job := range a.library.Jobs {
		if !films.InLibrary(root, job) || job.Stage == core.StageFailed {
			continue
		}
		if job.VideoPath != "" {
			have[job.VideoPath] = true
		}
		if path := queueLocalPath(job.Source); path != "" {
			have[path] = true
		}
	}
	for _, video := range videos {
		switch {
		case bare[video]:
			unmade++
		case !have[video]:
			fresh = append(fresh, video)
		}
	}
	return fresh, unmade
}

// dropAdd adds the films of the drop as the sheet chose and closes it: only into the library, where each is made when it is watched (libraryAddOnly), or into it and then each into the queue (libraryGenerateAdded); with the category checked, the films go in the category named after the folder, made when the library has none of that name.
func (a *app) dropAdd() {
	d, generate, folder := a.drop.found, a.drop.generate, a.dropFolder()
	category := a.drop.category && a.dropLook().Categories
	a.dropClose()
	m := a.messages()
	add := a.libraryAddOnly
	if generate {
		add = a.libraryGenerateAdded
	}
	if !add(d.Videos) {
		return
	}
	if category {
		a.categoryPutNamed(folder, d.Videos...)
	}
	if d.Skipped > 0 {
		a.lib.notice.Text = m.LibrarySkippedNotVideos(a.lib.notice.Text, d.Skipped)
	}
	a.lib.notice.Text = dropDisguisedNote(m, a.lib.notice.Text, d)
}

// dropOverlay is the sheet a large drop opens.
func (a *app) dropOverlay(*ns.Scope) ns.Node {
	return dropsheet.Sheet{
		Open:       live.Func(a, func() bool { return a.drop.open }),
		Look:       live.Func(a, a.dropLook),
		Generate:   live.Func(a, func() bool { return a.drop.generate }),
		OnGenerate: live.On(a, func(on bool) { a.drop.generate = on }),
		Category:   live.Func(a, func() bool { return a.drop.category }),
		OnCategory: live.On(a, func(on bool) { a.drop.category = on }),
		OnAdd:      live.Do(a, a.dropAdd),
		OnCancel:   live.Do(a, a.dropClose),
	}
}
