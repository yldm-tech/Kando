package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/shelf"
	psync "kando/packages/sync"
)

// phoneWorksLibrary is phoneSyncApp's library with two shows no job processed: River Walk's three episodes, the second moved to a storage location, and Harbor's two.
func phoneWorksLibrary(t *testing.T, a *app, dir string) (river []string) {
	t.Helper()
	for _, name := range []string{"River Walk S01E01.mp4", "River Walk S01E02.mp4", "River Walk S01E03.mp4"} {
		river = append(river, filepath.Join(dir, "River Walk", name))
	}
	remoteWriteFile(t, river[0], "video")
	remoteWriteFile(t, river[2], "video")
	a.library.Stored = map[string]shelf.Stored{river[1]: {Location: "nas", Path: "River Walk S01E02.mp4", Size: 5, Moved: time.Now()}}
	for _, name := range []string{"Harbor S01E01.mp4", "Harbor S01E02.mp4"} {
		remoteWriteFile(t, filepath.Join(dir, "Harbor", name), "video")
	}
	a.refreshLibrary()
	a.categorySync()
	return river
}

// phoneListed is the films the phones are told of the open library.
func phoneListed(t *testing.T, a *app, tt phoneTT) []phonesync.Film {
	t.Helper()
	list, err := phoneAsk(t, tt, func(ctx context.Context) ([]phonesync.Film, error) {
		libs, err := phoneSource{a}.Libraries(ctx)
		if err != nil || len(libs) == 0 {
			return nil, fmt.Errorf("libraries %v, %v", libs, err)
		}
		return phoneSource{a}.Films(ctx, libs[0])
	})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// phoneWorkOf is the work the library page folds the film of video into.
func phoneWorkOf(a *app, video string) (*films.Work, bool) {
	for _, item := range a.films() {
		if item.VideoPath == video {
			return a.libraryWorks().Of(item.Key)
		}
	}
	return nil, false
}

// phoneWorkGroups is the films of each work, by their protocol IDs, each work's sorted, the works in order.
func phoneWorkGroups(groups map[string][]string) [][]string {
	var out [][]string
	for _, g := range groups {
		out = append(out, slices.Sorted(slices.Values(g)))
	}
	slices.SortFunc(out, func(a, b []string) int { return strings.Compare(strings.Join(a, ","), strings.Join(b, ",")) })
	return out
}

// The phones are told the works the library page folds, cut alike from the same films, a video moved to a storage location among them, in the order each work plays; the same name as the control socket gives each; and a work the user split apart is none.
func TestPhonesAreToldTheWorksTheLibraryFolds(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	p := phoneTT{a: a, tt: tt, updates: updates}
	river := phoneWorksLibrary(t, a, dir)
	refresh(tt, a)

	desk := map[string][]string{}
	all := a.films()
	for _, item := range all {
		if w, ok := a.libraryWorks().Of(item.Key); ok {
			desk[w.Key] = append(desk[w.Key], phoneFilmID(item))
		}
	}
	if len(desk) != 2 {
		t.Fatalf("the library folds %v, want River Walk and Harbor", desk)
	}
	list := phoneListed(t, a, p)
	phone := map[string][]string{}
	for _, f := range list {
		if f.Work != "" {
			phone[f.Work] = append(phone[f.Work], f.ID)
		}
	}
	if got, want := phoneWorkGroups(phone), phoneWorkGroups(desk); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("the phones fold %v, the library %v", got, want)
	}
	second := slices.IndexFunc(all, func(f films.Film) bool { return f.VideoPath == river[1] })
	i := slices.IndexFunc(list, func(f phonesync.Film) bool { return second >= 0 && f.ID == phoneFilmID(all[second]) })
	if i < 0 {
		t.Fatal("the moved episode is not told")
	}
	if f, socket := list[i], a.remoteFilmOf(all[second]).Work; f.WorkTitle != "River Walk" || f.WorkIndex != 2 || f.Work != socket {
		t.Errorf("the moved episode is told as %q %q #%d, the socket names %q", f.Work, f.WorkTitle, f.WorkIndex, socket)
	}
	w, _ := phoneWorkOf(a, river[1])
	apply(tt, a, func() { a.SetWorkApart(w, true) })

	// Harbor stays a work; River Walk's episodes are told as films of their own, while how far each was watched is told still.
	for _, f := range phoneListed(t, a, p) {
		if strings.HasPrefix(f.Title, "River Walk") && (f.Work != "" || f.WorkTitle != "" || f.WorkIndex != 0) {
			t.Errorf("%s of a work split apart is told as of %q", f.Title, f.WorkTitle)
		}
		if strings.HasPrefix(f.Title, "Harbor") && f.WorkTitle != "Harbor" {
			t.Errorf("%s is told as of %q", f.Title, f.WorkTitle)
		}
	}
}

// The phones are told when each film last played, whether or not it was watched to its end, as a phone goes on with a work at the episode played last.
func TestPhonesAreToldWhenAFilmLastPlayed(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	talk := filepath.Join(dir, "Rust Talk", "talk.mp4")
	played := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	a.lib.durations = map[string]time.Duration{talk: time.Minute}
	a.palette.furthest = map[string]time.Duration{talk: 20 * time.Second}
	a.player.played = map[string]time.Time{talk: played}
	for _, f := range phoneListed(t, a, phoneTT{a: a, tt: tt, updates: updates}) {
		if want := f.Video == talk; f.Watched || f.PlayedAt.Equal(played) != want {
			t.Errorf("%s watched %v, played %v", f.Title, f.Watched, f.PlayedAt)
		}
	}
}

// The same list in two libraries is two works, whose names differ, as a phone lists every library of a computer together.
func TestAListInTwoLibrariesIsTwoWorks(t *testing.T) {
	w := &films.Work{Key: "list:youtube.com/playlist?list=PLriver"}
	first, second := filepath.Join(t.TempDir(), "一.kando"), filepath.Join(t.TempDir(), "二.kando")
	if w.ID(first) == w.ID(second) || w.ID(first) != w.ID(first+string(filepath.Separator)) || len(w.ID(first)) != 12 {
		t.Fatalf("ids %q %q", w.ID(first), w.ID(second))
	}
}

// A phone waiting for events hears that the films are to be listed again once the user splits a work apart, or a film is watched to its end, and not for a film played on short of it.
func TestPhonesHearWhenTheLibrarysWorksChange(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	p := phoneTT{a: a, tt: tt, updates: updates}
	river := phoneWorksLibrary(t, a, dir)
	refresh(tt, a)
	lib := phoneLibraryID(dir)
	heard := func(what string, change func()) bool {
		t.Helper()
		list, err := phoneAsk(t, p, c.Jobs)
		if err != nil {
			t.Fatal(err)
		}
		apply(tt, a, change)
		ev, err := phoneAsk(t, p, func(ctx context.Context) (psync.Events, error) {
			return c.Events(ctx, list.Epoch, list.Rev, 200*time.Millisecond)
		})
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return slices.ContainsFunc(ev.Events, func(e psync.Event) bool { return e.Kind == psync.EventFilms && e.ID == lib })
	}
	w, ok := phoneWorkOf(a, river[0])
	if !ok {
		t.Fatal("River Walk is no work")
	}
	if !heard("split apart", func() { a.SetWorkApart(w, true) }) {
		t.Error("splitting a work apart was not heard")
	}
	a.lib.durations = map[string]time.Duration{river[0]: time.Minute}
	if heard("played on", func() { a.palette.furthest = map[string]time.Duration{river[0]: 20 * time.Second} }) {
		t.Error("a film played on short of its end was heard")
	}
	if !heard("watched", func() { a.palette.furthest = map[string]time.Duration{river[0]: time.Minute} }) {
		t.Error("a film watched to its end was not heard")
	}
	// Starting the server with a film already watched does not emit a spurious films event on startup or the next change.
	apply(tt, a, func() { a.phoneSyncStop() })
	apply(tt, a, func() { a.phoneSyncStart() })
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil })
	list, err := phoneAsk(t, p, c.Jobs)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := phoneAsk(t, p, func(ctx context.Context) (psync.Events, error) {
		return c.Events(ctx, list.Epoch, 0, 50*time.Millisecond)
	})
	if err != nil {
		t.Fatalf("after restart: %v", err)
	}
	if slices.ContainsFunc(ev.Events, func(e psync.Event) bool { return e.Kind == psync.EventFilms && e.ID == lib }) {
		t.Error("restarting Phone Sync with an already-watched film emitted a spurious films event")
	}
}
