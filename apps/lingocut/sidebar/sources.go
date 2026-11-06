package sidebar

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// sources is the flat list of the library's sources, each with how many films of the open library come from it: 全部影片, YouTube, 哔哩哔哩, 其他网站 while it holds a film or is the one shown, and 本地文件. Choosing one, with a click or the arrow keys, shows the library page filtered by it. Off the library page, as on a settings page, none is chosen. An open film fills the window and hides the sidebar, so a click on the source already chosen has nothing to close.
func sources(c *ui.Context, a App) {
	current := ""
	if a.Page() == PageFilms {
		current = a.SourceFilter()
	}
	sel := current
	breeze.Sidebar(c, &sel, func() {
		for _, source := range []string{SourceAll, SourceYouTube, SourceBilibili, SourceWeb, SourceLocal} {
			n := a.SourceCount(source)
			if source == SourceWeb && n == 0 && current != SourceWeb {
				continue
			}
			breeze.SidebarItem(c, source, sourceIcon(source), SourceLabel(source)).Children(func() {
				breeze.Text(c, strconv.Itoa(n), breeze.TextSecondary).FontSize(12).Shrink(0)
			})
		}
	}).Background(ui.Transparent).Padding(0, 8).Grow(1)
	if sel != current {
		a.SetSourceFilter(sel)
	}
}

// SourceLabel is the name the sidebar gives source, which the library page can take as its title.
func SourceLabel(source string) string {
	switch source {
	case SourceYouTube:
		return "YouTube"
	case SourceBilibili:
		return "哔哩哔哩"
	case SourceWeb:
		return "其他网站"
	case SourceLocal:
		return "本地文件"
	default:
		return "全部影片"
	}
}

func sourceIcon(source string) *ui.SVG {
	switch source {
	case SourceYouTube:
		return iconGlobe
	case SourceBilibili:
		return iconTV
	case SourceWeb:
		return iconLink
	case SourceLocal:
		return iconDrive
	default:
		return iconFilms
	}
}
