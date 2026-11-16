package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/saved"

	ns "github.com/nsgo-dev/nsgo/core"
)

// TestSaveLineKeepsItInTheLibrary saves the line playing with B: it goes into saved-lines.json in the library folder, by the video's path under the folder, with its two lines and its times, the picture says so, and B again takes it out.
func TestSaveLineKeepsItInTheLibrary(t *testing.T) {
	a, tt, _ := playerOpen(t)
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return at }
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	dir := a.snap().LibraryDir
	book, err := saved.Load(dir)
	if err != nil || len(book.Lines) != 1 {
		t.Fatalf("saved-lines.json holds %+v, %v", book.Lines, err)
	}
	l := book.Lines[0]
	if l.Video != "talk/talk.mp4" || l.Start != time.Second || l.End != 2500*time.Millisecond || l.Original != "Hello." || l.Translation != "你好。" || l.Film != "talk" || !l.SavedAt.Equal(at) {
		t.Fatalf("the saved line is %+v", l)
	}
	if !a.LineSaved(0) || a.LineSaved(1) || a.player.film.osd != a.messages().SavedOsdSaved() {
		t.Fatalf("saved 0 %v, 1 %v, the picture says %q", a.LineSaved(0), a.LineSaved(1), a.player.film.osd)
	}
	if !tt.Exists("id:film-cues >> id:film-cue-saved") {
		t.Fatal("the saved line's row shows no star")
	}
	tt.Key("b")
	if book, _ := saved.Load(dir); len(book.Lines) != 0 || a.LineSaved(0) || a.player.film.osd != a.messages().SavedOsdRemoved() {
		t.Fatalf("B again left %+v, the picture says %q", book.Lines, a.player.film.osd)
	}
}

// TestSavedLinesFollowAMovedVideo checks that a saved line follows its video when the video's folder is renamed, as the resume positions do, and stays saved when the whole library folder moves, as it is kept by its path under it.
func TestSavedLinesFollowAMovedVideo(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	dir := a.snap().LibraryDir
	old, next := filepath.Join(dir, "talk", "talk.mp4"), filepath.Join(dir, "Talk 2", "talk.mp4")
	a.moveVideos(func(p string) (string, bool) { return next, p == old })
	book, _ := saved.Load(dir)
	if len(book.Lines) != 1 || book.Lines[0].Video != "Talk 2/talk.mp4" || book.Lines[0].Original != "Bye." {
		t.Fatalf("after the move saved-lines.json holds %+v", book.Lines)
	}
}

// TestSavedChipFollowsTheFilm narrows the open film's subtitle list to its saved line, and checks that the film opened next shows all its lines again.
func TestSavedChipFollowsTheFilm(t *testing.T) {
	a, tt, updates := playerOpen(t)
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	tt.Press("id:film-subs-saved")
	if !a.saved.only || tt.Exists("id:film-cues >> "+whole("你好。")) || !tt.Exists("id:film-cues >> "+whole("Bye.")) {
		t.Fatalf("Saved shows %q", tt.Texts())
	}
	item, _ := films.Find(a.films(), "talk")
	tt.Key("escape")
	playerOpenFilm(t, a, tt, updates, item)
	if a.saved.only || !tt.Exists("id:film-cues >> "+whole("你好。")) {
		t.Fatalf("the film opened again shows %q", tt.Texts())
	}
}

// TestSavedSheetOpensTheLine saves a line, opens Study › Saved Lines… from the library, and checks that the sheet lists it under its film, that a click plays the film from the line, and that × takes it out of saved-lines.json.
func TestSavedSheetOpensTheLine(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	tt.Key("escape")
	apply(tt, a, a.OpenSavedSheet)
	msgs := a.messages()
	if !tt.Exists(whole(msgs.SavedWhere("talk", "0:04.0"))) || !tt.Exists("id:saved >> "+whole("Bye.")) || !a.modalOpen() {
		t.Fatalf("the sheet shows %q, modal %v", tt.Texts(), a.modalOpen())
	}
	tt.Press("id:saved >> " + whole("Bye."))
	if a.saved.sheet || a.modalOpen() || a.openKey != "talk" || a.PlayHead() != 4*time.Second {
		t.Fatalf("a click left the sheet %v, opened %q at %v", a.saved.sheet, a.openKey, a.PlayHead())
	}
	tt.Key("escape")
	apply(tt, a, a.OpenSavedSheet)
	tt.Press("id:saved-remove-" + savedID(a.saved.book.Lines[0]))
	if book, _ := saved.Load(a.snap().LibraryDir); len(book.Lines) != 0 || !tt.Exists(whole(msgs.SavedEmpty())) {
		t.Fatalf("× left %+v, the sheet shows %q", book.Lines, tt.Texts())
	}
}

// TestExportSavedWritesTheNotes saves a line and exports it with File › Export for Anki… into the folder picked: the sheet opens, the notes' file holds the line, without its sound as the test's pipeline has no ffmpeg, and the sheet says so and offers to show the file.
func TestExportSavedWritesTheNotes(t *testing.T) {
	a, tt, updates := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	tt.Key("escape")
	out := t.TempDir()
	tt.PickDirectories = func(ns.DirectoriesRequest) []ns.Directory { return []ns.Directory{{Path: out}} }
	apply(tt, a, a.ExportSaved)
	waitUntil(t, tt, a, updates, func() bool { return a.saved.export.finished })
	if !a.saved.sheet || a.saved.export.err != nil || a.saved.export.result.Lines != 1 || !tt.Exists(whole(a.messages().SavedExportedNoAudio(1))) || !tt.Exists("id:saved-export-reveal") {
		t.Fatalf("export %+v, the sheet shows %q", a.saved.export, tt.Texts())
	}
	notes, err := os.ReadFile(filepath.Join(out, saved.ExportFile))
	if err != nil || !strings.Contains(string(notes), "Hello.\t你好。\ttalk\t0:01.0\t\n") {
		t.Fatalf("notes %q, %v", notes, err)
	}
}

// TestReviewSchedulesTheLines saves two lines and reviews them with N: each shows its original with its translation hidden, without a sound as the test's pipeline has no ffmpeg; Good puts a line a day away, Again brings it back at the end of the review and keeps it due, and the review ends once every line was known.
func TestReviewSchedulesTheLines(t *testing.T) {
	a, tt, _ := playerOpen(t)
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return now }
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	now = now.Add(time.Minute)
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	tt.Key("n")
	msgs := a.messages()
	if !a.saved.review.open || !a.modalOpen() || !tt.Exists("id:review >> "+whole("Hello.")) || tt.Exists("id:review >> "+whole("你好。")) || !tt.Exists(whole(msgs.SavedReviewNoAudio())) {
		t.Fatalf("the review shows %q", tt.Texts())
	}
	tt.Key("space")
	tt.Key("1")
	if !tt.Exists("id:review >> " + whole("Bye.")) {
		t.Fatalf("after Again the review shows %q", tt.Texts())
	}
	tt.Key("space")
	tt.Key("2")
	tt.Key("space")
	tt.Key("2")
	if !tt.Exists(whole(msgs.SavedReviewDone(3))) {
		t.Fatalf("once done the review shows %q", tt.Texts())
	}
	book, _ := saved.Load(a.snap().LibraryDir)
	for _, l := range book.Lines {
		if l.Box != 1 || !l.Due.Equal(now.Add(24*time.Hour)) {
			t.Fatalf("%q is in box %d due %v", l.Original, l.Box, l.Due)
		}
	}
	tt.Press("id:review-close")
	if a.saved.review.open || a.modalOpen() {
		t.Fatal("Done left the review open")
	}
}
