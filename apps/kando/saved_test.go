package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// TestSaveLineKeepsItInTheLibrary saves the line playing with B: it goes into the study record in the library folder, by the video's path under the folder, with its two lines and its times, the picture says so, and B again takes it out.
func TestSaveLineKeepsItInTheLibrary(t *testing.T) {
	a, tt, _ := playerOpen(t)
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return at }
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	dir := a.snap().LibraryDir
	book, err := saved.Load(dir)
	if err != nil || len(book.Lines) != 1 {
		t.Fatalf("the study record holds %+v, %v", book.Lines, err)
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

// TestSavedLineTakesItsTranslationLater saves a line of the open film that has no translation yet, as while the film is still being translated, and reads the film's subtitles again with the line translated: the saved line takes the translation, in the study record as well; a translated line saved before keeps the one it was saved with.
func TestSavedLineTakesItsTranslationLater(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	translated := slices.Clone(a.player.subs.cues)
	translated[0].Text = "您好。\nHello."
	translated[1].Text = "再见。\nBye."
	apply(tt, a, func() { a.player.subs.cues = translated })
	book, _ := saved.Load(a.snap().LibraryDir)
	if len(book.Lines) != 2 || book.Lines[0].Original != "Bye." || book.Lines[0].Translation != "再见。" || book.Lines[1].Translation != "你好。" {
		t.Fatalf("the study record holds %+v", book.Lines)
	}
	if lines := a.savedBook().Lines; lines[0].Translation != "再见。" {
		t.Fatalf("the saved lines hold %+v", lines)
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
		t.Fatalf("after the move the study record holds %+v", book.Lines)
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

// TestSavedSheetOpensTheLine saves a line, opens Study › Saved Lines… from the library, and checks that the sheet lists it under its film, that a click plays the film from the line, and that × takes it out of the study record.
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

// TestSavedLinesComeOverFromSavedLinesJSON opens a library whose lines an older Kando saved in saved-lines.json: they come over into the library's study record, which is written at once, stamped with no device as the old file names none, the old file stays as it was, and the sheet lists them.
func TestSavedLinesComeOverFromSavedLinesJSON(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Key("escape")
	dir := a.snap().LibraryDir
	legacy := filepath.Join(dir, "saved-lines.json")
	old := `{"lines": [{"video": "talk/talk.mp4", "start": 4, "end": 5, "original": "Bye.", "translation": "再见。", "film": "talk", "savedAt": "2026-11-16T09:00:00Z", "box": 1, "due": "2026-11-17T09:00:00Z"}]}`
	if err := os.WriteFile(legacy, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() {
		a.saved.book = saved.Book{}
		a.savedSync()
	})
	book, err := saved.Load(dir)
	if err != nil || len(book.Lines) != 1 || book.Migrated() {
		t.Fatalf("%s holds %+v, %v, migrated %v", saved.FileName, book.Lines, err, book.Migrated())
	}
	l := book.Lines[0]
	if l.ID == "" || l.Device != "" || l.Original != "Bye." || l.Translation != "再见。" || l.Box != 1 || !l.Due.Equal(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("the line came over as %+v", l)
	}
	if data, _ := os.ReadFile(legacy); string(data) != old {
		t.Fatalf("saved-lines.json became %q", data)
	}
	apply(tt, a, a.OpenSavedSheet)
	if !tt.Exists("id:saved >> " + whole("Bye.")) {
		t.Fatalf("the sheet shows %q", tt.Texts())
	}
}

// TestSavedLineNoteIsKept saves a line, writes a note on it in the Saved Lines sheet, and checks that the note is in the library's study record, stamped on this computer, shows under the line, and that a note emptied takes it away.
func TestSavedLineNoteIsKept(t *testing.T) {
	a, tt, _ := playerOpen(t)
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return at }
	playerPlayTo(tt, a, 4500*time.Millisecond)
	tt.Key("b")
	tt.Key("escape")
	apply(tt, a, a.OpenSavedSheet)
	id := a.saved.book.Lines[0].ID
	at = at.Add(time.Hour)
	tt.Press("id:saved-note-edit-" + id)
	tt.Type("id:saved-note-field-"+id, "  Bye = 再见  ")
	tt.Press("id:saved-note-save")
	dir := a.snap().LibraryDir
	book, _ := saved.Load(dir)
	if len(book.Lines) != 1 {
		t.Fatalf("lines %+v", book.Lines)
	}
	l := book.Lines[0]
	if l.Note != "Bye = 再见" || a.saved.device == "" || l.Device != a.saved.device || l.Stamps.Note.Device != a.saved.device || !l.Stamps.Note.At.Equal(at) || !l.Stamps.Original.At.Equal(at.Add(-time.Hour)) {
		t.Fatalf("the line is %+v, this computer %q", l, a.saved.device)
	}
	if !tt.Exists("id:saved-note-"+id) || tt.Exists("id:saved-note-field-"+id) {
		t.Fatalf("the sheet shows %q", tt.Texts())
	}
	tt.Press("id:saved-note-edit-" + id)
	if got := a.NoteDraft(); got != "Bye = 再见" {
		t.Fatalf("the field opened with %q", got)
	}
	apply(tt, a, func() { a.SetNoteDraft("") })
	tt.Press("id:saved-note-save")
	if book, _ := saved.Load(dir); book.Lines[0].Note != "" || tt.Exists("id:saved-note-"+id) {
		t.Fatalf("an emptied note left %+v", book.Lines[0])
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
	if err != nil || !strings.Contains(string(notes), "\"Hello.\"\t\"你好。\ntalk · 0:01.0\"\n") {
		t.Fatalf("notes %q, %v", notes, err)
	}
	// Without ffmpeg the sheet offers the settings that install it.
	tt.Press("id:saved-export-settings")
	if a.saved.sheet || a.modalOpen() {
		t.Fatalf("Open Settings left the sheet open %v, modal %v", a.saved.sheet, a.modalOpen())
	}
	wantSettings(t, a, settings.SectionGeneral)
}

// TestExportSavedCanBeStopped starts an export whose ffmpeg takes its time and stops it with the sheet's Cancel, which takes the place of Export for Anki… while it runs: the sheet says it stopped, not why a context ended, writes no notes and offers Export for Anki… again.
func TestExportSavedCanBeStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for ffmpeg is a shell script")
	}
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nexec sleep 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, updates, item := playerFilm(t, 12, media.Tools{FFmpeg: ffmpeg})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	tt.Key("escape")
	out := t.TempDir()
	tt.PickDirectories = func(ns.DirectoriesRequest) []ns.Directory { return []ns.Directory{{Path: out}} }
	apply(tt, a, a.ExportSaved)
	msgs := a.messages()
	if !a.saved.export.busy || tt.Exists("id:saved-export") {
		t.Fatalf("while it runs the sheet shows %q", tt.Texts())
	}
	tt.Press("id:saved-export-cancel")
	waitUntil(t, tt, a, updates, func() bool { return a.saved.export.finished })
	if !tt.Exists(whole(msgs.SavedExportStopped())) || tt.Exists("id:saved-export-reveal") || tt.Exists("id:saved-export-cancel") || tt.Find("id:saved-export").Props.Disabled {
		t.Fatalf("once stopped the sheet shows %q", tt.Texts())
	}
	if _, err := os.Stat(filepath.Join(out, saved.ExportFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a stopped export wrote its notes: %v", err)
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
	// Bye. has no translation, so there is nothing to show first: it is answered at once.
	if !tt.Exists("id:review >> "+whole("Bye.")) || !tt.Exists("id:review-good") || tt.Exists("id:review-show") {
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

// TestReviewCutsAndPlaysTheSound reviews a saved line with ffmpeg at hand (the fake one): B cuts the line's sound off the engine's goroutine into the library as the line is saved, the review N opens meanwhile plays it once it is cut, and Escape, closing the review, takes the review's own folder away and leaves the sound kept in the library.
func TestReviewCutsAndPlaysTheSound(t *testing.T) {
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a, tt, updates, item := playerFilm(t, 12, tools)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	tt.Key("n")
	dir := a.saved.review.dir
	if dir == "" || !a.saved.review.open {
		t.Fatalf("N opened no review with a folder for its sounds: %+v", a.saved.review)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	waitUntil(t, tt, a, updates, func() bool { return a.ReviewCard().Clip != "" })
	card := a.ReviewCard()
	kept := saved.StoredSound(a.saved.book, a.saved.book.Lines[0].ID)
	if card.Clip != kept || filepath.Dir(kept) != filepath.Join(a.snap().LibraryDir, saved.SoundDir) || !card.Playing || card.Plays != 1 || card.NoAudio {
		t.Fatalf("once cut the card is %+v, want its sound %s playing", card, kept)
	}
	if data, err := os.ReadFile(card.Clip); err != nil || !strings.HasPrefix(string(data), "kando fake sound ") {
		t.Fatalf("the sound holds %q, %v", data, err)
	}
	if v := tt.Find("id:review-clip").Video; v.Source != card.Clip || v.Paused {
		t.Fatalf("the review plays %+v, want %s", v, card.Clip)
	}
	tt.Key("escape")
	if a.saved.review.open || a.modalOpen() {
		t.Fatal("Escape left the review open")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the review's folder of sounds is still there: %v", err)
	}
	if !saved.HasSound(kept) {
		t.Fatal("the review took the kept sound away")
	}
}

// TestSavedLineKeepsItsSound saves a line with ffmpeg at hand: its sound is cut into the library as it is saved, the review then plays that sound without cutting it again, even once the video is gone, and B again takes the line out with its sound.
func TestSavedLineKeepsItsSound(t *testing.T) {
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a, tt, updates, item := playerFilm(t, 12, tools)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	kept := saved.StoredSound(a.saved.book, a.saved.book.Lines[0].ID)
	waitUntil(t, tt, a, updates, func() bool { return len(a.saved.keeping) == 0 })
	if !saved.HasSound(kept) {
		t.Fatalf("no sound kept at %s", kept)
	}
	if err := os.Rename(item.VideoPath, item.VideoPath+".gone"); err != nil {
		t.Fatal(err)
	}
	tt.Key("n")
	if card := a.ReviewCard(); card.Clip != kept || len(a.saved.review.cutting) != 0 || !card.Playing {
		t.Fatalf("the review of a line whose video is gone: %+v, cutting %v", card, a.saved.review.cutting)
	}
	tt.Key("escape")
	if err := os.Rename(item.VideoPath+".gone", item.VideoPath); err != nil {
		t.Fatal(err)
	}
	tt.Key("b")
	if saved.HasSound(kept) {
		t.Fatal("the sound of a line taken out stayed")
	}
}

// TestSavedLineTakenOutWhileItsSoundIsCut saves a line and takes it out again at once, while its sound is still being cut: once the cut ends no sound of it is left in the library.
func TestSavedLineTakenOutWhileItsSoundIsCut(t *testing.T) {
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a, tt, updates, item := playerFilm(t, 12, tools)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	kept := saved.StoredSound(a.saved.book, a.saved.book.Lines[0].ID)
	tt.Key("b")
	waitUntil(t, tt, a, updates, func() bool { return len(a.saved.keeping) == 0 })
	if len(a.saved.book.Lines) != 0 {
		t.Fatalf("%d lines saved", len(a.saved.book.Lines))
	}
	if saved.HasSound(kept) {
		t.Fatal("the sound of a line taken out while it was cut stayed")
	}
}

// TestReviewHasTheWordsAndTheNotes reviews a saved line with a note and a word of the vocabulary, both due: the line's note shows with its translation, and the word shows with its reading over the line it was looked up in, its meaning and the line's translation once revealed; Good schedules the word in the study record, and the review counts both.
func TestReviewHasTheWordsAndTheNotes(t *testing.T) {
	a, tt, _ := playerOpen(t)
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.saved.now = func() time.Time { return now }
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	apply(tt, a, func() {
		book := &a.saved.book
		book.SetNote(book.Lines[0].ID, "打招呼")
		now = now.Add(time.Minute)
		book.AddWord(saved.Word{Text: "bye", Reading: "/baɪ/", Meaning: "再见", Context: "Bye.", ContextTranslation: "再会。", Video: "talk/talk.mp4", Start: 4 * time.Second, End: 5 * time.Second, Film: "talk"})
		a.savedWrite()
	})
	if n := a.DueCount(); n != 2 {
		t.Fatalf("%d due, want the line and the word", n)
	}
	msgs := a.messages()
	tt.Key("n")
	if !tt.Exists("id:review >> "+whole("Hello.")) || tt.Exists("id:review-note") {
		t.Fatalf("the line shows %q", tt.Texts())
	}
	tt.Key("space")
	if !tt.Exists("id:review-note") || !tt.Exists("id:review >> "+whole("打招呼")) {
		t.Fatalf("the revealed line shows no note: %q", tt.Texts())
	}
	tt.Key("2")
	card := a.ReviewCard()
	if !card.Word || !tt.Exists("id:review >> "+whole("bye")) || !tt.Exists("id:review >> "+whole("/baɪ/")) || !tt.Exists("id:review >> "+whole("Bye.")) || tt.Exists("id:review-translation") {
		t.Fatalf("the word shows %+v: %q", card, tt.Texts())
	}
	tt.Key("space")
	if !tt.Exists("id:review >> "+whole("再见")) || !tt.Exists("id:review >> "+whole("再会。")) {
		t.Fatalf("the revealed word shows %q", tt.Texts())
	}
	tt.Key("2")
	if !tt.Exists(whole(msgs.SavedReviewDone(2))) {
		t.Fatalf("once done the review shows %q", tt.Texts())
	}
	book, _ := saved.Load(a.snap().LibraryDir)
	if w := book.Words[0]; w.Box != 1 || !w.Due.Equal(now.Add(24*time.Hour)) || w.Stamps.Review.Device != a.saved.device {
		t.Fatalf("the word is in box %d due %v", w.Box, w.Due)
	}
}

// TestNotesTabListsTheOpenFilms reads the player's 笔记 tab of the open film: its saved lines and its words, each by where its line starts, none of another film's; the × of a word takes it out of the study record.
func TestNotesTabListsTheOpenFilms(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Key("b")
	apply(tt, a, func() {
		book := &a.saved.book
		book.AddWord(saved.Word{Text: "bye", Meaning: "再见", Context: "Bye.", Video: "talk/talk.mp4", Start: 4 * time.Second})
		book.AddWord(saved.Word{Text: "elsewhere", Video: "other/other.mp4", Start: time.Second})
		a.savedWrite()
	})
	notes := a.Notes()
	if len(notes) != 2 || notes[0].Word || notes[0].Original != "Hello." || !notes[1].Word || notes[1].Text != "bye" || notes[1].Start != 4*time.Second {
		t.Fatalf("the notes are %+v", notes)
	}
	tt.Press("id:film-tab-notes")
	tt.Press("id:film-notes-words")
	if !tt.Exists(whole("bye　再见")) || tt.Exists(whole("elsewhere")) {
		t.Fatalf("the words tab shows %q", tt.Texts())
	}
	tt.Press("id:film-note-remove")
	book, _ := saved.Load(a.snap().LibraryDir)
	if book.WordIndex("bye", "") >= 0 || book.WordIndex("elsewhere", "") < 0 || a.player.film.osd != a.messages().VocabOsdRemoved("bye") {
		t.Fatalf("× left %+v; the picture says %q", book.Words, a.player.film.osd)
	}
}

// TestStudyDeviceIsKeptAcrossLaunches checks that the id this computer is given in the study record the first time a library's saved lines are read is written to settings.json at once, and that the next launch reads it back and stamps its books with it rather than making another.
func TestStudyDeviceIsKeptAcrossLaunches(t *testing.T) {
	for _, env := range []string{"KANDO_OUTPUT_DIR", "KANDO_LLM_BASE_URL", "KANDO_LLM_MODEL", "KANDO_LLM_API_KEY", "EVERYAPI_API_KEY", "KANDO_R2_SECRET"} {
		t.Setenv(env, "")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"language": "日本語"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	a := newApp(media.Tools{}, func(func()) {})
	a.settingsPath = path
	a.restoreSettings(a.loadSettings())
	book, err := a.savedLoad(library)
	if err != nil || a.saved.device == "" || book.Device != a.saved.device {
		t.Fatalf("the book is stamped by %q, this computer %q, %v", book.Device, a.saved.device, err)
	}
	if saved := loadSavedSettings(path); saved.StudyDevice != a.saved.device || saved.Language != "日本語" {
		t.Fatalf("settings.json holds device %q, language %q, want %q", saved.StudyDevice, saved.Language, a.saved.device)
	}

	b := newApp(media.Tools{}, func(func()) {})
	b.settingsPath = path
	b.restoreSettings(b.loadSettings())
	again, err := b.savedLoad(library)
	if err != nil || b.saved.device != a.saved.device || again.Device != a.saved.device {
		t.Fatalf("the next launch stamps by %q, this computer %q, want %q (%v)", again.Device, b.saved.device, a.saved.device, err)
	}
}
