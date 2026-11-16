package main

import (
	"strings"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// Once paired, the home page lists the desktop's libraries, a library its films with their languages, lengths and sizes, and pull to refresh reads them again.
func TestListsTheLibrariesAndTheirFilms(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.pair(d)
	desk := "id:home-desktop-" + store.DesktopID(d.Fingerprint)
	p.app.WaitFor("id:home-library-lib", wait)
	if got := p.texts(desk); !strings.Contains(got, "Studio") || !strings.Contains(got, "Japanese Class") || !strings.Contains(got, msgs.HomeFilmCount(1)) {
		t.Fatalf("the desktop's section shows %q", got)
	}
	p.app.Press("id:home-library-lib")
	p.app.WaitFor("id:library-film-f1", wait)
	if got := p.texts("id:library-film-f1"); !strings.Contains(got, "Morning Talk") || !strings.Contains(got, "日本語 → 简体中文 · 0:10 · 977 KB") {
		t.Fatalf("the film's row shows %q", got)
	}
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: sync.Film{ID: "f2", Title: "Evening Talk", Spoken: "ja"}})
	p.app.PullToRefresh("id:library-scroll")
	p.app.WaitFor("id:library-film-f2", wait)
}

// A desktop that does not answer says so under its name, and the films downloaded from it still show and open.
func TestOfflineTheDownloadsStillShow(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.pair(d)
	p.app.WaitUntil(wait, func() bool { kept, _ := store.Open(p.dir).Desktops(); return len(kept) == 1 })
	d.Close()
	dir := p.dir
	keepTalk(t, dir)
	p = startPhone(t, dir)
	p.app.WaitFor("id:home-film-f1", wait)
	p.app.WaitFor("id:home-desktop-problem", wait)
	if got, want := p.texts("id:home-desktop-problem"), msgs.HomeUnreachable("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's section says %q, want %q", got, want)
	}
	p.app.Press("id:home-film-f1")
	p.app.WaitFor("id:film-play", wait)
}

// A desktop that revoked the phone asks to pair again.
func TestARevokedPhoneIsAskedToPairAgain(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.pair(d)
	d.RevokeAll()
	p.app.PullToRefresh("id:home-scroll")
	p.app.WaitFor("id:home-pair-again", wait)
	if got, want := p.texts("id:home-desktop-problem"), msgs.HomeUnpaired("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's section says %q, want %q", got, want)
	}
}
