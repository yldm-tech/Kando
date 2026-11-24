package main

import (
	"context"
	"log"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/shelf"
)

// Indexing a media source's videos (docs/media-sources.md 5.2, 6): rather than copied into the library, a video can be indexed, its film then playing from the source through the loopback server (playback_linked.go). library.json keeps where each indexed video is (shelf.Library.Linked) by a placeholder path in the library's folder no video is at (mediasource.LinkedPath), which its subtitles and the files beside it are named after, so the film is studied, searched and exported as any other. Kando never writes, moves or removes the file on the source: taking the film off the shelf forgets its record alone. A source removed keeps its films' records, and its address without its secrets (mediaSourceUI.removed), so adding it back plays them again.

// mediaSourceLink indexes the videos at paths of the source shown: off the engine's goroutine it looks each up on the source (refusing a source on the internet while offline mode is on), then each gets a record in library.json under its placeholder path, whose folder is made in the library, and the page says how many were indexed, or why none could be. A video indexed or imported already is left out.
func (a *app) mediaSourceLink(paths []string) {
	m := &a.sources
	b := &m.browse
	i := a.mediaSourceIndex(b.source)
	if i < 0 || a.update == nil {
		return
	}
	s, pool, offline, local := m.list[i], m.pool, a.privacy.offline, m.localHost
	for _, p := range paths {
		delete(b.chosen, p)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mediasource.PoolOpenTimeout)
		defer cancel()
		found := map[string]mediasource.Entry{}
		err := mediaSourceOfflineCheck(ctx, offline, s, local)
		if err == nil {
			var d mediasource.Driver
			if d, err = pool.Driver(ctx, s.ID); err == nil {
				for _, p := range paths {
					e, statErr := d.Stat(ctx, p)
					if statErr != nil {
						err = statErr
						continue
					}
					if !e.Dir {
						found[p] = e
					}
				}
			}
		}
		a.update(func() { a.mediaSourceLinked(s, paths, found, err) })
	}()
}

// mediaSourceLinked keeps a record for each video of s found on it (found, by path), in the order of paths, under a placeholder path of the open library, saves library.json and reads the library again, and says on the page how many were indexed, or why none were.
func (a *app) mediaSourceLinked(s mediasource.Source, paths []string, found map[string]mediasource.Entry, err error) {
	b := &a.sources.browse
	root := a.snap().LibraryDir
	n := 0
	now := time.Now().UTC()
	for _, p := range paths {
		e, ok := found[p]
		if !ok || root == "" || a.mediaSourceIn(mediasource.Ref{Source: s.ID, Path: p}.String()) {
			continue
		}
		placeholder, perr := mediasource.LinkedPath(root, s.Name, p, a.mediaSourceTaken)
		if perr != nil {
			err = perr
			continue
		}
		if merr := os.MkdirAll(filepath.Dir(placeholder), 0o755); merr != nil {
			err = merr
			continue
		}
		if a.library.Linked == nil {
			a.library.Linked = map[string]shelf.Linked{}
		}
		a.library.Linked[placeholder] = shelf.Linked{Source: s.ID, Path: p, Size: e.Size, Modified: e.Modified.UTC(), Version: e.Version, Added: now}
		n++
	}
	if n > 0 {
		if serr := a.library.Save(); serr != nil {
			a.saveErr = a.shelfSaveErr(serr, a.messages().LibraryShelfSaveFailed)
		}
		a.refreshLibrary()
		b.note = func(m locales.Messages) string { return m.MediaSourceIndexed(n) }
		return
	}
	if err != nil {
		log.Printf("media source: indexing from a %s source failed: %v", s.Kind, err)
		r := mediasource.SMBRemote(s, "")
		b.note = func(m locales.Messages) string { return mediaSourceFailure(m, r, err) }
	}
}

// mediaSourceTaken reports whether p is a path a film of the library already has, or is on disk, so an indexed video is not given it.
func (a *app) mediaSourceTaken(p string) bool {
	if _, ok := a.library.Linked[p]; ok {
		return true
	}
	if _, ok := a.library.Stored[p]; ok {
		return true
	}
	for _, job := range a.library.Jobs {
		if job.VideoPath == p {
			return true
		}
	}
	_, err := os.Lstat(p)
	return err == nil
}

// mediaSourceIndexed is the placeholder path of the open library's film indexed from the video at the address ref, false when none is.
func (a *app) mediaSourceIndexed(ref string) (string, bool) {
	r, err := mediasource.ParseRef(ref)
	if err != nil {
		return "", false
	}
	root := a.snap().LibraryDir
	for _, p := range slices.Sorted(maps.Keys(a.library.Linked)) {
		if l := a.library.Linked[p]; l.Source == r.Source && l.Path == r.Path && films.InDir(root, p) {
			return p, true
		}
	}
	return "", false
}

// mediaSourceIn reports whether the video at the address ref is in the open library already: indexed, or imported by a job that did not fail.
func (a *app) mediaSourceIn(ref string) bool {
	if _, ok := a.mediaSourceIndexed(ref); ok {
		return true
	}
	_, ok := a.mediaSourceImported(ref)
	return ok
}

// mediaSourceLinkedCount is how many films are indexed from the source id, those of every library, as library.json keeps them all.
func (a *app) mediaSourceLinkedCount(id string) int {
	n := 0
	for _, l := range a.library.Linked {
		if l.Source == id {
			n++
		}
	}
	return n
}

// mediaSourceRelinkID is the ID a source added at s's address takes: that of a source removed while films were indexed from it at the same address (mediasource.SameAddress), which then is no longer kept as removed, so those films play from it again; else a new one.
func (a *app) mediaSourceRelinkID(s mediasource.Source) string {
	m := &a.sources
	for i, old := range m.removed {
		if mediasource.SameAddress(old, s) && a.mediaSourceIndex(old.ID) < 0 {
			m.removed = slices.Delete(m.removed, i, i+1)
			return old.ID
		}
	}
	return mediasource.NewID()
}

// mediaSourceName is the name of the source id: one in the settings, else one removed, "" for neither.
func (a *app) mediaSourceName(id string) string {
	for _, s := range slices.Concat(a.sources.list, a.sources.removed) {
		if s.ID == id {
			return s.Name
		}
	}
	return ""
}

// linkedOf is where the video of the film at path, indexed from a media source, is: false for any other film.
func (a *app) linkedOf(path string) (shelf.Linked, bool) {
	if path == "" {
		return shelf.Linked{}, false
	}
	l, ok := a.library.Linked[path]
	return l, ok
}

// linkedHas reports whether job's video is on the media source it was indexed from, which the library's folder then holds its subtitles of alone, as a film whose video was moved to a storage location (storedHas).
func (a *app) linkedHas(path string) bool {
	_, ok := a.linkedOf(path)
	return ok
}

// awayPaths are the paths of the films whose videos are not in the library's folder though the films are: moved to a storage location (storedPaths) or indexed from a media source, in order.
func (a *app) awayPaths() []string {
	return slices.Sorted(slices.Values(slices.Concat(a.storedPaths(), slices.Collect(maps.Keys(a.library.Linked)))))
}

// linkedMark sets Linked on each of items indexed from a media source (films.Film.Linked), by linked, library.json's records.
func linkedMark(items []films.Film, linked map[string]shelf.Linked) []films.Film {
	if len(linked) == 0 {
		return items
	}
	for i := range items {
		if l, ok := linked[items[i].VideoPath]; ok && items[i].VideoPath != "" {
			items[i].Linked = l.Source
		}
	}
	return items
}

// LinkedMark is what the mark of a film indexed from a media source says (films.App): the source's name, or that it is no longer in the settings. It only reads the settings and never reaches the source.
func (a *app) LinkedMark(_ locales.Messages, item films.Film) (place string, gone bool) {
	i := a.mediaSourceIndex(item.Linked)
	if i < 0 {
		return "", true
	}
	return a.sources.list[i].Name, false
}

// linkedForget forgets where the video of item, a film taken off the shelf (libraryForget), is on its media source. Nothing on the source is touched.
func (a *app) linkedForget(item films.Film) {
	if !a.linkedHas(item.VideoPath) {
		return
	}
	delete(a.library.Linked, item.VideoPath)
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
}

// linkedLengths keeps the length library.json noted of each indexed video as how long it plays (app.lib.durations), as no file here tells it.
func (a *app) linkedLengths() {
	for p, l := range a.library.Linked {
		if l.Duration > 0 {
			a.libraryLearnDuration(p, l.Duration)
		}
	}
}

// mediaSourceFindVideo shows the 媒体来源 page at the folder the open film's video was in on its source, to choose where it is now (mediaSourceRelinkTo), as 在来源里找 does for a video no longer at its path.
func (a *app) mediaSourceFindVideo() {
	p := a.player.playPath
	l, ok := a.linkedOf(p)
	if !ok || a.mediaSourceIndex(l.Source) < 0 {
		return
	}
	b := &a.sources.browse
	b.relink, b.source = p, l.Source
	b.dir = path.Dir(l.Path)
	if b.dir == "." {
		b.dir = ""
	}
	name := a.libraryJobNameOf(p)
	b.note = func(m locales.Messages) string { return m.MediaSourceRelinking(name) }
	b.shown = true
	a.mediaSourceOpenPage()
	a.mediaSourceList()
}

// mediaSourceRelinkTo points the film being looked for (mediaSourceBrowse.relink) at the video at p of the source shown, e as the listing found it: its record takes the new path and what the source says of the file, so it plays from there with the lines it had.
func (a *app) mediaSourceRelinkTo(p string, e mediasource.Entry) {
	b := &a.sources.browse
	placeholder := b.relink
	b.relink = ""
	l, ok := a.linkedOf(placeholder)
	if !ok || l.Source != b.source {
		b.note = nil
		return
	}
	l.Path, l.Size, l.Modified, l.Version = p, e.Size, e.Modified.UTC(), e.Version
	a.library.Linked[placeholder] = l
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
	name := a.libraryJobNameOf(placeholder)
	b.note = func(m locales.Messages) string { return m.MediaSourceRelinked(name) }
}

// libraryJobNameOf is the name the film at path shows, as the shelf lists it.
func (a *app) libraryJobNameOf(path string) string {
	for _, item := range a.films() {
		if item.VideoPath == path {
			return films.Name(item)
		}
	}
	return filepath.Base(path)
}

// linkedKeepChanged takes the open film's file on its source as it is now, as 知道了 on the bar saying it changed does: its record notes the size and version the source gave, so the bar goes.
func (a *app) linkedKeepChanged() {
	c := a.player.linkedChanged
	a.player.linkedChanged = nil
	if c == nil {
		return
	}
	l, ok := a.linkedOf(c.path)
	if !ok {
		return
	}
	l.Size, l.Modified, l.Version = c.now.Size, c.now.Modified.UTC(), c.now.Version
	a.library.Linked[c.path] = l
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
}

// linkedChange is the open indexed film whose file on its source is not the one it was indexed from: its placeholder path and what the source says of the file now.
type linkedChange struct {
	path string
	now  mediasource.Entry
}

// linkedChangedFrom reports whether e, the file on the source as it is now, is not the one l was indexed from: another size, or another version where the source gives one.
func linkedChangedFrom(l shelf.Linked, e mediasource.Entry) bool {
	if e.Size != l.Size {
		return true
	}
	return l.Version != "" && e.Version != "" && e.Version != l.Version
}

// linkedQueue makes item's lines when it is a film indexed from a media source, which no job holds yet, and reports whether it is one: the job's ID, or why it could not be queued with the settings section that fixes it. Making the lines of an indexed film from its source is not wired up yet, so none is queued.
func (a *app) linkedQueue(item films.Film) (id, msg, section string, linked bool) {
	if !a.linkedHas(item.VideoPath) {
		return "", "", "", false
	}
	return "", "", "", true
}

// mediaSourceIDs are the IDs of the sources in the settings, taken on the engine's goroutine for Phone Sync.
func (a *app) mediaSourceIDs() map[string]bool {
	out := map[string]bool{}
	for _, s := range a.sources.list {
		out[s.ID] = true
	}
	return out
}

// mediaSourcePhoneHandler serves a phone the video of l, a film indexed from a source of shelf, through the sources' pool, as Phone Sync downloads it, once offline mode as it is then (shelf.offline) lets the source be read: a source off the local network is refused with 503 while offline mode is on (docs/media-sources.md 12.4), the source never reached.
func mediaSourcePhoneHandler(shelf phoneShelf, l shelf.Linked) http.Handler {
	inner := shelf.sourcePool.Handler(l.Source, l.Path)
	var s mediasource.Source
	for _, x := range shelf.sourceList {
		if x.ID == l.Source {
			s = x
		}
	}
	offline, local := shelf.offline, shelf.localHost
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := mediaSourceOfflineCheck(req.Context(), offline != nil && offline.Load(), s, local); err != nil {
			http.Error(w, "offline mode keeps the media source closed", http.StatusServiceUnavailable)
			return
		}
		inner.ServeHTTP(w, req)
	})
}
