package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/sourcebrowser"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// pageMediaSources is the 媒体来源 page (package sourcebrowser), which the sidebar's row of that name shows while a source was added.
const pageMediaSources = sidebar.PageMediaSources

// mediaSourceBrowse is the state of the 媒体来源 page: the source shown (source) and the folder of it (dir, "" for its root); what it holds as listed, its folders and media files (entries) with the names of the videos a subtitle file goes with (subtitled), or why it could not be listed (err), and whether it is being listed (loading), gen numbering the listings so the answer of one the user left is dropped; the videos chosen to import, by path (chosen), and the folders chosen to import whole (chosenDirs), whose videos are being listed while expanding is set; the words the folder's rows are filtered by (query) and the row a choice with Shift extends from (anchor); what the page says about the last import (note); the import dialog (importing, the videos it imports, with their sizes, nil while it is closed, whose modal flag importRelease gives back), with the way it imports them, indexed and played from the source (importLink) or copied, and whether that way is remembered (importRemember); and the indexed film whose video is being looked for on the source (relink, its placeholder path, "" while none is, state_mediasource_import.go).
type mediaSourceBrowse struct {
	source         string
	dir            string
	entries        []mediasource.Entry
	subtitled      map[string]bool
	err            func(locales.Messages) string
	loading        bool
	gen            int
	chosen         map[string]int64
	chosenDirs     map[string]bool
	expanding      bool
	query          string
	anchor         string
	note           func(locales.Messages) string
	importing      map[string]int64
	importRelease  func()
	importLink     bool
	importRemember bool
	relink         string
	// shown is whether the page showed at the last change, so showing it again lists its folder anew (mediaSourceBrowseFollow).
	shown bool
}

// mediaSourceOpenPage shows the 媒体来源 page, which lists its folder as it shows (mediaSourceBrowseFollow).
func (a *app) mediaSourceOpenPage() { a.Select(pageMediaSources) }

// mediaSourceChoose shows the source id at its root.
func (a *app) mediaSourceChoose(id string) {
	if a.mediaSourceIndex(id) < 0 {
		return
	}
	b := &a.sources.browse
	b.source, b.dir, b.note = id, "", nil
	a.mediaSourceList()
}

// mediaSourceOpenDir shows the folder dir of the source shown.
func (a *app) mediaSourceOpenDir(dir string) {
	b := &a.sources.browse
	b.dir, b.note = dir, nil
	a.mediaSourceList()
}

// mediaSourcePoolSync gives the pool of connections the sources as the settings hold them, connecting with the secrets the secure store keeps (mediaSourceReadSecrets), which it reads off the engine's goroutine as the connection is made.
func (a *app) mediaSourcePoolSync() {
	m := &a.sources
	store, open := a.mediaSourceSecretStore(), m.open
	m.pool.Set(m.list, m.kinds.Kind, func(ctx context.Context, k mediasource.Kind, s mediasource.Source) (mediasource.Driver, error) {
		return open(ctx, k, s, mediaSourceReadSecrets(store, k, s, nil))
	})
}

// mediaSourceList lists the folder shown off the engine's goroutine, through the pool's connection to its source, and keeps what it holds, unless the user went elsewhere meanwhile. Offline mode lists a source on the local network alone. The videos chosen in another folder are let go, as the page shows one folder.
func (a *app) mediaSourceList() {
	m := &a.sources
	b := &m.browse
	b.gen++
	gen := b.gen
	b.entries, b.subtitled, b.err, b.chosen, b.chosenDirs, b.expanding, b.query, b.anchor = nil, nil, nil, nil, nil, false, "", ""
	i := a.mediaSourceIndex(b.source)
	if i < 0 {
		b.loading = false
		return
	}
	b.loading = true
	a.mediaSourcePoolSync()
	s, dir, pool, offline, local := m.list[i], b.dir, m.pool, a.privacy.offline, m.localHost
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mediasource.PoolOpenTimeout)
		defer cancel()
		var entries []mediasource.Entry
		err := mediaSourceOfflineCheck(ctx, offline, s, local)
		if err == nil {
			var d mediasource.Driver
			if d, err = pool.Driver(ctx, s.ID); err == nil {
				entries, err = d.List(ctx, dir)
			}
		}
		if err != nil && !errors.Is(err, media.ErrOffline) && !errors.Is(err, mediasource.ErrNotExist) {
			pool.Forget(s.ID)
		}
		a.update(func() { a.mediaSourceListed(gen, s, entries, err) })
	}()
}

// mediaSourceOfflineCheck is media.ErrOffline when offline mode is on and s is not on the local network (mediasource.Local, or local in its place when set), nil otherwise.
func mediaSourceOfflineCheck(ctx context.Context, offline bool, s mediasource.Source, local func(context.Context, string) (bool, error)) error {
	if !offline {
		return nil
	}
	if local == nil {
		local = mediasource.Local
	}
	if near, err := local(ctx, mediasource.Host(s)); err != nil || !near {
		return media.ErrOffline
	}
	return nil
}

// mediaSourceListed keeps what the listing gen found, unless another took its place: the folders, but hidden ones, and the media files Kando reads, whose subtitle files beside them it notes rather than lists; or why it failed, in the interface's language, the whole error going to the log without the secrets and addresses the app holds.
func (a *app) mediaSourceListed(gen int, s mediasource.Source, entries []mediasource.Entry, err error) {
	b := &a.sources.browse
	if b.gen != gen {
		return
	}
	b.loading = false
	if err != nil {
		log.Printf("media source: %s listing failed: %s", s.Kind, feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
		r, _ := mediasource.Remote(s, nil)
		k, ok := a.sources.kinds.Kind(s.Kind)
		b.err = func(m locales.Messages) string {
			if errors.Is(err, media.ErrOffline) {
				return m.MediaSourceOffline()
			}
			if ok && !k.Origin.Builtin() {
				return a.mediaSourcePluginFailure(m, k, err)
			}
			return mediaSourceFailure(m, r, err)
		}
		return
	}
	var names []string
	for _, e := range entries {
		if !e.Dir {
			names = append(names, e.Name)
		}
	}
	b.subtitled = map[string]bool{}
	var kept []mediasource.Entry
	for _, e := range entries {
		// A name Kando cannot write back the same way (mediasource.CleanPath), as one not in UTF-8, cannot be imported.
		if !mediasource.Importable(e) {
			continue
		}
		if !e.Dir && len(mediasource.SubtitlesOf(e.Name, names)) > 0 {
			b.subtitled[e.Name] = true
		}
		kept = append(kept, e)
	}
	slices.SortFunc(kept, func(x, y mediasource.Entry) int {
		if x.Dir != y.Dir {
			if x.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	b.entries = kept
}

// mediaSourceEntryPath is the path on the source of the entry named name in the folder shown.
func (b mediaSourceBrowse) entryPath(name string) string {
	if b.dir == "" {
		return name
	}
	return b.dir + "/" + name
}

// mediaSourceRef is the address of the video at p on the source shown.
func (a *app) mediaSourceRef(p string) string {
	return mediasource.Ref{Source: a.sources.browse.source, Path: p}.String()
}

// mediaSourceImported is the job of the open library that imported the video at the address ref, waiting, running or done; false for none, or one that failed or was stopped, which can be imported again. A video indexed without a job yet is mediaSourceIndexed.
func (a *app) mediaSourceImported(ref string) (core.Job, bool) {
	job, ok := a.queueFindSource(ref)
	if !ok || job.Stage == core.StageFailed {
		return core.Job{}, false
	}
	return job, true
}

// mediaSourceServer reports whether the source shown is a whole SMB server, whose root lists its shares.
func (a *app) mediaSourceServer() bool {
	i := a.mediaSourceIndex(a.sources.browse.source)
	if i < 0 {
		return false
	}
	s := a.sources.list[i]
	return s.Kind == mediasource.KindSMB && mediasource.SMBRemote(s, "").Share == ""
}

// mediaSourceToggle chooses the video at p to import, or the folder at p to import whole, or no more; a folder is not chosen while a film's video is looked for (relink).
func (a *app) mediaSourceToggle(p string) {
	b := &a.sources.browse
	name := path.Base(p)
	if b.expanding {
		return
	}
	for _, e := range b.entries {
		if e.Name != name || b.entryPath(e.Name) != p {
			continue
		}
		if e.Dir {
			if b.relink != "" {
				return
			}
			a.mediaSourceSetChosen(e, p, !b.chosenDirs[p])
			b.anchor, b.note = p, nil
			return
		}
		if a.sources.browse.relink != "" {
			a.mediaSourceRelinkTo(p, e)
			return
		}
		if a.mediaSourceIn(a.mediaSourceRef(p)) {
			return
		}
		_, on := b.chosen[p]
		a.mediaSourceSetChosen(e, p, !on)
		b.anchor, b.note = p, nil
		return
	}
}

// mediaSourceSetChosen chooses the folder or video e at p, or no more.
func (a *app) mediaSourceSetChosen(e mediasource.Entry, p string, on bool) {
	b := &a.sources.browse
	switch {
	case e.Dir && on:
		if b.chosenDirs == nil {
			b.chosenDirs = map[string]bool{}
		}
		b.chosenDirs[p] = true
	case e.Dir:
		delete(b.chosenDirs, p)
	case on:
		if b.chosen == nil {
			b.chosen = map[string]int64{}
		}
		b.chosen[p] = e.Size
	default:
		delete(b.chosen, p)
	}
}

// mediaSourceShown are the folder's entries the rows show: those whose name holds the words of the filter, each in any case, all of them without one.
func (a *app) mediaSourceShown() []mediasource.Entry {
	b := &a.sources.browse
	words := strings.Fields(strings.ToLower(b.query))
	if len(words) == 0 {
		return b.entries
	}
	var out []mediasource.Entry
	for _, e := range b.entries {
		name := strings.ToLower(e.Name)
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(name, w) }) {
			out = append(out, e)
		}
	}
	return out
}

// mediaSourceChoosable reports whether the entry e at p of the folder can be chosen: a folder, but not while a film's video is looked for, or a video not imported yet.
func (a *app) mediaSourceChoosable(e mediasource.Entry, p string) bool {
	if e.Dir {
		return a.sources.browse.relink == ""
	}
	return !a.mediaSourceIn(a.mediaSourceRef(p))
}

// mediaSourceExtend chooses, as Shift and a press do, the rows shown from the one chosen last (anchor) to the one at p, both included, taking the state p's press gives it: on when it was off. Without an anchor shown it chooses p alone (mediaSourceToggle).
func (a *app) mediaSourceExtend(p string) {
	b := &a.sources.browse
	shown := a.mediaSourceShown()
	from := slices.IndexFunc(shown, func(e mediasource.Entry) bool { return b.entryPath(e.Name) == b.anchor })
	to := slices.IndexFunc(shown, func(e mediasource.Entry) bool { return b.entryPath(e.Name) == p })
	if b.expanding || b.relink != "" || from < 0 || to < 0 {
		a.mediaSourceToggle(p)
		return
	}
	_, video := b.chosen[p]
	on := !video && !b.chosenDirs[p]
	lo, hi := min(from, to), max(from, to)
	for _, e := range shown[lo : hi+1] {
		q := b.entryPath(e.Name)
		if a.mediaSourceChoosable(e, q) {
			a.mediaSourceSetChosen(e, q, on)
		}
	}
	b.anchor, b.note = p, nil
}

// mediaSourceChoice is how many of the rows shown are chosen and how many can be: the folders and the videos not imported yet; none can be while a film's video is looked for, which picks one video.
func (a *app) mediaSourceChoice() (chosen, choosable int) {
	b := &a.sources.browse
	if b.relink != "" {
		return 0, 0
	}
	for _, e := range a.mediaSourceShown() {
		p := b.entryPath(e.Name)
		if !a.mediaSourceChoosable(e, p) {
			continue
		}
		choosable++
		if _, ok := b.chosen[p]; ok || b.chosenDirs[p] {
			chosen++
		}
	}
	return chosen, choosable
}

// mediaSourceChooseAll chooses every row shown that can be, or, when all of them are chosen, none of them, as 全选 does; on chooses them all whatever, as ⌘A does. Rows the filter hides keep their choice.
func (a *app) mediaSourceChooseAll(on bool) {
	b := &a.sources.browse
	chosen, choosable := a.mediaSourceChoice()
	if b.expanding || choosable == 0 {
		return
	}
	on = on || chosen < choosable
	for _, e := range a.mediaSourceShown() {
		if p := b.entryPath(e.Name); a.mediaSourceChoosable(e, p) {
			a.mediaSourceSetChosen(e, p, on)
		}
	}
	b.anchor, b.note = "", nil
}

// mediaSourceOpenEntry opens the folder at p, or the film imported from the video at p.
func (a *app) mediaSourceOpenEntry(p string) {
	b := &a.sources.browse
	for _, e := range b.entries {
		if b.entryPath(e.Name) != p {
			continue
		}
		if e.Dir {
			a.mediaSourceOpenDir(p)
			return
		}
		ref := a.mediaSourceRef(p)
		if placeholder, ok := a.mediaSourceIndexed(ref); ok {
			if item, found := films.Find(a.films(), placeholder); found {
				a.Open(item)
			}
			return
		}
		job, ok := a.mediaSourceImported(ref)
		if !ok {
			return
		}
		if item, found := films.Find(a.films(), job.ID); found {
			a.Open(item)
		} else {
			a.ShowJobs()
		}
		return
	}
}

// mediaSourceAskImport opens the dialog importing the videos chosen, holding the window's modal flag until it closes; with folders chosen, once their videos were listed (mediaSourceExpand).
func (a *app) mediaSourceAskImport() {
	b := &a.sources.browse
	if (len(b.chosen) == 0 && len(b.chosenDirs) == 0) || b.importing != nil || b.expanding {
		return
	}
	if len(b.chosenDirs) > 0 {
		a.mediaSourceExpand()
		return
	}
	a.mediaSourceOpenImport(maps.Clone(b.chosen))
}

// mediaSourceFolderTimeout bounds listing the folders chosen to import whole.
const mediaSourceFolderTimeout = 2 * time.Minute

// mediaSourceExpand lists the videos of the folders chosen off the engine's goroutine, through the pool's connection to the source shown, down to mediasource.FolderDepth folders and mediasource.FolderMax videos (docs/media-sources.md 4.1), then opens the import dialog on them and the videos chosen, but those imported already (mediaSourceExpanded). Offline mode lists a source on the local network alone.
func (a *app) mediaSourceExpand() {
	m := &a.sources
	b := &m.browse
	i := a.mediaSourceIndex(b.source)
	if i < 0 {
		return
	}
	b.expanding, b.note = true, nil
	a.mediaSourcePoolSync()
	gen, s, dirs, pool, offline, local := b.gen, m.list[i], slices.Sorted(maps.Keys(b.chosenDirs)), m.pool, a.privacy.offline, m.localHost
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mediaSourceFolderTimeout)
		defer cancel()
		var found map[string]int64
		var more bool
		err := mediaSourceOfflineCheck(ctx, offline, s, local)
		if err == nil {
			var d mediasource.Driver
			if d, err = pool.Driver(ctx, s.ID); err == nil {
				found, more, err = mediasource.Videos(ctx, d, dirs, mediasource.FolderDepth, mediasource.FolderMax)
			}
		}
		a.update(func() { a.mediaSourceExpanded(gen, s, found, more, err) })
	}()
}

// mediaSourceExpanded takes the videos the folders chosen hold, unless the page went to another folder meanwhile: with the videos chosen, but those imported already, it opens the import dialog, saying when there were more than mediasource.FolderMax; or it says that there were none, or that the folders could not be listed, the error going to the log.
func (a *app) mediaSourceExpanded(gen int, s mediasource.Source, found map[string]int64, more bool, err error) {
	b := &a.sources.browse
	if b.gen != gen || !b.expanding {
		return
	}
	b.expanding = false
	if err != nil {
		log.Printf("media source: %s listing folders to import failed: %s", s.Kind, feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
		b.note = locales.Messages.MediaSourceFoldersFailed
		return
	}
	videos := maps.Clone(b.chosen)
	if videos == nil {
		videos = map[string]int64{}
	}
	for p, size := range found {
		if !a.mediaSourceIn(a.mediaSourceRef(p)) {
			videos[p] = size
		}
	}
	if len(videos) == 0 {
		b.note = locales.Messages.MediaSourceFoldersEmpty
		return
	}
	if more {
		b.note = func(m locales.Messages) string { return m.MediaSourceFoldersTooMany(mediasource.FolderMax) }
	}
	a.mediaSourceOpenImport(videos)
}

// mediaSourceOpenImport opens the dialog importing videos, by path with their sizes, holding the window's modal flag until it closes.
func (a *app) mediaSourceOpenImport(videos map[string]int64) {
	b := &a.sources.browse
	b.importing = videos
	// The way remembered is chosen, else a copy (docs/media-sources.md 17.10).
	b.importLink, b.importRemember = a.sources.importLink && a.sources.rememberImport, a.sources.rememberImport
	if b.importRelease == nil {
		b.importRelease = a.holdModal()
	}
}

// mediaSourceCloseImport closes the import dialog and gives back its modal flag.
func (a *app) mediaSourceCloseImport() {
	b := &a.sources.browse
	b.importing = nil
	if b.importRelease != nil {
		b.importRelease()
		b.importRelease = nil
	}
}

// mediaSourceStartImport imports the videos of the import dialog in the order of their paths, the way it says: indexed and played from the source (mediaSourceLink), or each copied into the library by its job (media.SourceReader); keeps that way for the next import when asked to, or forgets one kept; closes the dialog, lets them go from the page's choice and says what was done: how many were queued, or why none could be, as the queue says it.
func (a *app) mediaSourceStartImport() {
	b := &a.sources.browse
	paths := slices.Sorted(maps.Keys(b.importing))
	link, remember := b.importLink, b.importRemember
	a.mediaSourceCloseImport()
	if len(paths) == 0 {
		return
	}
	if remember != a.sources.rememberImport || (remember && link != a.sources.importLink) {
		a.sources.rememberImport, a.sources.importLink = remember, link && remember
		if err := a.saveSettings(); err != nil {
			a.saveErr = a.messages().SettingsSourcesSaveFailed()
		}
	}
	a.mediaSourcePoolSync()
	if link {
		b.chosenDirs = nil
		a.mediaSourceLink(paths)
		return
	}
	refs := make([]string, len(paths))
	for i, p := range paths {
		refs[i] = a.mediaSourceRef(p)
	}
	added, err := a.queueEnqueue(refs)
	for _, p := range paths {
		delete(b.chosen, p)
	}
	b.chosenDirs = nil
	switch {
	case added > 0:
		b.note = func(m locales.Messages) string { return m.MediaSourceImported(added) }
	case err != nil:
		msg := err.Error()
		b.note = func(locales.Messages) string { return msg }
	}
}

// mediaSourceCopier is what copies a media source's video into the library for a job (media.Tools.Sources): through the pool's connection to its source, refusing a source on the internet while offline mode is on.
func (a *app) mediaSourceCopier() mediasource.Copier {
	return mediasource.Copier{Pool: a.sources.pool, Offline: a.privacy.now.Load, LocalHost: a.sources.localHost}
}

// mediaSourceBrowseFollow lists the folder anew as the 媒体来源 page comes to show, on the source shown last, else the first one; moves to the first source left once the one shown was removed; and closes the import dialog once the page went away. It reports whether it changed anything.
func (a *app) mediaSourceBrowseFollow() bool {
	b := &a.sources.browse
	on := a.page == pageMediaSources
	changed := false
	switch {
	case on && (!b.shown || (b.source != "" && a.mediaSourceIndex(b.source) < 0)):
		b.shown = true
		if a.mediaSourceIndex(b.source) < 0 {
			b.source, b.dir = "", ""
			if len(a.sources.list) > 0 {
				b.source = a.sources.list[0].ID
			}
		}
		a.mediaSourceList()
		changed = true
	case !on && b.shown:
		b.shown = false
	}
	if b.importing != nil && !on {
		a.mediaSourceCloseImport()
		changed = true
	}
	return changed
}

// mediaSourceOverlay is the import dialog over the window.
func (a *app) mediaSourceOverlay(*ns.Scope) ns.Node { return sourcebrowser.ImportDialog{App: a} }

// mediaSourceColumn is the 媒体来源 page, in the content column while the page shows (view.go).
func (a *app) mediaSourceColumn(*ns.Scope) ns.Node { return sourcebrowser.Page{App: a} }

// mediaSourceKeys: 浏览媒体来源 is a command of the File menu and the palette, with no key, while a source was added.
func (a *app) mediaSourceKeys(cx *ns.Scope) []shortcut {
	rows := []shortcut{{Group: groupFile, Label: locales.MediaSourceBrowse(cx), Run: a.mediaSourceOpenPage}}
	// Edit › 全选 stands in for the system's item while the page shows a folder, and ⌘A chooses all its rows shown, once no text field, as the filter, takes it.
	if a.page == pageMediaSources {
		rows = append(rows, shortcut{Group: groupEdit, Label: locales.MenuSelectAll(cx), Key: "mod+a", Role: nsdesktop.MenuSelectAll,
			When: func() bool { return a.page == pageMediaSources && a.sources.browse.importing == nil }, Run: func() { a.mediaSourceChooseAll(true) }})
	}
	return rows
}

// mediaSourceAddThenBrowse opens Settings › 媒体来源 with the dialog adding a source, which the page shows once it is saved (mediaSourceUI.thenBrowse).
func (a *app) mediaSourceAddThenBrowse() {
	a.OpenSettings(settings.SectionMediaSources)
	a.AddMediaSource()
	a.sources.thenBrowse = true
}

// mediaSourceBrowseFrom closes the settings and shows the source id on the 媒体来源 page, as its row's 浏览 in Settings › 媒体来源 and saving a source added from the page do.
func (a *app) mediaSourceBrowseFrom(id string) {
	if a.mediaSourceIndex(id) < 0 {
		return
	}
	a.closeSettings()
	if a.page == pageMediaSources && a.sources.browse.shown {
		a.mediaSourceChoose(id)
		return
	}
	// The page lists the source as it shows (mediaSourceBrowseFollow).
	a.Select(pageMediaSources)
	b := &a.sources.browse
	b.source, b.dir, b.note = id, "", nil
}

// mediaSourceEditShown opens Settings › 媒体来源 editing the source shown.
func (a *app) mediaSourceEditShown() {
	id := a.sources.browse.source
	a.OpenSettings(settings.SectionMediaSources)
	a.EditMediaSource(id)
}
