package main

import (
	"fmt"
	"strings"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

const (
	pageFilms     = ""
	pageYouTube   = "youtube"
	pageGeneral   = "general"
	pageSpeech    = "speech"
	pageAI        = "ai"
	pageTranslate = "translate"
)

func lingoTheme() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#16120e")
	t.Surface = ui.Hex("#2c241c")
	t.SurfaceHover = ui.Hex("#3a3126")
	t.SurfacePressed = ui.Hex("#4a3b2c")
	t.Border = ui.Hex("#3d342b")
	t.Text = ui.Hex("#f6f1e8")
	t.TextMuted = ui.Hex("#b3a38f")
	t.Accent = ui.Hex("#e39b2d")
	t.AccentHover = ui.Hex("#f0b04a")
	t.AccentPressed = ui.Hex("#c4841c")
	t.AccentText = ui.Hex("#1a140e")
	t.Danger = ui.Hex("#ef5350")
	t.Radius = 12
	return t
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(lingoTheme())
	t := c.Theme()
	films := a.films()
	ui.Row(c).Fill().Children(func() {
		a.sidebar(c, t, len(films))
		body := ui.Column(c).Fill().Padding(28, 32).Gap(20)
		if files := body.DroppedFiles(); len(files) > 0 {
			for _, file := range files {
				if media.IsVideoFile(file) {
					a.importLocal(file)
					break
				}
			}
		}
		if body.FileDragOver() {
			body.Background(t.Surface)
		}
		body.Children(func() {
			switch a.page {
			case pageGeneral, pageSpeech, pageAI, pageTranslate:
				a.settings(c, t)
			default:
				a.libraryPage(c, t, films)
			}
		})
	})
}

func (a *app) sidebar(c *ui.Context, t *ui.Theme, count int) {
	ui.Column(c).Fill().Width(248).Padding(18, 14).Gap(4).Children(func() {
		ui.Column(c).Padding(4, 8, 12, 8).Gap(2).Children(func() {
			ui.Text(c, "LingoCut").FontSize(18).Bold()
			ui.Text(c, countLabel(count)).TextColor(t.TextMuted).FontSize(12)
		})
		a.section(c, t, "影片")
		a.nav(c, t, pageFilms, "全部影片", fmt.Sprintf("%d", count))
		a.section(c, t, "来源")
		a.nav(c, t, pageYouTube, "YouTube", "")
		a.section(c, t, "设置")
		a.nav(c, t, pageGeneral, "通用", "")
		a.nav(c, t, pageSpeech, "语音识别", "")
		a.nav(c, t, pageAI, "AI 服务", "")
		a.nav(c, t, pageTranslate, "翻译", "")
		if a.saveErr != "" {
			ui.Text(c, a.saveErr).TextColor(t.Danger).Wrap().Padding(8, 8, 0, 8)
		}
	})
}

func (a *app) section(c *ui.Context, t *ui.Theme, label string) {
	ui.Text(c, label).TextColor(t.TextMuted).FontSize(12).Padding(14, 8, 4, 8)
}

func (a *app) nav(c *ui.Context, t *ui.Theme, page, label, badge string) {
	row := ui.Row(c).Padding(7, 10).Radius(8).Gap(8).AlignItems(ui.Center)
	if a.page == page {
		row.Background(ui.Hex("#4a3824"))
	}
	if row.Clicked() {
		a.page = page
	}
	row.Children(func() {
		ui.Text(c, label).Grow(1)
		if badge != "" {
			ui.Text(c, badge).TextColor(t.TextMuted).FontSize(12)
		}
	})
}

func (a *app) libraryPage(c *ui.Context, t *ui.Theme, films []film) {
	running := a.running()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &a.source).Placeholder("粘贴视频链接，或把本地视频拖进来").Label("视频链接").Grow(1).Disabled(running)
		if running {
			c.Key("cancel")
			if ui.Button(c, "取消").Clicked() {
				a.stop()
			}
		} else {
			c.Key("files")
			if ui.Button(c, "本地文件").Clicked() {
				a.pickVideos()
			}
			c.Key("start")
			if ui.PrimaryButton(c, "开始处理").Clicked() {
				a.start()
			}
		}
	})
	if a.job == nil && a.status != "" {
		color := t.Danger
		if a.status == "已选择本地视频" {
			color = t.TextMuted
		}
		ui.Text(c, a.status).TextColor(color).Wrap()
	}
	shown := films
	if a.page == pageYouTube {
		shown = nil
		for _, item := range films {
			if strings.Contains(item.source, "youtube.com") || strings.Contains(item.source, "youtu.be") {
				shown = append(shown, item)
			}
		}
	}
	if len(shown) == 0 {
		ui.Text(c, "还没有影片").TextColor(t.TextMuted)
		return
	}
	for start := 0; start < len(shown); start += 2 {
		end := min(start+2, len(shown))
		row := shown[start:end]
		ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
			for _, item := range row {
				a.card(c, t, item)
			}
		})
	}
}

func (a *app) card(c *ui.Context, t *ui.Theme, item film) {
	ui.Column(c).Width(360).Gap(8).Children(func() {
		ui.Column(c).Height(176).Radius(12).Background(posterColor(item.key)).Padding(12).Children(func() {
			ui.Row(c).Height(112)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if item.stage != "" {
					color := t.Text
					if item.stage == core.StageFailed {
						color = t.Danger
					}
					ui.Text(c, stageLabel(item.stage)).TextColor(color).Bold()
				}
				if item.jobID != "" && item.stage == core.StageFailed && !a.running() {
					if ui.Button(c, "移除").Clicked() {
						a.remove(item.jobID)
					}
					if ui.PrimaryButton(c, "重试").Clicked() {
						if job, ok := a.library.find(item.jobID); ok {
							a.retry(job)
						}
					}
				}
			})
		})
		if item.jobID != "" && item.stage == core.StageComplete {
			ui.Text(c, "翻译").TextColor(t.Accent).FontSize(12)
		}
		ui.Text(c, item.title)
		if item.detail != "" && item.detail != item.title {
			ui.Text(c, item.detail).TextColor(t.TextMuted).FontSize(12).Width(360).Wrap().Selectable()
		}
	})
}

func (a *app) settings(c *ui.Context, t *ui.Theme) {
	whisper, model, base, output := a.toolSettings()
	switch a.page {
	case pageGeneral:
		ui.Text(c, "通用").FontSize(24).Bold()
		ui.Text(c, "下载的视频和字幕放在这个目录。").TextColor(t.TextMuted)
		ui.Text(c, output).Selectable()
	case pageSpeech:
		ui.Text(c, "语音识别").FontSize(24).Bold()
		ui.Text(c, "本地转录使用 whisper.cpp。模型路径来自 LINGOCUT_WHISPER_MODEL。").TextColor(t.TextMuted)
		if whisper == "" {
			whisper = "未设置"
		}
		ui.Text(c, whisper).Selectable()
	case pageAI:
		ui.Text(c, "AI 服务").FontSize(24).Bold()
		ui.Text(c, "翻译走已登录的 EveryAPI。没有单独的 key。").TextColor(t.TextMuted)
		if base == "" {
			base = "当前没有配置翻译服务"
		}
		ui.Text(c, base).Selectable()
		if model != "" {
			ui.Text(c, "模型 "+model).TextColor(t.TextMuted)
		}
	case pageTranslate:
		ui.Text(c, "翻译").FontSize(24).Bold()
		ui.Text(c, "新任务使用这个目标语言。").TextColor(t.TextMuted)
		ui.TextInput(c, &a.language).Placeholder("简体中文").Label("目标语言").Width(240)
	}
}

func (a *app) toolSettings() (whisper, model, base, output string) {
	output = a.libraryDir
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return whisper, model, base, output
	}
	if output == "" {
		output = tools.OutputDir
	}
	return tools.WhisperModel, tools.Translator.Model, tools.Translator.BaseURL, output
}

type film struct {
	key    string
	title  string
	source string
	detail string
	stage  core.Stage
	jobID  string
}

func (a *app) films() []film {
	used := map[string]bool{}
	out := make([]film, 0, len(a.library.jobs)+len(a.videos))
	for i := range a.library.jobs {
		job := a.library.jobs[i]
		if job.VideoPath != "" {
			used[job.VideoPath] = true
		}
		out = append(out, film{
			key: job.ID, title: jobTitle(job), source: job.Source,
			detail: statusText(job), stage: job.Stage, jobID: job.ID,
		})
	}
	for _, video := range a.videos {
		if used[video.Path] {
			continue
		}
		out = append(out, film{key: video.Path, title: video.Name, detail: video.Path})
	}
	return out
}

func posterColor(key string) ui.Color {
	colors := []ui.Color{ui.Hex("#3a2c1e"), ui.Hex("#243044"), ui.Hex("#1e3330"), ui.Hex("#3a2430")}
	n := 0
	for _, r := range key {
		n += int(r)
	}
	return colors[n%len(colors)]
}

func countLabel(n int) string {
	return fmt.Sprintf("%d 部影片", n)
}
