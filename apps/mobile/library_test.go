package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// newDesktopWithMovies is newDesktop with a second library, Movies, of a film from Bilibili an hour older than the talk.
func newDesktopWithMovies(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := newDesktop(t)
	d.AddLibrary("movies", "Movies")
	d.AddFilm(synctest.Film{Library: "movies", Video: []byte("v"),
		Film: sync.Film{ID: "f2", Title: "Evening Talk", Source: "bilibili", DurationMS: 65_000, Spoken: "ja", Updated: now().Add(-time.Hour)}})
	return d
}

// alertLabels are the buttons of the alert shown last.
func (p *phone) alertLabels() []string {
	p.t.Helper()
	alerts := p.app.Alerts()
	if len(alerts) == 0 {
		p.t.Fatal("no alert shows")
	}
	var out []string
	for _, a := range alerts[len(alerts)-1].Actions {
		out = append(out, a.Label)
	}
	return out
}

// The Library lists the films of every library of the paired computer with those the phone keeps from another, the latest first, each with its source and length; pull to refresh reads them again.
func TestTheLibraryMergesTheComputersFilms(t *testing.T) {
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	keepTalk(t, dir)
	p := startPhone(t, dir, secrets)
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	if got := p.texts(talkRow(id)); !strings.Contains(got, "Morning Talk") || !strings.Contains(got, "YouTube · 0:10") {
		t.Fatalf("the talk's row shows %q", got)
	}
	if got := p.texts(filmRow(id, "f2")); !strings.Contains(got, "Bilibili · 1:05") {
		t.Fatalf("the Movies film's row shows %q", got)
	}
	if got := p.texts(talkRow("gonedesk")); !strings.Contains(got, "Other Sites · 0:10 · "+msgs.FilmDownloaded()) {
		t.Fatalf("the film kept from a computer gone shows %q", got)
	}
	if got := p.texts("id:films-count"); got != msgs.FilmsCount(3) {
		t.Fatalf("the list counts %q", got)
	}
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: sync.Film{ID: "f3", Title: "Night Talk", Spoken: "ja", Updated: now()}})
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor(filmRow(id, "f3"), wait)
}

// The chips narrow the list to a source, and the switcher to a library.
func TestChipsAndTheSwitcherNarrowTheList(t *testing.T) {
	d := newDesktopWithMovies(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	p.app.Press("id:films-source-bilibili")
	p.app.WaitForGone(talkRow(id), wait)
	p.app.Find(filmRow(id, "f2"))
	p.app.Press("id:films-source-local")
	p.app.WaitFor("id:films-empty", wait)
	if got := p.texts("id:films-empty"); got != msgs.FilmsNoSource() {
		t.Fatalf("a source with no films says %q", got)
	}
	p.app.Press("id:films-source-all")
	p.app.WaitFor(talkRow(id), wait)

	p.app.Press("id:films-library")
	want := []string{msgs.FilmsAllLibraries(), msgs.FilmsLibraryOn("Japanese Class", "Studio"), msgs.FilmsLibraryOn("Movies", "Studio"), msgs.CommonCancel()}
	if got := p.alertLabels(); !slices.Equal(got, want) {
		t.Fatalf("the switcher offers %q, want %q", got, want)
	}
	p.app.Answer(2)
	p.app.WaitForGone(talkRow(id), wait)
	if got := p.texts("id:films-library"); got != msgs.FilmsLibraryOn("Movies", "Studio") {
		t.Fatalf("the switcher says %q", got)
	}
	p.app.Press("id:films-library")
	p.app.Answer(0)
	p.app.WaitFor(talkRow(id), wait)
}

// A film left partway on the computer comes first to go on with, saying where; on the phone it plays from there, and one not on the phone opens its page to download it.
func TestContinueWatchingWhereTheComputerLeftOff(t *testing.T) {
	d := newDesktop(t)
	d.SetState("lib", now().Add(time.Hour), sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(4000)}}})
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	p := startPhone(t, dir, secrets)
	card := "id:films-continue-" + id + "-f1"
	p.app.WaitFor(card, wait)
	if got, want := p.texts("id:films-continue-meta-"+id+"-f1"), msgs.FilmsLeftOn("Studio", "0:04"); got != want {
		t.Fatalf("the card says %q, want %q", got, want)
	}
	if got := p.texts(card); !strings.Contains(got, msgs.FilmsLeftHere("Studio")) {
		t.Fatalf("the card's poster says %q, without the computer it was left on", got)
	}
	p.app.Press(card)
	p.app.WaitFor("id:film-download", wait)

	keepTalkFrom(t, dir, id)
	p = startPhone(t, dir, secrets)
	p.app.WaitFor(card, wait)
	p.app.Press(card)
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 4000 })
}

// A search finds the lines of the films on the phone, saying the line it found; pressed, the film plays from it. A search that finds nothing says so.
func TestSearchFindsTheLinesOnThePhone(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Type("id:films-search", "tide")
	p.app.WaitFor("id:films-empty", wait)
	if got, want := p.texts("id:films-empty"), msgs.FilmsNoResults("tide"); got != want {
		t.Fatalf("a search that finds nothing says %q, want %q", got, want)
	}
	if p.app.Exists("id:films-continue") {
		t.Fatal("the films to go on with show over the search's results")
	}
	p.app.Clear("id:films-search")
	p.app.Type("id:films-search", "HOW ARE")
	hit := "id:films-hit-gonedesk-f1"
	p.app.WaitFor(hit, wait)
	if got, want := p.texts(hit), msgs.FilmsHit("0:03", "How are you?"); got != want {
		t.Fatalf("the hit says %q, want %q", got, want)
	}
	if got := p.texts("id:films-count"); got != msgs.FilmsCount(1) {
		t.Fatalf("the results count %q", got)
	}
	p.app.Press(talkRow("gonedesk"))
	p.app.WaitFor("id:player-video", wait)
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 3000 })
}

// A film's options download it and, once it is on the phone, play it or delete it from the phone after asking.
func TestAFilmsOptionsDownloadAndDeleteIt(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	options := "id:films-options-" + id + "-f1"
	p.app.WaitFor(options, wait)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmsDownloadSize("977 KB"), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film not on the phone are %q, want %q", got, want)
	}
	if !p.app.Alerts()[0].Sheet {
		t.Fatal("a phone shows a film's options in something other than an action sheet")
	}
	p.app.Answer(0)
	folder := filepath.Join("films", id, "f1")
	p.waitFile(filepath.Join(folder, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor(talkRow(id)+" >> text:YouTube · 0:10 · "+msgs.FilmDownloaded(), wait)
	p.app.Press(options)
	if got, want := p.alertLabels(), []string{msgs.FilmPlay(), msgs.FilmDelete(), msgs.FilmsDetails(), msgs.CommonCancel()}; !slices.Equal(got, want) {
		t.Fatalf("the options of a film on the phone are %q, want %q", got, want)
	}
	p.app.Answer(1)
	if got := p.app.Alerts(); len(got) != 1 || got[0].Title != msgs.FilmDeleteTitle("Morning Talk") {
		t.Fatalf("deleting from the options asks %+v", got)
	}
	p.app.Answer(1)
	p.waitFile(folder, -1)
}

// A desktop that does not answer says so, and the films downloaded from it still show and open.
func TestOfflineTheDownloadsStillShow(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.pair(d)
	p.app.WaitUntil(wait, func() bool { kept, _ := store.Open(p.dir).Desktops(); return len(kept) == 1 })
	d.Close()
	dir := p.dir
	keepTalk(t, dir)
	p = startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	problem := "id:computers-problem-" + store.DesktopID(d.Fingerprint)
	p.app.WaitFor(problem, wait)
	if got, want := p.texts(problem), msgs.ComputersUnreachable("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's card says %q, want %q", got, want)
	}
	// A computer that does not answer offers to try again, not to pair again.
	id := store.DesktopID(d.Fingerprint)
	p.app.Find("id:computers-retry-" + id)
	if p.app.Exists("id:computers-pair-again-" + id) {
		t.Fatal("an unreachable desktop offers to pair again")
	}
	p.app.Press(talkRow("gonedesk"))
	p.app.WaitFor("id:film-play", wait)
}

// A desktop that revoked the phone asks to pair again, and its films go from the list.
func TestARevokedPhoneIsAskedToPairAgain(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.pair(d)
	d.RevokeAll()
	id := store.DesktopID(d.Fingerprint)
	p.app.PullToRefresh("id:films-scroll")
	p.app.WaitFor("id:computers-pair-again-"+id, wait)
	if got, want := p.texts("id:computers-problem-"+id), msgs.ComputersUnpaired("Studio"); !strings.Contains(got, want) {
		t.Fatalf("the desktop's card says %q, want %q", got, want)
	}
	if p.app.Exists(talkRow(id)) || p.app.Exists("id:computers-retry-"+id) {
		t.Fatal("beside the request to pair again the Library shows the desktop's films or a retry")
	}
}
