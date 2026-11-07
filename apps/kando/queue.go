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

// queueHooks: a job still writing into the library being left is stopped and marked as stopped by the switch, and jobs queued in the library opened start once it has ended. Jobs the app quit in the middle of were read back as queued; they run once the window is ready, so their background updates have a window to land in. The version of yt-dlp, which the card of a download it was too old for names, and the browsers whose logins the cards offer to borrow are read then and whenever the tools change. The player's resume positions are kept with the shelf: read when it is opened and saved when the player remembers one.
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
	}
}

// queueKeys are the queue's shortcuts.
func (a *app) queueKeys() []shortcut { return nil }

// errBusy was Enqueue's answer while a job ran, before jobs waited in a queue. Enqueue no longer returns it; it stays so code written against the one-job contract still builds.
var errBusy = errors.New("已有视频在处理，等它完成后再添加")

// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name. The jobs run one after another, after those already waiting. A source that cannot be added is skipped and the others are still added; the error is why the first one was not.
func (a *app) Enqueue(sources ...string) error {
	if len(sources) == 0 {
		return core.ErrEmptySource
	}
	var first error
	for _, source := range sources {
		job, err := a.newJob(source)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		a.queueAdd(job)
	}
	return first
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

// newJob makes the job for source in the open library and the target language, or returns why it cannot.
func (a *app) newJob(source string) (core.Job, error) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return core.Job{}, errors.New("先创建或导入一个资料库")
	}
	job, err := core.NewJob(source)
	if err != nil {
		return core.Job{}, err
	}
	language := strings.TrimSpace(a.prefs.language)
	if language == "" {
		return core.Job{}, errors.New("请填写目标语言")
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
		a.queue.status = "没有可导入的视频"
		return false
	}
	a.queue.source = (&url.URL{Scheme: "file", Path: path}).String()
	a.queue.status = "已选择本地视频"
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
		a.saveErr = "视频库无法保存：" + err.Error()
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
		a.saveErr = "视频库无法保存：" + err.Error()
	}
}

// retry queues a failed or cancelled job again under the same ID, so the card already on screen follows it.
func (a *app) retry(job core.Job) {
	if job.ID == a.queue.active || job.Stage == core.StageQueued {
		return
	}
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if job.Language == "" {
		a.queue.status = "请填写目标语言"
		return
	}
	a.page = pageFilms
	a.queue.selectedID = job.ID
	a.queueAdd(job)
}

func (a *app) newDraft() {
	a.queue.selectedID = ""
	a.queue.job = nil
	a.queue.source = ""
	a.queue.status = ""
}

// start queues the source typed on the library page. Its card is the one the page's status follows.
func (a *app) start() {
	job, err := a.newJob(a.queue.source)
	if err != nil {
		a.queue.job = nil
		a.queue.status = err.Error()
		return
	}
	a.queue.selectedID = job.ID
	a.queueAdd(job)
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
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.Translator.Prompt = a.prefs.translatePrompt
		tools.Translator.Context = a.prefs.translateContext
		a.pipeline = tools
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
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
		a.saveErr = "视频库无法保存：" + err.Error()
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
		return "任务已加入队列"
	case core.StageDownloading:
		return "正在下载视频…"
	case core.StageTranscribing:
		return "正在本机生成字幕…"
	case core.StageTranslating:
		return "正在翻译字幕…"
	case core.StageExporting:
		return "正在导出双语字幕…"
	case core.StageComplete:
		return "完成：" + job.SubtitlePath
	case core.StageFailed:
		if errors.Is(job.Err, context.Canceled) {
			return "已取消"
		}
		if errors.Is(job.Err, core.ErrLibrarySwitched) {
			return "已停止：切换了资料库"
		}
		return "失败：" + job.Err.Error()
	}
	return string(job.Stage)
}
