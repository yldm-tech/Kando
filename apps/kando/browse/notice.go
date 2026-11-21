package browse

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// notice is the message bar under the toolbar: what went wrong, or that something worked, with 打开设置 when a setting fixes what it says, 在资料库中显示 when it names a video the library has, Add Whole Playlist when the video it added is one of a playlist's, and its close button.
type notice struct{ a App }

// Render implements core.Node.
func (n notice) Render(cx *core.Scope) core.Node {
	a := n.a
	return core.Show{
		When: live.Func(a, func() bool { return a.BrowseNotice().Open }),
		Then: func(cx *core.Scope) core.Node { return noticeBar(cx, a) },
	}
}

// noticeBar is the bar itself, built while it shows: tinted as a warning or as a success, its text taking the room the buttons leave.
func noticeBar(cx *core.Scope, a App) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	text := live.Func(a, func() string { return a.BrowseNotice().Text })
	success := live.Memo(cx, a, func() bool { return a.BrowseNotice().Kind == "success" })
	fixable := live.Memo(cx, a, func() bool { return a.BrowseNotice().Section != "" })
	settings := core.Show{When: fixable, Then: func(*core.Scope) core.Node {
		// The button's words are set as it is built, so it is built again for another language.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Button{Text: locales.BrowseOpenSettings(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "browse-notice-settings",
				OnPress: live.Do(a, func() { a.OpenSettings(a.BrowseNotice().Section) })}
		})
	}}
	film := core.Show{When: live.Memo(cx, a, func() bool { return a.BrowseNotice().Film != "" }), Then: func(*core.Scope) core.Node {
		// The button's words are set as it is built, so it is built again for another language.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Button{Text: locales.BrowseShowInLibrary(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "browse-notice-film",
				OnPress: live.Do(a, func() {
					n := a.BrowseNotice()
					n.Open = false
					a.BrowseShowFilm(n.Film)
				})}
		})
	}}
	list := core.Show{When: live.Memo(cx, a, func() bool { return a.BrowseNotice().List != "" }), Then: func(*core.Scope) core.Node {
		// The button's words are set as it is built, so it is built again for another language.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Button{Text: locales.PlaylistAddWhole(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "browse-notice-list",
				OnPress: live.Do(a, func() { a.BrowseAddList(a.BrowseNotice().List) })}
		})
	}}
	closer := ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.BrowseNoticeClose(cx) }, Size: 22, TestID: "browse-notice-close",
		OnPress: live.Do(a, func() { a.BrowseNotice().Open = false })}
	bar := core.View{
		TestID: "browse-notice",
		Role:   core.RoleStatus,
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(36), CornerRadius: 8,
			Padding: core.Edges{Top: 6, Right: 6, Bottom: 6, Left: 14}},
		StyleFunc: func() core.Style {
			if success() {
				return core.Style{Background: tints.Up()}
			}
			return core.Style{Background: tints.Warn()}
		},
		Children: []core.Node{
			core.Text{ContentFunc: text, Style: ui.TypeBody.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: ui.Colored(colors.Text)},
			settings,
			film,
			list,
			closer,
		},
	}
	return core.View{Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch, Padding: core.Edges{Top: 10, Right: 14, Left: 14}}, Children: []core.Node{bar}}
}
