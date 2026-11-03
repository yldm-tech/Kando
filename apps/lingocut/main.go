package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lingocut/packages/core"
)

type app struct {
	source string
	status string
	job    *core.Job
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
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
			ui.TextInput(c, &a.source).Placeholder("https://...").Label("视频链接").Width(440)
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, "开始处理").Width(120).Clicked() {
					a.start()
				}
				if a.status != "" {
					ui.Text(c, a.status).TextColor(t.TextMuted)
				}
			})
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
	a.job = &job
	a.status = "任务已加入队列"
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "LingoCut",
			Width:     820,
			Height:    520,
			MinWidth:  680,
			MinHeight: 420,
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
