package settings

import (
	"path/filepath"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// general is the 通用 page: the library, the interface language and version, and the programs LingoCut runs.
func (s screen) general(c *ui.Context, t *ui.Theme) {
	preferencesGroup(c, "资料库", "每部影片是资料库里的一个文件夹，视频、封面和字幕都是普通文件。切换资料库会停止进行中的任务。从列表移出资料库不会删除里面的文件。", nil, func() {
		s.libraryRows(c)
	})
	preferencesGroup(c, "", "", nil, func() {
		breeze.ActionRow(c, "界面语言", "", func() {
			breeze.Text(c, "简体中文", breeze.TextSecondary)
		})
		breeze.ActionRow(c, "版本", "", func() {
			breeze.Text(c, s.version, breeze.TextSecondary).Selectable()
		})
	})
	tools, known := s.Tools()
	var redetect func()
	if known {
		redetect = func() {
			c.Key("redetect")
			if breeze.Button(c, "重新检测").Clicked() {
				s.Redetect()
			}
		}
	}
	preferencesGroup(c, "组件", "这些程序来自本机，LingoCut 不负责安装或更新。", redetect, func() {
		s.toolRow(c, t, "yt-dlp", "下载在线视频", tools.YTDLP)
		s.toolRow(c, t, "FFmpeg", "音频转换、截取封面、解码画面", tools.FFmpeg)
		s.toolRow(c, t, "whisper-cli", "语音识别", tools.Whisper)
	})
}

// libraryRows choose the library, show where it is, and take it off the list. Without a library they offer to make or open one.
func (s screen) libraryRows(c *ui.Context) {
	dir := s.LibraryDir()
	if dir == "" {
		breeze.ActionRow(c, "当前资料库", "还没有资料库", func() {
			c.Key("library-new")
			if breeze.Button(c, "新建资料库…").Clicked() {
				s.BeginNew()
			}
			c.Key("library-open")
			if breeze.Button(c, "打开资料库…").Clicked() {
				s.PickLibrary()
			}
		})
		return
	}
	dirs := s.KnownLibraries()
	labels := libraryLabels(dirs)
	chosen := ""
	for i, known := range dirs {
		if filepath.Clean(known) == filepath.Clean(dir) {
			chosen = labels[i]
		}
	}
	c.Key("library")
	if breeze.ComboRow(c, "当前资料库", "", &chosen, labels).Changed() {
		for i, label := range labels {
			if label == chosen && filepath.Clean(dirs[i]) != filepath.Clean(dir) {
				s.Choose(dirs[i])
			}
		}
	}
	breeze.ActionRow(c, "位置", films.DisplayPath(dir), func() {
		c.Key("library-reveal")
		if breeze.Button(c, "在访达中显示").Clicked() {
			s.Reveal(dir)
		}
	})
	if len(dirs) > 1 {
		breeze.ActionRow(c, "从列表移出", "只从 LingoCut 的列表里拿掉，文件留在原处。", func() {
			c.Key("library-forget")
			if breeze.Button(c, "移出").Clicked() {
				s.Forget(dir)
			}
		})
	}
}

// libraryLabels names each library in the drop-down by its title, or by its path where two share a title.
func libraryLabels(dirs []string) []string {
	count := map[string]int{}
	for _, dir := range dirs {
		count[sidebar.Title(dir)]++
	}
	labels := make([]string, len(dirs))
	for i, dir := range dirs {
		labels[i] = sidebar.Title(dir)
		if count[labels[i]] > 1 {
			labels[i] = films.DisplayPath(dir)
		}
	}
	return labels
}

// toolRow is one of the programs LingoCut runs: what it is for and where it was found, with a dot and its version, or 未安装.
func (s screen) toolRow(c *ui.Context, t *ui.Theme, name, use, path string) {
	where := "未安装"
	if path != "" {
		where = films.DisplayPath(path)
	}
	breeze.ActionRow(c, name, use+" · "+where, func() {
		dot, status := t.Danger, "未安装"
		if path != "" {
			dot, status = t.Success, "已安装"
			if version := s.ToolVersion(path); version != "" {
				status = version
			}
		}
		statusDot(c, dot)
		breeze.Text(c, status, breeze.TextSecondary).Selectable()
	})
}

// statusDot is the small colored dot before a status.
func statusDot(c *ui.Context, color ui.Color) {
	ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(color)
}
