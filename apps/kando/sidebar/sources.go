package sidebar

import (
	"strconv"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// sources is the flat list of the library's sources, each with how many films of the open library come from it: 全部影片, YouTube, 哔哩哔哩, 其他网站 while it holds a film or is the one shown, and 本地文件. Choosing one, with a click or the arrow keys, shows the library page filtered by it. 浏览, the built-in browser, sits right after 全部影片 without a count, and choosing it shows its page (PageBrowse). Off those pages, as on a settings page, none is chosen. An open film fills the window and hides the sidebar, so a click on the source already chosen has nothing to close.
func sources(c *ui.Context, a App) {
	current := ""
	switch a.Page() {
	case PageFilms:
		current = a.SourceFilter()
	case PageBrowse:
		current = PageBrowse
	}
	sel := current
	mac := breeze.PlatformOf(c) == breeze.Mac
	breeze.Sidebar(c, &sel, func() {
		for _, source := range []string{SourceAll, SourceYouTube, SourceBilibili, SourceWeb, SourceLocal} {
			n := a.SourceCount(source)
			if source == SourceWeb && n == 0 && current != SourceWeb {
				continue
			}
			// The prototype's rows stand 2 points apart, where a source list's touch.
			row := breeze.SidebarItem(c, source, sourceIcon(source), SourceLabel(source)).Margin(0, 0, 2, 0)
			if mac {
				// The prototype pads its rows 10 points at the end, for the count, and centers the icon 19 points in, where a source list's is 17.5: its 18-point icon starts 10 in.
				row.PaddingStart(11).PaddingEnd(10)
			}
			// SidebarItemDetail turns the count near white on the accent selection while the sidebar has the focus, as the label is, where secondary text would vanish into the blue.
			row.Children(func() {
				breeze.SidebarItemDetail(c, strconv.Itoa(n)).FontSize(12)
			})
			if source == SourceAll {
				browse := breeze.SidebarItem(c, PageBrowse, iconBrowse, locales.SidebarBrowse()).Margin(0, 0, 2, 0)
				if mac {
					browse.PaddingStart(11).PaddingEnd(10)
				}
			}
		}
	}).Background(ui.Transparent).Padding(0, insetEnd, 0, insetStart).Grow(1)
	switch {
	case sel == current:
	case sel == PageBrowse:
		a.Select(PageBrowse)
	default:
		a.SetSourceFilter(sel)
	}
}

// SourceLabel is the name the sidebar gives source in the interface's language, which the library page can take as its title.
func SourceLabel(source string) string {
	switch source {
	case SourceYouTube:
		return "YouTube"
	case SourceBilibili:
		return locales.SidebarSourceBilibili()
	case SourceWeb:
		return locales.SidebarSourceWeb()
	case SourceLocal:
		return locales.SidebarSourceLocal()
	default:
		return locales.SidebarSourceAll()
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
