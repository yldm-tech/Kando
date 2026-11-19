package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// pluginsIndex is the index's state: url and key are where it is read and what checks it (index.DefaultURL and script.IndexKey; tests set their own), client the HTTP client, nil for Go's own (tests set the TLS server's). open is set while 浏览插件 shows. idx is the index read, kept or new, read once the copy kept was read (or found missing), meta what is known of it, and hay the words each of its entries is found by, lowercased, in the order of its entries (set); reading is set while the copy kept is read, busy while a newer one is asked for, err why the last read or ask failed. query and kind are 浏览插件's search and kind filter.
//
// updates is set while 浏览插件 lists only the packs with an update (pluginsIndexUpdatable), as 有可用更新 at the top of the page shows it. wanted is the id of a pack a link of Kando's own scheme asked to install (index.ReadInstallLink), waiting for the index to be read, and asked whether a newer index was asked for since, as a pack the cached index does not list may be in a newer one; linkMissing is the id of such a pack the index does not list, and linkBad set for a link naming no valid id, which 浏览插件 says until the next link or until it closes.
//
// An ask for the index and a download each run with a cancel func (fetchCancel, dlCancel) and a generation (fetchGen, dlGen) that cancelling raises, so what a cancelled one brings back is dropped: both are cancelled as Kando goes offline and as 浏览插件 closes (pluginsIndexStop), and a download also from its row's 取消. downloading is the id of the pack whose download, staging or check runs, "" while none does; it holds none of the plugins' busy flag while it downloads, so the rest of the page works meanwhile, and only the staging does (pluginsIndexFollow). done is a download that ended, its file in a folder of its own or why it failed, waiting for no other install or confirmation to be under way.
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
	fetchGen    int
	fetchCancel context.CancelFunc
	downloading string
	dlGen       int
	dlCancel    context.CancelFunc
	done        *pluginsIndexDownload
	updates     bool
	wanted      string
	asked       bool
	linkMissing string
	linkBad     bool
}

// pluginsIndexDownload is a download that ended: the entry, the folder its file is in and the file, or why it failed.
type pluginsIndexDownload struct {
	entry     index.Entry
	dir, file string
	err       error
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

// pluginsIndexOpen shows or hides 浏览插件. Shown the first time, it reads the copy of the index kept, then asks for a newer one when that is due (pluginsIndexRead); hidden, what it asks for or downloads stops (pluginsIndexStop), and the filter to updates, a link's pack waiting for the index and what was said of a link go.
func (a *app) pluginsIndexOpen(open bool) {
	p := &a.plugins.index
	p.open = open
	if !open {
		a.pluginsIndexStop()
		p.updates, p.wanted, p.linkMissing, p.linkBad = false, "", "", false
		return
	}
	if !a.plugins.index.read {
		a.pluginsIndexRead()
	}
}

// pluginsIndexRead reads the copy of the index kept in the plugins folder off the UI thread, and once it is read asks for a newer one when Kando is online and the one kept is RefreshEvery old or more, or there is none, or it cannot be read: a file gone, another key's, another address's. Why a copy that is there could not be read shows until a newer one comes.
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
		due := err != nil || src.Due(time.Now())
		update(func() {
			p.reading, p.read = false, true
			switch {
			case err == nil:
				p.set(idx, meta)
			case !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, index.ErrOtherAddress):
				p.err = err
			}
			if due && !a.privacy.offline {
				a.pluginsIndexRefresh()
			}
		})
	}()
}

// pluginsIndexRefresh asks for a newer index off the UI thread (index.Source.Refresh), unless one is being asked for or Kando is offline; what comes replaces the index shown, and a failure keeps it and says why. What comes after the ask was cancelled, or once Kando went offline, is dropped.
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
	ctx, cancel := context.WithCancel(context.Background())
	p.fetchGen++
	gen := p.fetchGen
	p.busy, p.err, p.fetchCancel = true, nil, cancel
	if p.wanted != "" {
		p.asked = true
	}
	update := a.update
	go func() {
		idx, meta, err := src.Refresh(ctx, time.Now())
		update(func() {
			cancel()
			if gen != p.fetchGen || a.privacy.offline {
				return
			}
			p.busy, p.err, p.fetchCancel = false, err, nil
			if err == nil {
				p.set(idx, meta)
				p.read = true
			}
		})
	}()
}

// pluginsIndexInstall downloads the pack id the index lists into a folder of its own, off the UI thread (index.Source.Download, which checks its size and SHA-256); once no other install or confirmation is under way, pluginsIndexFollow hands it to the install path a picked file takes. It holds none of the plugins' busy flag while it downloads, so turning packs on and off, removing and installing them goes on meanwhile; one download runs at a time, and none while Kando is offline.
func (a *app) pluginsIndexInstall(id string) {
	p := &a.plugins.index
	entry, ok := p.idx.Lookup(id)
	if a.pluginsRoot() == "" || !ok || p.downloading != "" || a.privacy.offline {
		return
	}
	src, err := a.pluginsIndexSource()
	if err != nil {
		p.err = err
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.dlGen++
	gen := p.dlGen
	p.downloading, p.dlCancel = id, cancel
	update := a.update
	go func() {
		d := &pluginsIndexDownload{entry: entry}
		d.dir, d.err = os.MkdirTemp("", "kando-plugin-download-")
		if d.err == nil {
			d.file, d.err = src.Download(ctx, entry, d.dir)
		}
		update(func() {
			cancel()
			if gen != p.dlGen || a.privacy.offline {
				pluginsIndexDiscard(d)
				return
			}
			p.dlCancel, p.done = nil, d
			a.pluginsIndexFollow()
		})
	}()
}

// pluginsIndexDiscard removes the folder of the download d, off the UI thread.
func pluginsIndexDiscard(d *pluginsIndexDownload) {
	if d != nil && d.dir != "" {
		go os.RemoveAll(d.dir)
	}
}

// pluginsIndexCancel stops the download, staging or check of the pack being installed from the index: its request is cancelled, a file it brought is removed, and what it brings back later is dropped.
func (a *app) pluginsIndexCancel() {
	p := &a.plugins.index
	if p.downloading == "" {
		return
	}
	if p.dlCancel != nil {
		p.dlCancel()
	}
	pluginsIndexDiscard(p.done)
	p.dlGen++
	p.downloading, p.dlCancel, p.done = "", nil, nil
}

// pluginsIndexStop cancels the ask for the index and the download under way, as Kando goes offline or 浏览插件 closes.
func (a *app) pluginsIndexStop() {
	p := &a.plugins.index
	if p.fetchCancel != nil {
		p.fetchCancel()
		p.fetchGen++
		p.busy, p.fetchCancel = false, nil
	}
	a.pluginsIndexCancel()
}

// pluginsIndexFollow brings the index up to the app's state after each change (pluginsHooks' afterChange): Kando gone offline stops what it asks for and downloads; a download that ended, once no other install or confirmation is under way, opens the install confirmation, through the path a picked file takes: plugins.Stage, then for a script plugin script.Verify, then pluginsAskStaged, which installs nothing until the user confirms. The pack read from the file must be the one the entry names, by id, kind and version, and a script plugin's signature must come from where the entry says (index.ErrNotListed). A failure, the download's included, shows in the confirmation's failure view, as a file that cannot be installed does. Only the staging holds the plugins' busy flag. It reports whether it changed what a binding reads.
func (a *app) pluginsIndexFollow() bool {
	p := &a.plugins.index
	if a.privacy.offline && (p.fetchCancel != nil || p.downloading != "") {
		a.pluginsIndexStop()
		return true
	}
	if a.pluginsIndexWanted() {
		return true
	}
	d := p.done
	root := a.pluginsRoot()
	if d == nil || a.plugins.busy || a.plugins.pending != nil || root == "" {
		return false
	}
	p.done = nil
	if d.err != nil {
		pluginsIndexDiscard(d)
		p.downloading = ""
		a.pluginsIndexAsk(plugins.Pack{}, "", script.Verified{}, d.err)
		return true
	}
	gen := p.dlGen
	a.plugins.busy, a.plugins.err = true, nil
	update, verify := a.update, a.pluginsStagedVerifier()
	go func() {
		staged, pack, v, err := pluginsIndexStage(d.entry, d.file, root, verify)
		os.RemoveAll(d.dir)
		update(func() {
			a.plugins.busy = false
			if gen != p.dlGen {
				if staged != "" {
					go os.RemoveAll(staged)
				}
				return
			}
			p.downloading = ""
			a.pluginsIndexAsk(pack, staged, v, err)
		})
	}()
	return true
}

// pluginsIndexAsk opens the install confirmation of a pack from the index, which says where it comes from, or says why it cannot be installed.
func (a *app) pluginsIndexAsk(pack plugins.Pack, staged string, v script.Verified, err error) {
	a.pluginsAskStaged(pack, staged, v, err)
	if a.plugins.pending != nil {
		a.plugins.pending.listed = true
	}
}

// pluginsIndexStage stages the downloaded file of entry in the plugins folder root (plugins.Stage, which runs plugins.Validate on the copy) and verifies a script plugin (verify), checking the pack is the one entry lists. Off the engine's goroutine.
func pluginsIndexStage(entry index.Entry, file, root string, verify func(plugins.Pack) (script.Verified, error)) (string, plugins.Pack, script.Verified, error) {
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

// pluginsIndexOpenLinks takes the links of Kando's own scheme that ask to install a pack of the index (index.ReadInstallLink), which the directory's page on Kando's site opens: Settings › 插件 opens with 浏览插件 searching for the pack, and once the index is read (pluginsIndexWanted) the pack's download starts and its install confirmation opens, as its 安装 or 更新 does. Any page can open such a link, so it only names the pack: nothing the signed index does not list is downloaded, and nothing is installed without the confirmation. Any other link is left alone.
func (a *app) pluginsIndexOpenLinks(links []string) {
	for _, link := range links {
		id, ok := index.ReadInstallLink(link)
		if !ok {
			continue
		}
		a.OpenSettings(settings.SectionPlugins)
		p := &a.plugins.index
		p.updates, p.kind, p.linkMissing, p.linkBad = false, "", "", id == ""
		p.wanted, p.asked = id, false
		if id != "" {
			p.query = id
		}
		a.pluginsIndexOpen(true)
		if _, listed := p.idx.Lookup(id); id != "" && p.read && !listed {
			a.pluginsIndexRefresh()
		}
		a.pluginsIndexWanted()
		a.remoteRaise()
	}
}

// pluginsIndexWanted takes the pack a link asked to install once the index is read, no newer one is being asked for and no other download runs: a pack the index does not list is asked for in a newer index once, then said to be missing (linkMissing); one it lists that can be installed or updated starts downloading (pluginsIndexInstall) while Kando is online; one installed already, or needing a newer Kando, shows in its row. It reports whether it changed what a binding reads.
func (a *app) pluginsIndexWanted() bool {
	p := &a.plugins.index
	if p.wanted == "" || !p.read || p.reading || p.busy || p.downloading != "" {
		return false
	}
	id := p.wanted
	if errors.Is(p.err, index.ErrNotConfigured) {
		p.wanted = ""
		return true
	}
	// Whether the pack is installed, and at which version, is known once the plugins folder was read.
	if !a.plugins.scanned {
		return false
	}
	e, ok := p.idx.Lookup(id)
	if !ok && !p.asked && !a.privacy.offline {
		a.pluginsIndexRefresh()
		if p.busy {
			return true
		}
	}
	p.wanted = ""
	if !ok {
		p.linkMissing = id
		return true
	}
	if state, _ := a.pluginsIndexState(e); (state == settings.PluginIndexAvailable || state == settings.PluginIndexUpdate) && !a.privacy.offline {
		a.pluginsIndexInstall(id)
	}
	return true
}

// pluginsIndexPeek reads the copy of the index kept in the plugins folder off the UI thread once Kando starts, without asking for a newer one, so 有可用更新 and the count beside 插件 in the settings' sidebar say which installed packs it lists newer before 浏览插件 is opened. It asks the network nothing: the index is asked for only as 浏览插件 opens, at most daily, or when the user asks. A copy that cannot be read is left for 浏览插件 to say why.
func (a *app) pluginsIndexPeek() {
	src, err := a.pluginsIndexSource()
	if err != nil {
		return
	}
	update := a.update
	go func() {
		idx, meta, err := src.Cached()
		if err != nil {
			return
		}
		update(func() {
			if p := &a.plugins.index; !p.read && !p.reading && !p.busy {
				p.set(idx, meta)
			}
		})
	}()
}

// pluginsIndexUpdatable reports whether the index lists the pack of e newer than the version installed, in a version that runs on this Kando: what 有可用更新 counts.
func (a *app) pluginsIndexUpdatable(e index.Entry) bool {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == e.ID })
	return i >= 0 && index.Newer(e.Version, a.plugins.packs[i].Version) && e.RunsOn(appVersion)
}

// pluginsIndexUpdates is how many installed packs the index lists newer (pluginsIndexUpdatable).
func (a *app) pluginsIndexUpdates() int {
	n := 0
	for _, e := range a.plugins.index.idx.Entries {
		if a.pluginsIndexUpdatable(e) {
			n++
		}
	}
	return n
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

// pluginsIndexFound are the entries of the index that match the search, the kind filter and, while it is on, the filter to updates, in the index's order (by id): each of the search's words is in the entry's id, publisher, or name or summary in any language, letter case aside.
func (a *app) pluginsIndexFound() []index.Entry {
	p := a.plugins.index
	words := strings.Fields(strings.ToLower(p.query))
	var out []index.Entry
	for i, e := range p.idx.Entries {
		if p.kind != "" && e.Kind != p.kind || p.updates && !a.pluginsIndexUpdatable(e) {
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
