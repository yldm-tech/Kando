package main

import (
	"cmp"
	"slices"
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"
	kcore "kando/packages/core"
	"kando/packages/media"
	"kando/packages/study"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Exporting the user's own glossary or vocabulary as a pack (docs/plugin-system.md P2.5): 导出术语表为插件包… and 导出生词本为词表… in the 文件 menu and the command palette open a sheet over the main window (settings.PluginExportDialog) that asks the pack's name and id and what goes in it; the pack is written in the same format as any other, checked by plugins.Validate as an install checks it (plugins.Build), and saved where the user picks with the system's save panel, as a .kandopack another Kando installs from 从文件安装….

// pluginsExport is the export sheet while it is open: a glossary pack (glossary) or a word list; the name and id typed, the id following the name until it is typed itself (idTyped); for a glossary the terms' scope ("global", "film", or a series' id), the language they are in (from, whisper's code) and the one they translate into (to, as the settings name it); for a word list the language of the words (language, whisper's code); busy while the pack is built and the save panel shows, and err why the last try failed. release gives back the modal flag it holds (holdModal).
type pluginsExport struct {
	glossary bool
	name, id string
	idTyped  bool
	scope    string
	from, to string
	language string
	busy     bool
	err      error
	release  func()
}

// The scopes of a glossary export besides a series' id: the global terms and the open film's own.
const (
	pluginsExportGlobal = "global"
	pluginsExportFilm   = "film"
)

// pluginsExportKeys are the 文件 menu's rows that export the glossary or the vocabulary as a pack, which the command palette lists too; each shows while there is something to export.
func (a *app) pluginsExportKeys(cx *ns.Scope) []shortcut {
	return []shortcut{
		{Group: groupFile, Label: locales.PluginsExportGlossary(cx), When: func() bool { return len(a.pluginsExportScopes(a.messages())) > 0 }, Run: func() { a.pluginsExportOpen(true) }},
		{Group: groupFile, Label: locales.PluginsExportWords(cx), When: func() bool { return len(a.pluginsExportLanguages()) > 0 }, Run: func() { a.pluginsExportOpen(false) }},
	}
}

// pluginsDataKeys are the data packs' rows of the shortcut table, beside the script plugins' commands (pluginKeys): the exports, then the task packs.
func (a *app) pluginsDataKeys(cx *ns.Scope) []shortcut {
	return slices.Concat(a.pluginsExportKeys(cx), a.pluginsTaskKeys(cx))
}

// pluginsExportScope is a scope a glossary export can take its terms from, with its terms and how the sheet names it.
type pluginsExportScope struct {
	id, label string
	terms     []kcore.Term
}

// pluginsExportScopes are the scopes that have terms, in the words of m: the global terms, the open film's own, and each series'.
func (a *app) pluginsExportScopes(m locales.Messages) []pluginsExportScope {
	var out []pluginsExportScope
	add := func(id, label string, terms []kcore.Term) {
		if slices.ContainsFunc(terms, func(t kcore.Term) bool { return t.Valid() }) {
			out = append(out, pluginsExportScope{id: id, label: label, terms: terms})
		}
	}
	add(pluginsExportGlobal, m.PluginsExportScopeGlobal(), a.gloss.store.Global)
	if a.gloss.filmRead {
		add(pluginsExportFilm, m.PluginsExportScopeFilm(), a.gloss.film)
	}
	for _, s := range a.gloss.store.Series {
		add(s.ID, m.PluginsExportScopeSeries(cmp.Or(s.Name, s.ID)), s.Terms)
	}
	return out
}

// pluginsExportLanguages are the languages of the vocabulary's words, as whisper's codes, in the order they first come; a word whose language is not known is in none.
func (a *app) pluginsExportLanguages() []string {
	var out []string
	for _, w := range a.savedBook().Words {
		if code := strings.TrimSpace(w.Spoken); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

// pluginsExportOpen opens the export sheet of the glossary, or of the vocabulary, its fields filled in as far as Kando knows them: the open film's languages, else the settings'.
func (a *app) pluginsExportOpen(glossaryPack bool) {
	if a.plugins.export != nil {
		return
	}
	m := a.messages()
	e := &pluginsExport{glossary: glossaryPack}
	job, _ := a.glossaryJob()
	spoken := cmp.Or(job.SpokenLanguage, job.SpeechLanguage, a.SpeechLanguage())
	if glossaryPack {
		e.name = m.PluginsExportGlossaryName()
		scopes := a.pluginsExportScopes(m)
		if len(scopes) == 0 {
			return
		}
		e.scope = scopes[0].id
		if i := slices.IndexFunc(scopes, func(s pluginsExportScope) bool { return job.Series != "" && s.id == job.Series }); i >= 0 {
			e.scope = scopes[i].id
		}
		e.from, e.to = spoken, cmp.Or(job.Language, a.prefs.language)
		if !media.KnownSpeechLanguage(e.from) {
			e.from = media.SpeechLanguages()[0]
		}
		if kcore.TargetTag(e.to) == "" {
			e.to = subtitle.TargetLanguages()[0]
		}
	} else {
		languages := a.pluginsExportLanguages()
		if len(languages) == 0 {
			return
		}
		e.language = languages[0]
		if slices.Contains(languages, spoken) {
			e.language = spoken
		}
		e.name = m.PluginsExportWordsName()
	}
	fallback := "my-words"
	if glossaryPack {
		fallback = "my-glossary"
	}
	e.id = "local." + plugins.Slug(e.name, fallback)
	e.release = a.holdModal()
	a.plugins.export = e
}

// pluginsExportClose closes the export sheet unless it is busy, giving back its modal flag.
func (a *app) pluginsExportClose() {
	e := a.plugins.export
	if e == nil || e.busy {
		return
	}
	a.plugins.export = nil
	e.release()
}

// pluginsExportPack is the manifest and files of the pack the sheet describes, with how many terms or words it holds; zero when it holds none.
func (a *app) pluginsExportPack() (plugins.Manifest, map[string][]byte, int) {
	e := a.plugins.export
	m := plugins.Manifest{Schema: 1, ID: strings.TrimSpace(e.id), Version: a.saved.now().Format("2006.1.2"), License: pluginsExportLicense,
		Name: plugins.Text{"en": strings.TrimSpace(e.name)}}
	if e.glossary {
		var terms []plugins.Term
		seen := map[string]bool{}
		for _, s := range a.pluginsExportScopes(a.messages()) {
			if s.id != e.scope {
				continue
			}
			for _, t := range s.terms {
				source, target := strings.TrimSpace(t.Source), strings.TrimSpace(t.Target)
				key := strings.ToLower(source)
				// A term too long for a pack, or one more than a pack holds, is left out rather than refusing the whole pack.
				if !t.Valid() || seen[key] || len([]rune(source)) > plugins.TermMax || len([]rune(target)) > plugins.TermMax || len(terms) == plugins.TermsMax {
					continue
				}
				seen[key] = true
				terms = append(terms, plugins.Term{Source: source, Target: target})
			}
		}
		m.Kind = plugins.KindGlossary
		m.Glossary = &plugins.Glossary{From: e.from, To: kcore.TargetTag(e.to), Terms: terms}
		return m, nil, len(terms)
	}
	var b strings.Builder
	words := 0
	seen := map[string]bool{}
	for _, w := range a.savedBook().Words {
		text := strings.Join(strings.Fields(w.Text), " ")
		key := strings.ToLower(text)
		// A word list's line starting with # is a comment, so such a word cannot be listed.
		if w.Spoken != e.language || text == "" || strings.HasPrefix(text, "#") || seen[key] {
			continue
		}
		seen[key] = true
		tier := "learning"
		if w.Box >= study.MasteredBox {
			tier = "mastered"
		}
		b.WriteString(text + "\t" + tier + "\n")
		words++
	}
	m.Kind = plugins.KindWordlist
	m.Wordlist = &plugins.Wordlist{Language: e.language, File: "words.tsv", Tiers: []plugins.Tier{
		{ID: "learning", Label: plugins.TextOf(locales.Interface, func(tag string) string { return locales.In(tag).PluginsExportTierLearning() }), Tone: plugins.ToneAccent},
		{ID: "mastered", Label: plugins.TextOf(locales.Interface, func(tag string) string { return locales.In(tag).PluginsExportTierMastered() }), Tone: plugins.ToneSuccess},
	}}
	return m, map[string][]byte{"words.tsv": []byte(b.String())}, words
}

// pluginsExportLicense is the license a pack exported from the user's own glossary or vocabulary names: theirs, not given to anyone.
const pluginsExportLicense = "Private"

// pluginsExportSave builds the pack the sheet describes off the UI thread (plugins.Build, which checks it as an install would), and then asks where to save it with the system's save panel, as <id>.kandopack; the sheet closes once it is saved and the picture says where. A pack that cannot be built says why in the sheet, which stays open.
func (a *app) pluginsExportSave() {
	e := a.plugins.export
	if e == nil || e.busy || a.cx == nil {
		return
	}
	m, files, n := a.pluginsExportPack()
	if n == 0 {
		return
	}
	e.busy, e.err = true, nil
	update := a.update
	go func() {
		data, _, err := plugins.Build(m, files)
		update(func() {
			if a.plugins.export != e {
				return
			}
			if err != nil {
				e.busy, e.err = false, err
				return
			}
			a.cx.Run(func() {
				a.cx.SaveFile(ns.SaveOptions{Name: m.ID + plugins.PackExt, Data: data}, func(saved bool, to string, err error) {
					defer a.Changed()
					e.busy = false
					switch {
					case err != nil:
						e.err = err
					case saved:
						a.pluginsExportClose()
						a.flash(a.messages().PluginsExportSaved(m.Name["en"]))
					}
				})
			})
		})
	}()
}

// PluginExport is the export sheet as it shows (settings.PluginsApp).
func (a *app) PluginExport() settings.PluginExport {
	e := a.plugins.export
	if e == nil {
		return settings.PluginExport{}
	}
	msgs := a.messages()
	out := settings.PluginExport{Open: true, Glossary: e.glossary, Name: e.name, ID: e.id, Scope: e.scope, From: e.from, To: e.to, Language: e.language, Busy: e.busy}
	for _, s := range a.pluginsExportScopes(msgs) {
		out.Scopes = append(out.Scopes, settings.PluginExportScope{ID: s.id, Label: s.label})
	}
	out.Languages = a.pluginsExportLanguages()
	_, _, out.Count = a.pluginsExportPack()
	if e.err != nil {
		out.Err = pluginsErrorText(msgs, e.err)
	}
	return out
}

// SetPluginExportName changes the pack's name, and its id with it until the id was typed.
func (a *app) SetPluginExportName(name string) {
	if e := a.plugins.export; e != nil {
		e.name, e.err = name, nil
		if !e.idTyped {
			e.id = "local." + plugins.Slug(name, strings.TrimPrefix(e.id, "local."))
		}
	}
}

// SetPluginExportID changes the pack's id, which then no longer follows its name.
func (a *app) SetPluginExportID(id string) {
	if e := a.plugins.export; e != nil {
		e.id, e.idTyped, e.err = strings.ToLower(id), true, nil
	}
}

// SetPluginExportScope, SetPluginExportFrom, SetPluginExportTo and SetPluginExportLanguage change what the pack holds.
func (a *app) SetPluginExportScope(id string) {
	if e := a.plugins.export; e != nil {
		e.scope, e.err = id, nil
	}
}

func (a *app) SetPluginExportFrom(code string) {
	if e := a.plugins.export; e != nil {
		e.from, e.err = code, nil
	}
}

func (a *app) SetPluginExportTo(language string) {
	if e := a.plugins.export; e != nil {
		e.to, e.err = language, nil
	}
}

func (a *app) SetPluginExportLanguage(code string) {
	if e := a.plugins.export; e != nil {
		e.language, e.err = code, nil
	}
}

func (a *app) ConfirmPluginExport() { a.pluginsExportSave() }

func (a *app) CancelPluginExport() { a.pluginsExportClose() }
