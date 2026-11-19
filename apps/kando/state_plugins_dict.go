package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/dict"
	"kando/packages/core"
)

// The dictionary packs turned on, opened for the lookups of the picture and of paired phones (vocabResolve). Each is opened off the UI thread (dict.Open), which builds its index in the plugins folder's .index the first time a content of it is opened, as after it was installed; the lookups then read that index on disk. They are opened after each read of the plugins folder and each switch, closed when turned off, before a pack of their id is replaced or removed, and as the app quits.

// pluginsDict is a dictionary pack opened for the content hash; d is nil while it is being opened, and when it could not be, err says why.
type pluginsDict struct {
	hash string
	d    *dict.Dict
	err  error
}

// pluginsIndexDir is the folder of the dictionaries' indexes, .index in the plugins folder, which Scan does not read as packs.
func (a *app) pluginsIndexDir() string {
	return filepath.Join(a.pluginsRoot(), ".index")
}

// pluginsSyncDicts opens the dictionary packs turned on that are not open yet, off the UI thread, and closes those open that are turned off, gone or of another content now. A dictionary that cannot be opened is listed among the page's problems (PluginIssues): one Kando cannot read, as one with no entry it reads (dict.ErrEmpty), is not tried again until its content changes; one that failed for a reason that may pass, as a full disk (dict.Retry), is tried again at the next sync, as after the next read of the plugins folder or switch. A content of a pack still being opened, as when it is turned off and on again while its index is built, is not opened a second time: the open running gives it its dictionary (pluginsDictOpened).
func (a *app) pluginsSyncDicts() {
	p := &a.plugins
	if p.dicts == nil {
		p.dicts = map[string]*pluginsDict{}
	}
	if p.opening == nil {
		p.opening = map[string]bool{}
	}
	want := map[string]plugins.Pack{}
	for _, pack := range a.pluginsOn(plugins.KindDictionary) {
		want[pack.ID] = pack
	}
	for id, open := range p.dicts {
		if pack, ok := want[id]; ok && pack.Hash == open.hash && !dict.Retry(open.err) {
			continue
		}
		a.pluginsCloseDict(id)
	}
	cache, update := a.pluginsIndexDir(), a.update
	for id, pack := range want {
		if _, ok := p.dicts[id]; ok {
			continue
		}
		p.dicts[id] = &pluginsDict{hash: pack.Hash}
		name := dict.IndexName(id, pack.Hash)
		if p.opening[name] {
			continue
		}
		p.opening[name] = true
		go func() {
			d, err := dict.Open(pack, cache)
			update(func() { a.pluginsDictOpened(id, pack.Hash, d, err) })
		}()
	}
}

// pluginsDictOpened takes the dictionary of the pack id whose content is hash, opened off the UI thread, or why it could not be. It is kept when that content is still wanted and not open yet, and closed otherwise, as when it was turned off, replaced or removed while it opened. Then the pack's indexes of other contents are removed off the UI thread, all of them once it is not installed any more (pluginsForgetIndexes); this runs after each open, never inside one, so an open that finishes late, of a content replaced or removed meanwhile, removes no index of the content wanted now and leaves none of its own behind.
func (a *app) pluginsDictOpened(id, hash string, d *dict.Dict, err error) {
	p := &a.plugins
	delete(p.opening, dict.IndexName(id, hash))
	if entry := p.dicts[id]; entry != nil && entry.hash == hash && entry.d == nil && entry.err == nil {
		entry.d, entry.err = d, err
	} else if d != nil {
		d.Close()
	}
	a.pluginsForgetIndexes(id)
}

// pluginsForgetIndexes removes, off the UI thread, the index files of the pack id but that of the content installed now, whether it is turned on or off, so turning it on again does not build it again; all of them once the pack is not installed, as when its folder went before the read of the plugins folder after its removal.
func (a *app) pluginsForgetIndexes(id string) {
	keep, dir := "", ""
	if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id }); i >= 0 {
		pack := a.plugins.packs[i]
		keep, dir = dict.IndexName(id, pack.Hash), pack.Dir
	}
	cache := a.pluginsIndexDir()
	go func() {
		if dir != "" {
			if _, err := os.Stat(dir); err != nil {
				keep = ""
			}
		}
		dict.Forget(cache, id, keep)
	}()
}

// pluginsDictFailed takes a lookup in the dictionary d that failed with err, off the UI thread (lookup.Ask.Failed). The dictionary, while it is still the one open for its pack, is closed and listed among the page's problems; a damaged index (dict.ErrBadIndex) is removed and built again at once, as Open builds a missing one, and another error that may pass is tried again at the next sync (pluginsSyncDicts).
func (a *app) pluginsDictFailed(d lookup.Dictionary, err error) {
	p := &a.plugins
	for id, open := range p.dicts {
		if open.d == nil || lookup.Dictionary(open.d) != d {
			continue
		}
		open.d.Close()
		open.d, open.err = nil, err
		i := slices.IndexFunc(p.packs, func(pack plugins.Pack) bool { return pack.ID == id && pack.Hash == open.hash })
		name := dict.IndexName(id, open.hash)
		if !errors.Is(err, dict.ErrBadIndex) || i < 0 || p.opening[name] {
			return
		}
		pack, cache, update := p.packs[i], a.pluginsIndexDir(), a.update
		open.err = nil
		p.opening[name] = true
		go func() {
			os.Remove(filepath.Join(cache, name))
			d, err := dict.Open(pack, cache)
			update(func() { a.pluginsDictOpened(id, pack.Hash, d, err) })
		}()
		return
	}
}

// pluginsCloseDict closes the dictionary of the pack id, if it is open, and forgets it; one still being opened is closed as it opens.
func (a *app) pluginsCloseDict(id string) {
	if open, ok := a.plugins.dicts[id]; ok {
		if open.d != nil {
			open.d.Close()
		}
		delete(a.plugins.dicts, id)
	}
}

// pluginsCloseDicts closes every dictionary, as the app quits.
func (a *app) pluginsCloseDicts() {
	for id := range a.plugins.dicts {
		a.pluginsCloseDict(id)
	}
}

// pluginsDictsFor are the dictionaries open for a word of the language lang (whisper's code or a language tag; "" when not known, which every dictionary is for) in the order the settings keep: first those whose definitions are in answer (the tag of the language the lookup answers in, core.TargetTag; "" for any), then, in others, those of the word's language whose definitions are in another, which answer only when no AI service can (lookup.Ask).
func (a *app) pluginsDictsFor(lang, answer string) (first, others []lookup.Dictionary) {
	lang = core.BaseLanguage(lang)
	for _, pack := range a.pluginsOn(plugins.KindDictionary) {
		open := a.plugins.dicts[pack.ID]
		if open == nil || open.d == nil || open.hash != pack.Hash {
			continue
		}
		d := pack.Dictionary
		if lang != "" && !slices.ContainsFunc(d.From, func(from string) bool { return core.BaseLanguage(from) == lang }) {
			continue
		}
		if answer == "" || slices.ContainsFunc(d.To, func(to string) bool { return plugins.SameLanguage(to, answer) }) {
			first = append(first, open.d)
		} else {
			others = append(others, open.d)
		}
	}
	return first, others
}

// pluginsDictIssues are the dictionaries turned on that could not be opened, and why, for the page's problems.
func (a *app) pluginsDictIssues() []plugins.Issue {
	var out []plugins.Issue
	for _, pack := range a.pluginsOn(plugins.KindDictionary) {
		if open := a.plugins.dicts[pack.ID]; open != nil && open.err != nil {
			out = append(out, plugins.Issue{Dir: pack.Dir, Err: open.err})
		}
	}
	return out
}

// pluginsMove moves the dictionary pack id one place up or down among the dictionaries, the order lookups ask them in (savedPlugins.PluginOrder), and saves the settings.
func (a *app) pluginsMove(id string, up bool) {
	var dicts []string
	for _, r := range a.PluginRows() {
		if !r.Missing && r.Pack.Kind == plugins.KindDictionary {
			dicts = append(dicts, r.Pack.ID)
		}
	}
	i := slices.Index(dicts, id)
	j := i + 1
	if up {
		j = i - 1
	}
	if i < 0 || j < 0 || j >= len(dicts) {
		return
	}
	order := a.plugins.order
	oi, oj := slices.Index(order, dicts[i]), slices.Index(order, dicts[j])
	if oi < 0 || oj < 0 {
		return
	}
	order[oi], order[oj] = order[oj], order[oi]
	if err := a.saveSettingsChanged(); err != nil {
		a.saveErr = a.messages().SettingsSaveFailed(err.Error())
	}
}
