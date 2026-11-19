package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/plugins"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/apps/kando/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The Saved Lines sheet's reads and clicks (study.App), and the player's 笔记 tab's (film.Notebook). The sheet reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// savedOverlay is the Saved Lines sheet over the window while it is open (overlays). It is built as it opens, so its list starts at the top.
func (a *app) savedOverlay(*core.Scope) core.Node {
	return core.Show{
		When: live.Func(a, func() bool { return a.saved.sheet }),
		Then: func(*core.Scope) core.Node { return study.Sheet{App: a} },
	}
}

// OpenSavedSheet is the Study menu's Saved Lines…: the sheet of the library's saved lines, holding the window's modal flag while it is open.
func (a *app) OpenSavedSheet() {
	if a.saved.sheet {
		return
	}
	a.savedSync()
	a.saved.sheet = true
	a.saved.release = a.holdModal()
}

// CloseSaved closes the sheet, as Done, Escape and closing it do, and gives back the window's modal flag. A note being written is dropped, as Cancel drops it.
func (a *app) CloseSaved() {
	a.saved.sheet = false
	a.CancelNote()
	if a.saved.release != nil {
		a.saved.release()
		a.saved.release = nil
	}
}

// savedID names a saved line for the sheet and the review: its id, or for a line that has none, as one a test puts in the book, its video and its start.
func savedID(l saved.Line) string {
	if l.ID != "" {
		return l.ID
	}
	return fmt.Sprintf("%s@%d", l.Video, l.Start.Milliseconds())
}

// savedFind is the saved line the sheet names id, by its index in the book, or -1.
func (a *app) savedFind(id string) int {
	for i, l := range a.saved.book.Lines {
		if savedID(l) == id {
			return i
		}
	}
	return -1
}

// savedFilm is the library's film of the video of saved line l, if it is in the library now.
func (a *app) savedFilm(l saved.Line) (films.Film, bool) {
	video := filepath.Clean(a.saved.book.Abs(l))
	for _, f := range a.films() {
		if f.VideoPath != "" && filepath.Clean(f.VideoPath) == video {
			return f, true
		}
	}
	return films.Film{}, false
}

// SavedEntries is the library's saved lines as the sheet lists them, each under the title its film has now, or the one it had when the line was saved once the film is gone.
func (a *app) SavedEntries() []study.Entry {
	book := a.savedBook()
	out := make([]study.Entry, 0, len(book.Lines))
	for _, l := range book.Lines {
		e := study.Entry{ID: savedID(l), Film: l.Film, Original: l.Original, Translation: l.Translation, Note: l.Note, Start: l.Start, Gone: true}
		if f, ok := a.savedFilm(l); ok {
			e.Gone = false
			if f.Name != "" {
				e.Film = f.Name
			}
		}
		out = append(out, e)
	}
	return out
}

// OpenSaved closes the sheet and plays the film of saved line id from the line, where it shows with the film's subtitle delay.
func (a *app) OpenSaved(id string) {
	i := a.savedFind(id)
	if i < 0 {
		return
	}
	l := a.saved.book.Lines[i]
	item, ok := a.savedFilm(l)
	if !ok {
		return
	}
	a.CloseSaved()
	if a.page != pageFilms {
		a.Select(pageFilms)
	}
	a.OpenAt(item, max(l.Start+a.player.delays[item.VideoPath], 0))
}

// RemoveSaved takes saved line id out of the saved lines, with the sound kept of it.
func (a *app) RemoveSaved(id string) {
	i := a.savedFind(id)
	if i < 0 {
		return
	}
	l := a.saved.book.Lines[i]
	a.saved.book.Remove(l.Video, l.Start)
	a.savedWrite()
	a.savedDropSound(l.ID)
}

// NoteEditing is the saved line whose note the sheet is writing, by its id, "" while none is.
func (a *app) NoteEditing() string { return a.saved.note }

// EditNote is a saved line's pencil: the field under it opens with its note, after the note open under another line is kept (SaveNote). The pencil of the line whose note is open keeps it as typed.
func (a *app) EditNote(id string) {
	if a.saved.note == id {
		return
	}
	a.SaveNote()
	i := a.savedFind(id)
	if i < 0 {
		return
	}
	a.saved.note, a.saved.draft = id, a.saved.book.Lines[i].Note
}

// NoteDraft is the note as typed so far in the open field.
func (a *app) NoteDraft() string { return a.saved.draft }

// SetNoteDraft follows the note field as the learner types.
func (a *app) SetNoteDraft(text string) { a.saved.draft = text }

// SaveNote is the note field's Save: the note as typed becomes the line's, an empty one taking its note away, and the field closes.
func (a *app) SaveNote() {
	id, draft := a.saved.note, a.saved.draft
	a.CancelNote()
	if id == "" {
		return
	}
	if a.saved.book.SetNote(id, draft) {
		a.savedWrite()
	}
}

// CancelNote is the note field's Cancel: it closes without keeping what was typed.
func (a *app) CancelNote() { a.saved.note, a.saved.draft = "", "" }

// ExportState is how the export of the saved lines goes, as the sheet says it. An export the user stopped (StopExport) is Stopped, not failed with the context's error.
func (a *app) ExportState() study.Export {
	e := a.saved.export
	out := study.Export{Busy: e.busy, Finished: e.finished, Done: e.done, Total: e.total, Lines: e.result.Lines, Sounds: e.result.Sounds, NoFFmpeg: e.noFFmpeg,
		Template: e.template, File: filepath.Base(e.result.File), WithSound: e.sounds, Plugin: e.plugin, Sent: e.sent, Skipped: e.skipped, Soundless: e.soundless}
	switch {
	case errors.Is(e.err, context.Canceled):
		out.Stopped = true
	case e.err != nil:
		out.Err = e.err.Error()
		// A plugin that rejected the export with a code of its own did it on purpose, saying why (core.Error); anything else is Kando's or the runtime's error, or a refusal of Kando's the plugin passed on, which is not the user's to read.
		var coded *core.Error
		if e.plugin != "" && errors.As(e.err, &coded) && pluginsOwnCode(coded.Code) {
			out.Reason = plugins.Reason(coded.Message)
		}
	}
	return out
}

// StopExport is the sheet's Cancel: it stops the export running, which then says it stopped.
func (a *app) StopExport() { a.savedStopExport() }

// ExportSettings is the sheet's Open Settings after an export without ffmpeg: the sheet closes and the settings open at the section that installs it.
func (a *app) ExportSettings() {
	a.CloseSaved()
	a.OpenSettings(settings.SectionGeneral)
}

// RevealExport shows the notes' file the last export wrote in the file manager.
func (a *app) RevealExport() { a.reveal(a.saved.export.result.File) }

// Notes is the open film's saved lines and the words of the vocabulary looked up in it, as the player's 笔记 tab lists them: each by where its line starts, the earliest first (film.Notebook).
func (a *app) Notes() []film.Note {
	video := a.player.film.video
	book := a.savedBook()
	if video == "" || book.Dir() == "" {
		return nil
	}
	key := book.Key(video)
	var out []film.Note
	for _, l := range book.Lines {
		if l.Video == key {
			out = append(out, film.Note{ID: savedID(l), Start: l.Start, Original: l.Original, Translation: l.Translation, Note: l.Note})
		}
	}
	for _, w := range book.Words {
		if w.Video == key {
			out = append(out, film.Note{ID: w.ID, Word: true, Start: w.Start, Original: w.Context, Translation: w.ContextTranslation, Text: w.Text, Reading: w.Reading, Meaning: w.Meaning})
		}
	}
	slices.SortStableFunc(out, func(x, y film.Note) int { return cmp.Compare(x.Start, y.Start) })
	return out
}

// NotesWords is whether the 笔记 tab lists the words rather than the lines.
func (a *app) NotesWords() *bool { return &a.saved.notesWords }

// RemoveNote is the × of a line or a word of the 笔记 tab: it goes out of the study record with the sound kept of it, and the picture says so.
func (a *app) RemoveNote(n film.Note) {
	a.savedSync()
	book := &a.saved.book
	if n.Word {
		i := book.WordByID(n.ID)
		if i < 0 {
			return
		}
		w := book.Words[i]
		book.RemoveWord(w.Text, w.Spoken)
		a.flash(a.messages().VocabOsdRemoved(w.Text))
		a.savedWrite()
		a.savedDropSound(w.ID)
		return
	}
	if a.savedFind(n.ID) < 0 {
		return
	}
	a.RemoveSaved(n.ID)
	a.flash(a.messages().SavedOsdRemoved())
}
