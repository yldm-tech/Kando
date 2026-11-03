package main

import (
	"context"
	"errors"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lingocut/packages/core"
	"lingocut/packages/media"
)

type app struct {
	source     string
	language   string
	status     string
	selectedID string
	saveErr    string
	page       string
	libraryDir string
	videos     []media.Video
	removed    map[string]bool
	job        *core.Job
	library    library
	pipeline   media.Pipeline
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
}

func (a *app) refreshLibrary() {
	if a.libraryDir == "" {
		return
	}
	videos, err := media.ListVideos(a.libraryDir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.videos = videos
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{language: "简体中文", pipeline: pipeline, update: update}
}

func (a *app) openLibrary(path string) {
	lib, err := loadLibrary(path)
	a.library = lib
	if err != nil {
		a.saveErr = "视频库无法读取：" + err.Error()
	}
}

func (a *app) running() bool {
	for _, job := range a.library.jobs {
		if job.Stage != "" && !job.Stage.Done() {
			return true
		}
	}
	return false
}

func (a *app) remove(id string) {
	if a.removed == nil {
		a.removed = map[string]bool{}
	}
	a.removed[id] = true
	kept := make([]core.Job, 0, len(a.library.jobs))
	for _, job := range a.library.jobs {
		if job.ID != id {
			kept = append(kept, job)
		}
	}
	a.library.jobs = kept
	if a.selectedID == id {
		a.newDraft()
	}
	if err := a.library.save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	}
}

func (a *app) retry(job core.Job) {
	a.source = job.Source
	if job.Language != "" {
		a.language = job.Language
	}
	a.page = pageFilms
	a.start()
}

func (a *app) newDraft() {
	a.selectedID = ""
	a.job = nil
	a.source = ""
	a.status = ""
}

func (a *app) start() {
	job, err := core.NewJob(a.source)
	if err != nil {
		a.job = nil
		a.status = err.Error()
		return
	}
	language := strings.TrimSpace(a.language)
	if language == "" {
		a.job = nil
		a.status = "请填写目标语言"
		return
	}
	job.Language = language
	a.selectedID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.show(job)
	pipeline, update := a.pipeline, a.update
	go func() {
		defer cancel()
		media.Run(ctx, pipeline, job, language, func(j core.Job) {
			update(func() { a.show(j) })
		})
	}()
}

func (a *app) stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *app) show(job core.Job) {
	if a.removed[job.ID] {
		return
	}
	a.library.upsert(job)
	a.refreshLibrary()
	if err := a.library.save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	} else {
		a.saveErr = ""
	}
	if a.selectedID == job.ID {
		a.job = &job
		a.status = statusText(job)
	}
}

func jobTitle(job core.Job) string {
	if job.VideoPath != "" {
		base := filepath.Base(job.VideoPath)
		return shorten(strings.TrimSuffix(base, filepath.Ext(base)), 22)
	}
	parsed, err := url.Parse(job.Source)
	if err != nil || parsed.Host == "" {
		return shorten(job.Source, 22)
	}
	title := parsed.Host
	if parsed.Path != "" && parsed.Path != "/" {
		title += parsed.Path
	}
	return shorten(title, 22)
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func stageLabel(stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return "排队"
	case core.StageDownloading:
		return "下载中"
	case core.StageTranscribing:
		return "转录中"
	case core.StageTranslating:
		return "翻译中"
	case core.StageExporting:
		return "导出中"
	case core.StageComplete:
		return "已完成"
	case core.StageFailed:
		return "失败"
	default:
		return string(stage)
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

func main() {
	var win *mygo.Window
	tools := media.ToolsFromEnv()
	a := newApp(tools, func(fn func()) { win.Update(fn) })
	a.libraryDir = tools.OutputDir
	a.openLibrary(defaultLibraryPath())
	a.refreshLibrary()
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "LingoCut",
			Width:     1100,
			Height:    720,
			MinWidth:  900,
			MinHeight: 560,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
