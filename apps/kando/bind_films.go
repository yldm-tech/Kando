package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/yldm-tech/nodra"
)

// The library page's reads and clicks (films.App). The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// libraryColumn is the library in the content column (view.go's mainColumn): the film open over it, as the player shows it (filmPage, bind_film.go), or else the library page. A film is open while openKey names one the shelf or the queue still has (libraryOpenFilm); libraryFollowOpen lets go of one that went.
func (a *app) libraryColumn(cx *ns.Scope) ns.Node {
	// One memo looks the open film up once a change, and tells the page only when the film or whether one is open changed.
	type openFilm struct {
		item films.Film
		ok   bool
	}
	current := live.Memo(cx, a, func() openFilm {
		item, ok := a.libraryOpenFilm()
		return openFilm{item, ok}
	})
	return ns.Show{
		When: func() bool { return current().ok },
		Then: func(cx *ns.Scope) ns.Node { return a.filmPage(func() films.Film { return current().item }) },
		Else: func(*ns.Scope) ns.Node { return films.View{App: a} },
	}
}

// libraryOpenFilm is the film openKey names: one of the open library's, or a job's the queue runs for another library, false when openKey names none.
func (a *app) libraryOpenFilm() (films.Film, bool) {
	if a.openKey == "" {
		return films.Film{}, false
	}
	if item, ok := films.Find(a.films(), a.openKey); ok {
		return item, true
	}
	return a.JobFilm(a.openKey)
}

// libraryFollowOpen lets go of an open film the library no longer has, as one removed or one of a library switched away from, which takes the window back to the library; and stops the player when no film is open, as when a page closed it. It runs after every change (libraryHooks' afterChange) and reports whether it changed anything. The browser keeps the player as it is: it closes the film itself.
func (a *app) libraryFollowOpen() bool {
	if a.page == pageBrowse {
		return false
	}
	// Letting go of the open film changes what the window shows (openKey) even when no player was open for it, as for a film whose video never loaded.
	cleared := false
	if a.openKey != "" {
		if _, ok := a.libraryOpenFilm(); ok {
			return false
		}
		a.openKey = ""
		cleared = true
	}
	if a.PlaybackOpen() {
		a.StopPlayback()
		return true
	}
	return cleared
}

// Films is the open library's films (films.App).
func (a *app) Films() []films.Film { return a.films() }

func (a *app) MakingLibrary() bool { return a.snap().MakingLibrary }

func (a *app) OpenKey() string { return a.openKey }

func (a *app) SetOpenKey(key string) { a.openKey = key }

// libraryOwnsTitleBar reports whether the library page draws its own header bar at the top of the content column, keeping clear of the window controls and dragging the window, so the content has no drag band over it (contentBand). It does whenever it shows, on 全部影片 or YouTube with no film open over it: its top bar (films/topbar.go) is as tall as the title bar at least, across the whole content column, so mainColumn leaves the page's padding to the page.
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

func (a *app) NewLibraryName() string { return a.snap().LibraryName }

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
	file, err := libraryFileName(a.messages(), a.snap().LibraryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(a.libraryPlace(), file), nil
}

// Start adds the links in the top bar to the queue, several when what was pasted holds several, between spaces or on lines of their own; a playlist or a channel among them opens the dialog that lists its videos (playlistAsk). The field empties once they are added; otherwise the message bar says why and the links stay, to be fixed.
func (a *app) Start() {
	if a.libraryEnqueue(strings.Fields(a.queue.source)...) {
		a.queue.source = ""
	}
}

// AddList opens the dialog that lists the videos of the playlist at source, as the message bar's Add Whole Playlist does after one of its videos was added.
func (a *app) AddList(source string) {
	a.lib.notice.Open = false
	a.libraryEnqueue(source)
}

func (a *app) PickVideos() { a.pickVideos() }

func (a *app) PickVideosShortcut() string { return libraryOpenVideosKey }

func (a *app) DropFiles(paths []string) { a.dropFiles(paths) }

func (a *app) Poster(path string) ns.ImageSource { return a.posterOf(path) }

func (a *app) Thumbnail(item films.Film) ns.ImageSource { return a.thumbnailOf(item) }

func (a *app) Duration(path string) time.Duration { return a.lib.durations[path] }

func (a *app) Quote(item films.Film) (translation, original string) {
	q := a.lib.quotes[item.SubtitlePath]
	return q.translation, q.original
}

func (a *app) Open(item films.Film) {
	// Jumping to a line of the open film plays it from there as a seek does, keeping its state; opening any film, the one that just closed included, starts it afresh.
	a.player.playSame = a.player.playJump && item.Key != "" && item.Key == a.openKey
	// A film opened without a point picked for it (OpenAt), as from its poster, the hero, the notice or the menu bar, opens held where it stopped last time, as 继续 would play it, so playing it on keeps that place rather than starting over and saving a new one in its place.
	if !a.player.playJump {
		a.player.playFrom, _ = films.ResumeFrom(a, item)
	}
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

// AskReprocess opens the confirmation of 重新处理 for item, as processing a film again transcribes it and pays for its translation again. With no language to translate it into there is nothing to confirm: Reprocess says so at once. The confirmation holds the window's modal flag until it closes (CloseReprocess).
func (a *app) AskReprocess(item films.Film) {
	if a.libraryReprocessLanguage(item) == "" {
		a.Reprocess(item)
		return
	}
	a.lib.reprocessing = item
	a.lib.reprocessOpen = true
	if a.lib.reprocessRelease == nil {
		a.lib.reprocessRelease = a.holdModal()
	}
}

func (a *app) Reprocessing() (*bool, films.Film, string) {
	return &a.lib.reprocessOpen, a.lib.reprocessing, a.libraryReprocessLanguage(a.lib.reprocessing)
}

// libraryReprocessLanguage is the language 重新处理 translates item into: a job's film keeps its own (queueReprocessLanguage), and a video no job has processed takes the one the settings name.
func (a *app) libraryReprocessLanguage(item films.Film) string {
	if job, ok := a.library.Find(item.JobID); ok && item.JobID != "" {
		return a.queueReprocessLanguage(job)
	}
	return strings.TrimSpace(a.prefs.language)
}

// CloseReprocess closes the confirmation of 重新处理 and gives back the window's modal flag it held, so the menu bar's commands run again at once.
func (a *app) CloseReprocess() {
	a.lib.reprocessOpen = false
	if a.lib.reprocessRelease != nil {
		a.lib.reprocessRelease()
		a.lib.reprocessRelease = nil
	}
}

// ReprocessFilm is the film page's 重新处理整部影片: the open film is processed again once confirmed (AskReprocess), and stays open, following its job.
func (a *app) ReprocessFilm() {
	if item, ok := a.openFilm(); ok {
		a.AskReprocess(item)
	}
}

// libraryReprocessOverlay is the confirmation of 重新处理 over the window (overlays), so the film page asks too.
func (a *app) libraryReprocessOverlay(*ns.Scope) ns.Node { return films.ReprocessDialog{App: a} }

// Reprocess runs a done film through the pipeline again, into the language it was translated into (queueReprocessLanguage): a job again from its source, a video no job has processed from its file, which is also how 生成字幕 makes the subtitles of a video found in the folder without any (startFilm). Either stays the done film it was until the new subtitles are written (core.RedoAll, core.Job.FromFolder).
func (a *app) Reprocess(item films.Film) {
	delete(a.lib.quotes, item.SubtitlePath)
	if item.JobID != "" {
		a.Retry(item.JobID)
		return
	}
	a.startFilm(item)
}

// TranslateAgain translates a done film's transcript again into language, without downloading or transcribing it, as 重新翻译为 ▸ does from the library and the film page's ⋯ (film.App). Translating into another language than the film's replaces its translation, so that waits for the user to confirm (RetranslateDialog); the confirmation holds the window's modal flag until it closes (CloseRetranslate).
func (a *app) TranslateAgain(item films.Film, language string) {
	language = strings.TrimSpace(language)
	if current := strings.TrimSpace(item.Language); current == "" || current == language {
		a.libraryRetranslate(item, core.RedoTranslation, language)
		return
	}
	a.lib.retranslating, a.lib.retranslateLanguage, a.lib.retranslateOpen = item, language, true
	if a.lib.retranslateRelease == nil {
		a.lib.retranslateRelease = a.holdModal()
	}
}

func (a *app) TargetLanguage() string { return strings.TrimSpace(a.prefs.language) }

func (a *app) Retranslating() (*bool, films.Film, string) {
	return &a.lib.retranslateOpen, a.lib.retranslating, a.lib.retranslateLanguage
}

// CloseRetranslate closes the confirmation of 重新翻译为 and gives back the window's modal flag it held.
func (a *app) CloseRetranslate() {
	a.lib.retranslateOpen = false
	if a.lib.retranslateRelease != nil {
		a.lib.retranslateRelease()
		a.lib.retranslateRelease = nil
	}
}

// Retranslate translates the film the confirmation of 重新翻译为 asked about into its language, and closes it.
func (a *app) Retranslate() {
	if !a.lib.retranslateOpen {
		return
	}
	item, language := a.lib.retranslating, a.lib.retranslateLanguage
	a.CloseRetranslate()
	a.libraryRetranslate(item, core.RedoTranslation, language)
}

// libraryRetranslateOverlay is the confirmation of 重新翻译为 over the window (overlays), so the film page asks too.
func (a *app) libraryRetranslateOverlay(*ns.Scope) ns.Node { return films.RetranslateDialog{App: a} }

// TranslateMissing translates only the lines a done film's translation left untranslated, as the card's 补译未译的句子 does. The film page's ⋯ uses it too (film.App).
func (a *app) TranslateMissing(item films.Film) { a.libraryRetranslate(item, core.RedoMissing, "") }

// libraryRetranslate queues done film item to be translated again (queueRetranslate) into language, "" for the one queueRetranslate picks, on its card, or an open film's page, and says in the message bar why when it cannot. The film page draws no message bar, so there the settings also open where the target language is chosen. The hero's line is read again once the film is done.
func (a *app) libraryRetranslate(item films.Film, redo core.Redo, language string) {
	job, ok := a.library.Find(item.JobID)
	if !ok {
		return
	}
	delete(a.lib.quotes, item.SubtitlePath)
	if err := a.queueRetranslate(job, redo, language); err != nil {
		a.warn(err.Error(), settings.SectionTranslate)
		if a.openKey != "" {
			a.OpenSettings(settings.SectionTranslate)
		}
	}
}

func (a *app) Remove(id string) { a.remove(id) }

// Retry runs a failed or finished job again on the same card. When it cannot be queued, the message bar says why: a job already running or waiting says nothing, as its card says so; otherwise the settings name no target language (queueRetryable), and the bar links to where it is set. The film page draws no message bar, so there the settings open too, as libraryRetranslate does.
func (a *app) Retry(id string) {
	job, ok := a.library.Find(id)
	if !ok {
		return
	}
	ready, err := a.queueRetryable(job)
	a.queueAgain(ready, err)
	if err == nil || errors.Is(err, errInQueue) {
		return
	}
	a.warn(err.Error(), settings.SectionTranslate)
	if a.openKey != "" {
		a.OpenSettings(settings.SectionTranslate)
	}
}

func (a *app) FailureHint(item films.Film) (reason, section string) {
	return failureHint(a.messages(), item)
}

// TranslationHint reads why a done film translated none of its lines as failureHint reads a failed translation, so it says what to do in the same words a failed job's card would.
func (a *app) TranslationHint(item films.Film) (reason, section string) {
	if item.TranslationErr == "" {
		return "", ""
	}
	return translationHint(a.messages(), item.TranslationErr)
}

func (a *app) JobFilm(id string) (films.Film, bool) {
	job, ok := a.library.Find(id)
	if !ok {
		return films.Film{}, false
	}
	return films.FromJob(job, a.libraryJobName(job), statusText(a.messages(), job)), true
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
		a.warn(a.messages().LibrarySortSaveFailed(err.Error()), "")
	}
}

// AskRemove opens the confirmation of 从资料库移除, and reads now, rather than while the dialog is drawn, whether the film's folder or only its files would go to the Trash. The confirmation holds the window's modal flag until it closes (libraryCloseRemove).
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

// CloseRemove closes the confirmation of 从资料库移除 and moves nothing, as 取消, Enter and Escape do.
func (a *app) CloseRemove() { a.libraryCloseRemove() }

// libraryCloseRemove closes the confirmation of 从资料库移除 and gives back the window's modal flag it held, so the menu bar's commands run again at once.
func (a *app) libraryCloseRemove() {
	a.lib.removeOpen = false
	if a.lib.removeRelease != nil {
		a.lib.removeRelease()
		a.lib.removeRelease = nil
	}
}

// trashItem moves a file or a folder to the Trash, under the scope cx. Tests replace it.
var trashItem = desktop.Trash

// TrashFilm takes a film off the shelf, as 从资料库移除 confirms. A film inside the library goes to the Trash with its folder, or with only its subtitles when that folder holds other videos or folders too. A local video kept outside the library stays where it is and only leaves the shelf.
func (a *app) TrashFilm(item films.Film) {
	a.libraryCloseRemove()
	m := a.messages()
	name := item.Name
	if name == "" {
		name = item.Title
	}
	files := filmFiles(a.snap().LibraryDir, item)
	for _, path := range files {
		if err := trashItem(a.cx, path); err != nil {
			// desktop.Trash fails with errors.ErrUnsupported where there is no Trash it can use, and with desktop.ErrNotTrashed when the item could not go there; it logs what the system said, so the bar says each in a sentence of its own.
			if errors.Is(err, errors.ErrUnsupported) {
				a.warn(m.LibraryTrashUnsupported(name), "")
			} else {
				a.warn(m.LibraryTrashFailed(name), "")
			}
			return
		}
	}
	if a.openKey == item.Key {
		a.openKey = ""
		a.stopPlayback()
	}
	// The film leaves the shelf, and the title given to it in Kando with it; remove saves library.json for a job, and a video no job processed saves it here.
	named := a.library.Rename(item.VideoPath, "")
	if item.JobID != "" {
		a.remove(item.JobID)
	} else if named {
		if err := a.library.Save(); err != nil {
			a.saveErr = m.LibraryShelfSaveFailed(err.Error())
		}
	}
	a.refreshLibrary()
	if len(files) > 0 {
		a.tell(m.LibraryTrashed(name))
	} else {
		a.tell(m.LibraryRemoved(name))
	}
}

// filmFiles is what moving a film to the Trash moves: the folder of a film inside the library root, or, when that folder holds other videos or any folder too, or is the root itself, the video and its subtitles, Kando's transcript beside subtitles of the user's among them (media.TranscriptOf). A video outside the library has none.
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
	for _, sub := range []string{item.SubtitlePath, base + ".srt", media.TranscriptOf(video), base + ".bilingual.srt"} {
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
	return films.List(shot.LibraryDir, a.library.Jobs, shot.Videos, a.library.Names, jobName, a.statusOf)
}

// statusOf is what a job's card says of where it is, in the interface's language (statusText), as films.List and films.Queue take it.
func (a *app) statusOf(job core.Job) string { return statusText(a.messages(), job) }

// QueueFilms is the films of 处理中: the open library's jobs that are waiting, running, failed or were cancelled, and the jobs the queue runs or will run for other libraries (queueElsewhere). A waiting one says how many jobs run before it (queueAhead).
func (a *app) QueueFilms() []films.Film {
	items := films.Queue(a.snap().LibraryDir, a.library.Jobs, a.queueElsewhere, jobName, a.statusOf)
	ahead := a.queueAhead()
	for i := range items {
		items[i].Ahead = ahead[items[i].JobID]
	}
	return items
}

// queueAhead is how many jobs run before each waiting job, by ID: the running one, and those before it in the order the queue runs them (jobQueue.order). A waiting job the queue has not taken in yet, as one read back after a restart before the window is ready, is not counted.
func (a *app) queueAhead() map[string]int {
	ahead := map[string]int{}
	n := 0
	if a.queue.active != "" {
		n = 1
	}
	for _, id := range a.queue.order {
		if job, ok := a.library.Find(id); ok && job.Stage == core.StageQueued && id != a.queue.active {
			ahead[id] = n
			n++
		}
	}
	return ahead
}

// libraryEnqueue adds sources to the queue once precheck passes, and says in the message bar what happened. It reports whether any source was added, a failed job of the same video queued again included, or a playlist or a channel among them opened the dialog that lists its videos (playlistAsk), which adds them only once asked. A video the library already holds is not added; the bar says so and offers to open it.
func (a *app) libraryEnqueue(sources ...string) bool {
	videos, lists := a.playlistSplit(sources)
	msg, section := a.precheckReady()
	if len(videos) > 0 || len(lists) == 0 {
		msg, section = a.precheck(videos)
	}
	if msg != "" {
		a.warn(msg, section)
		a.lib.noticeReady = a.precheckAboutApp(msg)
		return false
	}
	added := len(videos) > 0 && a.libraryQueue(videos)
	if len(lists) > 0 {
		a.playlistAsk(lists, playlistFromLibrary)
		return true
	}
	return added
}

// libraryQueue adds sources, which precheck let through, to the queue and says in the message bar what happened, as libraryEnqueue does. A video of a YouTube playlist added alone, as watch?v=…&list=… is, is that video, and the bar offers to add the whole playlist (core.Link.WholeList); it then stays until it is closed, rather than closing by itself.
func (a *app) libraryQueue(sources []string) bool {
	added, err := a.queueEnqueue(sources)
	var dup *queueDuplicate
	m := a.messages()
	switch {
	case err == nil && added > 0 && a.untranslatable():
		a.warn(m.LibraryAddedUntranslated(), settings.SectionAI)
	case err != nil && added > 0:
		a.warn(m.LibraryAddedSome(added, sourceError(m, err)), "")
	case errors.As(err, &dup) && dup.Stage == core.StageComplete:
		a.warn(sourceError(m, err), "")
		a.lib.notice.Film = dup.ID
	case err != nil:
		a.warn(sourceError(m, err), "")
	case added > 1:
		a.tell(m.LibraryAddedStarting(added))
	default:
		a.tell(m.LibraryAddedOneStarting())
		if len(sources) == 1 {
			a.lib.notice.List = a.playlistWhole(sources[0])
		}
	}
	return added > 0
}

// dropFiles adds every video among files and folders dropped on the window, picked with 本地视频 or opened with Kando, to the queue: a folder adds the videos in it and in its folders, in their names' order (films.DroppedVideos). Files that are not videos are left out, and the message bar says how many after what was added; when none is a video, it says what can be added. Files dropped on another page or over an open film, or picked from there with ⌘O, take the window to the library, whose message bar and queue show what became of them.
func (a *app) dropFiles(paths []string) {
	if a.page != pageFilms || a.openKey != "" {
		a.Select(pageFilms)
	}
	videos, skipped := films.DroppedVideos(paths, a.prefs.interfaceLanguage)
	if len(videos) == 0 {
		a.warn(a.messages().PrecheckNoVideo(), "")
		return
	}
	sources := make([]string, len(videos))
	for i, path := range videos {
		sources[i] = core.FileURL(path)
	}
	if a.libraryEnqueue(sources...) && skipped > 0 {
		a.lib.notice.Text = a.messages().LibrarySkippedNotVideos(a.lib.notice.Text, skipped)
	}
}

// libraryKeys are the library's shortcuts. ⌘F searches the library's films in the sidebar's search field (sidebarSearch). It shares ⌘F with the player's 搜索字幕: this row holds only while no film is open and that one only while one is, so one of them at most runs it, and the menu bar shows ⌘F by that one (menuSharedKeyCarriers, menu.go).
func (a *app) libraryKeys(cx *ns.Scope) []shortcut {
	return []shortcut{
		a.libraryOpenVideos(cx),
		{Group: groupEdit, Label: locales.LibrarySearchMenu(cx), Key: "mod+f", When: a.sidebarSearchable, Run: a.sidebarSearch},
	}
}

// libraryOpenVideosKey is the key that asks for local videos, ⌘O, which the tooltip of 本地视频 shows (PickVideosShortcut).
const libraryOpenVideosKey = "mod+o"

// libraryOpenVideos is the shortcut that asks for local videos: its row of the File menu, and the shortcut the tooltip of 本地视频 shows (PickVideosShortcut).
func (a *app) libraryOpenVideos(cx *ns.Scope) shortcut {
	return shortcut{Group: groupFile, Label: locales.LibraryOpenVideosMenu(cx), Key: libraryOpenVideosKey, Run: a.pickVideos}
}
