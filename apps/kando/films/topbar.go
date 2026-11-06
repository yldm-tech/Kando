package films

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// barFade is the top bar's face: the page's white, nearly opaque at the top and fading out at its bottom, so what scrolls under it fades into it, with no blur and no line, as the prototype's toolbar.
var barFade = ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
	{Color: ui.RGBA(255, 255, 255, 0.96), At: 0},
	{Color: ui.RGBA(255, 255, 255, 0.86), At: 0.6},
	{Color: ui.RGBA(255, 255, 255, 0), At: 1},
}}

// topBar is the bar along the top of the library: the source being shown and the library's name, the link field, 本地视频 and 开始处理. Enter in the field adds the link as 开始处理 does. It lies over the page's scroll view, which scrolls under it.
func topBar(c *ui.Context, t *ui.Theme, a App, source string) {
	title, subtitle := PageTitle(source), LibraryName(a.LibraryDir())
	end := func() {
		linkField(c, t, a)
		tools := breeze.Toolbar(c, func() {
			localButton(c, a)
			c.Key("films-start")
			// The prototype's toolbar pills are 34 points high; the platform draws their faces.
			if breeze.PrimaryButton(c, "开始处理").Height(34).Radius(17).PaddingX(18).Shadow(0, 2, 8, 0, t.Accent.Alpha(0.3)).Clicked() {
				a.Start()
			}
		}).Shrink(0)
		tools.Padding(0).Gap(10).MinHeight(0).MarginStart(2)
	}
	mac := breeze.PlatformOf(c) == breeze.Mac
	var bar ui.Element
	if mac {
		// On macOS the bar takes the prototype's toolbar: 65 points high, its controls 10 from its top and 14 from its end, and its title 24 from its start at 15 points over the library's name; the sidebar holds the traffic lights, so none of its room is the window controls'.
		bar = breeze.HeaderBar(c, "", "", func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(15).Bold().LetterSpacing(-0.1).SingleLine()
				ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).SingleLine().Hidden(subtitle == "")
			})
		}, end)
		bar.MinHeight(65).Padding(10, 14, 18, 24).Label(title)
	} else {
		bar = breeze.HeaderBar(c, title, subtitle, nil, end)
	}
	bar.Absolute().Top(0).Left(0).Right(0).Background(ui.Transparent).LinearGradient(barFade).Draw(func(*ui.Painter, ui.Rect) {})
}

// localButton is 本地视频 as the prototype groups it: the platform's toolbar button, 28 points high, in a white pill 104 wide that lifts it off what scrolls under the bar.
func localButton(c *ui.Context, a App) {
	ui.Row(c).Width(104).Height(34).Padding(0, 3).Radius(17).AlignItems(ui.Center).Background(ui.RGBA(255, 255, 255, 0.75)).Border(0.5, ui.RGBA(0, 0, 0, 0.12)).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
		c.Key("films-local")
		if breeze.IconLabelButton(c, addVideoIcon, "本地视频").Tooltip("添加本地视频（⌘O）").Height(28).Radius(14).Grow(1).Padding(0, 12, 0, 10).Clicked() {
			a.PickVideos()
		}
	})
}

// linkField is the pill the link is pasted into, with the link's platform at its end once it reads as one, and the prototype's focus ring around it while it has the focus.
func linkField(c *ui.Context, t *ui.Theme, a App) {
	value := a.Source()
	field := ui.Row(c).Width(380).Shrink(1).MinWidth(200).Height(34).Radius(17).Padding(0, 8, 0, 12).Gap(7).AlignItems(ui.Center)
	field.Background(ui.RGBA(255, 255, 255, 0.75)).Border(0.5, ui.RGBA(0, 0, 0, 0.12)).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.06))
	focused := false
	field.Children(func() {
		ui.Icon(c, linkIcon).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
		in := ui.TextInputBase(c, value).Placeholder("粘贴 YouTube、哔哩哔哩等视频链接").Label("视频链接").Grow(1).MinWidth(0).FontSize(13).FocusRing(false)
		if in.Submitted() {
			a.Start()
		}
		focused = in.Focused()
		if label, color, ok := linkChip(*value); ok {
			ui.Text(c, label).FontSize(11).Bold().TextColor(ui.Hex("#FFFFFF")).Background(color).Radius(10).Padding(3, 8).MarginStart(16).Shrink(0).SingleLine()
		}
	})
	if focused {
		ring := t.Accent.Alpha(0.5)
		field.DrawOver(func(p *ui.Painter, r ui.Rect) {
			// outline: 3px solid rgba(10,102,216,.5); outline-offset: 1px.
			p.Stroke(ui.Rect{X: r.X - 2.5, Y: r.Y - 2.5, W: r.W + 5, H: r.H + 5}, ring, 19.5, 3)
		})
	}
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
