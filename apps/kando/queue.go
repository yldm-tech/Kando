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
	// cancel stops the running job; its cause becomes the job's error when it is core.ErrLibrarySwitched.
	cancel context.CancelCauseFunc
	// ytdlp is the yt-dlp the jobs download with and the browsers it can borrow logins from (queue_remedy.go).
	ytdlp ytdlpState
	// linkFocus asks the link field above the library for the keyboard focus once, as 换个链接 does.
	linkFocus bool
}

func newJobQueue() jobQueue { return jobQueue{} }

// stopActive asks the running job to stop. It ends, and the next one starts, when its pipeline returns.
func (q *jobQueue) stopActive(cause error) {
	if q.cancel != nil {
		q.cancel(cause)
	}
}

// savedQueue is the queue's part of settings.json. The jobs themselves are in the shelf's library.json.
type savedQueue struct{}

func (s *savedQueue) clean() {}

// queueHooks: a job still writing into the library being left is stopped and marked as stopped by the switch, and jobs queued in the library opened start once it has ended. Jobs the app quit in the middle of were read back as queued; they run once the window is ready, so their background updates have a window to land in. The version of yt-dlp, which the card of a download it was too old for names, and the browsers whose logins the cards offer to borrow are read then and whenever the tools change. The player's resume positions are kept with the shelf: read when it is opened and saved when the player remembers one, and keyed by the new paths of videos that moved.
func (a *app) queueHooks() areaHooks {
	return areaHooks{
		leavingLibrary: func() { a.queue.stopActive(core.ErrLibrarySwitched) },
		ready: func() {
			a.pump()
			a.queueReadYTDLP()
		},
		toolsChanged:     func(bool) { a.queueReadYTDLP() },
		libraryChanged:   a.pump,
		shelfLoaded:      func() { a.setResumePositions(maps.Clone(a.library.Positions)) },
		positionsChanged: a.savePositions,
		videosMoved:      func(move func(string) (string, bool)) { shelf.MoveKeys(a.resumePositions(), move) },
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
		if old, ok := a.queueRetryable(old); ok {
			a.queueAdd(old)
			return old.ID, nil
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
	a.queue.source = (&url.URL{Scheme: "file", Path: path}).String()
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

// savePositions writes the player's resume positions into library.json with the jobs.
func (a *app) savePositions() {
	a.library.Positions = maps.Clone(a.resumePositions())
	if err := a.library.Save(); err != nil {
		a.saveErr = locales.QueueSaveFailed(err.Error())
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
		a.saveErr = locales.QueueSaveFailed(err.Error())
	}
}

// retry queues a failed or cancelled job again under the same ID, so the card already on screen follows it, and shows that card on the library page.
func (a *app) retry(job core.Job) {
	job, ok := a.queueRetryable(job)
	if !ok {
		return
	}
	a.page = pageFilms
	a.queue.selectedID = job.ID
	a.queueAdd(job)
}

// queueRetryable is job ready to be queued again, with the language to use filled in, and whether it can be: not when it is running or already waiting, or when no language is set, which the page's status then says. It changes neither the page nor the selected card, so a video added again from kando add, MCP or the top bar of another page is queued where the user is (queueTake).
func (a *app) queueRetryable(job core.Job) (core.Job, bool) {
	if job.ID == a.queue.active || job.Stage == core.StageQueued {
		return job, false
	}
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if job.Language == "" {
		a.queue.status = locales.QueueNoLanguage()
		return job, false
	}
	return job, true
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
	job.CompletedAt = time.Time{}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	a.queue.order = append(a.queue.order, job.ID)
	a.show(job)
	a.pump()
}

// pump starts the oldest queued job of the open library when no job is running. Jobs queued in another library wait until it is open again, because the pipeline writes into the open one. A queued job on the shelf that the queue has not seen, such as one read back as queued after a restart, joins the end of the queue, oldest first.
func (a *app) pump() {
	if a.queue.active != "" {
		return
	}
	for i := len(a.library.Jobs) - 1; i >= 0; i-- {
		job := a.library.Jobs[i]
		if job.Stage == core.StageQueued && !slices.Contains(a.queue.order, job.ID) {
			a.queue.order = append(a.queue.order, job.ID)
		}
	}
	dir := a.snap().LibraryDir
	var next core.Job
	found := false
	kept := make([]string, 0, len(a.queue.order))
	for _, id := range a.queue.order {
		job, ok := a.library.Find(id)
		if !ok || job.Stage != core.StageQueued {
			continue
		}
		if !found && dir != "" && inQueueLibrary(job, dir) {
			next, found = job, true
			continue
		}
		kept = append(kept, id)
	}
	a.queue.order = kept
	if found {
		a.run(next)
	}
}

// inQueueLibrary reports whether job was queued in the library at dir. A job saved before jobs named their library runs in whichever library is open.
func inQueueLibrary(job core.Job, dir string) bool {
	return job.LibraryDir == "" || filepath.Clean(job.LibraryDir) == filepath.Clean(dir)
}

// run starts job in the background. It keeps job.ID, so a retry updates the card already on screen. When the pipeline returns, the next queued job starts.
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
	ctx, cancel := context.WithCancelCause(context.Background())
	a.queue.active, a.queue.cancel = job.ID, cancel
	pipeline, update := a.pipeline, a.update
	if tools, ok := pipeline.(media.Tools); ok {
		tools.Cookies = job.Cookies
		pipeline = tools
	}
	go func() {
		defer cancel(nil)
		// A report that only moves the progress waits in latest, and at most one update carries it to the UI thread: a tool that reports many times a second does not queue an update for each, and the card shows the newest when the window gets to it. Any other report goes as it comes, after the progress already waiting.
		var latest atomic.Pointer[core.Job]
		last := job
		media.Run(ctx, pipeline, job, job.Language, func(j core.Job) {
			if j.Stage == core.StageFailed && errors.Is(context.Cause(ctx), core.ErrLibrarySwitched) {
				j.Err = core.ErrLibrarySwitched
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
		update(func() { a.finished(job.ID) })
	}()
}

// finished clears the job that just returned from the pipeline and starts the next one.
func (a *app) finished(id string) {
	if a.queue.active == id {
		a.queue.active, a.queue.cancel = "", nil
	}
	a.pump()
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
	a.library.Upsert(job)
	if err := a.library.Save(); err != nil {
		a.saveErr = locales.QueueSaveFailed(err.Error())
	} else {
		a.saveErr = ""
	}
	if a.queue.selectedID == job.ID {
		a.queue.job = &job
		a.queue.status = statusText(job)
	}
	if a.openKey == job.ID {
		a.loadSubtitles(job.ID, job.SubtitlePath)
	}
	a.areasShelfChanged()
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
