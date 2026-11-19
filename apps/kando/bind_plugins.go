package main

import (
	"errors"
	"path/filepath"
	"slices"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/dict"
	"kando/apps/kando/plugins/wordlist"
	"kando/apps/kando/settings"
)

// The 插件 page's reads and edits (settings.PluginsApp). The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself. The words they say are in the interface's language (app.messages), which the page shows too; a pack's own words (its name, description) are the page's to pick in that language.

// PluginRows are the installed packs and the remembered ones that are missing, in the order the settings keep (pluginsUI.order); a pack not in it yet comes after. A dictionary's row says whether it can move up or down among the dictionaries, a task's its daily budget and what it spent today.
func (a *app) PluginRows() []settings.PluginRow {
	m := a.messages()
	var rows []settings.PluginRow
	seen := map[string]bool{}
	add := func(pack plugins.Pack) {
		seen[pack.ID] = true
		row := settings.PluginRow{Pack: pack, Enabled: a.plugins.state.EnabledPack(pack), Detail: settings.PluginDetail(m, pack)}
		if pack.Kind == plugins.KindScript {
			row.Script = a.pluginsScriptView(m, pack, false)
		}
		if pack.Kind == plugins.KindTask || pack.Kind == plugins.KindScript && pack.Declared().Has(plugins.PermLLMChat) {
			row.Budget, row.Spent = a.plugins.state.Budget(pack.ID), a.pluginsSpent(pack.ID)
		}
		rows = append(rows, row)
	}
	for _, id := range a.plugins.order {
		i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
		switch {
		case i >= 0:
			add(a.plugins.packs[i])
		case a.plugins.scanned:
			seen[id] = true
			rows = append(rows, settings.PluginRow{Pack: plugins.Pack{Manifest: plugins.Manifest{ID: id}}, Missing: true})
		}
	}
	for _, pack := range a.plugins.packs {
		if !seen[pack.ID] {
			add(pack)
		}
	}
	// The dictionaries are asked in this order, which their rows move up and down.
	var dicts []int
	for i, r := range rows {
		if !r.Missing && r.Pack.Kind == plugins.KindDictionary {
			dicts = append(dicts, i)
		}
	}
	for n, i := range dicts {
		rows[i].Up, rows[i].Down = n > 0, n < len(dicts)-1
	}
	return rows
}

// PluginIssues are the folders of the plugins folder that are not valid packs, the dictionaries turned on that could not be opened, and the word list chosen when it could not be read, each named by its folder and why, in the interface's language.
func (a *app) PluginIssues() []settings.PluginIssue {
	m := a.messages()
	out := make([]settings.PluginIssue, 0, len(a.plugins.issues))
	for _, issue := range slices.Concat(a.plugins.issues, a.pluginsDictIssues(), a.pluginsListIssues()) {
		out = append(out, settings.PluginIssue{Folder: filepath.Base(issue.Dir), Reason: pluginsErrorText(m, issue.Err)})
	}
	return out
}

// PluginsNote says that state.json could not be read as this version's, which turns every pack off until it is written again, or that a newer Kando wrote it, which this one does not write over.
func (a *app) PluginsNote() string {
	switch err := a.plugins.stateErr; {
	case errors.Is(err, plugins.ErrStateFormat):
		return a.messages().SettingsPluginsStateNewer()
	case errors.Is(err, plugins.ErrBadState):
		return a.messages().SettingsPluginsStateBad()
	}
	return ""
}

// PluginsErr is why the last install, removal, switch or look at the plugins folder failed, in the interface's language.
func (a *app) PluginsErr() string {
	if a.plugins.err == nil {
		return ""
	}
	return pluginsErrorText(a.messages(), a.plugins.err)
}

func (a *app) PluginsBusy() bool { return a.plugins.busy }

func (a *app) SetPluginEnabled(id string, on bool) { a.pluginsSetEnabled(id, on) }

func (a *app) SetPluginBudget(id string, tokens int) { a.pluginsSetBudget(id, tokens) }

func (a *app) PickPluginFile() { a.pluginsPickFile() }

// InstallPlugins installs the first of the files or folders dropped on the 插件 page, after its confirmation; a drop of several asks about the first alone, as each confirmation is one pack's.
func (a *app) InstallPlugins(paths []string) {
	if len(paths) > 0 {
		a.pluginsStage(paths[0])
	}
}

func (a *app) ShowPluginsFolder() { a.pluginsShowFolder() }

func (a *app) UninstallPlugin(id string) { a.pluginsUninstall(id) }

func (a *app) ForgetPlugin(id string) { a.pluginsForget(id) }

func (a *app) MovePlugin(id string, up bool) { a.pluginsMove(id, up) }

// PluginInstall is the install confirmation: the pack staged, what it adds, and the version of the pack of its id it replaces; or why the file cannot be installed.
func (a *app) PluginInstall() settings.PluginInstall {
	p := a.plugins.pending
	if p == nil {
		return settings.PluginInstall{}
	}
	m := a.messages()
	if p.err != nil {
		return settings.PluginInstall{Open: true, Err: pluginsErrorText(m, p.err)}
	}
	in := settings.PluginInstall{Open: true, Pack: p.pack, Detail: settings.PluginDetail(m, p.pack), Budget: a.plugins.state.Budget(p.pack.ID), Listed: p.listed}
	if p.pack.Kind == plugins.KindScript {
		in.Script = a.pluginsScriptView(m, p.pack, true)
	}
	if i := slices.IndexFunc(a.plugins.packs, func(o plugins.Pack) bool { return o.ID == p.pack.ID }); i >= 0 {
		in.Replaces = a.plugins.packs[i].Version
	}
	return in
}

// PluginInstallHere reports whether the 插件 page draws the install confirmation: while the settings show it in a window of their own. Over the main window, and while the settings show another page or none, the main window draws it (pluginsOverlay).
func (a *app) PluginInstallHere() bool {
	return a.plugins.pending != nil && a.prefs.open && !a.prefs.inWindow && a.prefs.section == settings.SectionPlugins
}

// PluginSiteAllowed reports whether the user allowed domain of the installed site pack id (plugins.State.Allowed); SetPluginSiteAllowed allows it or no longer does, saving state.json.
func (a *app) PluginSiteAllowed(id, domain string) bool { return a.plugins.state.Allowed(id, domain) }

func (a *app) SetPluginSiteAllowed(id, domain string, on bool) { a.pluginsAllowSite(id, domain, on) }

// PluginInstallSiteTicked reports whether domain of the site pack in the install confirmation is ticked, which installs it allowing the domain; SetPluginInstallSite ticks or unticks it.
func (a *app) PluginInstallSiteTicked(domain string) bool {
	p := a.plugins.pending
	return p != nil && slices.Contains(p.sites, domain)
}

func (a *app) SetPluginInstallSite(domain string, on bool) { a.pluginsTickSite(domain, on) }

func (a *app) ConfirmPluginInstall() { a.pluginsInstall() }

func (a *app) CancelPluginInstall() { a.pluginsClose(false) }

// pluginsErrorText is why a pack could not be read, installed or removed, in the words of m: each of the plugins package's errors has a message of its own (pluginsKnownErrorText), and an error of the system, as a folder that cannot be written, says what it was about in the system's words after one. An install or a removal done, after which state.json could not be saved (plugins.StateUnsaved), says it was done, and why the state was not saved, in its own message when it has one; an uninstall refused (plugins.ErrUninstallRefused) says it was not done.
func pluginsErrorText(m locales.Messages, err error) string {
	var unsaved plugins.StateUnsaved
	if errors.As(err, &unsaved) {
		if text, ok := pluginsKnownErrorText(m, unsaved.Err); ok {
			return m.PluginsErrStateUnsaved(text)
		}
		return m.PluginsErrStateUnsaved(unsaved.Err.Error())
	}
	if text, ok := pluginsKnownErrorText(m, err); ok {
		return text
	}
	return m.PluginsErrSystem(err.Error())
}

// pluginsKnownErrorText is the message of m for err when it is one of the errors the app knows, false for an error of the system.
func pluginsKnownErrorText(m locales.Messages, err error) (string, bool) {
	if text, ok := pluginsScriptErrorText(m, err); ok {
		return text, true
	}
	if text, ok := pluginsIndexErrorText(m, err); ok {
		return text, true
	}
	for _, e := range []struct {
		err  error
		text func() string
	}{
		{plugins.ErrUninstallRefused, m.PluginsErrUninstallRefused},
		{plugins.ErrBadManifest, m.PluginsErrBadManifest},
		{plugins.ErrUnknownKind, m.PluginsErrUnknownKind},
		{plugins.ErrUnsupportedKind, m.PluginsErrUnsupportedKind},
		{plugins.ErrSchema, m.PluginsErrSchema},
		{plugins.ErrBadID, m.PluginsErrBadID},
		{plugins.ErrReservedID, m.PluginsErrReservedID},
		{plugins.ErrBadName, m.PluginsErrBadName},
		{plugins.ErrReservedName, m.PluginsErrReservedName},
		{plugins.ErrBadField, m.PluginsErrBadField},
		{plugins.ErrInsecureURL, m.PluginsErrInsecureURL},
		{plugins.ErrBadSite, m.PluginsErrBadSite},
		{plugins.ErrSiteKnown, m.PluginsErrSiteKnown},
		{plugins.ErrBadShape, m.PluginsErrBadShape},
		{plugins.ErrUnsafePath, m.PluginsErrUnsafePath},
		{plugins.ErrBadArchive, m.PluginsErrBadArchive},
		{plugins.ErrTooLarge, m.PluginsErrTooLarge},
		{plugins.ErrMismatch, m.PluginsErrMismatch},
		{plugins.ErrIDTaken, m.PluginsErrIDTaken},
		{plugins.ErrNotInstalled, m.PluginsErrNotInstalled},
		{plugins.ErrStateFormat, m.SettingsPluginsStateNewer},
		{plugins.ErrBadState, m.SettingsPluginsStateBad},
		{errPluginsFolderNotShown, m.LibraryRevealNotOpened},
		{dict.ErrEmpty, m.PluginsErrDictEmpty},
		{wordlist.ErrEmpty, m.PluginsErrWordlistEmpty},
	} {
		if errors.Is(err, e.err) {
			return e.text(), true
		}
	}
	return "", false
}
