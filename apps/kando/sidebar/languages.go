package sidebar

import (
	"slices"
	"strconv"
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// SpokenAll is the spoken-language filter that keeps the films of every language, as SourceAll keeps those of every source.
const SpokenAll = "all"

// SpokenCount is how many done films of the open library were heard in a language, Code being whisper's code for it (ja, en).
type SpokenCount struct {
	Code  string
	Count int
}

// languages is the list of the languages heard in the open library's done films, each with how many, under 全部语言, shown only while the library holds films heard in more than one. Choosing one shows the library page's films heard in it, among those of the source chosen above, as both apply; 全部语言 shows them in every language. Off the library page none is chosen.
type languages struct{ a App }

// languageRow is one row of the list as it shows: comparable, so the list is built again only when a row's label or count changed.
type languageRow struct {
	code, label string
	count       int
}

// Render implements core.Node.
func (l languages) Render(cx *core.Scope) core.Node {
	a := l.a
	rows := core.NewMemoFunc(cx, live.Func(a, func() []languageRow { return languageRowsOf(cx, a.SpokenCounts()) }), slices.Equal[[]languageRow])
	current := live.Func(a, func() string {
		if a.Page() == PageFilms {
			return a.SpokenFilter()
		}
		return ""
	})
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	return core.Show{When: func() bool { return len(rows.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
		return core.View{
			// The list takes the room its rows need, under the sources, and scrolls when the sidebar is too short for it.
			Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0), Padding: core.Edges{Right: insetEnd, Left: insetStart}},
			Children: []core.Node{ui.Sidebar{
				TestID: "sidebar-languages",
				SectionsFunc: func() []ui.SidebarSection {
					list := rows.Get()
					items := make([]ui.SidebarItem, len(list))
					for i, r := range list {
						items[i] = ui.SidebarItem{ID: r.code, Text: rowName(cx, r.label, r.count, r.code != SpokenAll), TestID: "spoken-" + r.code, Content: languageContent(colors, k, r)}
					}
					return []ui.SidebarSection{{Title: locales.SidebarLanguages(cx), Items: items}}
				},
				Selected: current,
				OnSelect: live.On(a, func(code string) {
					if code != current() {
						a.SetSpokenFilter(code)
					}
				}),
			}},
		}
	}}
}

// languageRowsOf is the list as it shows for counts, the languages heard in the library's done films: 全部语言 and a row for each, or none while fewer than two languages were heard.
func languageRowsOf(cx *core.Scope, counts []SpokenCount) []languageRow {
	if len(counts) < 2 {
		return nil
	}
	rows := []languageRow{{code: SpokenAll, label: locales.SidebarLanguagesAll(cx)}}
	for _, c := range counts {
		rows = append(rows, languageRow{code: c.Code, label: subtitle.SpokenLanguageName(cx, c.Code), count: c.Count})
	}
	return rows
}

// languageContent is a row's mark, label and count, as sourceContent draws a source's: 全部语言 has the languages icon and no count, and a language its code in the icon's place, as the posters tag it (JA), in the accent ink, which reads at its size in both appearances.
func languageContent(colors ui.Colors, k theme.Colors, r languageRow) core.Node {
	label := ui.Text(r.label, 13, ui.Regular, colors.Text)
	label.MaxLines, label.Style.FlexGrow, label.Style.FlexShrink = 1, 1, 1
	if r.code == SpokenAll {
		return core.Group{ui.Icon(svg(iconLanguages), 16, colors.Accent), label}
	}
	tag := ui.Text(strings.ToUpper(r.code), 9, ui.Bold, k.AccentInk)
	tag.MaxLines, tag.Style.TextAlign = 1, core.TextAlignCenter
	count := ui.Tabular(ui.Text(strconv.Itoa(r.count), 12, ui.Regular, colors.Muted))
	count.TestID, count.Style.FlexShrink = "spoken-count-"+r.code, 0
	return core.Group{core.View{Style: core.Style{Width: core.Pt(16), FlexShrink: 0, AlignItems: core.AlignCenter}, Children: []core.Node{tag}}, label, count}
}
