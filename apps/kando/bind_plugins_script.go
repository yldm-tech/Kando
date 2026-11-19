package main

import (
	"errors"
	"os"
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The 插件 page's reads and edits of the script plugins (settings.PluginScriptsApp), as bind_plugins.go's of the packs: the page reads them in its bindings and calls the actions from its handlers, which tell the revision.

// pluginsScriptView is what the page and the install confirmation show of the script plugin p, in the words of m: where it comes from, whether it was verified, why it does not run, whether an update asks for more, the permissions it asks for with whether each is granted now (what state.json holds for its content and signer), and the commands whose keys Kando's own took.
func (a *app) pluginsScriptView(m locales.Messages, p plugins.Pack, installing bool) *settings.PluginScript {
	s := &a.plugins.script
	v, verified := s.verified[p.ID]
	out := &settings.PluginScript{Source: p.Source, Publisher: script.Publisher(p.ID), Verified: installing || verified && v.Hash == p.Hash && p.Signer != ""}
	var granted plugins.Grant
	if e, ok := a.plugins.state.Entry(p.ID); ok && !installing && e.Hash == p.Hash && e.Signer == p.Signer {
		granted = e.Grant
		out.Updated = e.OldHash != ""
	}
	declared := p.Declared()
	optional := func(perm plugins.Perm) bool { return slices.Contains(p.Optional, perm) }
	for _, perm := range declared.Perms {
		out.Perms = append(out.Perms, settings.PluginPerm{ID: string(perm), Text: pluginsPermText(m, perm), Optional: optional(perm), Granted: perm.Free() || granted.Has(perm), Free: perm.Free()})
	}
	if len(declared.Network) > 0 {
		out.Perms = append(out.Perms, settings.PluginPerm{ID: pluginsNetwork, Text: m.SettingsPluginsPermNetwork(plugins.RulesText(declared.Network)), Granted: len(granted.Network) > 0})
	}
	out.Risky = plugins.Risky(p.Manifest, declared)
	if !installing {
		out.Problem = a.pluginsScriptProblem(m, p)
		if a.cx != nil && s.loaded[p.ID] != nil {
			if ids := a.pluginsConflicts(a.cx)[p.ID]; len(ids) > 0 {
				var titles []string
				for _, c := range p.Contributes.Commands {
					if slices.Contains(ids, c.ID) {
						titles = append(titles, c.Title.In(m.Language()))
					}
				}
				out.Conflicts = m.SettingsPluginsConflicts(strings.Join(titles, ", "))
			}
		}
	}
	return out
}

// pluginsScriptProblem is why the script plugin p does not run, in the words of m: why its package was refused, or why Kando turned it off (health.json); "" when nothing keeps it from running.
func (a *app) pluginsScriptProblem(m locales.Messages, p plugins.Pack) string {
	s := &a.plugins.script
	for _, issue := range s.issues {
		if issue.Dir == p.Dir {
			return pluginsErrorText(m, issue.Err)
		}
	}
	if h, ok := plugins.HealthOf(s.health, p.ID, p.Hash); ok {
		return pluginsHealthText(m, h.Reason)
	}
	return ""
}

// pluginsHealthText says why Kando turned a plugin off, in the words of m.
func pluginsHealthText(m locales.Messages, reason string) string {
	switch reason {
	case plugins.ReasonInit:
		return m.SettingsPluginsStoppedInit()
	case plugins.ReasonFailed:
		return m.SettingsPluginsStoppedFailed()
	case plugins.ReasonSlow:
		return m.SettingsPluginsStoppedSlow()
	case plugins.ReasonMemory:
		return m.SettingsPluginsStoppedMemory()
	case plugins.ReasonRevoked:
		return m.PluginsErrRevoked()
	}
	return m.SettingsPluginsStoppedHung()
}

// pluginsPermText is what perm lets a plugin do, in the words of m; a plugin cannot word it itself.
func pluginsPermText(m locales.Messages, perm plugins.Perm) string {
	switch perm {
	case plugins.PermFilmCurrent:
		return m.SettingsPluginsPermFilmCurrent()
	case plugins.PermLineCurrent:
		return m.SettingsPluginsPermLineCurrent()
	case plugins.PermLinesSeen:
		return m.SettingsPluginsPermLinesSeen()
	case plugins.PermSavedRead:
		return m.SettingsPluginsPermSavedRead()
	case plugins.PermWordsRead:
		return m.SettingsPluginsPermWordsRead()
	case plugins.PermToast:
		return m.SettingsPluginsPermToast()
	case plugins.PermOpenURL:
		return m.SettingsPluginsPermOpenURL()
	}
	return m.SettingsPluginsPermSettingsGet()
}

func (a *app) SetPluginGrant(id, perm string, on bool) { a.pluginsSetGrant(id, perm, on) }

// PluginValue is the value of the plugin id's setting key, its default until the user set one (Manifest.Resolve).
func (a *app) PluginValue(id, key string) any {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if i < 0 {
		return nil
	}
	return a.plugins.packs[i].Resolve(a.plugins.script.values[id])[key]
}

func (a *app) SetPluginValue(id, key string, value any) { a.pluginsSetValue(id, key, value) }

func (a *app) PluginSecretKept(id, key string) bool { return a.plugins.script.secrets[id][key] }

func (a *app) SetPluginSecret(id, key, value string) { a.pluginsSetSecret(id, key, value) }

func (a *app) PluginDeveloper() bool { return a.plugins.script.developer }

func (a *app) SetPluginDeveloper(on bool) { a.pluginsSetDeveloper(on) }

// PluginDevKey reports whether this computer's developer key was read from, or made into, the secure store.
func (a *app) PluginDevKey() bool { return a.plugins.script.dev != nil }

func (a *app) MakePluginDevKey() { a.pluginsMakeDevKey() }

// PackPluginFolder asks for a developer's plugin folder, holding its kando-plugin.json and plugin.tsx, and packs it with the developer key (pluginsDevPack).
func (a *app) PackPluginFolder() {
	if a.plugins.busy || a.plugins.pending != nil {
		return
	}
	m := a.messages()
	a.pickFolder(ns.PickDirectoriesOptions{Title: m.SettingsPluginsDevPickTitle(), Button: m.SettingsPluginsPickButton()}, a.pluginsDevPack)
}

// ShowPluginAudit shows the audit log in the file manager, the plugins folder while there is none yet.
func (a *app) ShowPluginAudit() {
	root := a.pluginsRoot()
	if root == "" || a.cx == nil {
		return
	}
	a.plugins.err = nil
	path := plugins.AuditPath(root)
	update := a.update
	go func() {
		_, err := os.Stat(path)
		update(func() {
			target := path
			if err != nil {
				target = root
			}
			revealInFolder(a.cx, target, live.On(a, func(err error) {
				if err != nil {
					a.plugins.err = errPluginsFolderNotShown
				}
			}))
		})
	}()
}

// pluginsScriptErrorText is why a script plugin could not be installed, turned on or run, in the words of m, for the errors of plugins/script and the app's own; ok is false for another error.
func pluginsScriptErrorText(m locales.Messages, err error) (string, bool) {
	for _, e := range []struct {
		err  error
		text func() string
	}{
		{plugins.ErrUnknownPerm, m.PluginsErrUnknownPerm},
		{plugins.ErrBadCapabilities, m.PluginsErrBadCapabilities},
		{plugins.ErrTampered, m.PluginsErrTampered},
		{script.ErrUntrusted, m.PluginsErrUntrusted},
		{script.ErrRevoked, m.PluginsErrRevoked},
		{script.ErrRollback, m.PluginsErrRollback},
		{script.ErrRuntime, m.PluginsErrNeedsNewer},
		{script.ErrScope, m.PluginsErrScope},
		{script.ErrNoManifest, m.PluginsErrNoManifest},
		{script.ErrNoEntry, m.PluginsErrNoEntry},
		{errPluginsTooMany, m.PluginsErrTooManyScripts},
		{errPluginsDevID, m.PluginsErrDevID},
		{errPluginsNoDev, m.PluginsErrNoDevKey},
	} {
		if errors.Is(err, e.err) {
			return e.text(), true
		}
	}
	return "", false
}
