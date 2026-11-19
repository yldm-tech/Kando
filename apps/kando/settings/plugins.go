package settings

import (
	"slices"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// PluginRow is a pack as the 插件 page lists it: one installed (Pack, with whether it is turned on), or one the settings remember that is not in the plugins folder any more (Missing), of which only Pack.ID and Pack.Kind are known. Detail is what it adds, worded by the app in the interface's language, as how many terms a glossary has and between which languages, or the host an AI service talks to. Up and Down say whether a dictionary can move up or down among the dictionaries, the order lookups ask them in.
type PluginRow struct {
	Pack     plugins.Pack
	Enabled  bool
	Missing  bool
	Detail   string
	Up, Down bool
}

// PluginIssue is a folder of the plugins folder that is not a valid pack: its name, and why, in the interface's language.
type PluginIssue struct {
	Folder, Reason string
}

// PluginInstall is the install confirmation: Open while it shows, for Pack, staged and checked, with Detail as PluginRow's and Replaces the version of the pack of the same id it replaces, "" when none is installed; or, when Err is set, why the file could not be installed, which is all the dialog then says.
type PluginInstall struct {
	Open     bool
	Pack     plugins.Pack
	Detail   string
	Replaces string
	Err      string
}

// PluginsApp is what the 插件 page reads and the edits it reports. The page reads the getters in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of the actions calls Changed itself. Installing, removing and reading the plugins folder run off the UI thread and report back.
type PluginsApp interface {
	live.State

	// PluginRows are the packs, installed ones and those the settings remember that are missing, in the order the settings keep them; PluginIssues the folders that are not valid packs. PluginsNote says what is wrong with the plugins' state file (state.json), "" when nothing is; PluginsErr why the last install, removal or switch failed. PluginsBusy is set while one runs.
	PluginRows() []PluginRow
	PluginIssues() []PluginIssue
	PluginsNote() string
	PluginsErr() string
	PluginsBusy() bool
	// SetPluginEnabled turns the pack id on or off and saves that to state.json.
	SetPluginEnabled(id string, on bool)
	// PickPluginFile asks for a pack file with the system's file panel, and InstallPlugins takes the files or folders dropped on the page; either opens the install confirmation of the first, never installing without it.
	PickPluginFile()
	InstallPlugins(paths []string)
	// ShowPluginsFolder shows the plugins folder in the file manager.
	ShowPluginsFolder()
	// UninstallPlugin removes the pack id, its folder and its entry of state.json; ForgetPlugin forgets a pack that is missing. MovePlugin moves the dictionary id one place up or down among the dictionaries.
	UninstallPlugin(id string)
	ForgetPlugin(id string)
	MovePlugin(id string, up bool)
	// PluginInstall is the install confirmation; PluginInstallHere reports whether the page draws it, as when the settings are in a window of their own, rather than the main window (the app's pluginsOverlay). ConfirmPluginInstall installs the pack it shows, CancelPluginInstall closes it and throws the staged copy away; it does nothing while PluginsBusy, as while the install runs.
	PluginInstall() PluginInstall
	PluginInstallHere() bool
	ConfirmPluginInstall()
	CancelPluginInstall()
}

// pluginKinds are the kinds of packs the page groups the list by, in its order.
var pluginKinds = []plugins.Kind{plugins.KindGlossary, plugins.KindAIService, plugins.KindDictionary, plugins.KindWordlist}

// pluginKindTitle is the heading of the group of kind, in the language the app shows under cx.
func pluginKindTitle(cx *core.Scope, kind plugins.Kind) string {
	switch kind {
	case plugins.KindGlossary:
		return locales.SettingsPluginsKindGlossary(cx)
	case plugins.KindAIService:
		return locales.SettingsPluginsKindAIService(cx)
	case plugins.KindDictionary:
		return locales.SettingsPluginsKindDictionary(cx)
	}
	return locales.SettingsPluginsKindWordlist(cx)
}

// pluginName is the name of the pack in row in the language shown under cx, or its id when it is missing, as its name was in its folder.
func pluginName(cx *core.Scope, p plugins.Pack) string {
	if name := p.Name.In(i18n.Language(cx)); name != "" {
		return name
	}
	return p.ID
}

// Plugins is the 插件 page: 从文件安装… and the button that shows the plugins folder; the packs grouped by kind, each with its switch, 本地 for where it came from, its version and author, and 详情, which opens under it what it is with 卸载; a missing pack with 移除; the folders that are not valid packs and why; and what went wrong last. Files and folders dropped on the page are installed too, after the same confirmation (PluginInstallDialog), which the page draws when the settings are in a window of their own.
type Plugins struct{ App PluginsApp }

// Render implements core.Node.
func (p Plugins) Render(cx *core.Scope) core.Node {
	a := p.App
	colors := ui.ColorsOf(cx)
	busy := live.Func(a, a.PluginsBusy)
	install := form.Button{Text: locales.SettingsPluginsInstall(cx), TestID: "settings-plugins-install", Disabled: busy, OnPress: live.Do(a, a.PickPluginFile)}
	reveal := form.Button{Text: locales.SettingsPluginsShowFolder(cx, desktop.Platform(cx)), TestID: "settings-plugins-folder", OnPress: live.Do(a, a.ShowPluginsFolder)}
	note := core.Show{When: live.Func(a, func() bool { return a.PluginsNote() != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, a.PluginsNote), TestID: "settings-plugins-note", Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	failed := core.Show{When: live.Func(a, func() bool { return a.PluginsErr() != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, a.PluginsErr), TestID: "settings-plugins-error", Selectable: core.SelectableOn, Style: core.Style{FontSize: 11, LineHeight: 15, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	dragOver := core.NewSignal(cx, false)
	k := theme.Of(cx)
	page := form.Page(
		form.Label(locales.SettingsPluginsInstalled(cx), 6, core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{reveal, install}}),
		note,
		pluginList{a: a},
		pluginIssues{a: a},
		failed,
		core.Show{When: live.Func(a, a.PluginInstallHere), Then: func(*core.Scope) core.Node { return PluginInstallDialog{App: a, Open: func() bool { return true }} }},
	)
	return core.View{TestID: "settings-plugins",
		Style: core.Style{Width: core.Pct(100), AlignItems: core.AlignStretch, CornerRadius: 12},
		StyleFunc: func() core.Style {
			if dragOver.Get() {
				return core.Style{Background: k.Shade(0.05)}
			}
			return core.Style{}
		},
		OnFileDragEnter: func() { dragOver.Set(true) },
		OnFileDragLeave: func() { dragOver.Set(false) },
		OnFileDrop: live.On(a, func(uris []string) {
			dragOver.Set(false)
			if paths := desktop.DroppedPaths(uris); len(paths) > 0 {
				a.InstallPlugins(paths)
			}
		}),
		Children: []core.Node{page}}
}

// pluginList is the packs grouped by kind, or a card saying none is installed. It is built again only when a pack comes, goes or changes, not when the dictionaries move: each group's rows are kept by pack id (core.Each) and moved, so the button that moved a dictionary keeps the keyboard focus; a row reads its switch, and whether it can move, in a binding.
type pluginList struct{ a PluginsApp }

// Render implements core.Node.
func (l pluginList) Render(cx *core.Scope) core.Node {
	a := l.a
	keys := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, r := range a.PluginRows() {
			out = append(out, string(r.Pack.Kind)+"/"+r.Pack.ID+"/"+r.Pack.Hash)
		}
		// Sorted, so a move, which changes only the order, builds nothing again.
		slices.Sort(out)
		return strings.Join(out, ",")
	}))
	// open is the pack whose details show under its row, asking the one whose uninstall waits to be confirmed; "" for none.
	open := core.NewSignal(cx, "")
	asking := core.NewSignal(cx, "")
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if keys.Get() == "" {
			card := form.Card(form.Row{Title: locales.SettingsPluginsNone(cx), Subtitle: locales.SettingsPluginsNoneHint(cx), NoGroup: true, TestID: "settings-plugins-none"})
			return card
		}
		var groups []core.Node
		group := func(title string, in func(r PluginRow) bool) {
			if !slices.ContainsFunc(a.PluginRows(), in) {
				return
			}
			rows := live.Func(a, func() []PluginRow {
				var out []PluginRow
				for _, r := range a.PluginRows() {
					if in(r) {
						out = append(out, r)
					}
				}
				return out
			})
			list := core.Each(rows, func(r PluginRow) string { return r.Pack.ID }, func(cx *core.Scope, item func() PluginRow, index func() int) core.Node {
				separator := core.Show{When: func() bool { return index() > 0 }, Then: func(*core.Scope) core.Node { return form.Separator(true) }}
				return core.Group{separator, pluginRow(cx, a, item, open, asking)}
			})
			groups = append(groups, form.Label(title, 12), form.Card(list))
		}
		for _, kind := range pluginKinds {
			group(pluginKindTitle(cx, kind), func(r PluginRow) bool { return !r.Missing && r.Pack.Kind == kind })
		}
		// A missing pack's kind is not known, as it was read from its folder: those are listed after the others, under a heading of their own.
		group(locales.SettingsPluginsMissingGroup(cx), func(r PluginRow) bool { return r.Missing })
		return core.Group(groups)
	})
}

// pluginRow is the row of the pack item gives: its name with 本地 after it over its version and author, or over 未安装 when it is missing; a dictionary's buttons that move it up and down among the dictionaries, enabled as item says now; its switch and 详情, or 移除 for a missing one; and under it, while open names it, its details (pluginDetail). The rest is what the pack was as the row was built, as the list is built again when a pack changes.
func pluginRow(cx *core.Scope, a PluginsApp, item func() PluginRow, open, asking *core.Signal[string]) core.Node {
	r := item()
	colors := ui.ColorsOf(cx)
	id := r.Pack.ID
	name := pluginName(cx, r.Pack)
	subtitle := locales.SettingsPluginsVersion(cx, r.Pack.Version)
	if r.Pack.Author != "" {
		subtitle = locales.SettingsPluginsVersionAuthor(cx, r.Pack.Version, r.Pack.Author)
	}
	if r.Missing {
		subtitle = locales.SettingsPluginsMissing(cx)
	}
	title := core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{Content: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, LineHeight: 18, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		form.Badge(locales.SettingsPluginsSourceLocal(cx)),
	}}
	titles := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
		title,
		core.Text{Content: subtitle, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	var trailing []core.Node
	if r.Missing {
		trailing = append(trailing, form.Button{Text: locales.SettingsPluginsForget(cx), Label: locales.SettingsPluginsForgetLabel(cx, name), TestID: "settings-plugins-forget-" + id, OnPress: live.Do(a, func() { a.ForgetPlugin(id) })})
	} else {
		if r.Pack.Kind == plugins.KindDictionary {
			trailing = append(trailing,
				ui.IconButton{Icon: lucide.ArrowUp, LabelFunc: func() string { return locales.SettingsPluginsMoveUpLabel(cx, pluginName(cx, item().Pack)) }, Size: 22, TestID: "settings-plugins-up-" + id,
					Disabled: func() bool { return !item().Up }, OnPress: live.Do(a, func() { a.MovePlugin(id, true) })},
				ui.IconButton{Icon: lucide.ArrowDown, LabelFunc: func() string { return locales.SettingsPluginsMoveDownLabel(cx, pluginName(cx, item().Pack)) }, Size: 22, TestID: "settings-plugins-down-" + id,
					Disabled: func() bool { return !item().Down }, OnPress: live.Do(a, func() { a.MovePlugin(id, false) })},
			)
		}
		enabled := live.Func(a, func() bool {
			for _, row := range a.PluginRows() {
				if row.Pack.ID == id {
					return row.Enabled
				}
			}
			return false
		})
		trailing = append(trailing,
			ui.Switch{Label: locales.SettingsPluginsEnableLabel(cx, name), Size: ui.SizeSmall, TestID: "settings-plugins-switch-" + id, Checked: enabled,
				Disabled: live.Func(a, a.PluginsBusy),
				OnChange: live.On(a, func(on bool) { a.SetPluginEnabled(id, on) })},
			ui.IconButton{Icon: lucide.ChevronDown, LabelFunc: func() string { return locales.SettingsPluginsDetailsLabel(cx, name) }, Size: 22, TestID: "settings-plugins-details-" + id,
				OnPress: func() {
					if open.Get() == id {
						open.Set("")
						return
					}
					open.Set(id)
				}},
		)
	}
	row := core.View{Role: core.RoleGroup, Label: name, TestID: "settings-plugins-row-" + id,
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(46), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: append([]core.Node{titles}, trailing...)}
	if r.Missing {
		return row
	}
	details := core.Show{When: func() bool { return open.Get() == id }, Then: func(cx *core.Scope) core.Node {
		return pluginDetail(cx, a, r, name, asking)
	}}
	return core.Group{row, details}
}

// pluginDetail is what the pack in r is, under its row: its description, what it adds, its license, homepage, id and the start of its hash, the credit its license asks for, and 卸载…, which asks first, under it, with 卸载 and 取消.
func pluginDetail(cx *core.Scope, a PluginsApp, r PluginRow, name string, asking *core.Signal[string]) core.Node {
	colors := ui.ColorsOf(cx)
	k := theme.Of(cx)
	lang := i18n.Language(cx)
	id := r.Pack.ID
	text := func(s string, color func() core.Color) core.Node {
		return core.Text{Content: s, Selectable: core.SelectableOn, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(color)}
	}
	var lines []core.Node
	if d := r.Pack.Description.In(lang); d != "" {
		lines = append(lines, text(d, colors.Text))
	}
	if r.Detail != "" {
		lines = append(lines, text(r.Detail, colors.Text))
	}
	lines = append(lines, text(locales.SettingsPluginsLicense(cx, r.Pack.License), colors.Muted))
	if r.Pack.Homepage != "" {
		lines = append(lines, text(r.Pack.Homepage, colors.Muted))
	}
	if r.Pack.Kind == plugins.KindDictionary && r.Pack.Dictionary != nil {
		if credit := r.Pack.Dictionary.Attribution.In(lang); credit != "" {
			lines = append(lines, text(credit, colors.Muted))
		}
	}
	lines = append(lines, text(locales.SettingsPluginsIdentity(cx, id, shortHash(r.Pack.Hash)), colors.Muted))
	uninstall := form.Button{Text: locales.SettingsPluginsUninstall(cx), Label: locales.SettingsPluginsUninstallLabel(cx, name), TestID: "settings-plugins-uninstall-" + id,
		Disabled: live.Func(a, a.PluginsBusy),
		OnPress: func() {
			if asking.Get() == id {
				asking.Set("")
				return
			}
			asking.Set(id)
		}}
	lines = append(lines, core.View{Style: core.Style{Direction: core.Row, Margin: core.Edges{Top: 4}}, Children: []core.Node{uninstall}})
	confirm := core.Show{When: func() bool { return asking.Get() == id }, Then: func(cx *core.Scope) core.Node {
		done := func() { asking.Set("") }
		return core.View{Role: core.RoleGroup, Label: locales.SettingsPluginsUninstallTitle(cx, name), TestID: "settings-plugins-uninstall-ask-" + id,
			Style: core.Style{Gap: 4, Margin: core.Edges{Top: 6}}, Children: []core.Node{
				core.Text{Content: locales.SettingsPluginsUninstallTitle(cx, name), Role: core.RoleHeader, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
				text(locales.SettingsPluginsUninstallMessage(cx), colors.Muted),
				core.View{Style: core.Style{Direction: core.Row, Gap: 8, Margin: core.Edges{Top: 4}}, Children: []core.Node{
					ui.Button{Text: locales.SettingsPluginsUninstallConfirm(cx), Color: colors.Danger, Size: ui.SizeSmall, TestID: "settings-plugins-uninstall-confirm-" + id,
						OnPress: live.Do(a, func() {
							done()
							a.UninstallPlugin(id)
						})},
					ui.Button{Text: locales.SettingsPluginsCancel(cx), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "settings-plugins-uninstall-cancel-" + id, OnPress: done},
				}},
			}}
	}}
	lines = append(lines, confirm)
	return core.View{TestID: "settings-plugins-detail-" + id,
		Style:     core.Style{FlexShrink: 0, Gap: 3, Margin: core.Edges{Right: 14, Bottom: 10, Left: 14}, Padding: core.Symmetric(10, 12), CornerRadius: 10},
		StyleFunc: ui.Filled(k.Tile), Children: lines}
}

// shortHash is the first 12 hex digits of a pack's hash, enough to tell one content from another at a glance.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// pluginIssues is the card of the folders that are not valid packs, each with why, under its heading; nothing while there are none.
type pluginIssues struct{ a PluginsApp }

// Render implements core.Node.
func (p pluginIssues) Render(cx *core.Scope) core.Node {
	a := p.a
	keys := core.NewMemo(cx, live.Func(a, func() string {
		var out []string
		for _, issue := range a.PluginIssues() {
			out = append(out, issue.Folder+"\x00"+issue.Reason)
		}
		return strings.Join(out, "\x01")
	}))
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if keys.Get() == "" {
			return core.Group(nil)
		}
		var rows []core.Node
		for i, issue := range a.PluginIssues() {
			if i > 0 {
				rows = append(rows, form.Separator(true))
			}
			rows = append(rows, form.Row{Title: issue.Folder, Subtitle: issue.Reason, TestID: "settings-plugins-issue-" + issue.Folder})
		}
		return core.Group{form.Label(locales.SettingsPluginsIssues(cx), 12), form.Card(rows...)}
	})
}

// PluginInstallDialog is the install confirmation while Open and the app's PluginInstall say it is open: the pack's name, what it is and adds, its version, author, license, id and the start of its hash, that it comes from a file on this computer and is not signed, and the version it replaces, with 安装 and 取消; or, for a file that cannot be installed, why, with 好. 取消 is the default, so Enter and Escape install nothing. The 插件 page draws it when the settings are in a window of their own, the app's pluginsOverlay over the main window otherwise.
type PluginInstallDialog struct {
	App  PluginsApp
	Open func() bool
}

// Render implements core.Node.
func (d PluginInstallDialog) Render(cx *core.Scope) core.Node {
	a := d.App
	// key tells one confirmation from the next, so the dialog's words are built again for another pack.
	key := core.NewMemo(cx, live.Func(a, func() string {
		in := a.PluginInstall()
		if !in.Open {
			return ""
		}
		return in.Pack.ID + "\x00" + in.Pack.Hash + "\x00" + in.Err
	}))
	cancel := live.Do(a, a.CancelPluginInstall)
	return core.Show{When: func() bool { return d.Open() && key.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, OverModals: true}, cancel)
		// The dialog's rows are the pack's, so it is built again for another one, and its words are read as it is built, so it is built again when the interface's language changes too, as settings.Window is; over the main window nothing else would.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			_ = key.Get()
			_ = i18n.Language(cx)
			return pluginInstallDialog(cx, a, a.PluginInstall(), cancel)
		})
	}}
}

// pluginInstallDialog is the dialog of the confirmation in.
func pluginInstallDialog(cx *core.Scope, a PluginsApp, in PluginInstall, cancel func()) core.Node {
	colors := ui.ColorsOf(cx)
	onOpen := func(open bool, _ base.Reason) {
		if !open {
			cancel()
		}
	}
	if in.Err != "" {
		return ui.Dialog{TestID: "plugins-install-failed", Role: core.RoleAlertDialog, NoCloseOnBackdropPress: true, Open: func() bool { return true },
			Title: locales.SettingsPluginsInstallFailed(cx), Description: in.Err, OnOpenChange: onOpen,
			Footer: []core.Node{ui.Button{Text: locales.SettingsPluginsOK(cx), TestID: "plugins-install-done", OnPress: cancel}}}
	}
	p := in.Pack
	name := pluginName(cx, p)
	facts := func(s string) core.Node {
		return core.Text{Content: s, Selectable: core.SelectableOn, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)}
	}
	var children []core.Node
	if in.Detail != "" {
		children = append(children, core.Text{Content: in.Detail, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)})
	}
	children = append(children, facts(locales.SettingsPluginsInstallSource(cx)))
	version := locales.SettingsPluginsVersion(cx, p.Version)
	if p.Author != "" {
		version = locales.SettingsPluginsVersionAuthor(cx, p.Version, p.Author)
	}
	children = append(children, facts(version), facts(locales.SettingsPluginsLicense(cx, p.License)), facts(locales.SettingsPluginsIdentity(cx, p.ID, shortHash(p.Hash))))
	if in.Replaces != "" {
		children = append(children, facts(locales.SettingsPluginsInstallReplaces(cx, in.Replaces)))
	}
	return ui.Dialog{TestID: "plugins-install-confirm", Role: core.RoleAlertDialog, NoCloseOnBackdropPress: true, Open: func() bool { return true },
		Title: locales.SettingsPluginsInstallTitle(cx, name), Description: p.Description.In(i18n.Language(cx)), OnOpenChange: onOpen,
		Children: []core.Node{core.View{Style: core.Style{Gap: 3}, Children: children}},
		Footer: []core.Node{
			ui.Button{Text: locales.SettingsPluginsCancel(cx), Variant: ui.ButtonSecondary, TestID: "plugins-install-cancel", Disabled: live.Func(a, a.PluginsBusy), OnPress: cancel},
			ui.Button{Text: locales.SettingsPluginsInstallConfirm(cx), TestID: "plugins-install-ok", Disabled: live.Func(a, a.PluginsBusy), OnPress: live.Do(a, a.ConfirmPluginInstall)},
		}}
}
