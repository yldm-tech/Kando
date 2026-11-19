package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
)

// The community plugin index (docs/plugin-system.md, P5): Settings › 插件's 浏览插件 shows the packs Kando's signed index lists (plugins/index), which the user searches, filters by kind and installs or updates from there. The index is read only once the user opens 浏览插件: the copy kept in the plugins folder at once, and a newer one asked for when the one kept is a day old or more (index.RefreshEvery), or when the user asks; never while Kando is offline, which shows the copy kept. Installing downloads the pack's file, checked against the size and SHA-256 the index gives, and hands it to the same path as a file the user picked: plugins.Stage, plugins.Validate, for a script plugin script.Verify, and the install confirmation, which installs nothing until the user confirms; so a listed pack is granted nothing, a script plugin and a task pack are installed off, and an update keeps or narrows what the version before it was granted (plugins.InstallEntry).

// pluginsIndex is the index's state: url and key are where it is read and what checks it (index.DefaultURL and script.IndexKey; tests set their own), client the HTTP client, nil for Go's own (tests set the TLS server's). open is set while 浏览插件 shows. idx is the index read, kept or new, read once the copy kept was read (or found missing), meta what is known of it, and hay the words each of its entries is found by, lowercased, in the order of its entries (pluginsIndexSet); reading is set while the copy kept is read, busy while a newer one is asked for, err why the last ask failed. query and kind are 浏览插件's search and kind filter. downloading is the id of the pack being downloaded, "" while none is.
type pluginsIndex struct {
	url, key    string
	client      *http.Client
	open        bool
	idx         index.Index
	hay         []string
	meta        index.Meta
	read        bool
	reading     bool
	busy        bool
	err         error
	query       string
	kind        plugins.Kind
	downloading string
}

func newPluginsIndex() pluginsIndex {
	return pluginsIndex{url: index.DefaultURL, key: script.IndexKey}
}

// pluginsIndexRowsMax is the most packs 浏览插件 lists at once; the search narrows them down.
const pluginsIndexRowsMax = 50

// pluginsIndexSource is the index as the app reads it, with its key decoded; index.ErrNotConfigured while Kando has no key, or no plugins folder to keep it in.
func (a *app) pluginsIndexSource() (index.Source, error) {
	root := a.pluginsRoot()
	key, err := index.DecodeKey(a.plugins.index.key)
	if err == nil && root == "" {
		err = index.ErrNotConfigured
	}
	if err != nil {
		return index.Source{}, err
	}
	return index.Source{URL: a.plugins.index.url, Key: key, Dir: index.CacheDir(root), Client: a.plugins.index.client}, nil
}

// pluginsIndexOpen shows or hides 浏览插件. Shown the first time, it reads the copy of the index kept, then asks for a newer one when that is due (pluginsIndexRead).
func (a *app) pluginsIndexOpen(open bool) {
	a.plugins.index.open = open
	if open && !a.plugins.index.read {
		a.pluginsIndexRead()
	}
}

// pluginsIndexRead reads the copy of the index kept in the plugins folder off the UI thread, and once it is read asks for a newer one when Kando is online and the one kept is RefreshEvery old or more, or there is none.
func (a *app) pluginsIndexRead() {
	src, err := a.pluginsIndexSource()
	p := &a.plugins.index
	if err != nil {
		p.read, p.err = true, err
		return
	}
	if p.reading {
		return
	}
	p.reading = true
	update := a.update
	go func() {
		idx, meta, err := src.Cached()
		due := src.Due(time.Now())
		update(func() {
			p.reading, p.read = false, true
			if err == nil {
				p.set(idx, meta)
			}
			if due && !a.privacy.offline {
				a.pluginsIndexRefresh()
			}
		})
	}()
}

// pluginsIndexRefresh asks for a newer index off the UI thread (index.Source.Refresh), unless one is being asked for or Kando is offline; what comes replaces the index shown, and a failure keeps it and says why.
func (a *app) pluginsIndexRefresh() {
	p := &a.plugins.index
	if p.busy || a.privacy.offline {
		return
	}
	src, err := a.pluginsIndexSource()
	if err != nil {
		p.read, p.err = true, err
		return
	}
	p.busy, p.err = true, nil
	update := a.update
	go func() {
		idx, meta, err := src.Refresh(context.Background(), time.Now())
		update(func() {
			p.busy, p.err = false, err
			if err == nil {
				p.set(idx, meta)
				p.read = true
			}
		})
	}()
}

// pluginsIndexInstall downloads the pack id the index lists into a folder of its own, off the UI thread (index.Source.Download, which checks its size and SHA-256), and hands it to the install path a picked file takes: plugins.Stage, then for a script plugin script.Verify, then the install confirmation (pluginsAskStaged), which installs nothing until the user confirms. The pack read from the file must be the one the entry names, by id, kind and version, and a script plugin's signature must come from where the entry says (index.ErrNotListed). A failure, the download's included, shows in the confirmation's failure view, as a file that cannot be installed does. It waits while another install or a confirmation is under way, and does nothing while Kando is offline.
func (a *app) pluginsIndexInstall(id string) {
	root := a.pluginsRoot()
	p := &a.plugins.index
	entry, ok := p.idx.Lookup(id)
	if root == "" || !ok || a.plugins.busy || a.plugins.pending != nil || p.downloading != "" || a.privacy.offline {
		return
	}
	src, err := a.pluginsIndexSource()
	if err != nil {
		p.err = err
		return
	}
	a.plugins.busy, a.plugins.err, p.downloading = true, nil, id
	update, verify := a.update, a.pluginsStagedVerifier()
	go func() {
		staged, pack, v, err := pluginsIndexStage(src, entry, root, verify)
		update(func() {
			a.plugins.busy, p.downloading = false, ""
			a.pluginsAskStaged(pack, staged, v, err)
			if a.plugins.pending != nil {
				a.plugins.pending.listed = true
			}
		})
	}()
}

// pluginsIndexStage downloads the file of entry into a temporary folder, stages it in the plugins folder root (plugins.Stage, which runs plugins.Validate on the copy) and verifies a script plugin (verify), checking the pack is the one entry lists; the downloaded file goes once it is staged. Off the engine's goroutine.
func pluginsIndexStage(src index.Source, entry index.Entry, root string, verify func(plugins.Pack) (script.Verified, error)) (string, plugins.Pack, script.Verified, error) {
	tmp, err := os.MkdirTemp("", "kando-plugin-download-")
	if err != nil {
		return "", plugins.Pack{}, script.Verified{}, err
	}
	defer os.RemoveAll(tmp)
	file, err := src.Download(context.Background(), entry, tmp)
	if err != nil {
		return "", plugins.Pack{}, script.Verified{}, err
	}
	staged, pack, err := plugins.Stage(root, file)
	if err == nil {
		err = entry.Matches(pack)
	}
	v, err := pluginsVerifyStaged(verify, pack, err)
	if err == nil && pack.Kind == plugins.KindScript && v.Source != entry.Source {
		err = fmt.Errorf("%w: it is signed as %q, the index says %q", index.ErrNotListed, v.Source, entry.Source)
	}
	if err != nil && staged != "" {
		os.RemoveAll(staged)
		staged = ""
	}
	return staged, pack, v, err
}

// pluginsIndexState is where the pack of entry stands here: installed at its version or a newer one, installed at an older one (an update), being downloaded, needing a newer Kando, or ready to install; with the version installed, "" when none is.
func (a *app) pluginsIndexState(e index.Entry) (settings.PluginIndexState, string) {
	installed := ""
	if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == e.ID }); i >= 0 {
		installed = a.plugins.packs[i].Version
	}
	switch {
	case a.plugins.index.downloading == e.ID:
		return settings.PluginIndexDownloading, installed
	case installed != "" && !index.Newer(e.Version, installed):
		return settings.PluginIndexInstalled, installed
	case !e.RunsOn(appVersion):
		return settings.PluginIndexNeedsKando, installed
	case installed != "":
		return settings.PluginIndexUpdate, installed
	}
	return settings.PluginIndexAvailable, installed
}

// set takes idx as the index shown and meta as what is known of it, with the words each entry is found by: its id, its publisher, and its name and summary in every language they have, lowercased once here rather than at each search.
func (p *pluginsIndex) set(idx index.Index, meta index.Meta) {
	p.idx, p.meta = idx, meta
	p.hay = make([]string, len(idx.Entries))
	for i, e := range idx.Entries {
		words := []string{e.ID, e.Publisher}
		for _, t := range []plugins.Text{e.Name, e.Summary} {
			for _, s := range t {
				words = append(words, s)
			}
		}
		p.hay[i] = strings.ToLower(strings.Join(words, "\n"))
	}
}

// pluginsIndexFound are the entries of the index that match the search and the kind filter, in the index's order (by id): each of the search's words is in the entry's id, publisher, or name or summary in any language, letter case aside.
func (a *app) pluginsIndexFound() []index.Entry {
	p := a.plugins.index
	words := strings.Fields(strings.ToLower(p.query))
	var out []index.Entry
	for i, e := range p.idx.Entries {
		if p.kind != "" && e.Kind != p.kind {
			continue
		}
		if i < len(p.hay) && !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(p.hay[i], w) }) {
			out = append(out, e)
		}
	}
	return out
}

// pluginsIndexKinds are the kinds the index lists, in the order the 插件 page groups the installed packs, for 浏览插件's filter.
func (a *app) pluginsIndexKinds() []plugins.Kind {
	var out []plugins.Kind
	for _, e := range a.plugins.index.idx.Entries {
		if !slices.Contains(out, e.Kind) {
			out = append(out, e.Kind)
		}
	}
	return out
}
