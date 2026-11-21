package sidebar

import (
	"strconv"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// sources is the flat list of the library's sources, each with how many films of the open library come from it: 全部影片, YouTube, 哔哩哔哩, 其他网站 while it holds a film or is the one shown, and 本地文件, then 媒体来源, without a count, while a media source was added, whose page it shows (PageMediaSources). Choosing one, with a click or the arrow keys, shows the library page filtered by it. 浏览, the built-in browser, sits right after 全部影片 without a count, and choosing it shows its page (PageBrowse). Off those pages, as on a settings page, none is chosen. An open film fills the window and hides the sidebar, so a click on the source already chosen has nothing to close.
type sources struct{ a App }

// sourceRow is one row of the list as it shows: comparable, so the list is built again only when a row's label or count changed, not after every change of the app's state.
type sourceRow struct {
	id, label string
	icon      func() *core.SVG
	count     int
	counted   bool
}

// sourceRows are the rows in order; a fixed array keeps them comparable for a memo.
type sourceRows struct {
	rows [7]sourceRow
	n    int
}

// Render implements core.Node.
func (s sources) Render(cx *core.Scope) core.Node {
	a := s.a
	current := live.Func(a, func() string { return currentSource(a) })
	rows := core.NewMemoFunc(cx, live.Func(a, func() sourceRows { return sourceRowsOf(cx, a, currentSource(a)) }), func(x, y sourceRows) bool {
		if x.n != y.n {
			return false
		}
		for i := range x.n {
			p, q := x.rows[i], y.rows[i]
			if p.id != q.id || p.label != q.label || p.count != q.count || p.counted != q.counted {
				return false
			}
		}
		return true
	})
	colors := ui.ColorsOf(cx)
	return core.View{
		Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0), Padding: core.Symmetric(0, inset)},
		Children: []core.Node{ui.Sidebar{
			TestID: "sidebar-sources",
			SectionsFunc: func() []ui.SidebarSection {
				list := rows.Get()
				items := make([]ui.SidebarItem, list.n)
				for i, r := range list.rows[:list.n] {
					items[i] = ui.SidebarItem{ID: r.id, Text: rowName(cx, r.label, r.count, r.counted), TestID: "source-" + r.id, Content: sourceContent(colors, theme.Of(cx).Glyph, r)}
				}
				return []ui.SidebarSection{{Items: items}}
			},
			Selected: current,
			OnSelect: live.On(a, func(id string) {
				switch {
				case id == currentSource(a):
				case id == PageBrowse, id == PageMediaSources:
					a.Select(id)
				default:
					a.SetSourceFilter(id)
				}
			}),
		}},
	}
}

// currentSource is the row chosen: the source the library page shows, 浏览 on the browser's page, none elsewhere.
func currentSource(a App) string {
	switch a.Page() {
	case PageFilms:
		return a.SourceFilter()
	case PageBrowse, PageMediaSources:
		return a.Page()
	}
	return ""
}

// sourceRowsOf is the list as it shows now, current being the row chosen.
func sourceRowsOf(cx *core.Scope, a App, current string) sourceRows {
	var out sourceRows
	add := func(r sourceRow) {
		out.rows[out.n] = r
		out.n++
	}
	for _, source := range []string{SourceAll, SourceYouTube, SourceBilibili, SourceWeb, SourceLocal} {
		n := a.SourceCount(source)
		if source == SourceWeb && n == 0 && current != SourceWeb {
			continue
		}
		add(sourceRow{id: source, label: SourceLabel(cx, source), icon: sourceIcon(source), count: n, counted: true})
		if source == SourceAll {
			add(sourceRow{id: PageBrowse, label: locales.SidebarBrowse(cx), icon: svg(iconBrowse)})
		}
	}
	if a.MediaSourcesShown() {
		add(sourceRow{id: PageMediaSources, label: locales.MediaSourceSidebar(cx), icon: lucide.Server})
	}
	return out
}

// rowName is the accessible name of a source or a language row: its label, with the count it draws at its end when it is counted, as the count is not read from the row's content.
func rowName(cx *core.Scope, label string, count int, counted bool) string {
	if !counted {
		return label
	}
	return locales.SidebarRowCount(cx, label, count)
}

// sourceContent is a row's icon, label and count, as ui.Sidebar draws an item's icon and label, with the count at its end.
func sourceContent(colors ui.Colors, glyph func() core.Color, r sourceRow) core.Node {
	label := ui.Text(r.label, 13, ui.Regular, colors.Text)
	label.MaxLines, label.Style.FlexGrow, label.Style.FlexShrink = 1, 1, 1
	children := []core.Node{ui.Icon(r.icon, 16, glyph), label}
	if r.counted {
		count := ui.Tabular(ui.Text(strconv.Itoa(r.count), 12, ui.Regular, colors.Muted))
		count.TestID, count.Style.FlexShrink = "count-"+r.id, 0
		children = append(children, count)
	}
	return core.Group(children)
}

// SourceLabel is the name the sidebar gives source in the interface's language under cx, which the library page can take as its title.
func SourceLabel(cx *core.Scope, source string) string {
	switch source {
	case SourceYouTube:
		return "YouTube"
	case SourceBilibili:
		return locales.SidebarSourceBilibili(cx)
	case SourceWeb:
		return locales.SidebarSourceWeb(cx)
	case SourceLocal:
		return locales.SidebarSourceLocal(cx)
	default:
		return locales.SidebarSourceAll(cx)
	}
}

func sourceIcon(source string) func() *core.SVG {
	switch source {
	case SourceYouTube:
		return svg(iconGlobe)
	case SourceBilibili:
		return svg(iconTV)
	case SourceWeb:
		return svg(iconLink)
	case SourceLocal:
		return svg(iconDrive)
	default:
		return svg(iconFilms)
	}
}
