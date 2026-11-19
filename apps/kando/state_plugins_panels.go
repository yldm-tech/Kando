package main

import (
	"maps"
	"slices"
	"strings"

	"kando/apps/kando/film"
	"kando/apps/kando/plugins"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// The panels script plugins add to the player's 扩展 tab (docs/plugin-system.md E15): each a view of the plugin's own, its package run anew apart from the plugin the runner calls, in the same sandbox, with what the user granted the plugin (script.Runner.Panel). A panel's view has a host of its own, as a core.Host serves one app at a time; its handlers are the plugin's, which take it for the plugin's while it is the view shown (pluginsGuard), so it reads Kando through the same kando.* calls, under the same grant and quota. It hears the plugin's events, and line.changed as the line playing changes (pluginsPanelsLine). A view that fails shows so in the tab and counts for the plugin's breaker, and never takes the player down.

// pluginsPanels is the 扩展 tab's state: chosen is the panel chosen last (savedPlugins.PluginPanel), runs counts the plugins' loads (pluginsScript.run), and line is the line the panels last heard line.changed for, -1 for none, with sent false before the first.
type pluginsPanels struct {
	chosen string
	runs   int
	line   int
	sent   bool
}

// pluginsPanelsOf is the panels of the plugins running, each with the id the tab names it by (pluginsContribID), in the order the plugins are listed.
func (a *app) pluginsPanelsOf() []pluginsContribution[plugins.Panel] {
	return pluginsContributions(a, func(c *plugins.Contributes) []plugins.Panel { return c.Panels })
}

// Extensions are the panels of the script plugins running, their titles and their plugins' names in the language the interface shows under cx (film.Extender).
func (a *app) Extensions(cx *ns.Scope) []film.Extension {
	lang := i18n.Language(cx)
	var out []film.Extension
	for _, c := range a.pluginsPanelsOf() {
		out = append(out, film.Extension{ID: pluginsContribID(c.id, c.of.ID), Title: c.of.Title.In(lang), Plugin: c.name.In(lang), Icon: c.of.Icon, Run: a.plugins.script.loaded[c.id].run})
	}
	return out
}

// Extension is the panel chosen last, kept while its plugin is not running (film.Extender).
func (a *app) Extension() string { return a.plugins.panels.chosen }

// SetExtension chooses the panel id of a plugin running and saves the choice (film.Extender).
func (a *app) SetExtension(id string) {
	if id == a.plugins.panels.chosen || !slices.ContainsFunc(a.pluginsPanelsOf(), func(c pluginsContribution[plugins.Panel]) bool { return pluginsContribID(c.id, c.of.ID) == id }) {
		return
	}
	a.plugins.panels.chosen = id
	a.Save()
}

// ExtensionView builds the view of the panel id under cx, with a host of its own that the plugin's handlers answer while it is the one shown, and the grant its plugin was loaded with (film.Extender); nothing for a panel of no plugin running. failed shows in its place while it fails.
func (a *app) ExtensionView(cx *ns.Scope, id string, failed func(cx *ns.Scope, retry func()) ns.Node) ns.Node {
	pluginID, panelID, ok := strings.Cut(strings.TrimPrefix(id, pluginsContribPrefix), ":")
	l := a.plugins.script.loaded[pluginID]
	if !ok || !strings.HasPrefix(id, pluginsContribPrefix) || l == nil || l.pack.Contributes == nil || a.cx == nil {
		return ns.View{}
	}
	i := slices.IndexFunc(l.pack.Contributes.Panels, func(p plugins.Panel) bool { return p.ID == panelID })
	if i < 0 {
		return ns.View{}
	}
	host := ns.NewHost()
	a.pluginsHost(pluginID, host)
	l.panel = host
	// The line playing is told again to the view built now.
	a.plugins.panels.sent = false
	cx.Cleanup(func() {
		if l.panel == host {
			l.panel = nil
		}
	})
	return a.pluginsRunner().Panel(l.plugin, l.v, l.grant, host, l.pack.Contributes.Panels[i], func(cx *ns.Scope, _ error, retry func()) ns.Node {
		return failed(cx, retry)
	})
}

// pluginsPanelsLine sends line.changed, with the index of the line playing (pluginsCurrentLine), to the view of each panel shown whose plugin hears it and may read the line playing now, once the line changed since it last did; after each progress report of the player and each change of the app's state.
func (a *app) pluginsPanelsLine() {
	p := &a.plugins.panels
	line := a.pluginsCurrentLine()
	if p.sent && line == p.line {
		return
	}
	p.line, p.sent = line, true
	for _, id := range slices.Sorted(maps.Keys(a.plugins.script.loaded)) {
		l := a.plugins.script.loaded[id]
		if l.panel != nil && slices.Contains(l.pack.Events, plugins.EventLineChanged) && a.pluginsEffective(l.pack).Has(plugins.PermLineCurrent) {
			l.panel.Send(plugins.EventLineChanged, map[string]any{"index": line})
		}
	}
}
