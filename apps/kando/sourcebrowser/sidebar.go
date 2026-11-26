package sourcebrowser

import (
	"slices"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// SidebarList is the sidebar's 媒体来源 section: its title with the + button that adds a source (AddMediaBrowseSource), then a row for each source added, its name, which shows its folders on the 媒体来源 page. While there is none, a line says what a source is. The sidebar (package sidebar) places it under the library's sources.
type SidebarList struct{ App App }

// Render implements core.Node.
func (l SidebarList) Render(cx *core.Scope) core.Node {
	a := l.App
	list := core.NewMemoFunc(cx, live.Func(a, a.MediaBrowseSources), slices.Equal[[]Source])
	colors := ui.ColorsOf(cx)
	add := ui.IconButton{Icon: lucide.Plus, LabelFunc: func() string { return locales.MediaSourceAddSource(cx) }, Size: 22, TestID: "media-sources-sidebar-add", OnPress: live.Do(a, a.AddMediaBrowseSource)}
	actions := ui.Tooltip{TextFunc: func() string { return locales.MediaSourceAddSource(cx) }, Trigger: add, Style: core.Style{FlexShrink: 0}}
	empty := core.Text{TestID: "media-sources-sidebar-empty", ContentFunc: func() string { return locales.MediaSourceSidebarEmpty(cx) }, Style: core.Style{FontSize: 11, LineHeight: 16, Padding: core.Edges{Top: 2, Right: 10, Left: 10}}, StyleFunc: ui.Colored(colors.Muted)}
	return ui.Sidebar{
		TestID: "sidebar-media-sources",
		SectionsFunc: func() []ui.SidebarSection {
			sources := list.Get()
			items := make([]ui.SidebarItem, len(sources))
			for i, s := range sources {
				name := ui.Text(s.Name, 13, ui.Regular, colors.Text)
				name.MaxLines, name.Style.FlexGrow, name.Style.FlexShrink = 1, 1, 1
				items[i] = ui.SidebarItem{ID: s.ID, Text: s.Name, TestID: "media-sources-source-" + s.ID, Content: core.Group{ui.Icon(lucide.Server, 16, theme.Of(cx).Glyph), name}}
			}
			section := ui.SidebarSection{Title: locales.MediaSourceSidebar(cx), Items: items, Actions: actions}
			if len(sources) == 0 {
				section.Note = empty
			}
			return []ui.SidebarSection{section}
		},
		Selected: live.Func(a, a.MediaBrowseSidebarSource),
		OnSelect: live.On(a, func(id string) {
			if id != a.MediaBrowseSidebarSource() {
				a.ShowMediaBrowseSource(id)
			}
		}),
	}
}
