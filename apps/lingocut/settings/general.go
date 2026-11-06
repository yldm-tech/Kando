package settings

import (
	"path/filepath"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/settings/form"
	"lingocut/apps/lingocut/sidebar"

	"github.com/egoist/mygo/ui"
)

// general is the 通用 page: the library, the interface language and version, and the programs LingoCut runs.
func (s screen) general(c *ui.Context, t *ui.Theme) {
	form.Label(c, "资料库", 6, nil)
	form.Card(c, func() { s.libraryRows(c) })
	form.Note(c, "切换资料库会停止进行中的任务。从列表移出资料库不会删除里面的文件。", 10)
	form.Card(c, func() {
		form.Row(c, "界面语言", "", func() { form.Value(c, "简体中文", 13) })
		form.Separator(c, true)
		form.Row(c, "版本", "", func() { form.Value(c, s.version, 13).Selectable() })
	})
	tools, known := s.Tools()
	var redetect func()
	if known {
		redetect = func() {
			c.Key("redetect")
			if form.Button(c, "重新检测", true).Clicked() {
				s.Redetect()
			}
		}
	}
	form.Label(c, "组件", 14, redetect)
	form.Card(c, func() {
		s.toolRow(c, t, "FF", "ffmpeg", "音频转换、截取封面、解码画面", tools.FFmpeg)
		form.Separator(c, false)
		s.toolRow(c, t, "YT", "yt-dlp", "下载在线视频", tools.YTDLP)
		form.Separator(c, false)
		s.toolRow(c, t, "W", "whisper-cli", "语音识别", tools.Whisper)
	})
	form.Note(c, "这些程序来自本机，LingoCut 不负责安装或更新。", 0)
}

// libraryRows choose the library, show where it is, and take it off the list. Without a library they offer to make or open one.
func (s screen) libraryRows(c *ui.Context) {
	dir := s.LibraryDir()
	if dir == "" {
		form.Row(c, "当前资料库", "还没有资料库", func() {
			c.Key("library-new")
			if form.Button(c, "新建资料库…", false).Clicked() {
				s.BeginNew()
			}
			c.Key("library-open")
			if form.Button(c, "打开资料库…", false).Clicked() {
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
	form.Row(c, "当前资料库", "", func() {
		c.Key("library")
		if form.Select(c, &chosen, labels).Label("当前资料库").Changed() {
			for i, label := range labels {
				if label == chosen && filepath.Clean(dirs[i]) != filepath.Clean(dir) {
					s.Choose(dirs[i])
				}
			}
		}
	})
	form.Separator(c, true)
	form.Row(c, "位置", "", func() {
		form.Value(c, films.DisplayPath(dir), 12).Selectable()
		c.Key("library-reveal")
		if form.Button(c, "在访达中显示", false).Clicked() {
			s.Reveal(dir)
		}
	})
	if len(dirs) > 1 {
		form.Separator(c, true)
		form.Row(c, "从列表移出", "只从 LingoCut 的列表里拿掉，文件留在原处。", func() {
			c.Key("library-forget")
			if form.Button(c, "移出", false).Clicked() {
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

// toolRow is one of the programs LingoCut runs: its tile, its name over what it is for and where it was found, and a dot with its version, or 未安装.
func (s screen) toolRow(c *ui.Context, t *ui.Theme, mark, name, use, path string) {
	where := "未安装"
	if path != "" {
		where = films.DisplayPath(path)
	}
	ui.Row(c).Shrink(0).MinHeight(48).Padding(6, 14).Gap(12).AlignItems(ui.Center).Role(ui.RoleGroup).Label(name).Children(func() {
		ui.Row(c).Size(30, 30).Shrink(0).Radius(8).Background(ui.Hex("#F0F1F4")).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, mark).Font("monospace").FontSize(10).FontWeight(700).TextColor(ui.Hex("#3A3A3F")).Role(ui.RoleNone)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(form.Text)
			ui.Text(c, use+" · "+where).FontSize(11).TextColor(form.Secondary).Wrap()
		})
		dot, status := t.Danger, "未安装"
		if path != "" {
			dot, status = form.OK, "已安装"
			if version := s.ToolVersion(path); version != "" {
				status = version
			}
		}
		form.Status(c, dot, form.Secondary, status)
	})
}
