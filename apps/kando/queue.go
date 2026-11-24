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

	"kando/apps/kando/desktop"
	"kando/apps/kando/films"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// jobQueue is the job being made and the jobs being run. The link field and the line under it, and the job the page follows, are the job being made; removed is the jobs removed while they ran. The queue runs the shelf's jobs one at a time, oldest first. The shelf (a.library) is the truth about each job's stage; the queue only keeps the order jobs were asked for and the one running now.
type jobQueue struct {
	// batching is set while many jobs are queued at once, as a large drop's 全部生成字幕 queues them (libraryGenerateAdded): show leaves the shelf unsaved, and the one queueing saves it once they are all queued.
	batching   bool
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
	// redoEnded is the jobs that were processing or translating a done film again (core.Job.Redo) and failed or were stopped since they were last queued, as they ended, by ID. Each film is the done film it was (queueRedoEnded), so it is not said to be done anew (queueEndedElsewhere), and the film page says why its redo failed (RedoFailure), as it draws no message bar.
	redoEnded map[string]core.Job
	// refills is the jobs translating the lines a film left untranslated that the queue added by itself (queueRefill), by ID, while they wait or run: they end quietly. refilled is the films it added once since launch, so a film whose service fails again is not added again and again.
	refills, refilled map[string]bool
	// folderEnded is the jobs that were processing a video found in the library's folder (core.Job.FromFolder) and failed or were stopped since launch, as they ended, by ID. They left the shelf, the video back as it was (queueFolderEnded), so the control socket still answers how each ended to a client that follows it (remoteJob), rather than that it is gone.
	folderEnded map[string]core.Job
	// ran is the IDs of the jobs the queue has run to the end since launch, whichever library they saved into. jobs.list keeps listing them after they end even when another library is open (remoteQueue), so a client waiting on one sees how it ended rather than losing it.
	ran map[string]bool
	// cancel stops the running job; with errQuitting the job reports nothing more. done is closed once the running job's pipeline has returned.
	cancel context.CancelCauseFunc
	done   chan struct{}
	// watch is where the film the running job makes is watched (queuePlaying), nil while none runs.
	watch *queueWatch
	// preferred is the film last watched whose job was put first (queuePreferOpen); queueAdd forgets it when that job is queued again.
	preferred string
	// viewers are the paired phones watching a film being made, by device ID, and desk this computer's player as last seen, whose claims say which film is watched (queue_watched.go); lease looks at them again once a phone's claim stops counting. clock is a test's clock, nil for the time.
	viewers map[string]queueViewer
	desk    queueDesk
	lease   *time.Timer
	clock   func() time.Time
	// cancelled is set once the running job was asked to stop for good (stopActive with context.Canceled: Cancel, Withdraw, removing it), and cleared as the next one starts. A job the queue's pause stopped first then ends cancelled rather than going back to the queue (queuePausedJob), as the context keeps only the first cause it was stopped with.
	cancelled bool
	// stopped is set once the app quits (queueQuit): no job starts after it.
	stopped bool
	// paused is set while the user paused the whole queue (queue_pause.go): the job running then went back to the head of the queue, and no job starts until it is resumed. It lasts until Kando quits.
	paused bool
	// quitAsk is the confirmation of a quit while a job runs (queue_quit.go); quitRelease gives back the window's modal flag it holds. quitConfirmed is set once the user chose to quit anyway, or a signal asked the app to quit (queueWatchSignals), so the quit that follows is not asked again.
	quitAsk       bool
	quitRelease   func()
	quitConfirmed bool
	// ytdlp is the yt-dlp the jobs download with and the browsers it can borrow logins from (queue_remedy.go).
	ytdlp ytdlpState
	// linkFocus asks the link field above the library for the keyboard focus once, as 换个链接 does.
	linkFocus bool
	// keepAwake is desktop.KeepAwake once the window is ready (queueHooks), and nil before and in tests, which must not hold the machine awake; awake gives back the hold it took while a job runs, nil while there is none (queueKeepAwake).
	keepAwake func(reason string, display bool) (release func())
	awake     func()
	// sys is what the queue tells the system outside the window: notifications, and the badge and progress bar on the app's icon (queue_desktop.go).
	sys queueSystem
	// stamp, while it is set, gives each job queueTakeFrom queues, new or a failed one queued again, what the user chose for its link before it runs: the Add sheet's choices, and where the browser that sent it was (state_addsheet.go). It is set only around one call that queues, and nil otherwise.
	stamp func(job *core.Job)
}

func newJobQueue() jobQueue { return jobQueue{sys: newQueueSystem()} }

// stopActive asks the running job to stop. It ends, and the next one starts, when its pipeline returns.
func (q *jobQueue) stopActive(cause error) {
	if q.cancel != nil {
		q.cancelled = q.cancelled || errors.Is(cause, context.Canceled)
		q.cancel(cause)
	}
}

// savedQueue is the queue's part of settings.json. The jobs themselves are in the shelf's library.json. Notify is false when the user turned off the notifications of jobs that end while the app is in the background, and nil while they are on, as they are by default.
type savedQueue struct {
	Notify *bool `json:"notify,omitempty"`
	// YTDLPUpdated is when the daily update last brought Kando's own yt-dlp up to date (queueFreshenYTDLP).
	YTDLPUpdated time.Time `json:"ytdlp_updated,omitzero"`
}

func (s *savedQueue) clean() {}

func (a *app) saveQueue(saved *savedSettings) {
	saved.savedQueue = savedQueue{YTDLPUpdated: a.queue.ytdlp.freshened}
	if !a.queue.sys.notify {
		off := false
		saved.savedQueue.Notify = &off
	}
}

// restoreQueue takes back whether the user wants notifications. The clicks on them are listened for once the app runs (queueDesktopReady), which still hears the click that launched it.
func (a *app) restoreQueue(saved savedSettings) {
	if saved.Notify != nil {
		a.queue.sys.notify = *saved.Notify
	}
	a.queue.ytdlp.freshened = saved.YTDLPUpdated
}

// queueHooks: opening another library does not stop the running job, which goes on saving into its own library (queueRun); a job queued in the library opened starts once nothing runs. A quit while a job runs asks first, unless a signal asked for it, and the job still running when the app quits is stopped, left on the shelf as it stood so the next launch runs it again (queue_quit.go). Jobs the app quit in the middle of were read back as queued; they run once the window is ready, so their background updates have a window to land in. The queue starts telling the system about its jobs then too, and removes its notifications whenever the app comes to the front (queue_desktop.go). The version of yt-dlp, which the card of a download it was too old for names, and the browsers whose logins the cards offer to borrow are read then and whenever the tools change; when they were looked up again because the yt-dlp that card asked Kando to download is in, it is updated and the downloads it was too old for run again. The player's resume positions, when each video last played and its subtitle delays are kept with the shelf: read when it is opened and saved when the player remembers one, and keyed by the new paths of videos that moved. From the window being ready, the computer does not sleep while a job runs, and the hold ends as the app quits.
func (a *app) queueHooks() areaHooks {
	return areaHooks{
		save:    a.saveQueue,
		restore: a.restoreQueue,
		playing: a.queuePlaying,
		ready: func() {
			// The hold is taken under the app's scope, so it outlives the handler that started the job, as Enqueue from a button or the drop zone does.
			a.queue.keepAwake = func(reason string, display bool) (release func()) {
				a.cx.Run(func() { release = desktop.KeepAwake(a.cx, reason, display) })
				return release
			}
			a.queueDesktopReady()
			a.pump()
			a.queueKeepAwake()
			a.queueReadYTDLP()
			a.queueWatchSignals()
			a.queueFreshenYTDLP()
		},
		toolsChanged: func(redetected bool) {
			if redetected {
				a.queueRetryWithKit()
			}
			a.queueReadYTDLP()
			a.queueFreshenYTDLP()
		},
		libraryChanged: a.pump,
		llmTested: func(err error, _ bool) {
			if err == nil {
				a.queueRefill()
			}
		},
		shelfLoaded: func() {
			a.setResumePositions(maps.Clone(a.library.Positions))
			a.setPlayedTimes(maps.Clone(a.library.Played))
			a.setSubtitleDelays(maps.Clone(a.library.Delays))
			a.setSubtitleLifts(maps.Clone(a.library.Lifts))
		},
		becameActive:     a.queueBecameActive,
		positionsChanged: a.savePositions,
		videosMoved: func(move func(string) (string, bool)) {
			shelf.MoveKeys(a.resumePositions(), move)
			shelf.MoveKeys(a.playedTimes(), move)
			shelf.MoveKeys(a.subtitleDelays(), move)
			shelf.MoveKeys(a.subtitleLifts(), move)
		},
		afterChange: func() bool {
			quit, prefer := a.queueQuitModal(), a.queuePreferOpen()
			return quit || prefer
		},
		beforeQuit: a.queueBeforeQuit,
		quitting: func() {
			a.queueQuit()
			a.queueLetSleep()
		},
	}
}

// queueKeys are the queue's shortcuts.
func (a *app) queueKeys(cx *ns.Scope) []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.QueuePause(cx), Checked: func() bool { return a.queue.paused }, Run: a.queueTogglePause},
	}
}

// errBusy was Enqueue's answer while a job ran, before jobs waited in a queue. Enqueue no longer returns it; it stays so code written against the one-job contract still builds. It is made when it is said, in the language of m, the one shown then.
func errBusy(m locales.Messages) error { return errors.New(m.QueueBusy()) }

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
	// Nothing is added to a locked library.
	if a.lockShut() {
		msg, _ := a.precheckReady()
		return 0, errors.New(msg)
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

// queueTake queues the job for source and returns its ID: a new job, or, when the open library already has a failed or cancelled job for the same video, that job again on its card (retry), so the shelf keeps one card for it. A video already waiting, running or done is not queued again; the error is then a *queueDuplicate naming its job. A local video that is a film of the library's folder no job holds (films.List), as one dropped again from the library's own folder, is that film: with subtitles beside it, it is in the library already, and the *queueDuplicate names it, so its subtitles are neither paid for nor written again; without any, its job leaves it as it was if it fails or is stopped, as 生成字幕 does (queueFromFolder).
func (a *app) queueTake(source string) (string, error) {
	return a.queueTakeFrom(source, "", "", "")
}

// queueTakeFrom is queueTake for a link the paired phone named device sent, whose new job is that phone's from before it first runs (core.Job.Device), as the quality it downloads at depends on it (onlineFor), with the quality and the source of original lines the phone chose, each "" for the desktop's own. A failed job queued again keeps the ones it had.
func (a *app) queueTakeFrom(source, device string, quality core.Quality, subtitles core.SubtitleSource) (string, error) {
	// Offline mode adds no link, as it would be downloaded (state_privacy.go).
	if err := a.privacyRefusesLink(source); err != nil {
		return "", err
	}
	job, err := a.newJob(source)
	var dup *queueDuplicate
	if errors.As(err, &dup) && dup.Stage == core.StageFailed {
		old, _ := a.library.Find(dup.ID)
		ready, rerr := a.queueRetryable(old)
		if rerr == nil {
			if a.queue.stamp != nil {
				a.queue.stamp(&ready)
			}
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
	if path := queueLocalPath(job.Source); path != "" {
		if item, ok := films.Find(a.films(), path); ok && item.JobID == "" {
			if item.SubtitlePath != "" {
				return "", &queueDuplicate{ID: item.Key, Stage: core.StageComplete, m: a.messages()}
			}
			a.queueFolderJob(&job, item)
		}
	}
	job.Device = device
	if quality != "" {
		job.Quality = quality
	}
	if subtitles != "" {
		job.SubtitleSource = subtitles
	}
	if a.queue.stamp != nil {
		a.queue.stamp(&job)
	}
	a.queueAdd(job)
	return job.ID, nil
}

// queueLocalPath is the cleaned path of the video a local source names (a file:// URL), "" for a link.
func queueLocalPath(source string) string {
	link, ok := core.ParseLink(strings.TrimSpace(source))
	if !ok || link.Platform != core.PlatformLocal || link.ID == "" {
		return ""
	}
	return filepath.Clean(link.ID)
}

// queueDuplicate is why newJob makes no job for a source whose video the open library already has a job for: ID is that job, and Stage its stage, StageQueued for one that is running but has not returned from its pipeline yet. For a film of the library's folder that no job holds (queueTake), ID is the film's key and Stage StageComplete. Its message is in the language of m, the interface's when newJob made it.
type queueDuplicate struct {
	ID    string
	Stage core.Stage
	m     locales.Messages
}

func (e *queueDuplicate) Error() string {
	if e.Stage == core.StageComplete {
		return e.m.QueueAlreadyInLibrary()
	}
	return e.m.QueueAlreadyQueued()
}

// queueFindSource is the job of the open library for the same video as source (queueSourceKey), or, for a local video, the job whose video it is, as a downloaded film's video dropped on the window. A job waiting, running or done comes before a failed or cancelled one, so a video already processed is not queued again because an earlier try of it failed.
func (a *app) queueFindSource(source string) (core.Job, bool) {
	root := a.snap().LibraryDir
	key := queueSourceKey(source)
	video := queueLocalPath(source)
	same := func(job core.Job) bool {
		if queueSourceKey(job.Source) == key {
			return true
		}
		return video != "" && job.VideoPath != "" && filepath.Clean(job.VideoPath) == video
	}
	var failed core.Job
	found := false
	for _, job := range a.library.Jobs {
		if !films.InLibrary(root, job) || !same(job) || a.lockHidesJob(job) {
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
	job.PausedBy = ""
	a.show(job)
}

// Withdraw is the × of a card in 处理中: a waiting job that has done nothing yet is taken off the shelf, as if it had never been added (queueFresh); any other is cancelled and keeps its card (Cancel), so neither a download nor a film being processed again is lost to it.
func (a *app) Withdraw(id string) {
	if job, ok := a.library.Find(id); ok && job.Stage == core.StageQueued && id != a.queue.active && queueFresh(job) {
		a.remove(id)
		return
	}
	a.Cancel(id)
}

// queueFresh reports whether job has done nothing yet that removing it would lose: no video downloaded or found, no transcript, and no done film it is processing again.
func queueFresh(job core.Job) bool {
	return job.VideoPath == "" && !job.Transcribed && job.Redo == "" && job.CompletedAt.IsZero()
}

// queueFailures is the open library's failed jobs from source, oldest first, that a bulk action of 处理中 acts on: retryable those Retry All queues again, failed every one. source is the sidebar source the page shows, so a bulk action never touches a card 处理中 hides; sidebar.SourceAll takes those of every source. A cancelled job is neither.
func (a *app) queueFailures(source string) (retryable, failed []core.Job) {
	root := a.snap().LibraryDir
	for i := len(a.library.Jobs) - 1; i >= 0; i-- {
		job := a.library.Jobs[i]
		if job.Stage != core.StageFailed || errors.Is(job.Err, context.Canceled) || job.ID == a.queue.active || !films.InLibrary(root, job) {
			continue
		}
		if source != sidebar.SourceAll && films.SourceOf(films.Film{Source: job.Source}) != source {
			continue
		}
		failed = append(failed, job)
		if queueRetriesAlone(job.FailKind) {
			retryable = append(retryable, job)
		}
	}
	return retryable, failed
}

// queueRetriesAlone reports whether a download that failed for kind can be tried again with nothing changed: a failure no remedy is known for, as one past the download, or one the network, the site's name or its rate limit caused. The others wait for a choice of the user's, a login to borrow, another link or an update of yt-dlp, which their cards offer.
func queueRetriesAlone(kind core.FailKind) bool {
	switch kind {
	case "", core.FailNetwork, core.FailHost, core.FailRateLimit:
		return true
	}
	return false
}

func (a *app) Failures(source string) (retryable, failed int) {
	r, f := a.queueFailures(source)
	return len(r), len(f)
}

// RetryAll queues again, oldest first, every failed job of the open library from source that needs no choice of the user's (queueRetriesAlone). One that cannot be queued, as without a target language, says why in the message bar as Retry does, once.
func (a *app) RetryAll(source string) {
	retryable, _ := a.queueFailures(source)
	var first error
	for _, job := range retryable {
		ready, err := a.queueRetryable(job)
		if err != nil {
			if first == nil && !errors.Is(err, errInQueue) {
				first = err
			}
			continue
		}
		a.queueAdd(ready)
	}
	if first != nil {
		a.warn(first.Error(), settings.SectionTranslate)
	}
}

// RemoveFailed removes the card of every failed job of the open library from source; their downloads stay where they are.
func (a *app) RemoveFailed(source string) {
	_, failed := a.queueFailures(source)
	for _, job := range failed {
		a.remove(job.ID)
	}
}

// RunningJob is the ID of the job being processed now, or "" when none is. A job waiting in the queue is not running.
func (a *app) RunningJob() string { return a.queue.active }

// newJob makes the job for source in the open library and the target language, or returns why it cannot. When the library already has a job for the same video (queueFindSource), it makes none and returns a *queueDuplicate naming that job.
func (a *app) newJob(source string) (core.Job, error) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return core.Job{}, errors.New(a.messages().QueueNoLibrary())
	}
	job, err := core.NewJob(source)
	if err != nil {
		return core.Job{}, err
	}
	language := strings.TrimSpace(a.prefs.language)
	if language == "" {
		return core.Job{}, errors.New(a.messages().QueueNoLanguage())
	}
	if old, ok := a.queueFindSource(job.Source); ok {
		stage := old.Stage
		// A running job that was stopped is marked failed before its pipeline returns; until then it cannot be retried and is still in the queue.
		if old.ID == a.queue.active && stage == core.StageFailed {
			stage = core.StageQueued
		}
		return core.Job{}, &queueDuplicate{ID: old.ID, Stage: stage, m: a.messages()}
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

// chooseLocal puts the film's file at path in the link field as a file:// URL, the film it is the sound of when a video of its name is beside it (media.FilmOf), or says it is not a video or a sound. It reports whether it did.
func (a *app) chooseLocal(path string) bool {
	film, ok := media.FilmOf(path)
	if !ok {
		a.queue.status = a.messages().QueueNoVideoToImport()
		return false
	}
	a.queue.source = core.FileURL(film)
	a.queue.status = a.messages().QueueLocalChosen()
	return true
}

// startFilm processes item, a video found in the library's folder that no job holds, on a card of its own that leaves the film as it was if the job fails or is stopped (queueFromFolder), as 生成字幕 and 重新处理 do from the library and the player. An open film stays open on the job, its picture still playing. When the job cannot be queued the message bar says why; the film page, which draws no message bar, opens the settings that fix it, as Retry does, or else closes the film so the library shows why. A film without subtitles that will have no translator says so too, as adding a video does (untranslatable): its subtitles will be the original lines.
func (a *app) startFilm(item films.Film) {
	m := a.messages()
	// A video moved to a storage location is not on disk to be transcribed; the film stays open (state_stored.go).
	if err := a.storedNeedsFile(m, item.VideoPath, films.Name(item), locales.Messages.StoredReprocess); err != nil {
		a.warn(err.Error(), "")
		return
	}
	id, msg, section := a.queueFromFolderChecked(item)
	if msg != "" {
		a.warn(msg, section)
		if a.openKey != "" && a.openKey == item.Key {
			if section != "" {
				a.OpenSettings(section)
			} else {
				a.closeFilm()
			}
		}
		return
	}
	if id == "" {
		return
	}
	if item.SubtitlePath == "" && a.untranslatable() {
		a.warn(m.LibraryAddedUntranslated(), settings.SectionAI)
	}
	if a.openKey != "" && (a.openKey == item.Key || a.openKey == id) {
		a.openKey = id
		job, _ := a.library.Find(id)
		a.loadSubtitles(id, job.VideoPath, job.SubtitlePath)
	}
}

// queueFromFolderChecked processes item, a video found in the library's folder that no job holds, on a card of its own (queueFromFolder), once precheck lets it through, as startFilm and processing several films again (ReprocessFilms) do, and returns the job's ID, or why it could not, with the settings section that fixes it.
func (a *app) queueFromFolderChecked(item films.Film) (id, msg, section string) {
	if err := a.storedNeedsFile(a.messages(), item.VideoPath, films.Name(item), locales.Messages.StoredReprocess); err != nil {
		return "", err.Error(), ""
	}
	if id, msg, section, linked := a.linkedQueue(item); linked {
		return id, msg, section
	}
	msg, section = a.precheck([]string{core.FileURL(item.VideoPath)})
	if msg != "" {
		return "", msg, section
	}
	id, err := a.queueFromFolder(item)
	var dup *queueDuplicate
	switch {
	case errors.As(err, &dup):
		return "", sourceError(a.messages(), err), ""
	case err != nil:
		return "", err.Error(), settings.SectionTranslate
	}
	return id, "", ""
}

// savePositions writes the player's resume positions, when each video last played and its subtitle delays and places into library.json with the jobs.
func (a *app) savePositions() {
	a.library.Positions = maps.Clone(a.resumePositions())
	a.library.Played = maps.Clone(a.playedTimes())
	a.library.Delays = maps.Clone(a.subtitleDelays())
	a.library.Lifts = maps.Clone(a.subtitleLifts())
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().QueueSaveFailed)
	}
}

func (a *app) remove(id string) {
	if a.queue.removed == nil {
		a.queue.removed = map[string]bool{}
	}
	a.queue.removed[id] = true
	delete(a.queue.redoEnded, id)
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
		a.saveErr = a.shelfSaveErr(err, a.messages().QueueSaveFailed)
	}
	a.queueTellSystem()
}

// retry queues a failed, cancelled or finished job again under the same ID and goes to the library, where the card already on screen follows it. It goes on with what the film's document lacks (media.Make): the video it downloaded and the speech it heard are used again. A done film is processed again entirely but for its download, as Process Entire Film Again says (core.RedoAll): its speech is transcribed anew, and translated into the language it was translated into (queueReprocessLanguage), and it stays the done film it was until the new subtitles are written, so a run that fails or is stopped puts it back as it was (queueRedoEnded). A job still running or waiting is left alone; one without a language when the settings name none says so on the line under the link field.
func (a *app) retry(job core.Job) {
	a.queueAgain(a.queueRetryable(job))
}

// queueRetranslate queues done film job to be translated again from the subtitles it exported, without downloading or transcribing it (core.Job.Redo): with core.RedoTranslation every line, into into, or the target language the settings name now when into is "" (core.Job.RedoLanguage); with core.RedoMissing only the lines it left untranslated, into the language its other lines are in, or for a film that translated none of them (core.Job.TranslationErr) the one the settings name now. The job keeps saying what the film's subtitles are until it has rewritten them, so one that fails or is stopped puts the film back as it was (queueRedoEnded). A job that is not done is left alone. It returns why it could not queue the job, which is when no target language is given or set.
func (a *app) queueRetranslate(job core.Job, redo core.Redo, into string) error {
	if job.Stage != core.StageComplete || job.ID == a.queue.active {
		return nil
	}
	ready, err := a.queueRetranslatable(job, redo, into)
	if err != nil {
		return err
	}
	a.queueAgain(ready, nil)
	return nil
}

// queueRetranslatable is done film job ready for queueAdd to translate again as queueRetranslate says, or why it cannot be: no target language is given or set. It changes nothing, neither the page nor the selected card, so the control socket's jobs.translate, which must not move the window, starts from it too.
func (a *app) queueRetranslatable(job core.Job, redo core.Redo, into string) (core.Job, error) {
	job.Redo, job.RedoLanguage = redo, ""
	language := strings.TrimSpace(job.Language)
	switch {
	case redo == core.RedoTranslation && strings.TrimSpace(into) != "":
		language = strings.TrimSpace(into)
	// A film that was not translated at all has no other lines to match: it is translated into the language the settings name now, as processing it again would be (queueReprocessLanguage), or else the one its job was asked for.
	case redo == core.RedoTranslation || language == "" || (job.TranslationErr != "" && strings.TrimSpace(a.prefs.language) != ""):
		language = strings.TrimSpace(a.prefs.language)
	}
	if language == "" {
		return core.Job{}, errors.New(a.messages().QueueNoLanguage())
	}
	if redo == core.RedoTranslation {
		job.RedoLanguage = language
	} else {
		job.Language = language
	}
	return job, nil
}

// queueRedoEnded is job, which was processing or translating a done film again (core.Job.Redo) and has failed or been stopped, put back as the done film it was: its subtitles are still the ones it had, which the job still describes, and so is the language heard in it (media.Make). The message bar says why the job failed, and the film page too while the film is open (jobQueue.redoEnded); a job the user cancelled, or one stopped by a switch of library, says nothing.
func (a *app) queueRedoEnded(job core.Job) core.Job {
	if a.queue.redoEnded == nil {
		a.queue.redoEnded = map[string]core.Job{}
	}
	a.queue.redoEnded[job.ID] = job
	if a.queue.refills[job.ID] {
		// One the queue added by itself says nothing: the film is as it was, and the user can still translate its lines left.
		quiet := job
		quiet.Err = nil
		a.queue.redoEnded[job.ID] = quiet
	} else if !errors.Is(job.Err, context.Canceled) && !errors.Is(job.Err, core.ErrLibrarySwitched) {
		m := a.messages()
		reason, section := jobfailure.Hint(m, statusText(m, job), jobfailure.Of(job))
		switch job.Redo {
		case core.RedoAll:
			a.warn(m.QueueReprocessFailed(jobName(job), reason), section)
		case core.RedoVideo:
			a.warn(m.QueueVideoFailed(jobName(job), reason), section)
		default:
			a.warn(m.QueueRedoFailed(jobName(job), reason), section)
		}
		a.libraryNoticeJob(job.ID, core.StageComplete)
	}
	// A film whose video was not downloaded still keeps none, as the user chose: the quality queueVideo asked the download for goes, so processing it again later does not keep a video either.
	if job.Redo == core.RedoVideo && job.SubtitlesOnly {
		job.Quality = core.QualitySubtitles
	}
	job.Stage = core.StageComplete
	job.Err, job.FailKind, job.Progress = nil, "", core.Progress{}
	job.Redo, job.RedoLanguage, job.RedoGen = "", "", ""
	return job
}

// queueFromFolder queues a job for item, a video found in the library's folder that no job holds (films.List), on a card of its own in the open library, and returns its ID. Like a job's 重新处理, it leaves the film as it was until the new subtitles are written (core.Job.FromFolder): the job starts from the video where it is, and a film with subtitles found beside it keeps them until then and is translated again into the language the settings name now (core.RedoAll), while one without any is made as a video added now is. A video the open library has a job for already, as one whose job failed before it found the video, is queued as Enqueue queues it (queueTake). It returns why it could not queue the job, which is when no target language is set or that job is waiting, running or done.
func (a *app) queueFromFolder(item films.Film) (string, error) {
	source := core.FileURL(item.VideoPath)
	job, err := a.newJob(source)
	var dup *queueDuplicate
	if errors.As(err, &dup) {
		return a.queueTake(source)
	}
	if err != nil {
		return "", err
	}
	a.queueFolderJob(&job, item)
	a.queueAgain(job, nil)
	return job.ID, nil
}

// queueFolderJob makes job, a new job for item, a video found in the library's folder that no job holds, the job that processes it as the folder's film it is (queueFromFolder).
func (a *app) queueFolderJob(job *core.Job, item films.Film) {
	job.FromFolder, job.VideoPath = true, item.VideoPath
	if item.SubtitlePath != "" {
		job.SubtitlePath = item.SubtitlePath
		job.Redo, job.RedoLanguage = core.RedoAll, job.Language
	}
	// The job holds the video from now on, so the folder's film leaves the shelf as it is queued: a film open on it moves to the job first, or it would be closed as one that left (closeFilmThatLeft).
	if a.openKey != "" && a.openKey == item.Key {
		a.openKey = job.ID
	}
}

// queueFolderEnded takes job, which was processing a video found in the library's folder (core.Job.FromFolder) and has failed or been stopped, off the shelf, so the folder's film is back as it was, with the subtitles found beside it, and an open film stays open on it; the control socket still answers how the job ended (jobQueue.folderEnded). The message bar says why the job failed; a job the user cancelled, or one stopped by a switch of library, says nothing.
func (a *app) queueFolderEnded(job core.Job) {
	if !errors.Is(job.Err, context.Canceled) && !errors.Is(job.Err, core.ErrLibrarySwitched) {
		m := a.messages()
		reason, section := jobfailure.Hint(m, statusText(m, job), jobfailure.Of(job))
		if job.Redo == core.RedoAll {
			a.warn(m.QueueReprocessFailed(a.libraryJobName(job), reason), section)
		} else {
			a.warn(m.QueueMakeSubtitlesFailed(a.libraryJobName(job), reason), section)
		}
	}
	// The film open on the job goes back to the folder's video before the job leaves, as remove closes a film open on a job it removes.
	following := a.openKey == job.ID && job.VideoPath != ""
	if following {
		a.openKey = job.VideoPath
	}
	a.remove(job.ID)
	if a.queue.folderEnded == nil {
		a.queue.folderEnded = map[string]core.Job{}
	}
	a.queue.folderEnded[job.ID] = job
	if following {
		if item, ok := films.Find(a.films(), job.VideoPath); ok {
			a.loadSubtitles(item.Key, item.VideoPath, item.SubtitlePath)
		} else {
			a.closeFilm()
		}
	}
	// The areas hear of the shelf as show tells them of a job it keeps.
	a.areasShelfChanged()
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

// queueRetryable is job ready for queueAdd to queue again under the same ID, with the language the settings name when it has none, or why it cannot be: errInQueue for a job running or waiting already, or, in the interface's language, that no language is set or, for a done film, that its video is on a storage location (a *storedRefusal). A done film is made ready to be processed again entirely (core.RedoAll), into the language queueReprocessLanguage picks, which becomes its Language once it is done. It changes nothing, neither the page, the selected card nor the line under the link field, so the window's retry, queueTake (a video added again from kando add, MCP or the top bar of another page, queued where the user is) and the control socket's jobs.retry, which must not move the window, all start from it.
func (a *app) queueRetryable(job core.Job) (core.Job, error) {
	if job.ID == a.queue.active || job.Stage == core.StageQueued {
		return core.Job{}, errInQueue
	}
	if job.Stage == core.StageComplete {
		// Transcribing again needs the video's sound, which is not on disk while the video is on a storage location (state_stored.go).
		if err := a.storedNeedsFile(a.messages(), job.VideoPath, a.libraryJobName(job), locales.Messages.StoredReprocess); err != nil {
			return core.Job{}, err
		}
		language := a.queueReprocessLanguage(job)
		if language == "" {
			return core.Job{}, errors.New(a.messages().QueueNoLanguage())
		}
		job.Transcribed = false
		job.Redo, job.RedoLanguage = core.RedoAll, language
		return job, nil
	}
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if job.Language == "" {
		return core.Job{}, errors.New(a.messages().QueueNoLanguage())
	}
	return job, nil
}

// queueReprocessLanguage is the language processing done film job again entirely translates it into: the one it was translated into, so transcribing its speech again, as in the language the user said it is spoken in, does not change its translation; for a film that translated none of its lines (core.Job.TranslationErr), or has no language, the one the settings name now. "" when neither names one.
func (a *app) queueReprocessLanguage(job core.Job) string {
	if language := strings.TrimSpace(job.Language); language != "" && job.TranslationErr == "" {
		return language
	}
	return strings.TrimSpace(a.prefs.language)
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
		a.queue.status = statusText(a.messages(), job)
	}
}

// queueAdd puts job on the shelf as queued, behind the jobs already waiting, and starts it if nothing is running.
func (a *app) queueAdd(job core.Job) {
	delete(a.queue.redoEnded, job.ID)
	job.Stage = core.StageQueued
	job.Err = nil
	job.FailKind = ""
	job.PausedBy = ""
	// A job processing or translating a done film again keeps the film's completion until it completes again (core.Job.Redo).
	if job.Redo == "" {
		job.CompletedAt = time.Time{}
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	a.queue.order = append(a.queue.order, job.ID)
	// A job queued again, as a film translated again from the player while it is open, goes first again once queuePreferOpen sees it.
	if a.queue.preferred == job.ID {
		a.queue.preferred = ""
	}
	a.show(job)
	a.pump()
}

// pump starts the oldest queued job when no job is running. Each job saves into its own library (queueRun), so a job queued in a library that is not open runs too, as long as that library is one the app still knows (queueCanRun). A queued job on the shelf that the queue has not seen, such as one read back as queued after a restart, joins the end of the queue, oldest first.
func (a *app) pump() {
	if a.queue.active != "" || a.queue.stopped || a.queue.paused {
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
		if !found && !queueHeld(job) && a.queueCanRun(job) {
			next, found = job, true
			continue
		}
		kept = append(kept, id)
	}
	a.queue.order = kept
	if found {
		a.queueRun(next)
	}
	a.queueTellSystem()
}

// queueCanRun reports whether the queue can start job now that it is its turn: its library is the open one or another the app knows. A job waiting in a library the app has forgotten does not start until that library is opened again, so no new work begins in a folder the user took off the list. A job already running when its library is forgotten goes on into it: forgetting leaves the folder on disk, and a running job is never stopped for a library change. A job saved before jobs named their library runs in whichever library is open.
func (a *app) queueCanRun(job core.Job) bool {
	// A download waits while offline mode is on, and goes on once it is off (state_privacy.go).
	if a.privacyHolds(job) {
		return false
	}
	if job.LibraryDir == "" {
		return strings.TrimSpace(a.snap().LibraryDir) != "" && !a.lockFilesAway(a.snap().LibraryDir)
	}
	dir := filepath.Clean(job.LibraryDir)
	// A job in an encrypted library that is locked waits until it is unlocked: its folder holds only the image, and what the job wrote there would be neither encrypted nor seen once the image is attached over it (lockFilesAway).
	if a.lockFilesAway(dir) {
		return false
	}
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

// queueRun starts job in the background. It keeps job.ID, so a retry updates the card already on screen. The job saves into its own library, job.LibraryDir, whichever library is open meanwhile: opening another one does not stop it. When the pipeline returns, the next queued job starts.
func (a *app) queueRun(job core.Job) {
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	// A download that borrows no browser's sign-in of its own, as one a failed card's remedy chose, borrows the one Settings › 通用 › 浏览器登录 names, from its first run, and keeps it on its card, whose remedies name it. A link from a site pack's site borrows none (core.Job.PluginSite, queuePipeline).
	if job.Cookies == "" && job.PluginSite == "" {
		job.Cookies = a.signinFor(job.Source)
	}
	// A link the user chose no picture or original lines for takes those Settings › 通用 › 在线视频 names, from its first run, and keeps them (onlineFor).
	job = a.onlineFor(job)
	job, terms := a.glossaryStart(job)
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
		tools.Translator.ContextLines = a.prefs.translateContextLines
		a.pipeline = tools
	}
	// A job processing a done film again entirely, or translating every line of it again, translates into the language it was asked for, which becomes its Language once it is done.
	language := job.Language
	if (job.Redo == core.RedoAll || job.Redo == core.RedoTranslation) && strings.TrimSpace(job.RedoLanguage) != "" {
		language = job.RedoLanguage
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	ctx = media.WithGlossary(ctx, terms)
	// Once translated, the film is suggested terms for its glossary (state_glossary_suggest.go), unless Settings › 翻译 turned that off.
	if a.prefs.suggestTerms {
		ctx = media.WithTermSuggestions(ctx)
	}
	if a.prefs.review {
		ctx = media.WithReview(ctx, a.prefs.reviewAuto)
	}
	done := make(chan struct{})
	a.queue.active, a.queue.cancel, a.queue.done, a.queue.cancelled = job.ID, cancel, done, false
	a.queueKeepAwake()
	pipeline, update := a.queuePipeline(job), a.update
	// A film made for the first time is made nearest where it is watched first: where the browser it came from was, until it is opened here or on a phone (queueWatched).
	watch, device := newQueueWatch(job.ResumeAt), a.savedDevice()
	if at, ok := a.queueWatched(); ok && at.placed && at.film == job.ID {
		watch.move(at.head)
	}
	a.queue.watch = watch
	a.queue.whisperModel = ""
	if tools, ok := pipeline.(media.Tools); ok {
		a.queue.whisperModel = tools.WhisperModel
	}
	go func() {
		defer cancel(nil)
		defer close(done)
		// A job stopped because the app quits reports nothing more: its card would say 已取消 and keep it from running again at the next launch, and the next job must not start. Neither does one stopped because the queue was paused, which goes back to the head of the queue once its pipeline has returned (queuePausedJob).
		quitting := func() bool { return errors.Is(context.Cause(ctx), errQuitting) }
		pausing := func() bool { return errors.Is(context.Cause(ctx), errPausing) }
		// A report that only moves the progress waits in latest, and at most one update carries it to the engine's goroutine, no sooner than queueProgressEvery after the one before: a tool that reports many times a second neither queues an update for each nor has every binding follow each, and the card shows the newest when the update runs. Any other report goes as it comes and takes the place of a progress report still waiting, as it carries the newest progress itself, so the bar never moves back.
		var latest atomic.Pointer[core.Job]
		var sent atomic.Int64
		deliver := func() {
			update(func() {
				if p := latest.Swap(nil); p != nil {
					sent.Store(time.Now().UnixNano())
					a.showProgress(*p)
				}
			})
		}
		last := job
		result := media.Make(ctx, pipeline, job, language, device, watch, func(j core.Job) {
			if quitting() || pausing() {
				return
			}
			// A film made a part at a time shows its subtitles from the first lines written out, so a report that names them is shown as it comes.
			onlyProgress := j.Stage == last.Stage && j.Title == last.Title && j.Thumbnail == last.Thumbnail && j.VideoPath == last.VideoPath && j.SubtitlePath == last.SubtitlePath && j.RedoGen == last.RedoGen && j.Err == nil
			last = j
			if !onlyProgress {
				latest.Store(nil)
				update(func() { a.show(j) })
				return
			}
			if latest.Swap(&j) == nil {
				if wait := queueProgressEvery - time.Since(time.Unix(0, sent.Load())); wait > 0 {
					time.AfterFunc(wait, deliver)
				} else {
					deliver()
				}
			}
		})
		if quitting() {
			return
		}
		if pausing() {
			update(func() { a.queuePausedJob(job.ID, result) })
			return
		}
		update(func() { a.finished(job.ID) })
	}()
}

// queueProgressEvery is how often at most a running job's progress reaches the window, about ten times a second, which is as often as its card's line and bar can be read.
const queueProgressEvery = 100 * time.Millisecond

// queuePipeline is the pipeline job runs with: the app's, borrowing the browser login the job names and saving into the job's own library rather than the one open, so a job goes on into its library after another is opened. A job whose link came from a site pack's site (core.Job.PluginSite) borrows no browser's login and none of the browser pages' cookies, whatever its Cookies say, as a remedy may have set them: the pack, not Kando, said the site is one to download from.
func (a *app) queuePipeline(job core.Job) media.Pipeline {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return a.pipeline
	}
	tools.Cookies = job.Browser()
	if job.PluginSite != "" {
		tools.Cookies, tools.SiteCookies = "", nil
	}
	if job.LibraryDir != "" {
		tools.OutputDir = job.LibraryDir
	}
	tools.Sources = a.mediaSourceCopier()
	// A script plugin's translator chosen in Settings › 翻译 translates the film in place of the AI service (state_plugins_translate.go).
	return a.pluginsTranslateTools(tools, job.SpokenLanguage)
}

// finished clears the job that just returned from the pipeline, remembers that the queue ran it (jobQueue.ran), says in the message bar how a job of another library ended (queueEndedElsewhere), and starts the next one, letting the computer sleep again when there is none.
func (a *app) finished(id string) {
	if a.queue.active == id {
		a.queue.active, a.queue.cancel, a.queue.done, a.queue.whisperModel, a.queue.watch = "", nil, nil, "", nil
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
		a.queue.awake = a.queue.keepAwake(a.messages().QueueKeepAwake(), false)
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

// queueEndedElsewhere tells, in the library's message bar, that job id, which saved into a library that is not open, is done, done without a translation and why (core.Job.TranslationErr), or has failed: its card leaves 处理中 then, and only its own library shows it. A job that was cancelled or removed says nothing, and neither does one whose processing or translating a done film again failed or was stopped: its film is done as it was, not anew, and the bar already says why it failed (queueRedoEnded).
func (a *app) queueEndedElsewhere(id string) {
	job, ok := a.library.Find(id)
	if _, redo := a.queue.redoEnded[id]; !ok || redo || a.queue.removed[id] || job.LibraryDir == "" || films.InLibrary(a.snap().LibraryDir, job) {
		return
	}
	library := films.LibraryName(job.LibraryDir)
	switch {
	case job.Stage == core.StageComplete && job.TranslationErr != "":
		m := a.messages()
		reason, section := jobfailure.TranslationHint(m, job.TranslationErr, job.TranslationSource)
		a.warn(m.QueueDoneElsewhereUntranslated(jobName(job), library, reason), section)
	case job.Stage == core.StageComplete:
		a.tell(a.messages().QueueDoneElsewhere(jobName(job), library))
	case job.Stage == core.StageFailed && !errors.Is(job.Err, context.Canceled):
		a.warn(a.messages().QueueFailedElsewhere(jobName(job), library), "")
		a.libraryNoticeJob(id, core.StageFailed)
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
	// The phone a job came from stays on its card whatever reports it later, as the running pipeline's copy of a job a phone's link made before it was named (core.Job.Device).
	if job.Device == "" {
		job.Device = was.Device
	}
	// The series is the user's, as the glossary's 术语 tab changes it while the job runs (core.Job.Series): the pipeline's copy of a job the library holds does not change it.
	if was.ID != "" {
		job.Series = was.Series
		// So is the list it was added from (core.Job.List), which the app writes once the queue has the job.
		job.List, job.ListIndex = was.List, was.ListIndex
		// A film indexed from a media source that was copied into the library while the job ran (linkedCopied) is the library's from then on: the running pipeline's copy, made before, does not make it indexed again.
		if !was.Linked {
			job.Linked = false
		}
	}
	// A phone's pause is the shelf's too (queueHold): a report of the running pipeline, which made its copy before, does not let the job go on; the queue's own steps say whether it is held.
	if was.PausedBy != "" && job.Stage != core.StageQueued && !job.Stage.Done() {
		job.PausedBy = was.PausedBy
	}
	// A job that has just completed again under its subtitle path, as one translating its film again does, has rewritten the file the open film shows.
	rewritten := job.Stage == core.StageComplete && was.Stage != "" && was.Stage != core.StageComplete
	// A redo that failed is notified as the failure it was, though the film goes back to the done grid.
	ended := job
	if job.Stage == core.StageFailed && job.FromFolder {
		a.queueFolderEnded(job)
		a.queueNotifyEnded(was.Stage, ended)
		return
	}
	if job.Stage == core.StageFailed && job.Redo != "" {
		job = a.queueRedoEnded(job)
	}
	quiet := a.queue.refills[job.ID]
	if job.Stage.Done() {
		delete(a.queue.refills, job.ID)
	}
	// A job that has completed is a film of its own, no longer a video of the folder's to put back.
	if job.Stage == core.StageComplete {
		job.FromFolder = false
		// A video added without a job is the job's from now on (library_added.go).
		a.library.Unadd(job.LibraryDir, job.VideoPath)
	}
	// A film that kept no video and has just downloaded it (core.RedoVideo) is watched from that video, whose extension is the download's rather than its sound's: its place, how far it was played, its subtitle delay and what else is kept by its video's path follow it, as they do a video moved on disk.
	if was.SubtitlesOnly && !job.SubtitlesOnly && was.VideoPath != "" && job.VideoPath != "" && was.VideoPath != job.VideoPath {
		from, to := was.VideoPath, job.VideoPath
		a.moveVideos(func(path string) (string, bool) { return to, path == from })
	}
	// A link that played its sound until its video came (core.Job.SoundPath) is watched from the video now: what was kept by the path it went by meanwhile (films.PlayPath) follows the video, once, as the job's report says it came.
	if was.VideoPath == "" && was.SoundPath != "" && job.VideoPath != "" && job.SoundPath == "" {
		if from, to := media.SoundVideo(was.SoundPath), job.VideoPath; from != "" && from != to {
			a.moveVideos(func(path string) (string, bool) { return to, path == from })
		}
	}
	a.library.Upsert(job)
	// While a batch is queued, the one queueing saves the shelf once it is all queued.
	if !a.queue.batching {
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, a.messages().QueueSaveFailed)
		} else {
			a.saveErr = ""
		}
	}
	if a.queue.selectedID == job.ID {
		a.queue.job = &job
		a.queue.status = statusText(a.messages(), job)
	}
	if a.openKey == job.ID {
		if rewritten && a.player.subs.key == job.ID {
			a.player.subs.path = ""
		}
		a.loadSubtitles(job.ID, job.VideoPath, job.SubtitlePath)
		// The job translated the chapters' titles before it completed (media.Make), so the open film's 片段 and info tab read them again.
		if rewritten && a.player.film.infoFor != "" {
			a.readFilmInfo(a.player.film.infoFor)
		}
	}
	a.areasShelfChanged()
	if !quiet {
		a.queueNotifyEnded(was.Stage, ended)
	}
	// A film just translated says the AI service answers again; one that only downloaded its video (core.RedoVideo) asked the service nothing, so it says nothing of it, and does not spend each film's one refill since launch while the service may still be gone.
	if rewritten && was.Redo != core.RedoVideo && job.TranslationErr == "" && !job.SameLanguage {
		a.queueRefill()
	}
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

// statusText is what the line under the link field and a job's card say of where job is, in the language of m. failureHint reads a failed job's error back from it, in the same language.
func statusText(m locales.Messages, job core.Job) string {
	switch job.Stage {
	case core.StageQueued:
		return m.QueueStatusQueued()
	case core.StageDownloading:
		return m.QueueStatusDownloading()
	case core.StageTranscribing:
		return m.QueueStatusTranscribing()
	case core.StageTranslating:
		if job.Progress.Proofreading {
			return m.QueueStatusProofreading()
		}
		if job.Progress.Reviewing {
			return m.QueueStatusReviewing()
		}
		return m.QueueStatusTranslating()
	case core.StageExporting:
		return m.QueueStatusExporting()
	case core.StageComplete:
		return m.QueueStatusComplete(job.SubtitlePath)
	case core.StageFailed:
		if errors.Is(job.Err, context.Canceled) {
			return m.QueueStatusCanceled()
		}
		if errors.Is(job.Err, core.ErrLibrarySwitched) {
			return m.QueueStatusLibrarySwitched()
		}
		return m.QueueStatusFailed(job.Err.Error())
	}
	return string(job.Stage)
}
