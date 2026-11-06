package settings

import (
	"fmt"
	"path/filepath"
	"strings"

	"lingocut/apps/lingocut/settings/form"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// speech is the 语音识别 page: the whisper models, one of which recognizes speech, and their downloads.
func (s screen) speech(c *ui.Context, t *ui.Theme) {
	tools, _ := s.Tools()
	models := media.SpeechModels()
	installed := make([]string, len(models))
	chosen := ""
	for i, model := range models {
		installed[i] = s.InstalledSpeechModel(model.ID)
		if installed[i] != "" && filepath.Clean(installed[i]) == filepath.Clean(tools.WhisperModel) {
			chosen = model.ID
		}
	}
	sel := chosen
	form.Label(c, "whisper 模型", 6, nil)
	form.Card(c, func() {
		// One radio group, so that Tab stops once in it and the arrows choose among the models on disk.
		ui.RadioGroup(c, func() {
			for i, model := range models {
				if i > 0 {
					form.Separator(c, false)
				}
				c.Key(model.ID)
				s.modelRow(c, &sel, model, installed[i])
			}
		}).Label("whisper 模型").Gap(0).AlignItems(ui.Stretch)
	})
	form.Note(c, "自动识别语言，支持智能断句。越大越准，也越慢、越占空间；不确定就用推荐的。模型从 HuggingFace 下载，取消后不留下半截文件；其他应用已经下好的模型（Vimpo、hyperframes）会直接选用，不用复制。", 0)
	if sel != chosen {
		for i, model := range models {
			if model.ID == sel {
				s.UseSpeechModel(installed[i])
			}
		}
	}
	if s.DownloadErr() != "" {
		ui.Text(c, s.DownloadErr()).FontSize(11).TextColor(t.Danger).Wrap().Padding(0, 12)
	}
}

// modelRow is a whisper model as the prototype's engine rows: its radio button, its name with 推荐 after it over what it is good for and its speed, accuracy and size, and what it offers at the end. A model not on disk cannot be chosen, so its radio button is disabled.
func (s screen) modelRow(c *ui.Context, sel *string, model media.SpeechModel, installed string) {
	ui.Row(c).Shrink(0).MinHeight(54).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		breeze.Radio(c, sel, model.ID, "").Label(model.Name).Disabled(installed == "")
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, model.Name).FontSize(13).FontWeight(600).TextColor(form.Text)
				if model.Recommended {
					form.Badge(c, "推荐")
				}
			})
			ui.Text(c, model.Blurb+"\n"+fmt.Sprintf("速度 %s  准确 %s  %s", stars(model.Speed), stars(model.Accuracy), model.SizeLabel)).FontSize(11).FixedLineHeight(15).TextColor(form.Secondary).Wrap()
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() { s.modelAction(c, model, installed) })
	})
}

// modelAction is what a model offers at the end of its row: its download's progress with a way to cancel it, 已下载, or 下载.
func (s screen) modelAction(c *ui.Context, model media.SpeechModel, installed string) {
	if s.Downloading() == model.ID {
		progress := float64(-1)
		if s.DownloadTotal() > 0 {
			progress = float64(s.DownloadWritten()) / float64(s.DownloadTotal())
		}
		breeze.Progress(c, progress).Width(110).Label(model.Name + " 下载进度")
		c.Key(model.ID + "-cancel")
		if form.Button(c, "取消", false).Clicked() {
			s.CancelDownload()
		}
		return
	}
	if installed != "" {
		form.Value(c, "已下载", 12)
		return
	}
	c.Key(model.ID + "-download")
	if form.Button(c, "下载", false).Disabled(s.Downloading() != "").Clicked() && s.Downloading() == "" {
		s.DownloadModel(model.ID)
	}
}

func stars(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 5 {
		n = 5
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}
