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

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// openTalk pairs the phone with d and opens the talk's page from the Library.
func (p *phone) openTalk(d *synctest.Desktop) string {
	p.t.Helper()
	p.pair(d)
	p.app.Press(talkRow(store.DesktopID(d.Fingerprint)))
	p.app.WaitFor("id:film-download", wait)
	return filepath.Join("films", store.DesktopID(d.Fingerprint), "f1")
}

// A download paused partway keeps what it has, and resumed goes on from there by a range of the same file, ending whole.
func TestADownloadPausesAndResumes(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	dir := p.openTalk(d)
	d.Stall(300_000)
	p.app.Press("id:film-download")
	p.app.WaitFor("text:"+msgs.FilmDownloading(30), wait)
	// The bar takes a percentage, as its text does.
	if now := p.app.Find("id:film-progress").Props.Value.Now; now < 29 || now > 31 {
		t.Fatalf("the progress bar is at %v%%, its text at 30%%", now)
	}
	p.app.Press("id:film-pause")
	p.app.WaitFor("text:"+msgs.FilmPaused(30), wait)
	p.waitFile(filepath.Join(dir, store.PartFile), 300_000)

	d.Stall(0)
	p.app.Press("id:film-resume")
	p.app.WaitFor("id:film-play", wait)
	data, err := os.ReadFile(filepath.Join(p.dir, dir, "video.mp4"))
	if err != nil || !bytes.Equal(data, talkVideo) {
		t.Fatalf("the video kept is %d bytes, %v; want the desktop's %d", len(data), err, len(talkVideo))
	}
	r := d.Ranges()
	if len(r) != 2 || r[0] != "bytes=0-" || !strings.HasPrefix(r[1], "bytes=300000- \"") {
		t.Fatalf("ranges asked for %q", r)
	}
	if _, err := os.Stat(filepath.Join(p.dir, dir, "poster.jpg")); err != nil {
		t.Errorf("the poster was not kept: %v", err)
	}
	// The film's row now says it is on the phone, and the Library the space the films on it take.
	p.app.HeaderBack("id:film-page")
	p.openTab(filmsTab, msgs.FilmsTitle())
	row := talkRow(store.DesktopID(d.Fingerprint))
	p.app.WaitFor(row+" >> text:"+"YouTube · 0:10 · "+msgs.FilmDownloaded(), wait)
	if got := p.texts("id:films-used"); !strings.Contains(got, "MB") && !strings.Contains(got, "KB") {
		t.Errorf("the Library shows %q, without the space the downloads take", got)
	}
}

// A film its desktop serves from a NAS that keeps no ETag, named by its Last-Modified date alone, goes on from the part kept when its download is paused and resumed, the second request asking for the rest of the file against that date, and its progress goes on from the share it had rather than from 0.
func TestADownloadOfAFilmWithoutAnETagResumes(t *testing.T) {
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Poster: []byte("jpeg"), Cues: talkCues, NoETag: true,
		Film: sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now()}})
	d.Offer("123456")
	p := startPhone(t, "")
	dir := p.openTalk(d)
	d.Stall(300_000)
	p.app.Press("id:film-download")
	p.app.WaitFor("text:"+msgs.FilmDownloading(30), wait)
	p.app.Press("id:film-pause")
	p.app.WaitFor("text:"+msgs.FilmPaused(30), wait)
	p.waitFile(filepath.Join(dir, store.PartFile), 300_000)

	// Resumed, the next 500 KB take it from 30% to 80%; started over, they would take it to 50%.
	d.Stall(500_000)
	p.app.Press("id:film-resume")
	p.app.WaitFor("text:"+msgs.FilmDownloading(80), wait)
	p.app.Press("id:film-pause")
	p.app.WaitFor("text:"+msgs.FilmPaused(80), wait)
	p.waitFile(filepath.Join(dir, store.PartFile), 800_000)

	d.Stall(0)
	p.app.Press("id:film-resume")
	p.app.WaitFor("id:film-play", wait)
	data, err := os.ReadFile(filepath.Join(p.dir, dir, "video.mp4"))
	if err != nil || !bytes.Equal(data, talkVideo) {
		t.Fatalf("the video kept is %d bytes, %v; want the desktop's %d", len(data), err, len(talkVideo))
	}
	modified := time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)
	if r := d.Ranges(); !slices.Equal(r, []string{"bytes=0-", "bytes=300000- " + modified, "bytes=800000- " + modified}) {
		t.Fatalf("ranges asked for %q", r)
	}
}

// Cancelling a download under way deletes what it downloaded, and the film can be downloaded again.
func TestCancellingADownloadDeletesIt(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	dir := p.openTalk(d)
	d.Stall(200_000)
	p.app.Press("id:film-download")
	p.app.WaitFor("text:"+msgs.FilmDownloading(20), wait)
	p.app.Press("id:film-cancel")
	p.app.WaitFor("id:film-download", wait)
	p.waitFile(dir, -1)
}

// A film downloaded on its page and deleted from there is gone from the phone, and its page closes.
func TestDeletingADownloadedFilm(t *testing.T) {
	d := newDesktop(t)
	// A film without a poster, whose page shows no empty frame for one.
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Cues: talkCues, Film: sync.Film{ID: "f2", Title: "Evening Notes", Source: "youtube", DurationMS: 10_000, Updated: now()}})
	p := startPhone(t, "")
	p.pair(d)
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	p.app.Press(filmRow(id, "f2"))
	p.app.WaitFor("id:film-download", wait)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-delete", wait)
	if p.app.Exists("id:film-poster") {
		t.Fatal("a film with no poster shows an empty poster frame")
	}
	p.app.Press("id:film-delete")
	p.app.Answer(1)
	p.app.WaitForGone("id:film-page", wait)
	p.waitFile(filepath.Join("films", id, "f2"), -1)
	if p.app.Exists(filmRow(id, "f2") + " >> text:" + msgs.FilmDownloaded()) {
		t.Fatal("the deleted film's row still says it is on the phone")
	}
}

// A download is done once its last byte is kept, without waiting on the desktop to say which library the film is in, which the phone asks for after.
func TestADownloadEndsWithoutWaitingForItsLibrary(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.openTalk(d)
	release := d.Hold()
	defer release()
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	if films, _ := store.Open(p.dir).Films(); len(films) != 1 || films[0].LibraryID != "" {
		t.Fatalf("the film kept is %+v", films)
	}
	release()
	p.app.WaitUntil(wait, func() bool {
		films, _ := store.Open(p.dir).Films()
		return len(films) == 1 && films[0].LibraryID == "lib"
	})
}

// A film's page lists its facts in the Info group, and once the film is on the phone, the Offline group says so over the row that deletes it, with the space that frees.
func TestAFilmsPageGroupsItsFactsAndWhatThePhoneKeeps(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.openTalk(d)
	for id, want := range map[string]string{
		"source":    msgs.FilmInfoSource() + "\nYouTube",
		"languages": msgs.FilmInfoLanguages() + "\n日本語 → 简体中文",
		"length":    msgs.FilmInfoLength() + "\n0:10",
		"size":      msgs.FilmInfoSize() + "\n977 KB",
		"library":   msgs.FilmInfoLibrary() + "\nJapanese Class",
	} {
		if got := p.texts("id:film-info-" + id); got != want {
			t.Errorf("the Info group's %s row reads %q, want %q", id, got, want)
		}
	}
	if p.app.Exists("id:film-status") || p.app.Exists("id:film-delete") {
		t.Fatal("a film not on the phone shows the Offline group")
	}
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-delete", wait)
	if got := p.texts("id:film-status"); got != msgs.FilmDownloaded() {
		t.Errorf("the Offline group says %q", got)
	}
	if got, want := p.texts("id:film-delete"), msgs.FilmDelete()+"\n977 KB"; got != want {
		t.Errorf("the delete row reads %q, want %q", got, want)
	}
}

// A film's row plays a film whose video is on the phone, held where it was left as its page's Play does, without opening its page; a film not on the phone opens its page.
func TestAFilmsRowPlaysItOnceItIsOnThePhone(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.openTalk(d)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	p.app.HeaderBack("id:film-page")
	p.app.WaitForGone("id:film-page", wait)
	p.app.Press(talkRow(store.DesktopID(d.Fingerprint)))
	p.app.WaitFor("id:player-video", wait)
	if p.app.Exists("id:film-page") {
		t.Fatal("the row of a film on the phone opened its page under the player")
	}
}
