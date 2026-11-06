package films

import (
	"path/filepath"
	"strings"

	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// topBar is the bar along the top of the library: the source being shown and the library's name, the link field, 本地视频 and 开始处理. Enter in the field adds the link as 开始处理 does.
func topBar(c *ui.Context, t *ui.Theme, a App, source string) {
	headerBar(c, PageTitle(source), LibraryName(a.LibraryDir()), func() {
		linkField(c, t, a)
		breeze.Toolbar(c, func() {
			c.Key("films-local")
			if iconLabelButton(c, addVideoIcon, "本地视频").Tooltip("添加本地视频（⌘O）").Clicked() {
				a.PickVideos()
			}
			c.Key("films-start")
			if breeze.PrimaryButton(c, "开始处理").Clicked() {
				a.Start()
			}
		}).Shrink(0)
	})
}

// linkField is the pill the link is pasted into, with the link's platform at its end once it reads as one.
func linkField(c *ui.Context, t *ui.Theme, a App) {
	value := a.Source()
	field := ui.Row(c).Width(380).Shrink(1).MinWidth(200).Height(34).Radius(17).Padding(0, 8, 0, 12).Gap(8).AlignItems(ui.Center)
	field.Background(t.Background).Border(0.5, t.Text.Alpha(0.12)).Shadow(0, 2, 8, 0, t.Text.Alpha(0.06))
	field.Children(func() {
		ui.Icon(c, linkIcon).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
		in := ui.TextInputBase(c, value).Placeholder("粘贴 YouTube、哔哩哔哩等视频链接").Label("视频链接").Grow(1).MinWidth(0).FontSize(13)
		if in.Submitted() {
			a.Start()
		}
		if label, color, ok := linkChip(*value); ok {
			ui.Text(c, label).FontSize(11).Bold().TextColor(ui.Hex("#FFFFFF")).Background(color).Radius(10).Padding(2, 8).Shrink(0).SingleLine()
		}
	})
}

// linkChip names the platform of a link being typed, in the platform's color: YouTube's red, 哔哩哔哩's blue, and gray for another site. A list or a channel says so after the name.
func linkChip(raw string) (label string, color ui.Color, ok bool) {
	link, ok := core.ParseLink(strings.TrimSpace(raw))
	if !ok || link.Platform == core.PlatformLocal {
		return "", ui.Color{}, false
	}
	label = PlatformLabel(link)
	switch link.Kind {
	case core.LinkPlaylist:
		label += " · 合集"
	case core.LinkChannel:
		label += " · 频道"
	}
	switch link.Platform {
	case core.PlatformYouTube:
		color = ui.Hex("#E62117")
	case core.PlatformBilibili:
		color = ui.Hex("#00A1D6")
	default:
		color = ui.Hex("#6A6A70")
	}
	return label, color, true
}

// PageTitle is the library page's title for the source it shows.
func PageTitle(source string) string {
	switch source {
	case sidebar.SourceYouTube:
		return "YouTube"
	case sidebar.SourceBilibili:
		return "哔哩哔哩"
	case sidebar.SourceWeb:
		return "其他网站"
	case sidebar.SourceLocal:
		return "本地文件"
	}
	return "全部影片"
}

// LibraryName is a library folder's name without its .lingocut suffix.
func LibraryName(dir string) string {
	if dir == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(dir), media.LibraryExt)
}
