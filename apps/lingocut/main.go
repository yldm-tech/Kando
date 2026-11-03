package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lingocut/packages/core"
	"lingocut/packages/media"
)

type app struct {
	mu     sync.RWMutex
	source string
	status string
	job    *core.Job
	dl     media.Downloader
	win    *mygo.Window
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	a.mu.RLock()
	status := a.status
	a.mu.RUnlock()
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
				if status != "" {
					ui.Text(c, status).TextColor(t.TextMuted)
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
		a.mu.Lock()
		a.job = nil
		a.status = err.Error()
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	a.job = &job
	a.status = "正在下载…"
	a.mu.Unlock()
	if a.win != nil {
		go a.download(job)
	}
}

func (a *app) download(job core.Job) {
	directory, err := os.UserCacheDir()
	if err == nil {
		directory = filepath.Join(directory, "LingoCut", "Videos")
		err = os.MkdirAll(directory, 0o755)
	}
	path := ""
	if err == nil {
		path, err = a.dl.Download(context.Background(), job.Source, filepath.Join(directory, "%(title)s.%(ext)s"))
	}
	a.win.Update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.status = err.Error()
		} else {
			a.status = "下载完成：" + path
		}
	})
}

func main() {
	a := &app{dl: media.Downloader{}}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
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
