package settings

import (
	"fmt"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// PluginIndexState is where a pack of the community index stands on this computer.
type PluginIndexState int

const (
	// PluginIndexAvailable is a pack not installed, which 安装 downloads and offers in the install confirmation.
	PluginIndexAvailable PluginIndexState = iota
	// PluginIndexInstalled is a pack installed at the version the index lists or a newer one.
	PluginIndexInstalled
	// PluginIndexUpdate is a pack installed at an older version than the index lists, which 更新 downloads and offers in the install confirmation.
	PluginIndexUpdate
	// PluginIndexNeedsKando is a pack that runs only on a newer Kando than this one (index.Entry.MinKando).
	PluginIndexNeedsKando
	// PluginIndexDownloading is the pack being downloaded.
	PluginIndexDownloading
)

// PluginIndexRow is a pack of the community index as 浏览插件 lists it: the index's entry, where it stands here, and the version installed, "" when none is.
type PluginIndexRow struct {
	Entry     index.Entry
	State     PluginIndexState
	Installed string
}

// PluginIndexApp is what 浏览插件, the community plugin index on the 插件 page, reads and the edits it reports. The index is read only once the user opens it, and installing goes through the install confirmation (PluginInstallDialog) as a file picked does.
type PluginIndexApp interface {
	// PluginIndexOpen reports whether 浏览插件 shows, and SetPluginIndexOpen shows or hides it; shown the first time, the app reads the index.
	PluginIndexOpen() bool
	SetPluginIndexOpen(open bool)
	// PluginIndexStatus says how the list stands: being read, updated when, why it could not be updated, offline, or that this Kando has no index; PluginIndexFailed reports whether that is a failure. PluginIndexCanRefresh reports whether 刷新 can ask for a newer list now: Kando is online, has an index and is not asking already.
	PluginIndexStatus() string
	PluginIndexFailed() bool
	PluginIndexCanRefresh() bool
	RefreshPluginIndex()
	// PluginIndexQuery is the search and PluginIndexKind the kind filter, "" for every kind; PluginIndexKinds are the kinds the index lists.
	PluginIndexQuery() string
	SetPluginIndexQuery(query string)
	PluginIndexKind() plugins.Kind
	SetPluginIndexKind(kind plugins.Kind)
	PluginIndexKinds() []plugins.Kind
	// PluginIndexRows are the packs that match the search and the filter, at most a page of them, and PluginIndexMatches how many match in all. PluginIndexInstallable reports whether 安装 and 更新 can start now: Kando is online and no other install or confirmation is under way.
	PluginIndexRows() []PluginIndexRow
	PluginIndexMatches() int
	PluginIndexInstallable() bool
	// PluginIndexDropped is how many entries of the list this version of Kando cannot show.
	PluginIndexDropped() int
	// InstallFromPluginIndex downloads the pack id and opens its install confirmation; CancelPluginIndexDownload stops that before the confirmation opens.
	InstallFromPluginIndex(id string)
	CancelPluginIndexDownload()
	// PluginIndexUpdates is how many installed packs the index lists newer, from the list last read, which Kando reads from disk as it starts and asks the network for only as 浏览插件 opens. ShowPluginIndexUpdates shows 浏览插件 listing only them, PluginIndexUpdatesOnly reports whether it does, and ShowAllPluginIndex lists every pack again.
	PluginIndexUpdates() int
	ShowPluginIndexUpdates()
	PluginIndexUpdatesOnly() bool
	ShowAllPluginIndex()
	// PluginIndexLinkNote says why a link of Kando's site that asked to install a pack did not: it named no pack, or one the index does not list; "" otherwise.
	PluginIndexLinkNote() string
}

// pluginIndexUpdates is the line at the top of the 插件 page while the index lists installed packs newer: 有可用更新 with their count, and the button that shows 浏览插件 listing only them; hidden while it does.
type pluginIndexUpdates struct{ a PluginsApp }

// Render implements core.Node.
func (u pluginIndexUpdates) Render(cx *core.Scope) core.Node {
	a := u.a
	return core.Show{When: live.Func(a, func() bool { return a.PluginIndexUpdates() > 0 && !(a.PluginIndexOpen() && a.PluginIndexUpdatesOnly()) }), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		text := core.Text{TestID: "settings-plugins-updates", ContentFunc: live.Func(a, func() string { return locales.SettingsPluginsIndexUpdatesAvailable(cx, a.PluginIndexUpdates()) }),
			Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Medium, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)}
		show := form.Button{Text: locales.SettingsPluginsIndexUpdatesShow(cx), TestID: "settings-plugins-updates-show", OnPress: live.Do(a, a.ShowPluginIndexUpdates)}
		return core.View{Style: core.Style{Margin: core.Edges{Bottom: 6}}, Children: []core.Node{form.Card(core.View{
			Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, MinHeight: core.Pt(44), Padding: core.Symmetric(6, 14)},
			Children: []core.Node{ui.Icon(lucide.CircleArrowUp, 16, colors.Accent), text, show},
		})}}
	}}
}

// pluginIndexButton is the 插件 page's 浏览插件…, which shows 浏览插件 at the top of the page; off while it shows, as its header has the button that hides it.
func pluginIndexButton(cx *core.Scope, a PluginsApp) core.Node {
	return form.Button{Text: locales.SettingsPluginsIndexBrowse(cx), TestID: "settings-plugins-browse", Disabled: live.Func(a, a.PluginIndexOpen), OnPress: live.Do(a, func() { a.SetPluginIndexOpen(true) })}
}

// pluginIndex is 浏览插件 while it shows: its heading with 刷新, how the list stands, the search field and the kind filter, and the packs that match, each with its kind, summary, publisher, version and size, and 安装, 更新 or where it stands; a line under them when more match than show.
type pluginIndex struct{ a PluginsApp }

// Render implements core.Node.
func (p pluginIndex) Render(cx *core.Scope) core.Node {
	a := p.a
	return core.Show{When: live.Func(a, a.PluginIndexOpen), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		refresh := form.Button{Text: locales.SettingsPluginsIndexRefresh(cx), TestID: "settings-plugins-index-refresh",
			Disabled: live.Func(a, func() bool { return !a.PluginIndexCanRefresh() }), OnPress: live.Do(a, a.RefreshPluginIndex)}
		hide := form.Button{Text: locales.SettingsPluginsIndexHide(cx), TestID: "settings-plugins-index-hide", OnPress: live.Do(a, func() { a.SetPluginIndexOpen(false) })}
		status := core.Text{TestID: "settings-plugins-index-status", ContentFunc: live.Func(a, a.PluginIndexStatus), Selectable: core.SelectableOn,
			Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 12)},
			StyleFunc: live.Func(a, func() core.Style {
				if a.PluginIndexFailed() {
					return core.Style{Color: colors.Danger()}
				}
				return core.Style{Color: colors.Muted()}
			})}
		search := ui.Input{TestID: "settings-plugins-index-search", Label: locales.SettingsPluginsIndexSearch(cx), Placeholder: locales.SettingsPluginsIndexSearch(cx),
			Value: live.Func(a, a.PluginIndexQuery), OnChange: live.On(a, a.SetPluginIndexQuery), Style: core.Style{FlexGrow: 1, FlexShrink: 1}}
		// The filter's choices are the kinds the index lists, so it is built again when they change.
		kinds := core.NewMemo(cx, live.Func(a, func() string {
			var out []string
			for _, k := range a.PluginIndexKinds() {
				out = append(out, string(k))
			}
			return strings.Join(out, ",")
		}))
		filter := core.Dynamic(func(cx *core.Scope) core.Node {
			opts := []ui.Option[plugins.Kind]{{Value: "", Label: locales.SettingsPluginsIndexAllKinds(cx)}}
			for _, k := range pluginKinds {
				if strings.Contains(","+kinds.Get()+",", ","+string(k)+",") {
					opts = append(opts, ui.Option[plugins.Kind]{Value: k, Label: pluginKindTitle(cx, k)})
				}
			}
			return ui.Select[plugins.Kind]{Options: opts, Label: locales.SettingsPluginsIndexKind(cx), TestID: "settings-plugins-index-kind",
				Value: live.Func(a, a.PluginIndexKind), OnChange: live.On(a, a.SetPluginIndexKind), Style: core.Style{Width: core.Pt(150)}}
		})
		more := core.Show{When: live.Func(a, func() bool { return a.PluginIndexMatches() > len(a.PluginIndexRows()) }), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "settings-plugins-index-more", ContentFunc: live.Func(a, func() string {
				return locales.SettingsPluginsIndexMore(cx, len(a.PluginIndexRows()), a.PluginIndexMatches())
			}), Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Edges{Top: 6, Right: 12, Left: 12}}, StyleFunc: ui.Colored(colors.Muted)}
		}}
		linkNote := core.Show{When: live.Func(a, func() bool { return a.PluginIndexLinkNote() != "" }), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "settings-plugins-index-link", ContentFunc: live.Func(a, a.PluginIndexLinkNote), Selectable: core.SelectableOn,
				Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
		}}
		updatesOnly := core.Show{When: live.Func(a, a.PluginIndexUpdatesOnly), Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, 12)}, Children: []core.Node{
				core.Text{TestID: "settings-plugins-index-updates-only", Content: locales.SettingsPluginsIndexUpdatesOnly(cx), Style: core.Style{FontSize: 12, LineHeight: 16, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
				form.Button{Text: locales.SettingsPluginsIndexShowAll(cx), TestID: "settings-plugins-index-show-all", OnPress: live.Do(a, a.ShowAllPluginIndex)},
			}}
		}}
		dropped := core.Show{When: live.Func(a, func() bool { return a.PluginIndexDropped() > 0 }), Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "settings-plugins-index-dropped", ContentFunc: live.Func(a, func() string { return locales.SettingsPluginsIndexDropped(cx, a.PluginIndexDropped()) }),
				Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Muted)}
		}}
		return core.View{TestID: "settings-plugins-index", Style: core.Style{AlignItems: core.AlignStretch, Margin: core.Edges{Bottom: 6}}, Children: []core.Node{
			form.Label(locales.SettingsPluginsIndexTitle(cx), 6, core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{refresh, hide}}),
			status,
			linkNote,
			dropped,
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Margin: core.Edges{Top: 6, Bottom: 6}}, Children: []core.Node{search, filter}},
			updatesOnly,
			pluginIndexList{a: a},
			more,
		}}
	}}
}

// pluginIndexList is the card of the packs that match, or a row saying none does. It is built again when the packs that match, or where one stands, change; not while the search field keeps the focus, as that is outside it.
type pluginIndexList struct{ a PluginsApp }

// Render implements core.Node.
func (l pluginIndexList) Render(cx *core.Scope) core.Node {
	a := l.a
	keys := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, r := range a.PluginIndexRows() {
			out = append(out, fmt.Sprintf("%s/%s/%s/%d/%s", r.Entry.ID, r.Entry.Version, r.Entry.SHA256, r.State, r.Installed))
		}
		return strings.Join(out, ",")
	}))
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if keys.Get() == "" {
			return form.Card(form.Row{Title: locales.SettingsPluginsIndexEmpty(cx), NoGroup: true, TestID: "settings-plugins-index-empty"})
		}
		var rows []core.Node
		for i, r := range a.PluginIndexRows() {
			if i > 0 {
				rows = append(rows, form.Separator(true))
			}
			rows = append(rows, pluginIndexRow(cx, a, r))
		}
		return form.Card(rows...)
	})
}

// pluginIndexRow is the row of the pack r: its name with its kind, and for a script plugin where its signature comes from, beside it; its summary; its publisher, version, size and license; and 安装, 更新 (with the version installed), 已安装, 下载中… or that it needs a newer Kando.
func pluginIndexRow(cx *core.Scope, a PluginsApp, r PluginIndexRow) core.Node {
	colors := ui.ColorsOf(cx)
	e := r.Entry
	lang := i18n.Language(cx)
	name := e.Name.In(lang)
	badges := []core.Node{
		core.Text{Content: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, LineHeight: 18, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		form.Badge(pluginKindTitle(cx, e.Kind)),
	}
	if e.Kind == plugins.KindScript {
		badges = append(badges, form.Badge(pluginSourceBadge(cx, plugins.Pack{Manifest: plugins.Manifest{Kind: e.Kind}, Source: e.Source})))
	}
	small := func(s string) core.Node {
		return core.Text{Content: s, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
	}
	titles := []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: badges}}
	if summary := e.Summary.In(lang); summary != "" {
		titles = append(titles, core.Text{Content: summary, MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 12, LineHeight: 16}, StyleFunc: ui.Colored(colors.Text)})
	}
	titles = append(titles, small(locales.SettingsPluginsIndexFacts(cx, e.Publisher, e.Version, byteSize(e.Size), e.License)))
	var trailing core.Node
	disabled := live.Func(a, func() bool { return !a.PluginIndexInstallable() })
	install := func() { a.InstallFromPluginIndex(e.ID) }
	switch r.State {
	case PluginIndexAvailable:
		trailing = form.Button{Text: locales.SettingsPluginsIndexInstall(cx), Label: locales.SettingsPluginsIndexInstallLabel(cx, name), TestID: "settings-plugins-index-install-" + e.ID, Disabled: disabled, OnPress: live.Do(a, install)}
	case PluginIndexUpdate:
		trailing = core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
			small(locales.SettingsPluginsIndexInstalledVersion(cx, r.Installed)),
			form.Button{Text: locales.SettingsPluginsIndexUpdate(cx), Label: locales.SettingsPluginsIndexUpdateLabel(cx, name, e.Version), TestID: "settings-plugins-index-update-" + e.ID, Disabled: disabled, OnPress: live.Do(a, install)},
		}}
	case PluginIndexInstalled:
		trailing = small(locales.SettingsPluginsIndexInstalled(cx))
	case PluginIndexNeedsKando:
		trailing = small(locales.SettingsPluginsIndexNeedsKando(cx, e.MinKando))
	case PluginIndexDownloading:
		trailing = core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
			small(locales.SettingsPluginsIndexDownloading(cx)),
			form.Button{Text: locales.SettingsPluginsCancel(cx), Label: locales.SettingsPluginsIndexCancelLabel(cx, name), TestID: "settings-plugins-index-cancel-" + e.ID, OnPress: live.Do(a, a.CancelPluginIndexDownload)},
		}}
	}
	return core.View{Role: core.RoleGroup, Label: name, TestID: "settings-plugins-index-row-" + e.ID,
		Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(52), Padding: core.Symmetric(8, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: titles},
			core.View{TestID: "settings-plugins-index-state-" + e.ID, Style: core.Style{FlexShrink: 0}, Children: []core.Node{trailing}},
		}}
}
