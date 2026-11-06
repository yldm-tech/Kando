package main

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"lingocut/apps/lingocut/films"
	"lingocut/packages/core"
	"lingocut/packages/media"
)

// jobQueue is the job being made and run: the link field and the line under it, the job the page follows, the jobs removed while they ran and how to stop the one running.
type jobQueue struct {
	source     string
	status     string
	selectedID string
	job        *core.Job
	removed    map[string]bool
	cancel     context.CancelFunc
}

// errBusy is Enqueue's answer while a job runs. Jobs run one at a time and, until the queue keeps the others waiting, a source given while one runs is not added.
var errBusy = errors.New("已有视频在处理，等它完成后再添加")

// Enqueue adds a job for each source, a link or a local video's file:// URL, to the open library in the language the settings name, and runs it. One job runs at a time: Enqueue runs the first source and refuses the others while it runs, and refuses all of them while another job runs.
func (a *app) Enqueue(sources ...string) error {
	if len(sources) == 0 {
		return core.ErrEmptySource
	}
	for _, source := range sources {
		if a.running() {
			return errBusy
		}
		job, err := a.newJob(source)
		if err != nil {
			return err
		}
		a.run(job)
	}
	return nil
}

// Cancel stops job id if it is the one running. Its card stays, marked 已取消.
func (a *app) Cancel(id string) {
	job, ok := a.library.Find(id)
	if !ok || job.Stage.Done() || id != a.queue.selectedID {
		return
	}
	a.stop()
}

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

// importLocal points the next job at a video already on disk. The pipeline reads it directly and does not download it.
func (a *app) importLocal(path string) {
	if !media.IsVideoFile(path) {
		a.queue.status = "没有可导入的视频"
		return
	}
	a.queue.source = (&url.URL{Scheme: "file", Path: path}).String()
	a.queue.status = "已选择本地视频"
	a.page = pageFilms
	a.openKey = ""
}

// startFilm processes a video that is already in the library and stays on it.
func (a *app) startFilm(item films.Film) {
	a.importLocal(item.VideoPath)
	if a.queue.status != "已选择本地视频" {
		return
	}
	a.start()
	if a.queue.job == nil {
		return
	}
	a.openKey = a.queue.job.ID
	a.loadSubtitles(a.queue.job.ID, a.queue.job.SubtitlePath)
}

func (a *app) running() bool {
	for _, job := range a.library.Jobs {
		if job.Stage != "" && !job.Stage.Done() {
			return true
		}
	}
	return false
}

func (a *app) remove(id string) {
	if a.queue.removed == nil {
		a.queue.removed = map[string]bool{}
	}
	a.queue.removed[id] = true
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

func (a *app) retry(job core.Job) {
	if a.running() {
		return
	}
	job.Err = nil
	job.Stage = core.StageQueued
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.prefs.language)
	}
	if job.Language == "" {
		a.queue.status = "请填写目标语言"
		return
	}
	a.page = pageFilms
	a.run(job)
}

func (a *app) newDraft() {
	a.queue.selectedID = ""
	a.queue.job = nil
	a.queue.source = ""
	a.queue.status = ""
}

func (a *app) start() {
	if a.running() {
		return
	}
	job, err := a.newJob(a.queue.source)
	if err != nil {
		a.queue.job = nil
		a.queue.status = err.Error()
		return
	}
	a.run(job)
}

// run keeps job.ID, so a retry updates the card already on screen.
func (a *app) run(job core.Job) {
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.Translator.Prompt = a.prefs.translatePrompt
		tools.Translator.Context = a.prefs.translateContext
		a.pipeline = tools
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.snap().LibraryDir
	}
	a.queue.selectedID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	a.queue.cancel = cancel
	a.show(job)
	pipeline, update := a.pipeline, a.update
	go func() {
		defer cancel()
		media.Run(ctx, pipeline, job, job.Language, func(j core.Job) {
			update(func() { a.show(j) })
		})
	}()
}

func (a *app) stop() {
	if a.queue.cancel != nil {
		a.queue.cancel()
	}
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
}

func statusText(job core.Job) string {
	switch job.Stage {
	case core.StageQueued:
		return "任务已加入队列"
	case core.StageDownloading:
		return "正在下载视频…"
	case core.StageTranscribing:
		return "正在本地转录…"
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
		return "失败：" + job.Err.Error()
	}
	return string(job.Stage)
}
