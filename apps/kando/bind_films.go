package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/addsheet"
	"kando/apps/kando/desktop"
	"kando/apps/kando/films"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
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
	file, err := films.LibraryFolder(a.messages(), runtime.GOOS, a.snap().LibraryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(a.libraryPlace(), file), nil
}

// Start adds the links in the top bar to the queue, several when what was pasted holds several, between spaces or on lines of their own; a playlist or a channel among them opens the dialog that lists its videos (playlistAsk). The field empties once they are added; otherwise the message bar says why and the links stay, to be fixed.
func (a *app) Start() {
	sources := strings.Fields(a.queue.source)
	// One link to an online video asks first, on the Add sheet (state_addsheet.go); anything else is added as it is.
	if len(sources) == 1 && a.addSheetAsk(sources[0], addsheet.FromField, "", 0) {
		a.queue.source = ""
		return
	}
	if a.libraryEnqueue(sources...) {
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
	// A link whose video has not come plays its sound, by the path the video will have (films.PlayPath).
	play := item
	play.VideoPath = films.PlayPath(item)
	if !a.player.playJump {
		a.player.playFrom, _ = films.ResumeFrom(a, play)
	}
	a.openKey = item.Key
	a.loadSubtitles(item.Key, play.VideoPath, item.SubtitlePath)
	a.startPlayback(play.VideoPath)
}

// OpenAt opens a film playing from at, as the hero's 播放, 从 X 继续 and 从头看 do. It keeps the film's state as a seek does, so the player's line loop is told (playerLoopOpening).
func (a *app) OpenAt(item films.Film, at time.Duration) {
	a.playerLoopOpening(item.Key, at)
	a.player.playFrom = at
	a.player.playJump = true
	a.Open(item)
}

// AskReprocess opens the confirmation of 重新处理 for items, a film or several selected, as processing a film again transcribes it and pays for its translation again. With no language to translate a single film into there is nothing to confirm: Reprocess says so at once; several are confirmed whatever their languages, and ReprocessFilms says once which could not be queued. The confirmation holds the window's modal flag until it closes (CloseReprocess).
func (a *app) AskReprocess(items ...films.Film) {
	if len(items) == 0 {
		return
	}
	if len(items) == 1 && a.libraryReprocessLanguage(items[0]) == "" {
		a.Reprocess(items[0])
		return
	}
	a.lib.reprocessing = slices.Clone(items)
	a.lib.reprocessOpen = true
	if a.lib.reprocessRelease == nil {
		a.lib.reprocessRelease = a.holdModal()
	}
}

// Reprocessing is the confirmation of 重新处理: whether it is open, the films it asks about and the language they would be translated into, "" when they would not all be translated into the same one.
func (a *app) Reprocessing() (*bool, []films.Film, string) {
	language := ""
	for i, item := range a.lib.reprocessing {
		switch next := a.libraryReprocessLanguage(item); {
		case i == 0:
			language = next
		case next != language:
			language = ""
		}
	}
	return &a.lib.reprocessOpen, a.lib.reprocessing, language
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
	// A film that was not translated at all has no translation to replace, whatever language its job was asked for.
	current := strings.TrimSpace(item.Language)
	if item.TranslationErr != "" {
		current = ""
	}
	if current == "" || current == language {
		a.libraryRetranslate(item, core.RedoTranslation, language)
		return
	}
	a.libraryAskRetranslate([]films.Film{item}, language)
}

// libraryAskRetranslate opens the confirmation of translating items into language, which replaces a translation in another language, holding the window's modal flag until it closes (CloseRetranslate).
func (a *app) libraryAskRetranslate(items []films.Film, language string) {
	a.lib.retranslating, a.lib.retranslateLanguage, a.lib.retranslateOpen = items, language, true
	if a.lib.retranslateRelease == nil {
		a.lib.retranslateRelease = a.holdModal()
	}
}

func (a *app) TargetLanguage() string { return strings.TrimSpace(a.prefs.language) }

func (a *app) Retranslating() (*bool, []films.Film, string) {
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

// Retranslate translates the films the confirmation of 重新翻译为 asked about into its language, and closes it.
func (a *app) Retranslate() {
	if !a.lib.retranslateOpen {
		return
	}
	items, language := a.lib.retranslating, a.lib.retranslateLanguage
	a.CloseRetranslate()
	if len(items) == 1 {
		a.libraryRetranslate(items[0], core.RedoTranslation, language)
		return
	}
	a.libraryRetranslateFilms(items, language)
}

// libraryRetranslateOverlay is the confirmation of 重新翻译为 over the window (overlays), so the film page asks too.
func (a *app) libraryRetranslateOverlay(*ns.Scope) ns.Node { return films.RetranslateDialog{App: a} }

// TranslateMissing translates only the lines a done film's translation left untranslated, as the card's 补译未译的句子 does. The film page's ⋯ uses it too (film.App).
func (a *app) TranslateMissing(item films.Film) { a.libraryRetranslate(item, core.RedoMissing, "") }

// TranslateFilms translates every line of each film of items that was not translated at all (films.NotTranslated), in their order, behind the jobs waiting, as its own 翻译 does (TranslateMissing) and the control socket's jobs.translate does (remoteTranslate), into the language the settings name now (queueRetranslatable). It moves neither the page nor the selected card, as RetryAll does not. One that cannot be queued, without a target language, says why in the message bar, once.
func (a *app) TranslateFilms(items []films.Film) {
	var first error
	for _, item := range films.NotTranslated(items) {
		job, ok := a.library.Find(item.JobID)
		if !ok || job.Stage != core.StageComplete || job.ID == a.queue.active || job.TranslationErr == "" {
			continue
		}
		ready, err := a.queueRetranslatable(job, core.RedoMissing, "")
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		delete(a.lib.quotes, job.SubtitlePath)
		a.queueAdd(ready)
	}
	if first != nil {
		a.warn(first.Error(), settings.SectionTranslate)
	}
}

// CanTranslate reports whether an AI service is set up to translate with (untranslatable), which Translate N Films waits for.
func (a *app) CanTranslate() bool { return !a.untranslatable() }

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

// ContinueHere lets job id, which a paired phone paused, wait for its turn again (queueLetGo).
func (a *app) ContinueHere(id string) { a.queueLetGo(id) }

// Retry runs a failed or finished job again on the same card. When it cannot be queued, the message bar says why: a job already running or waiting says nothing, as its card says so; otherwise the settings name no target language (queueRetryable), and the bar links to where it is set. The film page draws no message bar, so there the settings open too, as libraryRetranslate does.
func (a *app) Retry(id string) {
	job, ok := a.library.Find(id)
	if !ok {
		return
	}
	ready, err := a.queueRetryable(job)
	var stored *storedRefusal
	if errors.As(err, &stored) {
		a.warn(err.Error(), "")
		return
	}
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
	return jobfailure.Hint(a.messages(), item.Detail, item.Failure)
}

// TranslationHint reads why a done film translated none of its lines as failureHint reads a failed translation, so it says what to do in the same words a failed job's card would.
func (a *app) TranslationHint(item films.Film) (reason, section string) {
	if item.TranslationErr == "" {
		return "", ""
	}
	return jobfailure.TranslationHint(a.messages(), item.TranslationErr, item.TranslationSource)
}

func (a *app) JobFilm(id string) (films.Film, bool) {
	job, ok := a.library.Find(id)
	if !ok || a.lockHidesJob(job) {
		return films.Film{}, false
	}
	return storedMark([]films.Film{films.FromJob(job, a.libraryJobName(job), statusText(a.messages(), job))}, a.library.Stored)[0], true
}

func (a *app) Notice() *films.Notice { return &a.lib.notice }

func (a *app) QueueExpanded() *bool { return &a.lib.queueOpen }

func (a *app) LibrarySort() films.Order { return a.lib.sort }

// SetLibrarySort shows the done films in order and keeps the order in settings.json for the next launch; the message bar says so when it cannot be kept.
func (a *app) SetLibrarySort(order films.Order) {
	order = order.Valid()
	if order == a.lib.sort {
		return
	}
	a.lib.sort = order
	if err := a.saveSettings(); err != nil {
		a.warn(a.messages().LibrarySortSaveFailed(err.Error()), "")
	}
}

func (a *app) LibraryView() string { return a.lib.view }

// SetLibraryView shows the done films as a grid or a list and keeps the choice in settings.json for the next launch; the message bar says so when it cannot be kept.
func (a *app) SetLibraryView(view string) {
	view = films.ValidView(view)
	if view == a.lib.view {
		return
	}
	a.lib.view = view
	if err := a.saveSettings(); err != nil {
		a.warn(a.messages().LibraryViewSaveFailed(err.Error()), "")
	}
}

// AskRemove opens the confirmation of 从资料库移除 for items, a film or several selected, and reads now, rather than while the dialog is drawn, whether a single film's folder or only its files would go to the Trash. The confirmation holds the window's modal flag until it closes (libraryCloseRemove).
func (a *app) AskRemove(items ...films.Film) {
	if len(items) == 0 {
		return
	}
	a.lib.removing = slices.Clone(items)
	a.lib.removeFolder = false
	if len(items) == 1 {
		item := items[0]
		files := filmFiles(a.snap().LibraryDir, item)
		a.lib.removeFolder = len(files) == 1 && files[0] == filepath.Dir(item.VideoPath)
	}
	a.lib.removeOpen = true
	if a.lib.removeRelease == nil {
		a.lib.removeRelease = a.holdModal()
	}
}

func (a *app) Removing() (*bool, []films.Film, bool) {
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

// trashItem moves a file or a folder to the Trash, under the scope cx, and calls done once it went or could not. Tests replace it.
var trashItem = desktop.Trash

// filmFiles is what moving a film to the Trash moves: the folder of a film inside the library root, or, when that folder holds other videos or any folder too, or is the root itself, the video, its subtitles, the hidden files a job keeps beside it (media.Companions) and the media files of its name, its own sound (media.SameNamed), so none is left behind in the folder. A video outside the library has none.
func filmFiles(root string, item films.Film) []string {
	video := item.VideoPath
	if video == "" || !films.InDir(root, video) {
		return nil
	}
	dir := filepath.Dir(video)
	if films.InDir(root, dir) && films.AloneIn(dir, video) {
		return []string{dir}
	}
	// A film that keeps no video (core.Job.SubtitlesOnly) has none where its sound was, to go with its subtitles, nor has one whose video was moved to a storage location, whose copy there is removed as it leaves the shelf (storedForget).
	files := []string{video}
	if item.SubtitlesOnly || item.Stored != "" {
		files = nil
	}
	for _, sub := range append(append([]string{item.SubtitlePath}, media.Companions(video)...), media.SameNamed(video)...) {
		if sub != "" && fileExists(sub) && !slices.Contains(files, sub) {
			files = append(files, sub)
		}
	}
	return files
}

// films is the open library's films, none while it is locked (lockShut).
func (a *app) films() []films.Film {
	if a.lockShut() {
		return nil
	}
	shot := a.snap()
	return goneMark(storedMark(films.List(shot.LibraryDir, a.library.Jobs, shot.Videos, a.library.Names, jobName, a.statusOf), a.library.Stored), shot.Gone)
}

// goneMark sets Missing on each of items whose video is one of gone, the videos kept outside the library that are not on disk now (session.Gone).
func goneMark(items []films.Film, gone map[string]bool) []films.Film {
	if len(gone) == 0 {
		return items
	}
	for i := range items {
		if items[i].VideoPath != "" && gone[items[i].VideoPath] {
			items[i].Missing = true
		}
	}
	return items
}

// statusOf is what a job's card says of where it is, in the interface's language (statusText), as films.List and films.Queue take it.
func (a *app) statusOf(job core.Job) string { return statusText(a.messages(), job) }

// QueueFilms is the films of 处理中: the open library's jobs that are waiting, running, failed or were cancelled, and the jobs the queue runs or will run for other libraries (queueElsewhere), but for those of a locked library (lockVisible). A waiting one says how many jobs run before it (queueAhead), or that the queue is paused (queue_pause.go).
func (a *app) QueueFilms() []films.Film {
	items := films.Queue(a.snap().LibraryDir, a.lockVisible(a.library.Jobs), a.queueElsewhere, jobName, a.statusOf)
	ahead := a.queueAhead()
	for i := range items {
		items[i].Ahead = ahead[items[i].JobID]
		items[i].Paused = a.queue.paused && items[i].Stage == core.StageQueued
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
		if job, ok := a.library.Find(id); ok && job.Stage == core.StageQueued && !queueHeld(job) && id != a.queue.active {
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
	if added > 0 {
		a.sidebarShowQueued(sources)
	}
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

// libraryKeys are the library's shortcuts. ⌘F searches the library's films in the sidebar's search field (sidebarSearch). It shares ⌘F with the player's 搜索字幕: this row holds only while no film is open and that one only while one is, so one of them at most runs it, and the menu bar shows ⌘F by that one (menuSharedKeyCarriers, menu.go). File › 移到资料库 moves the open film to another library (libraryMoveRow). View › 为图标 and 为列表 show the done films as posters or as a list, and 排序方式 is the sort button's menu, as Finder's View menu has them; they have no key, ⌘1 and ⌘2 being Window › 资料库 and 播放器.
func (a *app) libraryKeys(cx *ns.Scope) []shortcut {
	view := func(v string) shortcut {
		label := locales.MenuViewAsIcons(cx)
		if v == films.ViewList {
			label = locales.MenuViewAsList(cx)
		}
		return shortcut{Group: groupView, Label: label, When: a.libraryShown, Checked: func() bool { return a.lib.view == v }, Run: func() { a.SetLibraryView(v) }}
	}
	return slices.Concat([]shortcut{
		a.libraryOpenVideos(cx),
		{Group: groupEdit, Label: locales.LibrarySearchMenu(cx), Key: "mod+f", When: a.sidebarSearchable, Run: a.sidebarSearch},
		a.libraryMoveRow(cx),
	}, a.librarySelectionKeys(cx), []shortcut{
		view(films.ViewGrid),
		view(films.ViewList),
		{Group: groupView, Label: locales.MenuViewSortBy(cx), When: a.libraryShown, Items: func() []nsdesktop.MenuItem {
			items := films.SortMenu(cx, a)
			for i := range items {
				if fn := items[i].OnSelect; fn != nil {
					items[i].OnSelect = a.menuSelect(false, fn)
				}
			}
			return items
		}},
	})
}

// libraryShown reports whether the window shows the library's films, so View › 为图标, 为列表 and 排序方式 have something to show: the library page, no film open over it, and a film in the library.
func (a *app) libraryShown() bool {
	return a.page != pageBrowse && a.openKey == "" && a.FilmCount() > 0
}

// libraryOpenVideosKey is the key that asks for local videos, ⌘O, which the tooltip of 本地视频 shows (PickVideosShortcut).
const libraryOpenVideosKey = "mod+o"

// libraryOpenVideos is the shortcut that asks for local videos: its row of the File menu, and the shortcut the tooltip of 本地视频 shows (PickVideosShortcut).
func (a *app) libraryOpenVideos(cx *ns.Scope) shortcut {
	return shortcut{Group: groupFile, Label: locales.LibraryOpenVideosMenu(cx), Key: libraryOpenVideosKey, Run: a.pickVideos}
}
