package main

import (
	"os"
	"path/filepath"
	"slices"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/saved"
	"kando/apps/kando/study"
	recordstudy "kando/packages/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The study stats dialog's reads and clicks (study.StatsApp). The dialog reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// The dialog's tabs.
const (
	vocabTabStats  = study.TabStats
	vocabTabExport = study.TabExport
)

// vocabStats is the study stats dialog: open, at tab, with release giving back the window's modal flag it holds while it is (holdModal); the study records of the other libraries it counts (vocabReadOthers); the export's options, and the file the last export wrote or why it failed.
type vocabStats struct {
	open     bool
	release  func()
	tab      string
	others   []saved.Book
	reading  int
	options  saved.VocabOptions
	exported study.VocabExported
}

// vocabLoadBook reads a library's study record for the stats, off the engine's goroutine; the tests watch through it which libraries are read.
var vocabLoadBook = saved.Load

func newVocabStats() vocabStats {
	return vocabStats{tab: vocabTabStats, options: saved.VocabOptions{Format: saved.VocabAnki, Context: true, Translation: true}}
}

// vocabOverlay is the study stats dialog over the window while it is open (overlays).
func (a *app) vocabOverlay(*core.Scope) core.Node {
	return core.Show{
		When: live.Func(a, func() bool { return a.vocab.stats.open }),
		Then: func(*core.Scope) core.Node { return study.StatsDialog{App: a} },
	}
}

// OpenStats is the Study menu's Study Stats and Export Vocabulary…: the dialog opens at tab, holding the window's modal flag while it is open. The time the open film played so far goes into the stats first.
func (a *app) OpenStats(tab string) {
	a.vocabFlushWatch()
	a.savedSync()
	a.vocab.stats.tab = tab
	a.vocab.stats.exported = study.VocabExported{}
	if a.vocab.stats.open {
		return
	}
	a.vocab.stats.open = true
	a.vocab.stats.release = a.holdModal()
	a.vocabReadOthers()
}

// vocabStatsCounts reports whether the stats count the library at dir: it is one Kando knows and it is not locked. A locked library's study record is never read, so its films stay closed (state_lock.go).
func (a *app) vocabStatsCounts(dir string) bool {
	return dir != "" && !a.LibraryLocked(dir)
}

// vocabReadOthers reads, off the engine's goroutine, the study records of the libraries the stats count besides the open one, whose own the saved lines keep (savedBook), and keeps them for the dialog unless it was opened again meanwhile. A library whose folder is not there now, as on a drive taken out, or whose study record cannot be read, is left out. Nothing is written to them.
func (a *app) vocabReadOthers() {
	s := &a.vocab.stats
	s.others = nil
	s.reading++
	run := s.reading
	open := filepath.Clean(a.snap().LibraryDir)
	var dirs []string
	for _, dir := range a.knownLibraries() {
		if dir = filepath.Clean(dir); dir != open && a.vocabStatsCounts(dir) {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 || a.update == nil {
		return
	}
	update := a.update
	go func() {
		var books []saved.Book
		for _, dir := range dirs {
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				continue
			}
			book, err := vocabLoadBook(dir)
			if err != nil {
				continue
			}
			books = append(books, book)
		}
		update(func() {
			if run == a.vocab.stats.reading && a.vocab.stats.open {
				a.vocab.stats.others = books
			}
		})
	}()
}

// vocabCounted is the study records the stats count now: the open library's, unless it is locked, then the others read for the dialog that are still unlocked and are not the open one.
func (a *app) vocabCounted() []saved.Book {
	open := filepath.Clean(a.snap().LibraryDir)
	var books []saved.Book
	if book := a.savedBook(); book.Dir() != "" {
		books = append(books, book)
	}
	for _, book := range a.vocab.stats.others {
		if dir := filepath.Clean(book.Dir()); dir != open && a.vocabStatsCounts(dir) {
			books = append(books, book)
		}
	}
	return books
}

// CloseStats closes the dialog, as Done, Escape and closing it do, and gives back the window's modal flag.
func (a *app) CloseStats() {
	a.vocab.stats.open = false
	a.vocab.stats.others = nil
	a.vocab.stats.reading++
	if a.vocab.stats.release != nil {
		a.vocab.stats.release()
		a.vocab.stats.release = nil
	}
}

// StudyStats is what the study records of the libraries the stats count say of the learner's study now, all of them together (StatsLibraries).
func (a *app) StudyStats() saved.Stats {
	return recordstudy.StatsOf(a.saved.now(), a.saved.device, a.vocabCounted()...)
}

// StatsLibraries is the names of the libraries the stats count, the open one first; StatsLocked is how many libraries Kando knows that they leave out as they are locked.
func (a *app) StatsLibraries() []string {
	var names []string
	for _, book := range a.vocabCounted() {
		names = append(names, films.LibraryName(book.Dir()))
	}
	return names
}

func (a *app) StatsLocked() int {
	n := 0
	for _, dir := range a.knownLibraries() {
		if dir != "" && a.LibraryLocked(dir) {
			n++
		}
	}
	return n
}

// StatsTab is the tab the dialog shows; SetStatsTab shows another.
func (a *app) StatsTab() string { return a.vocab.stats.tab }

func (a *app) SetStatsTab(tab string) { a.vocab.stats.tab = tab }

// VocabOptions is what the export writes; SetVocabOptions changes it.
func (a *app) VocabOptions() saved.VocabOptions { return a.vocab.stats.options }

func (a *app) SetVocabOptions(o saved.VocabOptions) {
	a.vocab.stats.options = o
	a.vocab.stats.exported = study.VocabExported{}
}

// vocabRows is the records the export writes with its options now.
func (a *app) vocabRows() [][]string {
	book := a.savedBook()
	return saved.VocabRows(book.Words, book.Lines, a.vocab.stats.options)
}

// VocabPreview is the first three records as the file will hold them.
func (a *app) VocabPreview() string {
	return saved.VocabPreview(a.vocabRows(), a.vocab.stats.options, 3)
}

// VocabSummary is how many words the export holds, and saved lines when it holds those too, and from how many films.
func (a *app) VocabSummary() study.VocabSummary {
	book := a.savedBook()
	var films []string
	out := study.VocabSummary{Words: len(book.Words)}
	for _, w := range book.Words {
		films = append(films, w.Film)
	}
	if a.vocab.stats.options.Lines {
		out.Lines = len(book.Lines)
		for _, l := range book.Lines {
			films = append(films, l.Film)
		}
	}
	films = slices.DeleteFunc(films, func(f string) bool { return f == "" })
	slices.Sort(films)
	out.Films = len(slices.Compact(films))
	return out
}

// ExportVocab is the Export Vocabulary tab's Export…: in the folder the user picks, the file of the vocabulary in the format chosen (saved.ExportVocab), the dialog then saying where it went or why it did not.
func (a *app) ExportVocab() {
	if len(a.vocabRows()) == 0 {
		return
	}
	a.pickFolder(core.PickDirectoriesOptions{}, func(dir string) {
		path, err := saved.ExportVocab(dir, a.vocabRows(), a.vocab.stats.options)
		if err != nil {
			a.vocab.stats.exported = study.VocabExported{Err: err.Error()}
			return
		}
		a.vocab.stats.exported = study.VocabExported{File: path, Shown: films.DisplayPath(a.messages(), path)}
	})
}

// VocabExported is the file the last export wrote, or why it failed, the zero value before one.
func (a *app) VocabExported() study.VocabExported { return a.vocab.stats.exported }

// RevealVocabExport shows the file the last export wrote in the file manager.
func (a *app) RevealVocabExport() { a.reveal(a.vocab.stats.exported.File) }
