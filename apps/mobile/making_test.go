package main

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// playerSource is the address the player opens, "" while it shows no video.
func (p *phone) playerSource() string {
	if !p.app.Exists("id:player-video") {
		return ""
	}
	return p.app.Find("id:player-video").Video.Source
}

// fetch is what the player would read at link: its status and body.
func fetch(t *testing.T, link string) (int, string) {
	t.Helper()
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// A film the computer is still making is listed as being made, and plays from the computer on its network as it is made: nothing until its sound is in, then a link's sound and its lines as they come, then its video where it was, all while the phone tells the computer where it is watched, until the film is done and kept as any film streamed.
func TestAFilmBeingMadePlaysAsItIsMade(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(synctest.Film{Library: "lib", Film: sync.Film{ID: "m1", Title: "Night Walk", Source: "youtube", DurationMS: 10_000, Making: true, Updated: now()}})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	row := filmRow(id, "m1")
	p.app.WaitFor(row, wait)
	if got := p.texts(row); !strings.Contains(got, msgs.FilmMaking()) {
		t.Fatalf("the row of a film being made says %q", got)
	}

	p.app.Press(row)
	p.app.WaitFor("id:film-stream", wait)
	if p.app.Exists("id:film-download") || p.texts("id:film-making") != msgs.FilmMakingNote("Studio") {
		t.Fatalf("the page of a film being made shows %q", p.app.Texts())
	}
	p.app.Press("id:film-stream")
	// Nothing of it is in yet: the player says it plays once it is, and the computer hears it is watched, its first word a claim.
	p.app.WaitFor("id:player-making", wait)
	if got := p.texts("id:player-making"); got != msgs.PlayerMakingWait("Studio") {
		t.Fatalf("the player says %q", got)
	}
	p.app.WaitUntil(wait, func() bool { return len(d.Watches()) >= 2 })
	if w := d.Watches(); w[0].ID != "m1" || !w[0].Claim || w[1].Claim {
		t.Fatalf("the computer was told %+v", w)
	}
	kept, err := store.Open(dir).Films()
	if err != nil || len(kept) != 1 || !kept[0].Streamed || !kept[0].Film.Making || kept[0].LibraryID != "lib" {
		t.Fatalf("kept %+v, %v", kept, err)
	}

	// Its sound comes with its first lines: the player plays the sound from the phone's loopback server, and shows the lines.
	d.MakeMore("m1", talkCues[:2], []byte("the sound"), sync.PlaysSound)
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/sound.m4a") })
	sound := p.playerSource()
	if !strings.HasPrefix(sound, "http://127.0.0.1:") {
		t.Fatalf("the player plays %q", sound)
	}
	if code, body := fetch(t, sound); code != http.StatusOK || body != "the sound" {
		t.Fatalf("the sound's address answers %d %q", code, body)
	}
	p.app.WaitUntil(wait, func() bool { cues, _, _ := store.Open(dir).Cues(id, "m1"); return len(cues) == 2 })

	// Its video comes: the player opens it in the sound's place.
	d.MakeMore("m1", talkCues[:3], talkVideo, sync.PlaysVideo)
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/video.mp4") })
	if code, body := fetch(t, p.playerSource()); code != http.StatusOK || len(body) != len(talkVideo) {
		t.Fatalf("the video's address answers %d, %d bytes", code, len(body))
	}
	if code, _ := fetch(t, sound); code != http.StatusOK {
		t.Fatalf("the sound's address answers %d once the video came", code)
	}

	// Done, it is a film kept to stream as any: its lines are all there, and the phone stops telling the computer it is watched.
	d.FinishMaking("m1", talkCues)
	p.app.WaitUntil(wait, func() bool {
		f, ok := filmOf(dir, id, "m1")
		cues, _, _ := store.Open(dir).Cues(id, "m1")
		return ok && !f.Film.Making && len(cues) == len(talkCues)
	})
	told := len(d.Watches())
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/video.mp4") })
	if got := len(d.Watches()); got > told+1 {
		t.Fatalf("the computer was told %d more times once the film was done", got-told)
	}
	if slices.ContainsFunc(d.Watches(), func(w sync.FilmWatch) bool { return w.ID != "m1" }) {
		t.Fatalf("the computer was told of other films: %+v", d.Watches())
	}
}

// filmOf is the record the phone keeps of film of desktop.
func filmOf(dir, desktop, film string) (store.Film, bool) {
	kept, _ := store.Open(dir).Films()
	for _, f := range kept {
		if f.Desktop == desktop && f.Film.ID == film {
			return f, true
		}
	}
	return store.Film{}, false
}

// A computer from before films being made were listed to phones lists none, and a phone lists its films as before.
func TestAComputerFromBeforeListsNoFilmBeingMade(t *testing.T) {
	d := newDesktop(t)
	d.SetNoWatch(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	if p.app.Exists(filmRow(id, "m1")) || slices.ContainsFunc(p.app.Texts(), func(s string) bool { return strings.Contains(s, msgs.FilmMaking()) }) {
		t.Fatalf("a computer from before lists %q", p.app.Texts())
	}
}
