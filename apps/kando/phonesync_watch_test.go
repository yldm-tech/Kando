package main

import (
	"context"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"
	psync "kando/packages/sync"
)

// The film watched is the one claimed last, on this computer or a phone; a phone's regular word keeps its claim without taking the film back, and its claim stops counting once it falls silent.
func TestTheFilmWatchedIsTheOneClaimedLast(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	a.queue.clock = func() time.Time { return now }
	watched := func() string {
		w, ok := a.queueWatched()
		if !ok {
			return ""
		}
		return w.film + "@" + w.phone
	}
	step := func(d time.Duration, fn func()) {
		now = now.Add(d)
		onEngine(a, fn)
	}

	step(0, func() { a.openKey = "open"; a.queueDeskSeen() })
	if got := watched(); got != "open@" {
		t.Fatalf("with a film open, %q is watched", got)
	}
	step(time.Second, func() { a.queueWatchedBy("phone", "making", time.Minute, true) })
	if got := watched(); got != "making@phone" {
		t.Fatalf("once a phone claims its film, %q is watched", got)
	}
	// This computer's player reports on as its film plays, the phone's word keeps it: neither claims.
	step(psync.WatchEvery, func() { a.queueDeskSeen(); a.queueWatchedBy("phone", "making", time.Minute+psync.WatchEvery, false) })
	if got := watched(); got != "making@phone" {
		t.Fatalf("after the players' reports, %q is watched", got)
	}
	// Another film opened here claims it back, and the phone's word does not take it again.
	step(time.Second, func() { a.openKey = "other"; a.queueDeskSeen() })
	step(psync.WatchEvery, func() { a.queueWatchedBy("phone", "making", time.Minute+2*psync.WatchEvery, false) })
	if got := watched(); got != "other@" {
		t.Fatalf("after a film opened here, %q is watched", got)
	}
	// A second phone opens another film: its first word is a claim, whatever it says.
	step(time.Second, func() { a.queueWatchedBy("tablet", "lesson", 0, false) })
	if got := watched(); got != "lesson@tablet" {
		t.Fatalf("once a second phone opens its film, %q is watched", got)
	}
	// Both phones fall silent: the film open here is watched again.
	step(psync.WatchLease+time.Second, func() { a.queueLease() })
	if got := watched(); got != "other@" || len(a.queue.viewers) != 0 {
		t.Fatalf("with the phones silent, %q is watched, viewers %+v", got, a.queue.viewers)
	}
	step(0, func() { a.openKey = ""; a.queueDeskSeen() })
	if got := watched(); got != "" {
		t.Fatalf("with nothing open, %q is watched", got)
	}
}

// A film a phone opens while it is made is listed to it alone among the films being made, is made first, the film being made giving way to it, and is listed as done under the same ID once it is; a done film made again stays listed as done meanwhile.
func TestAFilmAPhoneWatchesIsMadeFirst(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	p := &preferPipeline{makerPipeline: makerPipeline{length: 90 * time.Second, released: make(chan struct{})}}
	a.pipeline = p
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	clips := t.TempDir()
	first, second := filepath.Join(clips, "first", "first.mp4"), filepath.Join(clips, "second", "second.mp4")
	writeFakeClip(t, first, 90, "202020", "101010")
	writeFakeClip(t, second, 90, "202020", "101010")
	for _, v := range []string{first, second} {
		if err := a.Enqueue(core.FileURL(v)); err != nil {
			t.Fatal(err)
		}
	}
	refresh(tt, a)
	var firstID, secondID string
	for _, j := range a.library.Jobs {
		switch {
		case strings.HasSuffix(j.Source, "first.mp4"):
			firstID = j.ID
		case strings.HasSuffix(j.Source, "second.mp4"):
			secondID = j.ID
		}
	}
	queueUntilState(t, updates, func() bool { return stageOf(t, a, firstID).SubtitlePath != "" })
	lib := phoneLibraryID(dir)
	ids := func(list []psync.Film) []string {
		var out []string
		for _, f := range list {
			out = append(out, f.ID)
		}
		return out
	}
	done, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, lib) })
	if err != nil || slices.Contains(ids(done), firstID) || slices.Contains(ids(done), secondID) {
		t.Fatalf("the done films are %q, %v", ids(done), err)
	}
	making, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Making(ctx, lib) })
	if err != nil || !slices.Contains(ids(making), firstID) || !slices.Contains(ids(making), secondID) {
		t.Fatalf("the films being made are %q, %v", ids(making), err)
	}

	w, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.FilmWatching, error) {
		return c.WatchFilm(ctx, secondID, 30*time.Second, true)
	})
	// Waiting, nothing of it plays yet; claimed, it runs at once and its video is in.
	if err != nil || !w.Making || w.Plays != "" {
		t.Fatalf("watching the second answered %+v, %v", w, err)
	}
	queueUntilState(t, updates, func() bool { return stageOf(t, a, secondID).Stage == core.StageComplete })
	if job := stageOf(t, a, firstID); job.Stage.Done() {
		t.Fatalf("the first ended %s while the second was made", job.Stage)
	}
	if runs := p.runs(); !slices.Contains(runs, "second") || runs[0] != "first" {
		t.Fatalf("runs %v, want the first, then the second", runs)
	}
	w, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.FilmWatching, error) {
		return c.WatchFilm(ctx, secondID, 35*time.Second, false)
	})
	if err != nil || w.Making || w.Plays != psync.PlaysVideo {
		t.Fatalf("watching the second once done answered %+v, %v", w, err)
	}

	close(p.released)
	settle(t, a, tt, updates)
	// Made again, the second stays among the done films and is not one being made.
	apply(tt, a, func() { a.Retry(secondID) })
	if job := stageOf(t, a, secondID); job.Redo == "" || job.Stage.Done() {
		t.Fatalf("made again, the second is %+v", job)
	}
	done, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, lib) })
	if err != nil || !slices.Contains(ids(done), secondID) || !slices.Contains(ids(done), firstID) {
		t.Fatalf("while the second is made again the done films are %q, %v", ids(done), err)
	}
	making, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Making(ctx, lib) })
	if err != nil || len(making) != 0 {
		t.Fatalf("while the second is made again the films being made are %q, %v", ids(making), err)
	}
	if _, ok := films.Find(a.films(), secondID); !ok {
		t.Fatal("the second left the library")
	}
	settle(t, a, tt, updates)
}

// A link whose sound plays until its video comes is listed by where its video will be, as the player keeps its place: its sound is what its video's address serves, and its lines are those named after the video.
func TestALinkBeingMadePlaysItsSoundOnAPhone(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "talk")
	sound, subtitles := filepath.Join(folder, ".talk.sound.m4a"), filepath.Join(folder, "talk.bilingual.srt")
	writeFiles(t, map[string]string{sound: "the sound", subtitles: "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n"})
	apply(tt, a, func() {
		a.queue.paused = true
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: "talk", Source: "https://example.com/talk", Language: "简体中文", Stage: core.StageQueued, SoundPath: sound, SubtitlePath: subtitles})
	})
	lib := phoneLibraryID(dir)
	making, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Making(ctx, lib) })
	i := slices.IndexFunc(making, func(f psync.Film) bool { return f.ID == "talk" })
	if err != nil || i < 0 || making[i].VideoSize != int64(len("the sound")) {
		t.Fatalf("the films being made are %+v, %v", making, err)
	}
	got, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (string, error) {
		part, err := c.Video(ctx, "talk", 0, -1, "")
		if err != nil {
			return "", err
		}
		defer part.Body.Close()
		data, err := io.ReadAll(part.Body)
		return string(data), err
	})
	if err != nil || got != "the sound" {
		t.Fatalf("the link plays %q, %v", got, err)
	}
	cues, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Cue, error) { return c.Subtitles(ctx, "talk") })
	if err != nil || len(cues) != 1 || cues[0].Original != "Hello" || cues[0].Translation != "你好" {
		t.Fatalf("the link's lines are %+v, %v", cues, err)
	}
	w, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.FilmWatching, error) { return c.WatchFilm(ctx, "talk", 0, true) })
	if err != nil || !w.Making || w.Plays != psync.PlaysSound {
		t.Fatalf("watching the link answered %+v, %v", w, err)
	}
	// The queue is paused: watched, the link waits as it would opened here.
	if a.running() || stageOf(t, a, "talk").Stage != core.StageQueued {
		t.Fatalf("a paused queue ran the link watched: %+v", stageOf(t, a, "talk"))
	}
}
