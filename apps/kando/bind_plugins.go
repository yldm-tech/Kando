package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"
	"kando/packages/core"
)

// The 插件 page's reads and edits (settings.PluginsApp). The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself. The words they say are in the interface's language (app.messages), which the page shows too; a pack's own words (its name, description) are the page's to pick in that language.

// PluginRows are the installed packs and the remembered ones that are missing, in the order the settings keep (pluginsState.order); a pack not in it yet comes after.
func (a *app) PluginRows() []settings.PluginRow {
	m := a.messages()
	var rows []settings.PluginRow
	seen := map[string]bool{}
	add := func(pack plugins.Pack) {
		seen[pack.ID] = true
		rows = append(rows, settings.PluginRow{Pack: pack, Enabled: a.plugins.state.Enabled(pack.ID), Detail: pluginsDetail(m, pack)})
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
	return rows
}

// pluginsDetail is what pack adds, in the words of m: a glossary's number of terms and its languages, an AI service's host, which the user's key goes to, a dictionary's languages, a word list's language and number of levels.
func pluginsDetail(m locales.Messages, pack plugins.Pack) string {
	switch {
	case pack.Glossary != nil:
		return m.SettingsPluginsGlossaryDetail(len(pack.Glossary.Terms), pluginsLanguageName(m, pack.Glossary.From), pluginsLanguageName(m, pack.Glossary.To))
	case pack.Service != nil:
		return m.SettingsPluginsServiceDetail(settings.Host(pack.Service.BaseURL))
	case pack.Dictionary != nil:
		return m.SettingsPluginsDictionaryDetail(strings.Join(pack.Dictionary.From, ", "), strings.Join(pack.Dictionary.To, ", "))
	case pack.Wordlist != nil:
		return m.SettingsPluginsWordlistDetail(pack.Wordlist.Language, len(pack.Wordlist.Tiers))
	}
	return ""
}

// pluginsLanguageName names a pack's BCP 47 language tag in the words of m, as the settings name the languages Kando translates into and hears: 简体中文 for zh-Hans, 日语 for ja. A tag Kando does not name is shown as the settings show an unknown spoken language.
func pluginsLanguageName(m locales.Messages, tag string) string {
	switch strings.ToLower(tag) {
	case "zh-hans":
		return subtitle.TargetLanguageNameIn(m, "简体中文")
	case "zh-hant":
		return subtitle.TargetLanguageNameIn(m, "繁体中文")
	}
	return subtitle.SpokenLanguageNameIn(m, core.BaseLanguage(tag))
}

// PluginIssues are the folders of the plugins folder that are not valid packs, each named by its folder and why it is not, in the interface's language.
func (a *app) PluginIssues() []settings.PluginIssue {
	m := a.messages()
	out := make([]settings.PluginIssue, 0, len(a.plugins.issues))
	for _, issue := range a.plugins.issues {
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
	in := settings.PluginInstall{Open: true, Pack: p.pack, Detail: pluginsDetail(m, p.pack)}
	if i := slices.IndexFunc(a.plugins.packs, func(o plugins.Pack) bool { return o.ID == p.pack.ID }); i >= 0 {
		in.Replaces = a.plugins.packs[i].Version
	}
	return in
}

// PluginInstallHere reports whether the 插件 page draws the install confirmation: while the settings show it in a window of their own. Over the main window, and while the settings show another page or none, the main window draws it (pluginsOverlay).
func (a *app) PluginInstallHere() bool {
	return a.plugins.pending != nil && a.prefs.open && !a.prefs.inWindow && a.prefs.section == settings.SectionPlugins
}

func (a *app) ConfirmPluginInstall() { a.pluginsInstall() }

func (a *app) CancelPluginInstall() { a.pluginsClose(false) }

// pluginsErrorText is why a pack could not be read, installed or removed, in the words of m: each of the plugins package's errors has a message of its own, and an error of the system, as a folder that cannot be written, says what it was about in the system's words after one.
func pluginsErrorText(m locales.Messages, err error) string {
	for _, e := range []struct {
		err  error
		text func() string
	}{
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
	} {
		if errors.Is(err, e.err) {
			return e.text()
		}
	}
	return m.PluginsErrSystem(err.Error())
}
