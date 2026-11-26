package main

import (
	"context"
	"log"
	"path"
	"slices"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
)

// Following a folder of a media source (docs/media-sources.md 4.4): the folder's row on the 媒体来源 page follows it, as a show's whose episodes come out one after another, and Kando imports by itself each video that shows up in it from then on, the way the user remembered, else the source's own (mediaSourceDefaultLink). The videos there as it is followed are left as they are, to be chosen as any other. settings.json keeps the folders followed (savedMediaSources.MediaSourceFollows), with the videos seen in each.

// mediaSourceFollowEvery is how long Kando waits between two looks at the folders followed; it looks once as the window comes up too.
const mediaSourceFollowEvery = 30 * time.Minute

// mediaSourceFollowing is the place of the follow of the folder dir of the source id in the list, -1 for none.
func (a *app) mediaSourceFollowing(id, dir string) int {
	return slices.IndexFunc(a.sources.follows, func(f mediasource.Follow) bool { return f.Source == id && f.Dir == dir })
}

// mediaSourceFollowKey names the folder dir of the source id among those being listed (mediaSourceUI.followBusy).
func mediaSourceFollowKey(id, dir string) string { return id + "\x00" + dir }

// mediaSourceToggleFollow follows the folder at p of the source shown, or no more: following it lists its videos first, off the engine's goroutine, so they are seen, and none of them is imported (mediaSourceFollowStarted). The page says which it did.
func (a *app) mediaSourceToggleFollow(p string) {
	m := &a.sources
	b := &m.browse
	name := path.Base(p)
	if i := a.mediaSourceFollowing(b.source, p); i >= 0 {
		m.follows = slices.Delete(m.follows, i, i+1)
		a.mediaSourceFollowSave()
		b.note = func(m locales.Messages) string { return m.MediaSourceUnfollowed(name) }
		return
	}
	i := a.mediaSourceIndex(b.source)
	key := mediaSourceFollowKey(b.source, p)
	if i < 0 || m.followBusy[key] {
		return
	}
	s, link := m.list[i], a.mediaSourceDefaultLink()
	a.mediaSourceFollowList(s, p, func(found map[string]int64, err error) {
		a.mediaSourceFollowStarted(mediasource.Follow{Source: s.ID, Dir: p, Link: link}, found, err)
	})
}

// mediaSourceFollowStarted keeps f, the folder just followed, with the videos found in it seen, or says on the page that it could not be listed and is not followed.
func (a *app) mediaSourceFollowStarted(f mediasource.Follow, found map[string]int64, err error) {
	b := &a.sources.browse
	name := path.Base(f.Dir)
	if err != nil {
		b.note = func(m locales.Messages) string { return m.MediaSourceFollowFailed(name) }
		return
	}
	if a.mediaSourceIndex(f.Source) < 0 || a.mediaSourceFollowing(f.Source, f.Dir) >= 0 {
		return
	}
	f.See(found)
	a.sources.follows = append(a.sources.follows, f)
	a.mediaSourceFollowSave()
	b.note = func(m locales.Messages) string {
		way := m.MediaSourceWayCopy()
		if f.Link {
			way = m.MediaSourceWayLink()
		}
		return m.MediaSourceFollowed(name, way)
	}
}

// mediaSourceFollowList lists the videos of the folder dir of s off the engine's goroutine, as importing it whole does (mediasource.Videos), through the pool's connection to s, refusing a source on the internet while offline mode is on, and gives them to done back on the engine's goroutine. One folder is listed once at a time; done does not run for a second ask meanwhile.
func (a *app) mediaSourceFollowList(s mediasource.Source, dir string, done func(found map[string]int64, err error)) {
	m := &a.sources
	key := mediaSourceFollowKey(s.ID, dir)
	if a.update == nil || m.followBusy[key] {
		return
	}
	if m.followBusy == nil {
		m.followBusy = map[string]bool{}
	}
	m.followBusy[key] = true
	a.mediaSourcePoolSync()
	pool, offline, local, update := m.pool, a.privacy.offline, m.localHost, a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mediaSourceFolderTimeout)
		defer cancel()
		var found map[string]int64
		err := mediaSourceOfflineCheck(ctx, offline, s, local)
		if err == nil {
			var d mediasource.Driver
			if d, err = pool.Driver(ctx, s.ID); err == nil {
				found, _, err = mediasource.Videos(ctx, d, []string{dir}, mediasource.FolderDepth, mediasource.FolderMax)
			}
		}
		update(func() {
			delete(m.followBusy, key)
			done(found, err)
		})
	}()
}

// mediaSourceFollowLook looks at each folder followed of a source in the settings (mediaSourceFollowFound), and looks again after mediaSourceFollowEvery.
func (a *app) mediaSourceFollowLook() {
	m := &a.sources
	if m.followStop != nil {
		m.followStop()
	}
	m.followStop = a.cx.AfterFunc(mediaSourceFollowEvery, live.Do(a, a.mediaSourceFollowLook))
	for _, f := range m.follows {
		i := a.mediaSourceIndex(f.Source)
		if i < 0 {
			continue
		}
		s, dir := m.list[i], f.Dir
		a.mediaSourceFollowList(s, dir, func(found map[string]int64, err error) { a.mediaSourceFollowFound(s, dir, found, err) })
	}
}

// mediaSourceFollowFound imports the videos found in the folder dir of s, followed, that it had not seen and the library does not hold, the follow's way, and notes them all as seen; the library's message bar says how many came. A folder that could not be listed, as a NAS asleep, goes to the log alone and is looked at again next time.
func (a *app) mediaSourceFollowFound(s mediasource.Source, dir string, found map[string]int64, err error) {
	i := a.mediaSourceFollowing(s.ID, dir)
	if i < 0 || a.mediaSourceIndex(s.ID) < 0 {
		return
	}
	if err != nil {
		log.Printf("media source: looking at a followed folder of a %s source failed: %s", s.Kind, feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
		return
	}
	f := &a.sources.follows[i]
	var fresh []string
	for _, p := range f.Fresh(found) {
		if !a.mediaSourceIn(mediasource.Ref{Source: s.ID, Path: p}.String()) {
			fresh = append(fresh, p)
		}
	}
	link := f.Link
	if f.See(found) {
		a.mediaSourceFollowSave()
	}
	if len(fresh) == 0 {
		return
	}
	name := path.Base(dir)
	if link {
		a.mediaSourcePoolSync()
		a.mediaSourceIndexFrom(s, fresh, func(n int, why string, err error) {
			if n > 0 {
				a.tell(a.messages().MediaSourceFollowNew(n, name))
			}
			if why != "" {
				a.warn(why, "")
			}
		})
		return
	}
	refs := make([]string, len(fresh))
	for i, p := range fresh {
		refs[i] = mediasource.Ref{Source: s.ID, Path: p}.String()
	}
	added, qerr := a.queueEnqueue(refs)
	switch {
	case added > 0:
		a.tell(a.messages().MediaSourceFollowNew(added, name))
	case qerr != nil:
		log.Printf("media source: queuing the new videos of a followed folder failed: %v", qerr)
	}
}

// mediaSourceFollowSave writes the folders followed to settings.json.
func (a *app) mediaSourceFollowSave() {
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSourcesSaveFailed()
	}
}
