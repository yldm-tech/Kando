package main

import (
	"errors"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/settings"
)

// 浏览插件's reads and edits (settings.PluginIndexApp), read in the 插件 page's bindings and called from its handlers, which tell the revision (package live), so none of them calls Changed itself.

func (a *app) PluginIndexOpen() bool { return a.plugins.index.open }

func (a *app) SetPluginIndexOpen(open bool) { a.pluginsIndexOpen(open) }

// pluginsIndexDate is when the list was last asked for, as 浏览插件 says it.
func pluginsIndexDate(meta index.Meta) string { return meta.Fetched.Local().Format("2006-01-02 15:04") }

// PluginIndexStatus says how the list stands, in the interface's language: that this Kando has no index, that it is being read or updated, why it could not be updated (the list kept still shows), that Kando is offline and shows the list kept, or when it was last updated.
func (a *app) PluginIndexStatus() string {
	m := a.messages()
	p := a.plugins.index
	fetched := !p.meta.Fetched.IsZero()
	switch {
	case errors.Is(p.err, index.ErrNotConfigured):
		return m.SettingsPluginsIndexNotConfigured()
	case p.reading:
		return m.SettingsPluginsIndexLoading()
	case p.busy:
		return m.SettingsPluginsIndexUpdating()
	case p.err != nil:
		return m.SettingsPluginsIndexFailed(pluginsErrorText(m, p.err))
	case a.privacy.offline && fetched:
		return m.SettingsPluginsIndexOffline(pluginsIndexDate(p.meta))
	case a.privacy.offline:
		return m.SettingsPluginsIndexOfflineNone()
	case fetched:
		return m.SettingsPluginsIndexUpdated(pluginsIndexDate(p.meta))
	}
	return ""
}

// PluginIndexFailed reports whether the last ask for the list failed, other than for want of an index.
func (a *app) PluginIndexFailed() bool {
	p := a.plugins.index
	return p.err != nil && !p.reading && !p.busy && !errors.Is(p.err, index.ErrNotConfigured)
}

// PluginIndexCanRefresh reports whether 刷新 can ask for a newer list: Kando has an index and is online, and it is not being read or asked for.
func (a *app) PluginIndexCanRefresh() bool {
	p := a.plugins.index
	_, err := a.pluginsIndexSource()
	return err == nil && !a.privacy.offline && !p.busy && !p.reading
}

func (a *app) RefreshPluginIndex() { a.pluginsIndexRefresh() }

func (a *app) PluginIndexQuery() string { return a.plugins.index.query }

func (a *app) SetPluginIndexQuery(query string) { a.plugins.index.query = query }

func (a *app) PluginIndexKind() plugins.Kind { return a.plugins.index.kind }

func (a *app) SetPluginIndexKind(kind plugins.Kind) { a.plugins.index.kind = kind }

func (a *app) PluginIndexKinds() []plugins.Kind { return a.pluginsIndexKinds() }

// PluginIndexRows are the first pluginsIndexRowsMax packs that match the search and the filter, each with where it stands here.
func (a *app) PluginIndexRows() []settings.PluginIndexRow {
	found := a.pluginsIndexFound()
	found = found[:min(len(found), pluginsIndexRowsMax)]
	rows := make([]settings.PluginIndexRow, 0, len(found))
	for _, e := range found {
		state, installed := a.pluginsIndexState(e)
		rows = append(rows, settings.PluginIndexRow{Entry: e, State: state, Installed: installed})
	}
	return rows
}

func (a *app) PluginIndexMatches() int { return len(a.pluginsIndexFound()) }

// PluginIndexInstallable reports whether 安装 and 更新 can start: Kando is online, and no download, install or confirmation is under way.
func (a *app) PluginIndexInstallable() bool {
	return !a.privacy.offline && !a.plugins.busy && a.plugins.pending == nil && a.plugins.index.downloading == ""
}

func (a *app) InstallFromPluginIndex(id string) { a.pluginsIndexInstall(id) }

// pluginsIndexErrorText is the message of m for an error of the community index, false for another error.
func pluginsIndexErrorText(m locales.Messages, err error) (string, bool) {
	for _, e := range []struct {
		err  error
		text func() string
	}{
		{index.ErrNotConfigured, m.SettingsPluginsIndexNotConfigured},
		{index.ErrSignature, m.PluginsErrIndexSignature},
		{index.ErrFormat, m.PluginsErrIndexFormat},
		{index.ErrVersion, m.PluginsErrIndexVersion},
		{index.ErrTooLarge, m.PluginsErrIndexTooLarge},
		{index.ErrBadEntry, m.PluginsErrIndexBadEntry},
		{index.ErrRollback, m.PluginsErrIndexRollback},
		{index.ErrChecksum, m.PluginsErrIndexChecksum},
		{index.ErrHTTP, m.PluginsErrIndexHTTP},
		{index.ErrRedirect, m.PluginsErrIndexRedirect},
		{index.ErrNotListed, m.PluginsErrIndexNotListed},
	} {
		if errors.Is(err, e.err) {
			return e.text(), true
		}
	}
	return "", false
}
