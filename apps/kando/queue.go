package main

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// jobQueue is the job being made and the jobs being run. The link field and the line under it, and the job the page follows, are the job being made; removed is the jobs removed while they ran. The queue runs the shelf's jobs one at a time, oldest first. The shelf (a.library) is the truth about each job's stage; the queue only keeps the order jobs were asked for and the one running now.
type jobQueue struct {
	source     string
	status     string
	selectedID string
	job        *core.Job
	removed    map[string]bool
	// order is the IDs of queued jobs in the order they were added or retried. A job that is no longer queued or no longer on the shelf is dropped when the queue looks for the next one.
	order []string
	// active is the ID of the running job, or "".
	active string
	// whisperModel is the whisper model file the running job took from the pipeline when it started, "" while none runs. The 语音识别 page counts it as in use, so it is not moved to the Trash under the job.
	whisperModel string
	// ran is the IDs of the jobs the queue has run to the end since launch, whichever library they saved into. jobs.list keeps listing them after they end even when another library is open (remoteQueue), so a client waiting on one sees how it ended rather than losing it.
	ran map[string]bool
	// cancel stops the running job; with errQuitting the job reports nothing more. done is closed once the running job's pipeline has returned.
	cancel context.CancelCauseFunc
	done   chan struct{}
	// stopped is set once the app quits (queueQuit): no job starts after it.
	stopped bool
	// quitAsk is the confirmation of a quit while a job runs (queue_quit.go); quitRelease gives back the window's modal flag it holds. quitConfirmed is set once the user chose to quit anyway, or a signal asked the app to quit (queueWatchSignals), so the quit that follows is not asked again.
	quitAsk       bool
	quitRelease   func()
	quitConfirmed bool
	// ytdlp is the yt-dlp the jobs download with and the browsers it can borrow logins from (queue_remedy.go).
	ytdlp ytdlpState
	// linkFocus asks the link field above the library for the keyboard focus once, as 换个链接 does.
	linkFocus bool
	// keepAwake is mygo.Power.KeepAwake once the window is ready (queueHooks), and nil before and in tests, which must not hold the machine awake; awake gives back the hold it took while a job runs, nil while there is none (queueKeepAwake).
	keepAwake func(reason string, display bool) (release func())
	awake     func()
	// sys is what the queue tells the system outside the window: notifications, and the badge and progress bar on the app's icon (queue_desktop.go).
	sys queueSystem
}

func newJobQueue() jobQueue { return jobQueue{sys: newQueueSystem()} }

// stopActive asks the running job to stop. It ends, and the next one starts, when its pipeline returns.
func (q *jobQueue) stopActive(cause error) {
	if q.cancel != nil {
		q.cancel(cause)
	}
}

// savedQueue is the queue's part of settings.json. The jobs themselves are in the shelf's library.json. Notify is false when the user turned off the notifications of jobs that end while the app is in the background, and nil while they are on, as they are by default.
type savedQueue struct {
	Notify *bool `json:"notify,omitempty"`
}

func (s *savedQueue) clean() {}

func (a *app) saveQueue(saved *savedSettings) {
	saved.savedQueue = savedQueue{}
	if !a.queue.sys.notify {
		off := false
		saved.savedQueue.Notify = &off
	}
}

// restoreQueue takes back whether the user wants notifications, and listens for clicks on them before App.Run (queueListenForClicks).
func (a *app) restoreQueue(saved savedSettings) {
	if saved.Notify != nil {
		a.queue.sys.notify = *saved.Notify
	}
	a.queueListenForClicks()
}

// queueHooks: opening another library does not stop the running job, which goes on saving into its own library (run); a job queued in the library opened starts once nothing runs. A quit while a job runs asks first, unless a signal asked for it, and the job still running when the app quits is stopped, left on the shelf as it stood so the next launch runs it again (queue_quit.go). Jobs the app quit in the middle of were read back as queued; they run once the window is ready, so their background updates have a window to land in. The queue starts telling the system about its jobs then too, and removes its notifications whenever the app comes to the front (queue_desktop.go). The version of yt-dlp, which the card of a download it was too old for names, and the browsers whose logins the cards offer to borrow are read then and whenever the tools change; when they were looked up again because the yt-dlp that card asked Kando to download is in, it is updated and the downloads it was too old for run again. The player's resume positions and subtitle delays are kept with the shelf: read when it is opened and saved when the player remembers one, and keyed by the new paths of videos that moved. From the window being ready, the computer does not sleep while a job runs, and the hold ends as the app quits.
func (a *app) queueHooks() areaHooks {
	return areaHooks{
		save:    a.saveQueue,
		restore: a.restoreQueue,
		ready: func() {
			a.queue.keepAwake = mygo.Power.KeepAwake
			a.queueDesktopReady()
			a.pump()
			a.queueKeepAwake()
			a.queueReadYTDLP()
			a.queueWatchSignals()
		},
		toolsChanged: func(redetected bool) {
			if redetected {
				a.queueRetryWithKit()
			}
			a.queueReadYTDLP()
		},
		libraryChanged: a.pump,
		shelfLoaded: func() {
			a.setResumePositions(maps.Clone(a.library.Positions))
			a.setSubtitleDelays(maps.Clone(a.library.Delays))
		},
		becameActive:     a.queueBecameActive,
		positionsChanged: a.savePositions,
		videosMoved: func(move func(string) (string, bool)) {
			shelf.MoveKeys(a.resumePositions(), move)
			shelf.MoveKeys(a.subtitleDelays(), move)
		},
		afterFrame: a.queueQuitModal,
		beforeQuit: a.queueBeforeQuit,
		quitting: func() {
			a.queueQuit()
			a.queueLetSleep()
		},
	}
}

// queueKeys are the queue's shortcuts.
func (a *app) queueKeys() []shortcut { return nil }

// errBusy was Enqueue's answer while a job ran, before jobs waited in a queue. Enqueue no longer returns it; it stays so code written against the one-job contract still builds. It is made when it is said, in the language shown then.
func errBusy() error { return errors.New(locales.QueueBusy()) }

// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name. The jobs run one after another, after those already waiting. A source that cannot be added is skipped and the others are still added; the error is why the first one was not. A video the library already has a job for is not added twice (queueTake), and a source naming the same video as one before it is left out.
func (a *app) Enqueue(sources ...string) error {
	_, err := a.queueEnqueue(sources)
	return err
}

// queueEnqueue is Enqueue, which also says how many of sources it queued: new jobs, and failed or cancelled jobs of the same videos queued again on their cards.
func (a *app) queueEnqueue(sources []string) (added int, err error) {
	if len(sources) == 0 {
		return 0, core.ErrEmptySource
	}
	seen := map[string]bool{}
	var first error
	for _, source := range sources {
		key := queueSourceKey(source)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := a.queueTake(source); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		added++
	}
	return added, first
}

// queueTake queues the job for source and returns its ID: a new job, or, when the open library already has a failed or cancelled job for the same video, that job again on its card (retry), so the shelf keeps one card for it. A video already waiting, running or done is not queued again; the error is then a *queueDuplicate naming its job.
func (a *app) queueTake(source string) (string, error) {
	job, err := a.newJob(source)
	var dup *queueDuplicate
	if errors.As(err, &dup) && dup.Stage == core.StageFailed {
		old, _ := a.library.Find(dup.ID)
		ready, rerr := a.queueRetryable(old)
		if rerr == nil {
			a.queueAdd(ready)
			return ready.ID, nil
		}
		if !errors.Is(rerr, errInQueue) {
			a.queue.status = rerr.Error()
		}
	}
	if err != nil {
		return "", err
	}
	a.queueAdd(job)
	return job.ID, nil
}

// queueDuplicate is why newJob makes no job for a source whose video the open library already has a job for: ID is that job, and Stage its stage, StageQueued for one that is running but has not returned from its pipeline yet. Its message is made when it is read, in the language shown then.
type queueDuplicate struct {
	ID    string
	Stage core.Stage
}

func (e *queueDuplicate) Error() string {
	if e.Stage == core.StageComplete {
		return locales.QueueAlreadyInLibrary()
	}
	return locales.QueueAlreadyQueued()
}

// queueFindSource is the job of the open library for the same video as source (queueSourceKey). A job waiting, running or done comes before a failed or cancelled one, so a video already processed is not queued again because an earlier try of it failed.
func (a *app) queueFindSource(source string) (core.Job, bool) {
	root := a.snap().LibraryDir
	key := queueSourceKey(source)
	var failed core.Job
	found := false
	for _, job := range a.library.Jobs {
		if !films.InLibrary(root, job) || queueSourceKey(job.Source) != key {
			continue
		}
		if job.Stage != core.StageFailed {
			return job, true
		}
		if !found {
			failed, found = job, true
		}
	}
	return failed, found
}

// queueSourceKey is what two sources of the same video have in common: the platform and the video's ID that core.ParseLink reads, with the part of a bilibili video in several parts, so youtu.be/x and youtube.com/watch?v=x&t=9 are one video; the cleaned path of a local file; and the link itself for a site whose videos ParseLink does not name, or the source as it is when it is not a link.
func queueSourceKey(source string) string {
	source = strings.TrimSpace(source)
	link, ok := core.ParseLink(source)
	switch {
	case !ok:
		return source
	case link.Platform == core.PlatformLocal:
		return string(link.Platform) + ":" + filepath.Clean(link.ID)
	case link.ID == "":
		return link.URL
	}
	key := string(link.Platform) + ":" + link.ID
	if link.Platform == core.PlatformBilibili {
		// A video without p= is its first part.
		if u, err := url.Parse(link.URL); err == nil {
			if part := u.Query().Get("p"); part != "" && part != "1" {
				key += "?p=" + part
			}
		}
	}
	return key
}

// Cancel stops job id if it is running, or takes it out of the queue if it is waiting. Its card stays, marked 已取消. A finished or unknown job is left alone.
func (a *app) Cancel(id string) {
	if id != "" && id == a.queue.active {
		a.queue.stopActive(context.Canceled)
		return
	}
	job, ok := a.library.Find(id)
	if !ok || job.Stage != core.StageQueued {
		return
	}
	a.queue.order = slices.DeleteFunc(a.queue.order, func(queued string) bool { return queued == id })
	job.Stage = core.StageFailed
	job.Err = context.Canceled
	a.show(job)
}

// RunningJob is the ID of the job being processed now, or "" when none is. A job waiting in the queue is not running.
func (a *app) RunningJob() string { return a.queue.active }

// newJob makes the job for source in the open library and the target language, or returns why it cannot. When the library already has a job for the same video (queueFindSource), it makes none and returns a *queueDuplicate naming that job.
func (a *app) newJob(source string) (core.Job, error) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return core.Job{}, errors.New(locales.QueueNoLibrary())
	}
	job, err := core.NewJob(source)
	if err != nil {
		return core.Job{}, err
	}
	language := strings.TrimSpace(a.prefs.language)
	if language == "" {
		return core.Job{}, errors.New(locales.QueueNoLanguage())
	}
	if old, ok := a.queueFindSource(job.Source); ok {
		stage := old.Stage
		// A running job that was stopped is marked failed before its pipeline returns; until then it cannot be retried and is still in the queue.
		if old.ID == a.queue.active && stage == core.StageFailed {
			stage = core.StageQueued
		}
		return core.Job{}, &queueDuplicate{ID: old.ID, Stage: stage}
	}
	job.Language = language
	return job, nil
}

// importLocal points the next job at a video already on disk and goes back to the library, closing the open film. The pipeline reads the video directly and does not download it.
func (a *app) importLocal(path string) {
	if !a.chooseLocal(path) {
		return
	}
	a.page = pageFilms
	a.closeFilm()
}

// chooseLocal puts the video at path in the link field as a file:// URL, or says it is not a video. It reports whether it did.
func (a *app) chooseLocal(path string) bool {
	if !media.IsVideoFile(path) {
		a.queue.status = locales.QueueNoVideoToImport()
		return false
	}
	a.queue.source = core.FileURL(path)
	a.queue.status = locales.QueueLocalChosen()
	return true
}

// startFilm processes a video that is already in the library and stays on it, with its picture still playing. A job that cannot be added closes the film.
func (a *app) startFilm(item films.Film) {
	if !a.chooseLocal(item.VideoPath) {
		return
	}
	a.page = pageFilms
	a.start()
	if a.queue.job == nil {
		a.closeFilm()
		return
	}
	a.openKey = a.queue.job.ID
	a.loadSubtitles(a.queue.job.ID, a.queue.job.SubtitlePath)
}

// savePositions writes the player's resume positions and subtitle delays into library.json with the jobs.
func (a *app) savePositions() {
	a.library.Positions = maps.Clone(a.resumePositions())
	a.library.Delays = maps.Clone(a.subtitleDelays())
	if err := a.library.Save(); err != nil {
		a.saveErr = shelfSaveErr(err, locales.QueueSaveFailed)
	}
}

func (a *app) remove(id string) {
	if a.queue.removed == nil {
		a.queue.removed = map[string]bool{}
	}
	a.queue.removed[id] = true
	if id == a.queue.active {
		a.queue.stopActive(context.Canceled)
	}
	if a.openKey == id {
		a.closeFilm()
	}
	kept := make([]core.Job, 0, len(a.library.Jobs))
	for _, job := range a.library.Jobs {
		if job.ID != id {
			kept = append(kept, job)
		}
	}
	a.library.Jobs = kept
	if a.queue.selectedID == id {
		a.newDraft()
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = shelfSaveErr(err, locales.QueueSaveFailed)
	}
	a.queueTellSystem()
}

// retry queues a failed, cancelled or finished job again under the same ID and goes to the library, where the card already on screen follows it. It resumes after the stages it finished (media.Run): the video it downloaded and the speech it transcribed are used again. A done film is processed again entirely but for its download, as Process Entire Film Again says: its speech is transcribed anew, into the target language the settings name now rather than the one it was translated into. A job still running or waiting is left alone; one without a language when the settings name none says so on the line under the link field.
func (a *app) retry(job core.Job) {
	a.queueAgain(a.queueRetryable(job))
}

// queueRetranslate queues done film job to be translated again from the subtitles it exported, without downloading or transcribing it (core.Job.Redo): with core.RedoTranslation every line, into the target language the settings name now (core.Job.RedoLanguage); with core.RedoMissing only the lines it left untranslated, into the language its other lines are in. The job keeps saying what the film's subtitles are until it has rewritten them, so one that fails or is stopped puts the film back as it was (queueRedoEnded). A job that is not done is left alone. It returns why it could not queue the job, which is when the settings name no target language.
func (a *app) queueRetranslate(job core.Job, redo core.Redo) error {
	if job.Stage != core.StageComplete || job.ID == a.queue.active {
		return nil
	}
	job.Redo, job.RedoLanguage = redo, ""
	language := strings.TrimSpace(job.Language)
	if redo == core.RedoTranslation || language == "" {
		language = strings.TrimSpace(a.prefs.language)
	}
	if language == "" {
		return errors.New(locales.QueueNoLanguage())
	}
	if redo == core.RedoTranslation {
		job.RedoLanguage = language
	} else {
		job.Language = language
	}
	a.queueAgain(job, nil)
	return nil
}

// queueRedoEnded is job, which was translating a done film again (core.Job.Redo) and has failed or been stopped, put back as the done film it was: its subtitles are still the ones it had, which the job still describes. The message bar says why the translation failed; a job the user cancelled, or one stopped by a switch of library, says nothing.
func (a *app) queueRedoEnded(job core.Job) core.Job {
	if !errors.Is(job.Err, context.Canceled) && !errors.Is(job.Err, core.ErrLibrarySwitched) {
		reason, section := failureHint(films.Film{Detail: statusText(job)})
		a.warn(locales.QueueRedoFailed(jobName(job), reason), section)
	}
	job.Stage = core.StageComplete
	job.Err, job.FailKind, job.Progress = nil, "", core.Progress{}
	job.Redo, job.RedoLanguage = "", ""
	return job
}

// queueAgain queues job, made ready by queueRetryable or queueRetranslate, again under the same ID and goes to the library, where the card already on screen follows it. err is why the job cannot be queued: errInQueue leaves everything as it is, and any other is said on the line under the link field.
func (a *app) queueAgain(job core.Job, err error) {
	if errors.Is(err, errInQueue) {
		return
	}
	if err != nil {
		a.queue.status = err.Error()
		return
	}
	a.page = pageFilms
	a.queue.selectedID = job.ID
	a.queueAdd(job)
}

// errInQueue is queueRetryable's answer for a job that is running or waiting already.
var errInQueue = errors.New("the job is already running or waiting in the queue")

// queueRetryable is job ready for queueAdd to queue again under the same ID, with the language the settings name when it has none, or why it cannot be: errInQueue for a job running or waiting already, or, in the interface's language, that no language is set. A done film is made ready to be processed again entirely, into the language the settings name now. It changes nothing, neither the page, the selected card nor the line under the link field, so the window's retry, queueTake (a video added again from kando add, MCP or the top bar of another page, queued where the user is) and the control socket's jobs.retry, which must not move the window, all start from it.
func (a *app) queueRetryable(job core.Job) (core.Job, error) {
	if job.ID == a.queue.active || job.Stage == core.StageQueued {
		return core.Job{}, errInQueue
	}
	if job.Stage == core.StageComplete {
		job.Transcribed = false
		job.Redo, job.RedoLanguage = "", ""
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if job.Language == "" {
		return core.Job{}, errors.New(locales.QueueNoLanguage())
	}
	return job, nil
}

func (a *app) newDraft() {
	a.queue.selectedID = ""
	a.queue.job = nil
	a.queue.source = ""
	a.queue.status = ""
}

// start queues the source typed on the library page. Its card is the one the page's status follows.
func (a *app) start() {
	id, err := a.queueTake(a.queue.source)
	if err != nil {
		a.queue.job = nil
		a.queue.status = err.Error()
		return
	}
	a.queue.selectedID = id
	if job, ok := a.library.Find(id); ok {
		a.queue.job = &job
		a.queue.status = statusText(job)
	}
}

// queueAdd puts job on the shelf as queued, behind the jobs already waiting, and starts it if nothing is running.
func (a *app) queueAdd(job core.Job) {
	job.Stage = core.StageQueued
	job.Err = nil
	job.FailKind = ""
	// A job translating a done film again keeps the film's completion until it completes again (core.Job.Redo).
	if job.Redo == "" {
		job.CompletedAt = time.Time{}
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	a.queue.order = append(a.queue.order, job.ID)
	a.show(job)
	a.pump()
}

// pump starts the oldest queued job when no job is running. Each job saves into its own library (run), so a job queued in a library that is not open runs too, as long as that library is one the app still knows (queueCanRun). A queued job on the shelf that the queue has not seen, such as one read back as queued after a restart, joins the end of the queue, oldest first.
func (a *app) pump() {
	if a.queue.active != "" || a.queue.stopped {
		return
	}
	for i := len(a.library.Jobs) - 1; i >= 0; i-- {
		job := a.library.Jobs[i]
		if job.Stage == core.StageQueued && !slices.Contains(a.queue.order, job.ID) {
			a.queue.order = append(a.queue.order, job.ID)
		}
	}
	var next core.Job
	found := false
	kept := make([]string, 0, len(a.queue.order))
	for _, id := range a.queue.order {
		job, ok := a.library.Find(id)
		if !ok || job.Stage != core.StageQueued {
			continue
		}
		if !found && a.queueCanRun(job) {
			next, found = job, true
			continue
		}
		kept = append(kept, id)
	}
	a.queue.order = kept
	if found {
		a.run(next)
	}
	a.queueTellSystem()
}

// queueCanRun reports whether the queue can start job now that it is its turn: its library is the open one or another the app knows. A job waiting in a library the app has forgotten does not start until that library is opened again, so no new work begins in a folder the user took off the list. A job already running when its library is forgotten goes on into it: forgetting leaves the folder on disk, and a running job is never stopped for a library change. A job saved before jobs named their library runs in whichever library is open.
func (a *app) queueCanRun(job core.Job) bool {
	if job.LibraryDir == "" {
		return strings.TrimSpace(a.snap().LibraryDir) != ""
	}
	dir := filepath.Clean(job.LibraryDir)
	return slices.ContainsFunc(a.knownLibraries(), func(known string) bool { return filepath.Clean(known) == dir })
}

// queueElsewhere reports whether job is one the queue runs, or will run, for a library that is not the open one: the running job, wherever it saves, or a job waiting for its turn that pump can start (queueCanRun). The open library's page shows these in 处理中 beside its own, so a job keeps its card after another library is opened.
func (a *app) queueElsewhere(job core.Job) bool {
	if a.queue.removed[job.ID] || films.InLibrary(a.snap().LibraryDir, job) {
		return false
	}
	if job.ID == a.queue.active {
		return true
	}
	return job.Stage == core.StageQueued && a.queueCanRun(job)
}

// run starts job in the background. It keeps job.ID, so a retry updates the card already on screen. The job saves into its own library, job.LibraryDir, whichever library is open meanwhile: opening another one does not stop it. When the pipeline returns, the next queued job starts.
func (a *app) run(job core.Job) {
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	// Jobs written by older versions, or jobs interrupted before their language
	// was saved, can come back from the shelf without a target language. Use the
	// current translation setting before handing the job to the pipeline; an
	// empty language makes ChatTranslator reject the whole translation stage.
	if strings.TrimSpace(job.Language) == "" {
		if language := strings.TrimSpace(a.prefs.language); language != "" {
			job.Language = language
			a.show(job)
		}
	}
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.Translator.Prompt = a.prefs.translatePrompt
		tools.Translator.Context = a.prefs.translateContext
		a.pipeline = tools
	}
	// A job translating every line of a done film again translates into the language it was asked for, which becomes its Language once it is done.
	language := job.Language
	if job.Redo == core.RedoTranslation && strings.TrimSpace(job.RedoLanguage) != "" {
		language = job.RedoLanguage
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan struct{})
	a.queue.active, a.queue.cancel, a.queue.done = job.ID, cancel, done
	a.queueKeepAwake()
	pipeline, update := a.queuePipeline(job), a.update
	a.queue.whisperModel = ""
	if tools, ok := pipeline.(media.Tools); ok {
		a.queue.whisperModel = tools.WhisperModel
	}
	go func() {
		defer cancel(nil)
		defer close(done)
		// A job stopped because the app quits reports nothing more: its card would say 已取消 and keep it from running again at the next launch, and the next job must not start.
		quitting := func() bool { return errors.Is(context.Cause(ctx), errQuitting) }
		// A report that only moves the progress waits in latest, and at most one update carries it to the UI thread: a tool that reports many times a second does not queue an update for each, and the card shows the newest when the window gets to it. Any other report goes as it comes, after the progress already waiting.
		var latest atomic.Pointer[core.Job]
		last := job
		media.Run(ctx, pipeline, job, language, func(j core.Job) {
			if quitting() {
				return
			}
			onlyProgress := j.Stage == last.Stage && j.Title == last.Title && j.Thumbnail == last.Thumbnail && j.VideoPath == last.VideoPath && j.Err == nil
			last = j
			if !onlyProgress {
				update(func() { a.show(j) })
				return
			}
			if latest.Swap(&j) == nil {
				update(func() {
					if p := latest.Swap(nil); p != nil {
						a.showProgress(*p)
					}
				})
			}
		})
		if quitting() {
			return
		}
		update(func() { a.finished(job.ID) })
	}()
}

// queuePipeline is the pipeline job runs with: the app's, borrowing the browser login the job names and saving into the job's own library rather than the one open, so a job goes on into its library after another is opened.
func (a *app) queuePipeline(job core.Job) media.Pipeline {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return a.pipeline
	}
	tools.Cookies = job.Cookies
	if job.LibraryDir != "" {
		tools.OutputDir = job.LibraryDir
	}
	return tools
}

// finished clears the job that just returned from the pipeline, remembers that the queue ran it (jobQueue.ran), says in the message bar how a job of another library ended (queueEndedElsewhere), and starts the next one, letting the computer sleep again when there is none.
func (a *app) finished(id string) {
	if a.queue.active == id {
		a.queue.active, a.queue.cancel, a.queue.done, a.queue.whisperModel = "", nil, nil, ""
	}
	if a.queue.ran == nil {
		a.queue.ran = map[string]bool{}
	}
	a.queue.ran[id] = true
	a.queueEndedElsewhere(id)
	a.pump()
	a.queueKeepAwake()
}

// queueKeepAwake keeps the computer from sleeping while a job runs, so a long download or transcription is not cut off by idle sleep, and lets it sleep again once none does. The display may still turn off. One hold covers jobs that run one after another. It does nothing until queueHooks' ready has set keepAwake, so tests, which never run ready, take no hold on the machine running them.
func (a *app) queueKeepAwake() {
	switch {
	case a.queue.active != "" && a.queue.awake == nil && a.queue.keepAwake != nil:
		a.queue.awake = a.queue.keepAwake(locales.QueueKeepAwake(), false)
	case a.queue.active == "":
		a.queueLetSleep()
	}
}

// queueLetSleep gives back the hold that keeps the computer awake, if the queue has one.
func (a *app) queueLetSleep() {
	if a.queue.awake != nil {
		a.queue.awake()
		a.queue.awake = nil
	}
}

// queueEndedElsewhere tells, in the library's message bar, that job id, which saved into a library that is not open, is done or has failed: its card leaves 处理中 then, and only its own library shows it. A job that was cancelled or removed says nothing.
func (a *app) queueEndedElsewhere(id string) {
	job, ok := a.library.Find(id)
	if !ok || a.queue.removed[id] || job.LibraryDir == "" || films.InLibrary(a.snap().LibraryDir, job) {
		return
	}
	library := films.LibraryName(job.LibraryDir)
	switch {
	case job.Stage == core.StageComplete:
		a.tell(locales.QueueDoneElsewhere(jobName(job), library))
	case job.Stage == core.StageFailed && !errors.Is(job.Err, context.Canceled):
		a.warn(locales.QueueFailedElsewhere(jobName(job), library), "")
	}
}

// stop cancels the running job. The queue goes on with the next one.
func (a *app) stop() {
	a.queue.stopActive(context.Canceled)
}

// running reports whether a job is being processed now. Jobs still waiting in the queue do not count.
func (a *app) running() bool {
	return a.queue.active != ""
}

func (a *app) show(job core.Job) {
	if a.queue.removed[job.ID] {
		return
	}
	was, _ := a.library.Find(job.ID)
	// A job that has just completed again under its subtitle path, as one translating its film again does, has rewritten the file the open film shows.
	rewritten := job.Stage == core.StageComplete && was.Stage != "" && was.Stage != core.StageComplete
	// A redo that failed is notified as the failure it was, though the film goes back to the done grid.
	ended := job
	if job.Stage == core.StageFailed && job.Redo != "" {
		job = a.queueRedoEnded(job)
	}
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = shelfSaveErr(err, locales.QueueSaveFailed)
	} else {
		a.saveErr = ""
	}
	if a.queue.selectedID == job.ID {
		a.queue.job = &job
		a.queue.status = statusText(job)
	}
	if a.openKey == job.ID {
		if rewritten && a.player.subs.key == job.ID {
			a.player.subs.path = ""
		}
		a.loadSubtitles(job.ID, job.SubtitlePath)
	}
	a.areasShelfChanged()
	a.queueNotifyEnded(was.Stage, ended)
	a.queueTellSystem()
}

// showProgress puts how far job has gone on its card. Only its progress changed since show last saw it, which library.json does not keep, so the shelf is not saved and the areas are not told its films changed. A job removed meanwhile, or one that has moved on to another stage since, is left alone.
func (a *app) showProgress(job core.Job) {
	if a.queue.removed[job.ID] {
		return
	}
	shown, ok := a.library.Find(job.ID)
	if !ok || shown.Stage != job.Stage {
		return
	}
	shown.Progress = job.Progress
	a.library.Upsert(shown)
	if a.queue.selectedID == job.ID {
		a.queue.job = &shown
	}
	a.queueTellSystem()
}

func statusText(job core.Job) string {
	switch job.Stage {
	case core.StageQueued:
		return locales.QueueStatusQueued()
	case core.StageDownloading:
		return locales.QueueStatusDownloading()
	case core.StageTranscribing:
		return locales.QueueStatusTranscribing()
	case core.StageTranslating:
		return locales.QueueStatusTranslating()
	case core.StageExporting:
		return locales.QueueStatusExporting()
	case core.StageComplete:
		return locales.QueueStatusComplete(job.SubtitlePath)
	case core.StageFailed:
		if errors.Is(job.Err, context.Canceled) {
			return locales.QueueStatusCanceled()
		}
		if errors.Is(job.Err, core.ErrLibrarySwitched) {
			return locales.QueueStatusLibrarySwitched()
		}
		return locales.QueueStatusFailed(job.Err.Error())
	}
	return string(job.Stage)
}
