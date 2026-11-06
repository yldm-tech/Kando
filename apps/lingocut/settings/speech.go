package settings

import (
	"fmt"
	"path/filepath"
	"strings"

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
	breeze.PreferencesGroup(c, "whisper 模型", "自动识别语言，支持智能断句。越大越准，也越慢、越占空间；不确定就用推荐的。模型从 HuggingFace 下载，取消后不留下半截文件；其他应用已经下好的模型（Vimpo、hyperframes）会直接选用，不用复制。", nil, func() {
		for i, model := range models {
			c.Key(model.ID)
			subtitle := model.Blurb + "\n" + fmt.Sprintf("速度 %s  准确 %s  %s", stars(model.Speed), stars(model.Accuracy), model.SizeLabel)
			suffix := func() {
				if model.Recommended {
					breeze.Badge(c, "推荐")
				}
				s.modelAction(c, model, installed[i])
			}
			if installed[i] != "" {
				breeze.RadioRow(c, &sel, model.ID, model.Name, subtitle, suffix)
			} else {
				breeze.ActionRow(c, model.Name, subtitle, suffix)
			}
		}
	})
	if sel != chosen {
		for i, model := range models {
			if model.ID == sel {
				s.UseSpeechModel(installed[i])
			}
		}
	}
	if s.DownloadErr() != "" {
		breeze.Text(c, s.DownloadErr(), breeze.TextCaption).TextColor(t.Danger).Wrap()
	}
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
		if breeze.Button(c, "取消").Clicked() {
			s.CancelDownload()
		}
		return
	}
	if installed != "" {
		breeze.Text(c, "已下载", breeze.TextSecondary)
		return
	}
	c.Key(model.ID + "-download")
	if breeze.Button(c, "下载").Disabled(s.Downloading() != "").Clicked() && s.Downloading() == "" {
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
