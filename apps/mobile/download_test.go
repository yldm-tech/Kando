package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync/synctest"
)

// openTalk pairs the phone with d and opens the talk's page from its library.
func (p *phone) openTalk(d *synctest.Desktop) string {
	p.t.Helper()
	p.pair(d)
	p.app.WaitFor("id:computers-library-lib", wait)
	p.app.Press("id:computers-library-lib")
	p.app.WaitFor("id:library-film-f1", wait)
	p.app.Press("id:library-film-f1")
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
	// The film now shows among the downloads, with the space it takes.
	p.app.HeaderBack("id:film-page")
	p.app.HeaderBack("id:library-page")
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:downloads-film-f1", wait)
	if got := p.texts("id:downloads-used"); !strings.Contains(got, "MB") && !strings.Contains(got, "KB") {
		t.Errorf("the downloads show %q, without the space they take", got)
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

// A downloaded film deleted from its page is gone from the phone.
func TestDeletingADownloadedFilm(t *testing.T) {
	p := startPhone(t, "")
	f := keepTalk(t, p.dir)
	p = startPhone(t, p.dir)
	p.app.WaitFor("id:downloads-film-f1", wait)
	p.app.Press("id:downloads-film-f1")
	p.app.WaitFor("id:film-delete", wait)
	if p.app.Exists("id:film-poster") {
		t.Fatal("a film with no poster shows an empty poster frame")
	}
	p.app.Press("id:film-delete")
	p.app.Answer(1)
	p.app.WaitForGone("id:downloads-film-f1", wait)
	p.waitFile(filepath.Join("films", f.Desktop, f.Film.ID), -1)
}
