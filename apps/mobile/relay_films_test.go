package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// Away from the computer's network, the Library lists the films it put in the user's storage: a film's page streams its video from the storage, keeping its lines on the phone to study, or downloads it from there by ranges, as from the computer; a film shared as its lines alone opens for its lines and says its video is on the computer. Back on the network, the computer lists its films itself again.
func TestAwayFromTheComputerItsFilmsComeFromTheStorage(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(synctest.Film{Library: "lib", Cues: talkCues, Film: sync.Film{ID: "f2", Title: "Evening Notes", Source: "youtube", DurationMS: 10_000, Updated: now()}})
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	d.ShareFilms(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	folder := sync.RelayPrefix(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 && len(bucket.Keys(folder+"outbox/")) > 0 })
	p.waitRelayJoined(id)
	p.app.WaitFor(talkRow(id), wait)

	d.SetOff(true)
	p.waitRelayed(id)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-stream", reconnect)
	if !p.app.Exists("id:film-download") || p.app.Exists("id:film-lines-only") {
		t.Fatalf("a film with its video in the storage: %q", p.app.Texts())
	}
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	if src := p.app.Find("id:player-video").Video.Source; !strings.Contains(src, folder+"films/") {
		t.Fatalf("the player plays %q, not the storage's video", src)
	}
	kept, err := store.Open(dir).Films()
	if err != nil || len(kept) != 1 || !kept[0].Streamed || kept[0].Done() || kept[0].LibraryID != "lib" {
		t.Fatalf("kept %+v, %v", kept, err)
	}
	if cues, ok, _ := store.Open(dir).Cues(id, "f1"); !ok || len(cues) != len(talkCues) {
		t.Fatalf("the lines kept to study: %v %+v", ok, cues)
	}
	p.app.Press("id:player-minimize")

	// Downloaded from the storage by ranges, the film is the phone's.
	p.app.WaitFor("id:film-download", wait)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	data, err := os.ReadFile(filepath.Join(dir, "films", id, "f1", "video.mp4"))
	if err != nil || !bytes.Equal(data, talkVideo) {
		t.Fatalf("the video kept is %d bytes, %v", len(data), err)
	}
	if !slices.ContainsFunc(bucket.Gets(), func(g string) bool { return strings.HasSuffix(g, "/video.mp4 bytes=0-") }) {
		t.Fatalf("the storage answered %q", bucket.Gets())
	}
	p.app.HeaderBack("id:film-page")

	// A film whose video is not in the storage opens for its lines.
	p.app.Press(filmRow(id, "f2"))
	p.app.WaitFor("id:film-lines", reconnect)
	if got := p.texts("id:film-lines-only"); got != msgs.FilmVideoAtHome("Studio") || p.app.Exists("id:film-download") {
		t.Fatalf("a film shared as its lines: %q", p.app.Texts())
	}
	p.app.Press("id:film-lines")
	p.app.WaitFor("id:player-no-video", wait)
	if p.app.Exists("id:player-video") {
		t.Fatal("the player opened a video the storage does not hold")
	}
	// Where the video would play, it says the video is on the computer, with nothing to press away from its network.
	if got := p.texts("id:player-no-video-status"); got != msgs.FilmVideoAtHome("Studio") || p.app.Exists("id:player-no-video-download") || p.app.Exists("id:player-no-video-ask") {
		t.Fatalf("the player says %q", p.app.Texts())
	}
	p.app.Press("id:player-minimize")

	// Back on the network, the film's page streams and downloads from the computer again.
	d.SetOff(false)
	p.app.WaitFor("id:film-download", reconnect)
	if !p.app.Exists("id:film-stream") || p.app.Exists("id:film-lines") {
		t.Fatalf("back home, the page still offers the storage: %q", p.app.Texts())
	}
}

// Back on its computer's network, a film kept to stream plays from the computer, and from the storage again should the player fail to open it from there.
func TestAFilmStreamedPlaysFromTheComputerAtHome(t *testing.T) {
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	d.ShareFilms(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	folder := sync.RelayPrefix(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 && len(bucket.Keys(folder+"outbox/")) > 0 })
	p.waitRelayJoined(id)
	p.app.WaitFor(talkRow(id), wait)
	d.SetOff(true)
	p.waitRelayed(id)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-stream", reconnect)
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	if src := p.playerSource(); !strings.Contains(src, folder+"films/") {
		t.Fatalf("away, the player plays %q", src)
	}

	d.SetOff(false)
	// The phone may try the storage once more as connections dropped while the computer was off are found closed: the computer's address holds once it answers on its network again.
	onNetwork := func() bool { return strings.HasPrefix(p.playerSource(), "http://127.0.0.1:") }
	p.app.WaitUntil(reconnect, onNetwork)
	time.Sleep(20 * relayEvery)
	p.app.WaitUntil(reconnect, onNetwork)
	if code, body := fetch(t, p.playerSource()); code != http.StatusOK || len(body) != len(talkVideo) {
		t.Fatalf("at home, the computer's address answers %d, %d bytes", code, len(body))
	}
	p.app.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: p.app.Find("id:player-video").ID, Text: "the system's player could not open it"})
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.playerSource(), folder+"films/") })
	if p.app.Exists("id:player-failed") {
		t.Fatal("the player says it cannot play a film the storage holds")
	}
	// Failing from the storage too, it cannot play.
	p.app.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: p.app.Find("id:player-video").ID, Text: "the system's player could not open it"})
	p.app.WaitFor("id:player-failed", wait)
}
