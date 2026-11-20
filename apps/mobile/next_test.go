package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
)

// keepNextTalk puts the talk on the phone with the evening talk after it, as its computer names it (sync.Film.Next), kept too.
func keepNextTalk(t *testing.T, dir string) (store.Film, store.Film) {
	t.Helper()
	first := keepTalk(t, dir)
	st := store.Open(dir)
	first.Film.Next = "f2"
	if err := st.SaveFilm(first); err != nil {
		t.Fatal(err)
	}
	next := store.Film{Desktop: first.Desktop, Library: first.Library, Video: "video.mp4", Added: now(),
		Film: sync.Film{ID: "f2", Title: "Evening Talk", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", VideoSize: int64(len(talkVideo))}}
	if err := st.SaveFilm(next); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(next.Desktop, next.Film.ID, talkCues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(next.Desktop, next.Film.ID), next.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	return first, next
}

// openNextTalk opens the talk with the evening talk after it in the player, as openPlayer does.
func openNextTalk(t *testing.T) (*phone, store.Film, store.Film) {
	t.Helper()
	dir := t.TempDir()
	first, next := keepNextTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow(first.Desktop), wait)
	p.app.Press(talkRow(first.Desktop))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-line") })
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	return p, first, next
}

// playing reports whether the player plays the film kept under id.
func (p *phone) playing(id string) bool {
	return strings.Contains(filepath.ToSlash(p.app.Find("id:player-video").Video.Source), "/"+id+"/")
}

// The film played to its end shows the next one the phone keeps on a card that counts down and then plays it in the same player, the sleep timer still running.
func TestTheNextFilmPlaysOnThePhone(t *testing.T) {
	p, _, _ := openNextTalk(t)
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-sleep-30")
	p.app.Back()
	p.at(10 * sec)
	p.app.WaitFor("id:player-up-next", wait)
	if got := p.texts("id:player-up-next-title"); got != "Evening Talk" {
		t.Fatalf("the card names %q", got)
	}
	if got := p.texts("id:player-up-next-in"); got != msgs.PlayerNextIn(8) {
		t.Fatalf("the card says %q", got)
	}
	p.app.Advance(8 * sec)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:player-video") && p.playing("f2") })
	if !p.app.Exists("id:player-pill-sleep") {
		t.Fatal("the sleep timer stopped as the next film played")
	}
}

// The sleep timer's End of Film holds the film at its end, and 取消 puts the card away.
func TestTheNextFilmWaitsForTheSleepTimerAndCancel(t *testing.T) {
	p, _, _ := openNextTalk(t)
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-sleep-film")
	p.app.Back()
	p.at(10 * sec)
	if p.app.Exists("id:player-up-next") {
		t.Fatal("End of Film went on to the next film")
	}
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: core.Size{Width: 1920, Height: 1080}})
	p.at(10 * sec)
	p.app.WaitFor("id:player-up-next", wait)
	p.app.Press("id:player-up-next-cancel")
	p.app.Advance(8 * sec)
	if p.app.Exists("id:player-up-next") || !p.playing("f1") {
		t.Fatal("取消 did not keep the film")
	}
}

// The phone keeps with each film it keeps what its computer names next, from every listing, so a film downloaded before its next was made goes on to it once that is listed and kept, away from any network too.
func TestThePhoneKeepsTheNextFilmFromItsComputer(t *testing.T) {
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Cues: talkCues,
		Film: sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now(), Next: "f2"}})
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Cues: talkCues,
		Film: sync.Film{ID: "f2", Title: "Evening Talk", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now()}})
	d.Offer("123456")
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(dir).Films()
		for _, f := range films {
			if f.Film.ID == "f1" {
				return f.Film.Next == "f2"
			}
		}
		return false
	})
}
