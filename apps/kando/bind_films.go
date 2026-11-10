package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

// The library page's reads and clicks (films.App).

func (a *app) MakingLibrary() bool { return a.snap().MakingLibrary }

func (a *app) OpenKey() string { return a.openKey }

func (a *app) SetOpenKey(key string) { a.openKey = key }

// libraryOwnsTitleBar reports whether the library page draws its own header bar at the top of the content column, keeping clear of the window controls and dragging the window, so the content has no drag band over it (contentBand). It does whenever it shows, on 全部影片 or YouTube with no film open over it: its top bar is a breeze.HeaderBar (films/topbar.go), as tall as the title bar at least, across the whole content column, so mainColumn leaves the page's padding to the page.
func (a *app) libraryOwnsTitleBar() bool {
	return (a.page == pageFilms || a.page == pageYouTube) && a.openKey == ""
}

func (a *app) Source() *string { return &a.queue.source }

func (a *app) LinkFocus() *bool { return &a.queue.linkFocus }

func (a *app) Status() string { return a.queue.status }

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

// NewLibraryPath is the folder createLibrary would make for the name and place the new-library sheet holds.
func (a *app) NewLibraryPath() (string, error) {
	file, err := libraryFileName(a.snap().LibraryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(a.libraryPlace(), file), nil
}

// Start adds the link in the top bar to the queue. The field empties once the link is added; otherwise the message bar says why and the link stays, to be fixed.
func (a *app) Start() {
	if a.libraryEnqueue(a.queue.source) {
		a.queue.source = ""
	}
}

func (a *app) PickVideos() { a.pickVideos() }

func (a *app) PickVideosShortcut() (ui.Modifiers, ui.Key) {
	k := a.libraryOpenVideos()
	return k.Mods, k.Key
}

func (a *app) DropFiles(paths []string) { a.dropFiles(paths) }

func (a *app) Poster(path string) ui.ImageSource { return a.posterOf(path) }

func (a *app) Thumbnail(item films.Film) ui.ImageSource { return a.thumbnailOf(item) }

func (a *app) Duration(path string) time.Duration { return a.lib.durations[path] }

func (a *app) Quote(item films.Film) (translation, original string) {
	q := a.lib.quotes[item.SubtitlePath]
	return q.translation, q.original
}

func (a *app) Open(item films.Film) {
	// Jumping to a line of the open film plays it from there as a seek does, keeping its state; opening any film, the one that just closed included, starts it afresh.
	a.player.playSame = a.player.playJump && item.Key != "" && item.Key == a.openKey
	a.openKey = item.Key
	a.loadSubtitles(item.Key, item.SubtitlePath)
	a.startPlayback(item.VideoPath)
}

// OpenAt opens a film playing from at, as the hero's 播放, 从 X 继续 and 从头看 do. It keeps the film's state as a seek does, so the player's line loop is told (playerLoopOpening).
func (a *app) OpenAt(item films.Film, at time.Duration) {
	a.playerLoopOpening(item.Key, at)
	a.player.playFrom = at
	a.player.playJump = true
	a.Open(item)
}

// Reprocess runs a done film through the pipeline again, into the target language set now: a job again from its source, a video no job has processed from its file.
func (a *app) Reprocess(item films.Film) {
	delete(a.lib.quotes, item.SubtitlePath)
	if item.JobID != "" {
		a.Retry(item.JobID)
		return
	}
	a.libraryEnqueue(core.FileURL(item.VideoPath))
}

// TranslateAgain translates a done film's transcript again into the target language set now, without downloading or transcribing it. The film page's ⋯ uses it too (film.App).
func (a *app) TranslateAgain(item films.Film) { a.libraryRetranslate(item, core.RedoTranslation) }

// TranslateMissing translates only the lines a done film's translation left untranslated, as the card's 补译未译的句子 does. The film page's ⋯ uses it too (film.App).
func (a *app) TranslateMissing(item films.Film) { a.libraryRetranslate(item, core.RedoMissing) }

// libraryRetranslate queues done film item to be translated again (queueRetranslate) on its card, or an open film's page, and says in the message bar why when it cannot. The film page draws no message bar, so there the settings also open where the target language is chosen. The hero's line is read again once the film is done.
func (a *app) libraryRetranslate(item films.Film, redo core.Redo) {
	job, ok := a.library.Find(item.JobID)
	if !ok {
		return
	}
	delete(a.lib.quotes, item.SubtitlePath)
	if err := a.queueRetranslate(job, redo); err != nil {
		a.warn(err.Error(), settings.SectionTranslate)
		if a.openKey != "" {
			a.OpenSettings(settings.SectionTranslate)
		}
	}
}

func (a *app) Remove(id string) { a.remove(id) }

// Retry runs a failed or finished job again on the same card. When it cannot start now, the message bar says why.
func (a *app) Retry(id string) {
	job, ok := a.library.Find(id)
	if !ok {
		return
	}
	a.retry(job)
	if now, _ := a.library.Find(id); now.Stage != job.Stage {
		return
	}
	if a.RunningJob() != "" {
		a.warn(errBusy().Error(), "")
	} else if a.queue.status != "" {
		a.warn(a.queue.status, "")
	}
}

func (a *app) FailureHint(item films.Film) (reason, section string) { return failureHint(item) }

func (a *app) JobFilm(id string) (films.Film, bool) {
	job, ok := a.library.Find(id)
	if !ok {
		return films.Film{}, false
	}
	return films.FromJob(job, jobName(job), statusText(job)), true
}

func (a *app) Notice() *films.Notice { return &a.lib.notice }

func (a *app) QueueExpanded() *bool { return &a.lib.queueOpen }

func (a *app) LibrarySort() string { return a.lib.sort }

// SetLibrarySort shows the done grid in order and keeps the order in settings.json for the next launch; the message bar says so when it cannot be kept.
func (a *app) SetLibrarySort(order string) {
	order = films.ValidSort(order)
	if order == a.lib.sort {
		return
	}
	a.lib.sort = order
	if err := a.saveSettings(); err != nil {
		a.warn(locales.LibrarySortSaveFailed(err.Error()), "")
	}
}

// AskRemove opens the confirmation of 从资料库移除, and reads now, rather than while the dialog is drawn, whether the film's folder or only its files would go to the Trash.
func (a *app) AskRemove(item films.Film) {
	a.lib.removing = item
	files := filmFiles(a.snap().LibraryDir, item)
	a.lib.removeFolder = len(files) == 1 && files[0] == filepath.Dir(item.VideoPath)
	a.lib.removeOpen = true
	if a.lib.removeRelease == nil {
		a.lib.removeRelease = a.holdModal()
	}
}

func (a *app) Removing() (*bool, films.Film, bool) {
	return &a.lib.removeOpen, a.lib.removing, a.lib.removeFolder
}

// trashItem moves a file or a folder to the Trash. Tests replace it.
var trashItem = mygo.Shell.TrashItem

// TrashFilm takes a film off the shelf, as 从资料库移除 confirms. A film inside the library goes to the Trash with its folder, or with only its subtitles when that folder holds other videos or folders too. A local video kept outside the library stays where it is and only leaves the shelf.
func (a *app) TrashFilm(item films.Film) {
	name := item.Name
	if name == "" {
		name = item.Title
	}
	files := filmFiles(a.snap().LibraryDir, item)
	for _, path := range files {
		if err := trashItem(path); err != nil {
			a.warn(locales.LibraryTrashFailed(name, err.Error()), "")
			return
		}
	}
	if a.openKey == item.Key {
		a.openKey = ""
		a.stopPlayback()
	}
	if item.JobID != "" {
		a.remove(item.JobID)
	}
	a.refreshLibrary()
	if len(files) > 0 {
		a.tell(locales.LibraryTrashed(name))
	} else {
		a.tell(locales.LibraryRemoved(name))
	}
}

// filmFiles is what moving a film to the Trash moves: the folder of a film inside the library root, or, when that folder holds other videos or any folder too, or is the root itself, the video and its subtitles. A video outside the library has none.
func filmFiles(root string, item films.Film) []string {
	video := item.VideoPath
	if video == "" || !films.InDir(root, video) {
		return nil
	}
	dir := filepath.Dir(video)
	if films.InDir(root, dir) && aloneIn(dir, video) {
		return []string{dir}
	}
	files := []string{video}
	base := strings.TrimSuffix(video, filepath.Ext(video))
	for _, sub := range []string{item.SubtitlePath, base + ".srt", base + ".bilingual.srt"} {
		if sub != "" && fileExists(sub) && !slices.Contains(files, sub) {
			files = append(files, sub)
		}
	}
	return files
}

// aloneIn reports whether video is the only video in dir and dir has no folder inside it. The shelf lists only the videos directly in a film's folder, so a folder inside may hold videos, or anything else, the user never saw there; the folder then stays, and only the film's files go.
func aloneIn(dir, video string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || (media.IsVideoFile(entry.Name()) && filepath.Join(dir, entry.Name()) != video) {
			return false
		}
	}
	return true
}

func (a *app) films() []films.Film {
	shot := a.snap()
	return films.List(shot.LibraryDir, a.library.Jobs, shot.Videos, jobName, statusText)
}

// QueueFilms is the films of 处理中: the open library's jobs that are waiting, running, failed or were cancelled, and the jobs the queue runs or will run for other libraries (queueElsewhere).
func (a *app) QueueFilms() []films.Film {
	return films.Queue(a.snap().LibraryDir, a.library.Jobs, a.queueElsewhere, jobName, statusText)
}

// libraryEnqueue adds sources to the queue once precheck passes, and says in the message bar what happened. It reports whether any source was added, a failed job of the same video queued again included. A video the library already holds is not added; the bar says so and offers to open it.
func (a *app) libraryEnqueue(sources ...string) bool {
	if msg, section := a.precheck(sources); msg != "" {
		a.warn(msg, section)
		return false
	}
	added, err := a.queueEnqueue(sources)
	var dup *queueDuplicate
	switch {
	case err != nil && added > 0:
		a.warn(locales.LibraryAddedSome(added, sourceError(err)), "")
	case errors.As(err, &dup) && dup.Stage == core.StageComplete:
		a.warn(sourceError(err), "")
		a.lib.notice.Film = dup.ID
	case err != nil:
		a.warn(sourceError(err), "")
	case added > 1:
		a.tell(locales.LibraryAddedStarting(added))
	default:
		a.tell(locales.LibraryAddedOneStarting())
	}
	return added > 0
}

// dropFiles adds every video among files dropped on the window, or picked with 本地视频, to the queue. Files that are not videos are left out; when none is a video, the message bar says what can be added. Files dropped on another page or over an open film, or picked from there with ⌘O, take the window to the library, whose message bar and queue show what became of them.
func (a *app) dropFiles(paths []string) {
	if a.page != pageFilms || a.openKey != "" {
		a.Select(pageFilms)
	}
	var sources []string
	for _, path := range paths {
		if media.IsVideoFile(path) {
			sources = append(sources, core.FileURL(path))
		}
	}
	if len(sources) == 0 {
		a.warn(locales.PrecheckNoVideo(), "")
		return
	}
	a.libraryEnqueue(sources...)
}

// libraryKeys are the library's shortcuts. ⌘F searches the library's films in the sidebar's search field (sidebarSearch). It shares ⌘F with the player's 搜索字幕: this row holds only while no film is open and that one only while one is, so one of them at most runs it, and the menu bar shows ⌘F by that one (menuSharedKeyCarriers, menu.go).
func (a *app) libraryKeys() []shortcut {
	return []shortcut{
		a.libraryOpenVideos(),
		{Group: groupEdit, Label: locales.LibrarySearchMenu(), Mods: ui.Cmd, Key: ui.KeyF, When: a.sidebarSearchable, Run: a.sidebarSearch},
	}
}

// libraryOpenVideos is the shortcut that asks for local videos: its row of the File menu, and the shortcut the tooltip of 本地视频 shows (PickVideosShortcut).
func (a *app) libraryOpenVideos() shortcut {
	return shortcut{Group: groupFile, Label: locales.LibraryOpenVideosMenu(), Mods: ui.Cmd, Key: ui.KeyO, Run: a.pickVideos}
}
