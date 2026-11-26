package main

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/packages/core"
)

// A folder's bell follows it: the videos there then are seen and left alone, each one that shows up later is imported by itself the way the source's imports go (a NAS's copied), once, and the library's message bar says how many came; settings.json keeps the follow, and the bell again stops it.
func TestMediaSourceFollowFolder(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	s, src := browseSource(a, map[string]fakeSourceFile{
		"Show/Season 1/E01.mkv": {data: "1"},
		"Show/Season 1/E02.mkv": {data: "2"},
		"Other/x.mkv":           {data: "x"},
	})
	refresh(tt, a)
	tt.Press("id:media-sources-source-" + s.ID)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	if !tt.Exists("id:media-sources-follow-Show") || tt.Exists("id:media-sources-follow-Show/Season 1/E01.mkv") {
		t.Fatalf("the bell is not on the folders alone: %q", tt.Texts())
	}
	tt.Press("id:media-sources-follow-Show")
	waitUntil(t, tt, a, updates, func() bool { return a.mediaSourceFollowing(s.ID, "Show") >= 0 })
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceFollowed("Show", msgs.MediaSourceWayCopy()))
	if f := a.sources.follows[0]; f.Link || !slices.Equal(f.Seen, []string{"Show/Season 1/E01.mkv", "Show/Season 1/E02.mkv"}) {
		t.Fatalf("follow %+v", f)
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("the videos there already were imported: %+v", a.library.Jobs)
	}
	if saved := a.loadSettings(); len(saved.MediaSourceFollows) != 1 || saved.MediaSourceFollows[0].Dir != "Show" {
		t.Fatalf("settings.json keeps %+v", saved.MediaSourceFollows)
	}

	src.mu.Lock()
	src.files["Show/Season 2/E01.mkv"] = fakeSourceFile{data: "new"}
	src.mu.Unlock()
	onEngine(a, a.mediaSourceFollowLook)
	fresh := mediasource.Ref{Source: s.ID, Path: "Show/Season 2/E01.mkv"}.String()
	waitUntil(t, tt, a, updates, func() bool { _, ok := a.queueFindSource(fresh); return ok })
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if n := a.lib.notice; !n.Open || n.Text != msgs.MediaSourceFollowNew(1, "Show") {
		t.Fatalf("notice %+v", n)
	}
	// Taken off the shelf once made, it is not imported again.
	waitUntil(t, tt, a, updates, func() bool {
		j, _ := a.queueFindSource(fresh)
		return j.Stage == core.StageComplete && a.RunningJob() == ""
	})
	onEngine(a, func() { a.library.Jobs = nil })
	onEngine(a, a.mediaSourceFollowLook)
	waitUntil(t, tt, a, updates, func() bool { return len(a.sources.followBusy) == 0 })
	if len(a.library.Jobs) != 0 {
		t.Fatalf("a video seen was imported again: %+v", a.library.Jobs)
	}

	tt.Press("id:media-sources-follow-Show")
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceUnfollowed("Show"))
	if len(a.sources.follows) != 0 || len(a.loadSettings().MediaSourceFollows) != 0 {
		t.Fatalf("still followed: %+v", a.sources.follows)
	}
}

// A folder followed with indexing remembered indexes its new videos, which play from the source; one that cannot be listed as it is followed is not followed, and a source unreachable later only waits for the next look.
func TestMediaSourceFollowIndexes(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	s, src := browseSource(a, map[string]fakeSourceFile{"Show/E01.mkv": {data: "1"}})
	a.sources.rememberImport, a.sources.importLink = true, true
	refresh(tt, a)
	tt.Press("id:media-sources-source-" + s.ID)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })

	src.mu.Lock()
	src.unreachable = errors.New("connection refused")
	src.mu.Unlock()
	onEngine(a, func() { a.sources.pool.Close(); a.sources.pool = &mediasource.Pool{} })
	tt.Press("id:media-sources-follow-Show")
	waitUntil(t, tt, a, updates, func() bool { return len(a.sources.followBusy) == 0 && a.sources.browse.note != nil })
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceFollowFailed("Show"))
	if len(a.sources.follows) != 0 {
		t.Fatalf("followed though it could not be listed: %+v", a.sources.follows)
	}

	src.mu.Lock()
	src.unreachable = nil
	src.mu.Unlock()
	tt.Press("id:media-sources-follow-Show")
	waitUntil(t, tt, a, updates, func() bool { return a.mediaSourceFollowing(s.ID, "Show") >= 0 })
	if !a.sources.follows[0].Link {
		t.Fatalf("follow %+v", a.sources.follows[0])
	}
	src.mu.Lock()
	src.files["Show/E02.mkv"] = fakeSourceFile{data: "2"}
	src.mu.Unlock()
	onEngine(a, a.mediaSourceFollowLook)
	indexed := mediasource.Ref{Source: s.ID, Path: "Show/E02.mkv"}.String()
	waitUntil(t, tt, a, updates, func() bool { _, ok := a.mediaSourceIndexed(indexed); return ok })
	if _, ok := a.mediaSourceIndexed(mediasource.Ref{Source: s.ID, Path: "Show/E01.mkv"}.String()); ok {
		t.Fatal("the video there already was indexed")
	}
}

// settings.json keeps a folder followed of a source in the settings once, at a plain path, and drops the rest.
func TestSavedMediaSourceFollowsClean(t *testing.T) {
	id := "0123456789abcdef"
	s := savedMediaSources{
		MediaSources: []savedMediaSource{{ID: id, Kind: "smb", Name: "NAS"}},
		MediaSourceFollows: []mediasource.Follow{
			{Source: id, Dir: "Show/", Seen: []string{"Show/E01.mkv"}},
			{Source: id, Dir: "Show"},
			{Source: "fedcba9876543210", Dir: "Gone"},
			{Source: id, Dir: "../up"},
			{Source: id, Dir: ""},
		},
	}
	s.clean()
	if len(s.MediaSourceFollows) != 1 || s.MediaSourceFollows[0].Dir != "Show" || len(s.MediaSourceFollows[0].Seen) != 1 {
		t.Fatalf("follows %+v", s.MediaSourceFollows)
	}
}
