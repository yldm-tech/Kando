package main

import (
	"net/http"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
)

// libraryScene starts the phone paired with a computer of two libraries running a job, as the Library's scenes show it, and waits for its list; it returns the computer's ID.
func libraryScene(t *testing.T) (*phone, string) {
	t.Helper()
	d := newDesktopWithMovies(t)
	running := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 2, 0.84)
	running.Title, running.Device = "Morning Market in Kyoto", ""
	d.SetJob(running)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(filmRow(id, "f2"), wait)
	p.app.WaitFor("id:films-jobs-card", wait)
	return p, id
}

// The Library's sheets and states the design draws (screenshot_test.go), a test each, as a sheet closed on a computer's target stays in its tree: the library switcher's sheet (films-library-sheet), a film's options (films-options-sheet), a source with no film (films-filter-empty), and the page in the dark (films-dark).

func TestScreenshotsLibrarySwitch(t *testing.T) {
	shot := screenshots(t)
	p, _ := libraryScene(t)
	p.app.Press("id:films-library")
	p.app.WaitFor("id:films-library-option-2", wait)
	shot(p, "films-library-sheet")
}

func TestScreenshotsLibraryOptions(t *testing.T) {
	shot := screenshots(t)
	p, id := libraryScene(t)
	p.app.Press("id:films-options-" + id + "-f1")
	p.app.WaitFor("id:films-options-title", wait)
	// The sheet slides up over 300ms.
	p.app.Advance(500 * time.Millisecond)
	shot(p, "films-options-sheet")
}

// The options of a computer's film streamed once, which keeps its lines on the phone but no video (films-options-streamed).
func TestScreenshotsLibraryStreamedOptions(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetNoFilmOps(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	options := "id:films-options-" + store.DesktopID(d.Fingerprint) + "-f1"
	p.app.WaitFor(options, wait)
	p.app.Press(options)
	p.choose(msgs.FilmPlay())
	p.app.WaitFor("id:player-video", wait)
	p.app.Press("id:player-minimize")
	p.app.Press(options)
	p.app.WaitFor("id:films-options-title", wait)
	p.app.Advance(500 * time.Millisecond)
	shot(p, "films-options-streamed")
}

// The options of a film kept by its lines whose computer has its video now (films-options-lines).
func TestScreenshotsLibraryLinesOptions(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetNoFilmOps(true)
	lines := linesFilm()
	lines.Video, lines.Film.SubtitlesOnly = talkVideo, false
	d.AddFilm(lines)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	kept := linesFilm().Film
	kept.VideoSize = int64(len(talkVideo))
	st := store.Open(dir)
	if err := st.SaveFilm(store.Film{Desktop: id, Library: "Japanese Class", Film: kept, LinesOnly: true, Added: now()}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(id, kept.ID, talkCues); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	options := "id:films-options-" + id + "-f3"
	p.app.WaitFor(filmRow(id, "f3")+" >> text:YouTube · 0:10 · "+msgs.FilmLinesSaved(), wait)
	p.app.Press(options)
	p.app.WaitFor("id:films-options-title", wait)
	p.app.Advance(500 * time.Millisecond)
	shot(p, "films-options-lines")
}

func TestScreenshotsLibraryFilterEmpty(t *testing.T) {
	shot := screenshots(t)
	p, _ := libraryScene(t)
	p.app.Press("id:films-source-local")
	p.app.WaitFor("id:films-empty", wait)
	shot(p, "films-filter-empty")
}

func TestScreenshotsLibraryDark(t *testing.T) {
	shot := screenshots(t)
	p, id := libraryScene(t)
	// Dark, as Settings' Appearance steps from the system's look to light, then dark.
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-appearance")
	p.app.Press("id:settings-appearance")
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-page").Style.Background == core.RGB(0x000000) })
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor(filmRow(id, "f2"), wait)
	shot(p, "films-dark")
}

// TestScreenshotsLibraryStorage renders the Library away from the computer's network once the storage no longer takes the computer's key (films-storage).
func TestScreenshotsLibraryStorage(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.64)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.app.WaitFor(talkRow(id), wait)
	d.SetOff(true)
	bucket.Refuse(http.StatusForbidden, "SignatureDoesNotMatch")
	p.app.WaitFor("id:films-storage", reconnect)
	shot(p, "films-storage")
}

// TestScreenshotsLibraryWork renders a show its computer folds into one row (films-work) and the show's page (films-work-page).
func TestScreenshotsLibraryWork(t *testing.T) {
	shot := screenshots(t)
	d := newDesktopWithShow(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(workRow(id, "w1"), wait)
	shot(p, "films-work")
	p.app.Press(workRow(id, "w1"))
	p.app.WaitFor("id:work-page", wait)
	shot(p, "films-work-page")
}
