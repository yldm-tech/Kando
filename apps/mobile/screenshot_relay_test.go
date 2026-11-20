package main

import (
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// TestScreenshotsFilmsAway renders a film's page away from the computer's network (screenshot_test.go): Morning Talk, whose video is in the storage, to stream or download from there (film-storage); and Evening Notes, shared as its lines alone, which opens for its lines and says its video is on the computer (film-lines-only), and its player (player-lines-only).
func TestScreenshotsFilmsAway(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.AddFilm(synctest.Film{Library: "lib", Cues: talkCues, Film: sync.Film{ID: "f2", Title: "Evening Notes", Source: "youtube", DurationMS: 10_000, Updated: now()}})
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	d.ShareFilms(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.waitRelayJoined(id)
	p.app.WaitFor(talkRow(id), wait)
	d.SetOff(true)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-stream", reconnect)
	shot(p, "film-storage")
	p.app.HeaderBack("id:film-page")
	p.app.Press(filmRow(id, "f2"))
	p.app.WaitFor("id:film-lines", reconnect)
	shot(p, "film-lines-only")
	p.app.Press("id:film-lines")
	p.app.WaitFor("id:player-no-video", wait)
	shot(p, "player-lines-only")
}
