package subscribe

import (
	"slices"
	"strconv"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// List is the sidebar's 订阅 section: its title with the + button that opens the dialog a link is pasted in, then a row for each subscription of the open library, its site's mark, its name and, at its end, an orange ! when its site renamed or deleted it, how many new videos wait for the user, or that it is being checked. Choosing a row shows the subscription's page. While there is none, a line says how to subscribe. The sidebar (package sidebar) places it under its other lists.
type List struct{ App App }

// Render implements ns.Node.
func (l List) Render(cx *ns.Scope) ns.Node {
	a := l.App
	rows := ns.NewMemoFunc(cx, live.Func(a, a.Subscriptions), slices.Equal[[]Item])
	colors := ui.ColorsOf(cx)
	add := ui.IconButton{Icon: lucide.Plus, LabelFunc: func() string { return locales.SubscribeAddButton(cx) }, Size: 22, TestID: "subscribe-add", OnPress: live.Do(a, a.AskSubscription)}
	actions := ui.Tooltip{TextFunc: func() string { return locales.SubscribeAddTooltip(cx) }, Trigger: add, Style: ns.Style{FlexShrink: 0}}
	empty := ns.Text{TestID: "subscribe-empty", ContentFunc: func() string { return locales.SubscribeEmpty(cx) }, Style: ns.Style{FontSize: 11, LineHeight: 16, Padding: ns.Edges{Top: 2, Right: 10, Left: 10}}, StyleFunc: ui.Colored(colors.Muted)}
	return ui.Sidebar{
		TestID: "sidebar-subscriptions",
		SectionsFunc: func() []ui.SidebarSection {
			list := rows.Get()
			items := make([]ui.SidebarItem, len(list))
			for i, it := range list {
				items[i] = ui.SidebarItem{ID: it.ID, Text: rowName(cx, it), TestID: "subscription-" + it.ID, Content: rowContent(cx, it)}
			}
			section := ui.SidebarSection{Title: locales.SubscribeSection(cx), Items: items, Actions: actions}
			if len(list) == 0 {
				section.Note = empty
			}
			return []ui.SidebarSection{section}
		},
		Selected: live.Func(a, a.SubscriptionShown),
		OnSelect: live.On(a, func(id string) {
			if id != a.SubscriptionShown() {
				a.ShowSubscription(id)
			}
		}),
	}
}

// rowName is a row's accessible name: the subscription's name, with how many new videos wait, which the row draws as a number.
func rowName(cx *ns.Scope, it Item) string {
	if it.Waiting > 0 && it.Warn == WarnNone {
		return locales.SubscribeRowNew(cx, it.Name, it.Waiting)
	}
	return it.Name
}

// rowContent is a row's mark, name and what is at its end: the warning, the new videos' count or 检查中…, one of them, in that order of weight. A paused subscription's name is faded.
func rowContent(cx *ns.Scope, it Item) ns.Node {
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	name := ui.Text(it.Name, 13, ui.Regular, colors.Text)
	name.MaxLines, name.Style.FlexGrow, name.Style.FlexShrink = 1, 1, 1
	if it.Paused {
		name.StyleFunc = ui.Colored(colors.Muted)
	}
	children := []ns.Node{Mark(it.Platform, 18), name}
	switch {
	case it.Warn != WarnNone:
		tip := locales.SubscribeWarnRenamed(cx)
		if it.Warn == WarnGone {
			tip = locales.SubscribeWarnGone(cx)
		}
		bang := ns.Text{Content: "!", Label: tip, TestID: "subscription-warn-" + it.ID, MaxLines: 1,
			Style:     ns.Style{Width: ns.Pt(16), Height: ns.Pt(16), CornerRadius: 8, FontSize: 11, LineHeight: 16, FontWeight: ui.Bold, TextAlign: ns.TextAlignCenter, Color: ns.RGB(0xFFFFFF), FlexShrink: 0},
			StyleFunc: func() ns.Style { return ns.Style{Background: k.WarnInk()} }}
		children = append(children, ui.Tooltip{Text: tip, Trigger: bang, Style: ns.Style{FlexShrink: 0}})
	case it.Checking:
		children = append(children, ns.Text{Content: locales.SubscribeChecking(cx), MaxLines: 1, Style: ns.Style{FontSize: 11, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)})
	case it.Waiting > 0:
		count := ui.Tabular(ns.Text{Content: strconv.Itoa(it.Waiting), TestID: "subscription-waiting-" + it.ID, MaxLines: 1,
			Style:     ns.Style{FontSize: 10, LineHeight: 16, FontWeight: ui.Bold, Padding: ns.Symmetric(0, 6), CornerRadius: 8, FlexShrink: 0},
			StyleFunc: func() ns.Style { return ns.Style{Background: colors.AccentFill(), Color: colors.OnAccent()} }})
		children = append(children, count)
	}
	return ns.Group(children)
}

// Mark is a site's mark, size points square: YouTube's red with a ▶, bilibili's blue with a B, any other site's grey with a ◎, the white glyph on the site's own color as the sidebar of the design draws them.
func Mark(platform core.Platform, size ns.Points) ns.Node {
	glyph, fill := "◎", ns.RGB(0x8E8E93)
	switch platform {
	case core.PlatformYouTube:
		glyph, fill = "▶", ns.RGB(0xE62117)
	case core.PlatformBilibili:
		glyph, fill = "B", ns.RGB(0x00A1D6)
	}
	return ns.View{Role: ns.RoleNone, Style: ns.Style{Width: ns.Pt(size), Height: ns.Pt(size), CornerRadius: size * 5 / 18, Background: fill, AlignItems: ns.AlignCenter, JustifyContent: ns.JustifyCenter, FlexShrink: 0},
		Children: []ns.Node{ns.Text{Content: glyph, Role: ns.RoleNone, MaxLines: 1, Style: ns.Style{FontSize: size * 10 / 18, FontWeight: ui.Bold, Color: ns.RGB(0xFFFFFF)}}}}
}
