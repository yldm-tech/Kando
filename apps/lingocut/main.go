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
	job        *core.Job
	library    library
	rows       ui.ListState
	pipeline   media.Pipeline
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
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

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	running := a.running()
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(220).Fill().Padding(16).Gap(8).Children(func() {
			ui.Text(c, "LingoCut").FontSize(24).Bold()
			ui.Text(c, "下载、转录、翻译视频").TextColor(t.TextMuted)
			if ui.Button(c, "新建").Clicked() {
				a.newDraft()
			}
			ui.Text(c, "视频库").FontSize(14).Bold().Padding(12, 0, 0, 0)
			if len(a.library.jobs) == 0 {
				ui.Text(c, "还没有视频").TextColor(t.TextMuted)
			} else {
				ui.List(c, &a.rows, len(a.library.jobs), func(i int) {
					job := a.library.jobs[i]
					row := ui.Column(c).Padding(6, 8).Gap(2).Radius(6)
					if job.ID == a.selectedID {
						row.Background(t.Surface)
					}
					if row.Clicked() {
						a.selectJob(job.ID)
					}
					row.Children(func() {
						ui.Text(c, jobTitle(job))
						ui.Text(c, stageLabel(job.Stage)).TextColor(t.TextMuted).FontSize(12)
					})
				}).Grow(1)
			}
			if a.saveErr != "" {
				ui.Text(c, a.saveErr).TextColor(t.TextMuted).Wrap()
			}
		})
		ui.Column(c).Fill().Padding(32).Gap(16).Children(func() {
			heading := "新建任务"
			if job, ok := a.library.find(a.selectedID); ok {
				heading = jobTitle(job)
			}
			ui.Text(c, heading).FontSize(26).Bold()
			ui.Text(c, "粘贴一个视频链接，LingoCut 会在本机完成处理。").TextColor(t.TextMuted)
			ui.TextInput(c, &a.source).Placeholder("https://...").Label("视频链接").Width(440).Disabled(running)
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "翻译成").TextColor(t.TextMuted)
				ui.TextInput(c, &a.language).Placeholder("简体中文").Label("目标语言").Width(160).Disabled(running)
			})
			if running {
				c.Key("cancel")
				if ui.Button(c, "取消").Width(120).Clicked() {
					a.stop()
				}
			} else {
				c.Key("start")
				if ui.PrimaryButton(c, "开始处理").Width(120).Clicked() {
					a.start()
				}
			}
			if a.status != "" {
				ui.Text(c, a.status).TextColor(t.TextMuted).Width(440).Wrap().Selectable()
			}
			ui.Text(c, "处理流程").FontSize(14).Bold().Padding(18, 0, 0, 0)
			ui.Text(c, "下载 → 本地转录 → 翻译 → 导出双语字幕").TextColor(t.TextMuted)
		})
	})
}

func (a *app) newDraft() {
	a.selectedID = ""
	a.job = nil
	a.source = ""
	a.status = ""
}

func (a *app) selectJob(id string) {
	job, ok := a.library.find(id)
	if !ok {
		return
	}
	a.selectedID = id
	a.job = &job
	a.source = job.Source
	if job.Language != "" {
		a.language = job.Language
	}
	a.status = statusText(job)
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
	a.library.upsert(job)
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
	a := newApp(media.ToolsFromEnv(), func(fn func()) { win.Update(fn) })
	a.openLibrary(defaultLibraryPath())
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "LingoCut",
			Width:     860,
			Height:    560,
			MinWidth:  720,
			MinHeight: 460,
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
