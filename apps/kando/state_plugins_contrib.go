package main

import (
	"errors"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
	"kando/apps/kando/study"
	kcore "kando/packages/core"
)

// What the script plugins running contribute besides commands (docs/plugin-system.md 2.2): sources of word lookups, asked after the dictionary packs and before the AI service (E10, pluginsLookups); formats of the subtitle export, whose file Kando writes (E11, state_subexport.go); targets of the saved lines' export (E12, pluginsExporters); and annotators of the lines (E13, state_plugins_marks.go). Each calls a function the plugin exports through the runner, one call at a time under its watchdog, as a command does; a plugin turned off, stopped or not loaded yet contributes nothing.

// pluginsContrib is what the contributions keep: formatting is set while the subtitle export waits for a plugin's format, which no second export starts during.
type pluginsContrib struct {
	formatting bool
}

// pluginsLookupWait is the longest a lookup waits for a plugin's lookUp before it asks the next source.
const pluginsLookupWait = 15 * time.Second

// errPluginsNotRunning is a contribution of a plugin that is not running now.
var errPluginsNotRunning = errors.New("the plugin is not running")

// pluginsContribCall calls the function export of the plugin id running with args through the runner, and has done hear what it returned on the engine's goroutine; errPluginsNotRunning when it is not running.
func (a *app) pluginsContribCall(id, export string, args []any, done func(any, error)) {
	l := a.plugins.script.loaded[id]
	if l == nil || a.cx == nil {
		done(nil, errPluginsNotRunning)
		return
	}
	a.pluginsRunner().Call(l.plugin, export, args, done)
}

// pluginsContributions is each contribution of the plugins running that pick takes from their manifests, with the id of its plugin, in the order the plugins are listed.
func pluginsContributions[T any](a *app, pick func(c *plugins.Contributes) []T) []pluginsContribution[T] {
	var out []pluginsContribution[T]
	for _, id := range a.pluginsLoadedOrder() {
		l := a.plugins.script.loaded[id]
		if l.pack.Contributes == nil {
			continue
		}
		for _, x := range pick(l.pack.Contributes) {
			out = append(out, pluginsContribution[T]{id: id, name: l.pack.Name, of: x})
		}
	}
	return out
}

// pluginsContribution is a contribution of the plugin id, named name.
type pluginsContribution[T any] struct {
	id   string
	name plugins.Text
	of   T
}

// pluginsLookup is a lookup source of a script plugin (lookup.Dictionary), for the lookup of q: LookUp, off the engine's goroutine, calls the plugin's export with the word and its line, and waits for its answer, pluginsLookupWait at most.
type pluginsLookup struct {
	a      *app
	id     string
	source string
	export string
	q      lookup.Query
}

// ID names the source as the lookup's card and a phone's say where an entry came from (pluginsLookupNames): plugin:<plugin id>:<source id>.
func (p pluginsLookup) ID() string { return pluginsContribID(p.id, p.source) }

// LookUp asks the plugin {word, base, reading, line, spoken, language}: the word as clicked, its dictionary form and reading when known, the line it was clicked in, the language the film is spoken in and the one the meaning is to be in. Its answer is an entry or null (lookup.FromPlugin).
func (p pluginsLookup) LookUp(word, base, reading string) (lookup.Entry, bool, error) {
	type answer struct {
		v   any
		err error
	}
	got := make(chan answer, 1)
	query := map[string]any{"word": word, "base": base, "reading": reading, "line": p.q.Line, "spoken": p.q.Spoken, "language": kcore.TargetTag(p.q.Language)}
	p.a.update(func() {
		p.a.pluginsContribCall(p.id, p.export, []any{query}, func(v any, err error) { got <- answer{v, err} })
	})
	select {
	case r := <-got:
		if r.err != nil {
			return lookup.Entry{}, false, r.err
		}
		return lookup.FromPlugin(word, r.v)
	case <-time.After(pluginsLookupWait):
		return lookup.Entry{}, false, errPluginsSlowLookup
	}
}

// errPluginsSlowLookup is a plugin's lookup that did not answer in time.
var errPluginsSlowLookup = errors.New("the plugin's lookup did not answer in time")

// pluginsLookups are the lookup sources of the plugins running for the word looked up as q: those of no language, or of the word's (lookup.WordLanguage), all of them when it is not known, as for the dictionary packs, in the order the plugins are listed. A lookup asks them after the dictionary packs and before the AI service (vocabResolve).
func (a *app) pluginsLookups(q lookup.Query) []lookup.Dictionary {
	lang := kcore.BaseLanguage(lookup.WordLanguage(q))
	var out []lookup.Dictionary
	for _, c := range pluginsContributions(a, func(c *plugins.Contributes) []plugins.Lookup { return c.Lookups }) {
		if lang != "" && len(c.of.Languages) > 0 && !slices.ContainsFunc(c.of.Languages, func(l string) bool { return kcore.BaseLanguage(l) == lang }) {
			continue
		}
		out = append(out, pluginsLookup{a: a, id: c.id, source: c.of.ID, export: c.of.Export, q: q})
	}
	return out
}

// pluginsLookupNames are the names of the plugin and of its lookup source that id names (pluginsLookup.ID), and whether it names one of a plugin installed; the names of a source that went since it answered are its plugin's id and its own.
func (a *app) pluginsLookupNames(id string) (plugin, source plugins.Text, ok bool) {
	rest, ok := strings.CutPrefix(id, pluginsContribPrefix)
	if !ok {
		return nil, nil, false
	}
	pluginID, sourceID, _ := strings.Cut(rest, ":")
	plugin, source = plugins.Text{"en": pluginID}, plugins.Text{"en": sourceID}
	if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == pluginID }); i >= 0 {
		p := a.plugins.packs[i]
		plugin = p.Name
		if p.Contributes != nil {
			if j := slices.IndexFunc(p.Contributes.Lookups, func(l plugins.Lookup) bool { return l.ID == sourceID }); j >= 0 {
				source = p.Contributes.Lookups[j].Label
			}
		}
	}
	return plugin, source, true
}

// The ids of what the plugins contribute to the subtitle export and the saved lines' export: plugin:<plugin id>:<contribution id>, which a pack's id, with no colon, never is.
const pluginsContribPrefix = "plugin:"

// pluginsContribID is the id of the contribution c of the plugin id, as the exports name it.
func pluginsContribID(id, c string) string { return pluginsContribPrefix + id + ":" + c }

// pluginsSubtitleFormats are the subtitle formats of the plugins running, each with the id the export names it by (pluginsContribID) and its label in the interface's language.
func (a *app) pluginsSubtitleFormats() []pluginsContribution[plugins.SubtitleFormat] {
	return pluginsContributions(a, func(c *plugins.Contributes) []plugins.SubtitleFormat { return c.SubtitleFormats })
}

// pluginsSubtitleFormat is the subtitle format of a plugin running that the export names value, and whether there is one.
func (a *app) pluginsSubtitleFormat(value string) (pluginsContribution[plugins.SubtitleFormat], bool) {
	if !strings.HasPrefix(value, pluginsContribPrefix) {
		return pluginsContribution[plugins.SubtitleFormat]{}, false
	}
	for _, f := range a.pluginsSubtitleFormats() {
		if pluginsContribID(f.id, f.of.ID) == value {
			return f, true
		}
	}
	return pluginsContribution[plugins.SubtitleFormat]{}, false
}

// pluginsExporters are the export targets of the plugins running.
func (a *app) pluginsExporters() []pluginsContribution[plugins.Exporter] {
	return pluginsContributions(a, func(c *plugins.Contributes) []plugins.Exporter { return c.Exporters })
}

// pluginsExporter is the export target of a plugin running the sheet names value, and whether there is one.
func (a *app) pluginsExporter(value string) (pluginsContribution[plugins.Exporter], bool) {
	if !strings.HasPrefix(value, pluginsContribPrefix) {
		return pluginsContribution[plugins.Exporter]{}, false
	}
	for _, e := range a.pluginsExporters() {
		if pluginsContribID(e.id, e.of.ID) == value {
			return e, true
		}
	}
	return pluginsContribution[plugins.Exporter]{}, false
}

// pluginsExportTargets are the export targets of the plugins running as the Saved Lines sheet offers them, each named by its label and its plugin's name.
func (a *app) pluginsExportTargets() []study.ExportTarget {
	m := a.messages()
	var out []study.ExportTarget
	for _, e := range a.pluginsExporters() {
		out = append(out, study.ExportTarget{ID: pluginsContribID(e.id, e.of.ID), Label: m.SavedExportTargetPlugin(e.of.Label.In(m.Language()))})
	}
	return out
}

// pluginsExportTo sends the saved lines, the words of the vocabulary or both, as the target e takes them, to its plugin's export, exportNotes(items, settings): items the lines and the words as kando.saved.read and kando.words.read give them, each with its kind ("line" or "word"), at most pluginsListMax of each, the newest first; settings the plugin's own (kando.settings.get's). The Saved Lines sheet says how many it sent and skipped, as the plugin answered ({sent, skipped}), or that it failed.
func (a *app) pluginsExportTo(e pluginsContribution[plugins.Exporter]) {
	l := a.plugins.script.loaded[e.id]
	if l == nil {
		return
	}
	book := a.savedBook()
	var items []any
	if slices.Contains(e.of.Takes, plugins.TakesLines) {
		for i := len(book.Lines) - 1; i >= 0 && len(items) < pluginsListMax; i-- {
			v := a.pluginsSavedLine(l, book.Lines[i])
			v["kind"] = "line"
			items = append(items, v)
		}
	}
	if slices.Contains(e.of.Takes, plugins.TakesWords) {
		n := 0
		for i := len(book.Words) - 1; i >= 0 && n < pluginsListMax; i-- {
			v := a.pluginsWord(l, book.Words[i])
			v["kind"] = "word"
			items = append(items, v)
			n++
		}
	}
	if items == nil {
		items = []any{}
	}
	name := l.pack.Name.In(a.messages().Language())
	a.saved.export.gen++
	gen := a.saved.export.gen
	a.saved.export = savedExport{busy: true, total: len(items), gen: gen, plugin: name}
	a.pluginsContribCall(e.id, e.of.Export, []any{items, l.pack.Resolve(a.plugins.script.values[e.id])}, func(v any, err error) {
		defer a.Changed()
		if a.saved.export.gen != gen {
			return
		}
		sent, skipped, ok := plugins.Exported(v)
		if err == nil && !ok {
			err = errPluginsBadExport
		}
		a.saved.export = savedExport{finished: true, gen: gen, plugin: name, sent: sent, skipped: skipped, err: err, total: len(items), done: len(items)}
	})
}

// errPluginsBadExport is an export target that answered something other than {sent, skipped}.
var errPluginsBadExport = errors.New("the plugin's export answered something else than what it sent")
