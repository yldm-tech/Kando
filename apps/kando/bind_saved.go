package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/apps/kando/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The Saved Lines sheet's reads and clicks (study.App). The sheet reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

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

// CloseSaved closes the sheet, as Done, Escape and closing it do, and gives back the window's modal flag.
func (a *app) CloseSaved() {
	a.saved.sheet = false
	if a.saved.release != nil {
		a.saved.release()
		a.saved.release = nil
	}
}

// savedID names a saved line for the sheet: its video and its start.
func savedID(l saved.Line) string { return fmt.Sprintf("%s@%d", l.Video, l.Start.Milliseconds()) }

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
		e := study.Entry{ID: savedID(l), Film: l.Film, Original: l.Original, Translation: l.Translation, Start: l.Start, Gone: true}
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

// RemoveSaved takes saved line id out of the saved lines.
func (a *app) RemoveSaved(id string) {
	i := a.savedFind(id)
	if i < 0 {
		return
	}
	l := a.saved.book.Lines[i]
	a.saved.book.Remove(l.Video, l.Start)
	a.savedWrite()
}

// ExportState is how the export of the saved lines goes, as the sheet says it. An export the user stopped (StopExport) is Stopped, not failed with the context's error.
func (a *app) ExportState() study.Export {
	e := a.saved.export
	out := study.Export{Busy: e.busy, Finished: e.finished, Done: e.done, Total: e.total, Lines: e.result.Lines, Sounds: e.result.Sounds, NoFFmpeg: e.noFFmpeg}
	switch {
	case errors.Is(e.err, context.Canceled):
		out.Stopped = true
	case e.err != nil:
		out.Err = e.err.Error()
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
