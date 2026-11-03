package main

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lingocut/packages/core"
	"lingocut/packages/media"
)

type app struct {
	source   string
	language string
	status   string
	job      *core.Job
	pipeline media.Pipeline
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{language: "简体中文", pipeline: pipeline, update: update}
}

func (a *app) running() bool {
	return a.job != nil && !a.job.Stage.Done()
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	running := a.running()
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(190).Fill().Padding(20).Gap(12).Children(func() {
			ui.Text(c, "LingoCut").FontSize(24).Bold()
			ui.Text(c, "下载、转录、翻译视频").TextColor(t.TextMuted)
			ui.Text(c, "视频库").FontSize(14).Bold().Padding(24, 0, 0, 0)
			ui.Text(c, "所有视频").TextColor(t.TextMuted)
		})
		ui.Column(c).Fill().Padding(32).Gap(16).Children(func() {
			ui.Text(c, "新建任务").FontSize(26).Bold()
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
	a.job = &job
	a.status = statusText(job)
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
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "LingoCut",
			Width:     820,
			Height:    560,
			MinWidth:  680,
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
