package settings

import (
	"slices"
	"strconv"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

// PluginScript is what the 插件 page and the install confirmation show of a script plugin, worded by the app in the interface's language: where it comes from (Source), whether its signature was verified for its content (Verified), why it does not run when Kando turned it off or refused it (Problem), that an update asked for more than the version before (Updated), the permissions it asks for with whether each is granted, the combination of reading the lines and the network flagged (Risky), and the commands whose keys were taken by Kando's own (Conflicts).
type PluginScript struct {
	Source plugins.Source
	// Publisher is the publisher its certificate covers, the segment of its id before the first dot, which a verified plugin is said to come from.
	Publisher string
	Verified  bool
	Problem   string
	Updated   bool
	Perms     []PluginPerm
	Risky     bool
	Conflicts string
	// Adds are what it adds besides commands, each worded on a line: a lookup source, a subtitle format, an export target, an annotator.
	Adds []string
	// Credentials are the secrets of other services it sends through Kando.
	Credentials []PluginCredential
}

// PluginCredential is a credential of a script plugin as its details and its install confirmation show it: the key it is kept under (SecretKey), its title, the hosts it is only sent to, written out, and a warning when those hosts are not the ones of the service its name says, or are of none Kando knows for a plugin that is not Kando's own ("" for none).
type PluginCredential struct {
	Key, Title, Hosts, Warning string
}

// PluginPerm is a permission a script plugin asks for, or its network as a whole (ID "network"): what it lets the plugin do (Text), whether the plugin can do without it (Optional), whether it is granted, and whether it needs no grant (Free), which has no switch.
type PluginPerm struct {
	ID       string
	Text     string
	Optional bool
	Granted  bool
	Free     bool
}

// PluginScriptsApp is what the 插件 page reads and edits of the script plugins: the grants, their declared settings, developer mode and the audit log.
type PluginScriptsApp interface {
	// SetPluginGrant grants the script plugin id the permission perm ("network" for its network) or takes it back.
	SetPluginGrant(id, perm string, on bool)
	// PluginValue is the value of the plugin id's setting key, its default while the user never set it; SetPluginValue sets it. PluginSecretKept reports whether its secret setting key is kept, which SetPluginSecret keeps, or removes for "".
	PluginValue(id, key string) any
	SetPluginValue(id, key string, value any)
	PluginSecretKept(id, key string) bool
	SetPluginSecret(id, key, value string)
	// PluginDeveloper is developer mode, which SetPluginDeveloper turns on or off; PluginDevKey reports whether this computer has a developer key, which MakePluginDevKey makes; PackPluginFolder asks for a developer's plugin folder, signs it with that key and opens its install confirmation.
	PluginDeveloper() bool
	SetPluginDeveloper(on bool)
	PluginDevKey() bool
	MakePluginDevKey()
	PackPluginFolder()
	// ShowPluginAudit shows the plugins' audit log in the file manager.
	ShowPluginAudit()
}

// pluginSourceBadge is the badge after the name of a pack from source: 官方 or 开发者 for a script plugin, 本地 for a data pack.
func pluginSourceBadge(cx *core.Scope, p plugins.Pack) string {
	switch {
	case p.Kind == plugins.KindScript && p.Source == plugins.SourceOfficial:
		return locales.SettingsPluginsSourceOfficial(cx)
	case p.Kind == plugins.KindScript && p.Source == plugins.SourceVerified:
		return locales.SettingsPluginsSourceVerified(cx)
	case p.Kind == plugins.KindScript && p.Source == plugins.SourceDeveloper:
		return locales.SettingsPluginsSourceDeveloper(cx)
	}
	return locales.SettingsPluginsSourceLocal(cx)
}

// pluginScriptRow finds the script plugin id among the page's rows now, for the bindings of its details.
func pluginScriptRow(a PluginsApp, id string) (PluginScript, bool) {
	for _, r := range a.PluginRows() {
		if r.Pack.ID == id && r.Script != nil {
			return *r.Script, true
		}
	}
	return PluginScript{}, false
}

// pluginScriptDetail is what the details of the script plugin in r add: why it does not run, that an update asks for more, the permissions it asks for, each with a switch that grants it, needed ones first, the risky combination on a line of its own, the commands whose keys were taken, its settings, and 查看审计记录.
func pluginScriptDetail(cx *core.Scope, a PluginsApp, r PluginRow) []core.Node {
	colors := ui.ColorsOf(cx)
	id := r.Pack.ID
	note := func(text func() string, color func() core.Color, testID string) core.Node {
		return core.Show{When: func() bool { return text() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: text, TestID: testID, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(color)}
		}}
	}
	row := func() PluginScript { s, _ := pluginScriptRow(a, id); return s }
	nodes := []core.Node{
		note(live.Func(a, func() string { return row().Problem }), colors.Danger, "settings-plugins-problem-"+id),
		note(live.Func(a, func() string {
			if row().Updated {
				return locales.SettingsPluginsUpdated(cx)
			}
			return ""
		}), colors.Warn, "settings-plugins-updated-"+id),
	}
	perms := r.Script.Perms
	if len(perms) > 0 {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsPermissions(cx), Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)})
	}
	for _, p := range perms {
		nodes = append(nodes, pluginPermRow(cx, a, id, p))
	}
	nodes = append(nodes,
		note(live.Func(a, func() string {
			if row().Risky {
				return locales.SettingsPluginsRisky(cx)
			}
			return ""
		}), colors.Danger, "settings-plugins-risky-"+id),
		note(live.Func(a, func() string { return row().Conflicts }), colors.Warn, "settings-plugins-conflicts-"+id),
	)
	if len(r.Script.Adds) > 0 {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsAdds(cx), Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)})
		for _, add := range r.Script.Adds {
			nodes = append(nodes, core.Text{Content: "· " + add, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Text)})
		}
	}
	if len(r.Script.Credentials) > 0 {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsCredentials(cx), Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)})
		for _, c := range r.Script.Credentials {
			nodes = append(nodes, pluginCredentialRow(cx, a, id, c)...)
		}
	}
	if len(r.Pack.Settings) > 0 {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsSettings(cx), Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)})
		for _, s := range r.Pack.Settings {
			nodes = append(nodes, pluginSettingRow(cx, a, id, s))
		}
	}
	nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, Margin: core.Edges{Top: 4}}, Children: []core.Node{
		form.Button{Text: locales.SettingsPluginsAudit(cx), TestID: "settings-plugins-audit-" + id, OnPress: live.Do(a, a.ShowPluginAudit)},
	}})
	return nodes
}

// pluginPermRow is the line of the permission p of the plugin id: what it lets the plugin do, 可选 for one it can do without, and the switch that grants it, which reads whether it is granted now; a permission that needs no grant has none.
func pluginPermRow(cx *core.Scope, a PluginsApp, id string, p PluginPerm) core.Node {
	colors := ui.ColorsOf(cx)
	texts := []core.Node{core.Text{Content: p.Text, Style: core.Style{FontSize: 12, LineHeight: 17, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)}}
	if p.Optional {
		texts = append(texts, form.Badge(locales.SettingsPluginsOptional(cx)))
	}
	children := []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 6, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), AlignItems: core.AlignCenter}, Children: texts}}
	if !p.Free {
		granted := live.Func(a, func() bool {
			s, _ := pluginScriptRow(a, id)
			i := slices.IndexFunc(s.Perms, func(o PluginPerm) bool { return o.ID == p.ID })
			return i >= 0 && s.Perms[i].Granted
		})
		children = append(children, ui.Switch{Label: p.Text, Size: ui.SizeSmall, TestID: "settings-plugins-grant-" + id + "-" + p.ID, Checked: granted,
			Disabled: live.Func(a, a.PluginsBusy), OnChange: live.On(a, func(on bool) { a.SetPluginGrant(id, p.ID, on) })})
	}
	return core.View{Role: core.RoleGroup, Label: p.Text, Style: core.Style{Direction: core.Row, Gap: 10, MinHeight: core.Pt(28), AlignItems: core.AlignCenter}, Children: children}
}

// pluginSettingRow is the field of the plugin id's declared setting s: a switch, a menu of its choices, a field for a number or a text, or, for a secret, a masked field kept in the system's secure store with 已保存 and 清除 once one is kept.
func pluginSettingRow(cx *core.Scope, a PluginsApp, id string, s plugins.Setting) core.Node {
	colors := ui.ColorsOf(cx)
	lang := i18n.Language(cx)
	title := s.Title.In(lang)
	testID := "settings-plugins-setting-" + id + "-" + s.Key
	var field core.Node
	switch s.Type {
	case plugins.SettingBool:
		field = ui.Switch{Label: title, Size: ui.SizeSmall, TestID: testID,
			Checked:  live.Func(a, func() bool { v, _ := a.PluginValue(id, s.Key).(bool); return v }),
			OnChange: live.On(a, func(on bool) { a.SetPluginValue(id, s.Key, on) })}
	case plugins.SettingChoice:
		var options []ui.Option[string]
		for _, c := range s.Choices {
			options = append(options, ui.Option[string]{Value: c.ID, Label: c.Label.In(lang)})
		}
		field = ui.Select[string]{Options: options, Label: title, TestID: testID, Style: core.Style{Width: core.Pt(180)},
			Value:    live.Func(a, func() string { v, _ := a.PluginValue(id, s.Key).(string); return v }),
			OnChange: live.On(a, func(v string) { a.SetPluginValue(id, s.Key, v) })}
	case plugins.SettingNumber:
		field = ui.Input{TestID: testID, Label: title, Style: core.Style{Width: core.Pt(120)},
			Value: live.Func(a, func() string {
				v, _ := a.PluginValue(id, s.Key).(float64)
				return strconv.FormatFloat(v, 'f', -1, 64)
			}),
			OnChange: live.On(a, func(text string) {
				if n, err := strconv.ParseFloat(text, 64); err == nil {
					a.SetPluginValue(id, s.Key, n)
				}
			})}
	case plugins.SettingText:
		field = ui.Input{TestID: testID, Label: title, Style: core.Style{Width: core.Pt(220)},
			Value:    live.Func(a, func() string { v, _ := a.PluginValue(id, s.Key).(string); return v }),
			OnChange: live.On(a, func(text string) { a.SetPluginValue(id, s.Key, text) })}
	case plugins.SettingSecret:
		field = pluginSecretField(cx, a, id, s.Key, title, testID)
	}
	return core.View{Role: core.RoleGroup, Label: title, Style: core.Style{Direction: core.Row, Gap: 10, MinHeight: core.Pt(32), AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{Content: title, Style: core.Style{FontSize: 12, LineHeight: 17, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		field,
	}}
}

// pluginCredentialRow is the field of the plugin id's credential c, which Kando keeps in the system's secure store as it keeps a secret setting, with the hosts it is only sent to written over it, and the warning of hosts that are not the service's, if any.
func pluginCredentialRow(cx *core.Scope, a PluginsApp, id string, c PluginCredential) []core.Node {
	colors := ui.ColorsOf(cx)
	testID := "settings-plugins-credential-" + id + "-" + strings.TrimPrefix(c.Key, "credential.")
	out := []core.Node{
		core.View{Role: core.RoleGroup, Label: c.Title, Style: core.Style{Direction: core.Row, Gap: 10, MinHeight: core.Pt(32), AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{Content: c.Title, Style: core.Style{FontSize: 12, LineHeight: 17, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
			pluginSecretField(cx, a, id, c.Key, c.Title, testID),
		}},
		core.Text{Content: locales.SettingsPluginsCredentialHosts(cx, c.Hosts), TestID: testID + "-hosts", Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
	}
	if c.Warning != "" {
		out = append(out, core.Text{Content: c.Warning, TestID: testID + "-warning", Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Danger)})
	}
	return out
}

// pluginSecretField is a secret setting's field: while none is kept, a masked field and 保存, which keeps what it holds in the system's secure store, out of the plugin's reach; once one is kept, 已保存 and 清除.
func pluginSecretField(cx *core.Scope, a PluginsApp, id, key, title, testID string) core.Node {
	text := core.NewSignal(cx, "")
	kept := live.Func(a, func() bool { return a.PluginSecretKept(id, key) })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if kept() {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
				form.Badge(locales.SettingsPluginsSecretSaved(cx)),
				form.Button{Text: locales.SettingsPluginsSecretClear(cx), TestID: testID + "-clear", OnPress: live.Do(a, func() { a.SetPluginSecret(id, key, "") })},
			}}
		}
		return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
			ui.Input{TestID: testID, Label: title, Masked: true, Style: core.Style{Width: core.Pt(180)}, Value: text.Get, OnChange: text.Set},
			form.Button{Text: locales.SettingsPluginsSecretSave(cx), TestID: testID + "-save", OnPress: live.Do(a, func() {
				if v := text.Get(); v != "" {
					a.SetPluginSecret(id, key, v)
					text.Set("")
				}
			})},
		}}
	})
}

// pluginScriptInstall is what the install confirmation adds for a script plugin: where it comes from, a warning for a developer plugin, what it asks for, needed and then optional, nothing granted yet, the risky combination on a line of its own, and that it stays off until the user grants it and turns it on.
func pluginScriptInstall(cx *core.Scope, s PluginScript) []core.Node {
	colors := ui.ColorsOf(cx)
	line := func(text string, color func() core.Color) core.Node {
		return core.Text{Content: text, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(color)}
	}
	var source string
	switch s.Source {
	case plugins.SourceOfficial:
		source = locales.SettingsPluginsInstallSourceOfficial(cx)
	case plugins.SourceVerified:
		source = locales.SettingsPluginsInstallSourceVerified(cx, s.Publisher)
	default:
		source = locales.SettingsPluginsInstallSourceDeveloper(cx)
	}
	nodes := []core.Node{line(source, colors.Muted)}
	if s.Source == plugins.SourceDeveloper {
		nodes = append(nodes, line(locales.SettingsPluginsDeveloperWarning(cx), colors.Warn))
	}
	for _, optional := range []bool{false, true} {
		var lines []core.Node
		for _, p := range s.Perms {
			if p.Optional == optional {
				lines = append(lines, line("· "+p.Text, colors.Text))
			}
		}
		if len(lines) == 0 {
			continue
		}
		heading := locales.SettingsPluginsWants(cx)
		if optional {
			heading = locales.SettingsPluginsWantsOptional(cx)
		}
		nodes = append(nodes, core.Text{Content: heading, Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 4}}, StyleFunc: ui.Colored(colors.Text)})
		nodes = append(nodes, lines...)
	}
	if len(s.Adds) > 0 {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsAdds(cx), Role: core.RoleHeader, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Semibold, Margin: core.Edges{Top: 4}}, StyleFunc: ui.Colored(colors.Text)})
		for _, add := range s.Adds {
			nodes = append(nodes, line("· "+add, colors.Text))
		}
	}
	for _, c := range s.Credentials {
		nodes = append(nodes, core.Text{Content: c.Title + " · " + locales.SettingsPluginsCredentialHosts(cx, c.Hosts), TestID: "plugins-install-credential-" + strings.TrimPrefix(c.Key, "credential."), Style: core.Style{FontSize: 12, LineHeight: 17, Margin: core.Edges{Top: 4}}, StyleFunc: ui.Colored(colors.Text)})
		if c.Warning != "" {
			nodes = append(nodes, line(c.Warning, colors.Danger))
		}
	}
	if s.Risky {
		nodes = append(nodes, core.Text{Content: locales.SettingsPluginsRisky(cx), TestID: "plugins-install-risky", Style: core.Style{FontSize: 12, LineHeight: 17, Margin: core.Edges{Top: 4}}, StyleFunc: ui.Colored(colors.Danger)})
	}
	nodes = append(nodes, line(locales.SettingsPluginsScriptStaysOff(cx), colors.Muted))
	return nodes
}

// pluginDeveloper is the page's 开发者 card: developer mode's switch and, while it is on, the developer key, made once, and 打包并安装文件夹….
type pluginDeveloper struct{ a PluginsApp }

// Render implements core.Node.
func (d pluginDeveloper) Render(cx *core.Scope) core.Node {
	a := d.a
	mode := form.SwitchRow{Title: locales.SettingsPluginsDeveloperMode(cx), Subtitle: locales.SettingsPluginsDeveloperModeHint(cx), TestID: "settings-plugins-developer",
		On: live.Func(a, a.PluginDeveloper), OnChange: live.On(a, a.SetPluginDeveloper)}
	tools := core.Show{When: live.Func(a, a.PluginDeveloper), Then: func(cx *core.Scope) core.Node {
		key := core.Dynamic(func(cx *core.Scope) core.Node {
			if live.Func(a, a.PluginDevKey)() {
				return form.Row{Title: locales.SettingsPluginsDevKey(cx), Subtitle: locales.SettingsPluginsDevKeyReady(cx), TestID: "settings-plugins-devkey",
					Trailing: []core.Node{form.Button{Text: locales.SettingsPluginsDevPack(cx), TestID: "settings-plugins-devpack", Disabled: live.Func(a, a.PluginsBusy), OnPress: live.Do(a, a.PackPluginFolder)}}}
			}
			return form.Row{Title: locales.SettingsPluginsDevKey(cx), Subtitle: locales.SettingsPluginsDevKeyNone(cx), TestID: "settings-plugins-devkey",
				Trailing: []core.Node{form.Button{Text: locales.SettingsPluginsDevKeyMake(cx), TestID: "settings-plugins-devkey-make", OnPress: live.Do(a, a.MakePluginDevKey)}}}
		})
		return core.Group{form.Separator(true), key}
	}}
	return core.Group{form.Label(locales.SettingsPluginsDeveloper(cx), 12), form.Card(mode, tools)}
}
