package main

import (
	"slices"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/saved"
	"kando/apps/kando/study"

	"github.com/nsgo-dev/nsgo/core"
)

// The study stats dialog's reads and clicks (study.StatsApp). The dialog reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself.

// The dialog's tabs.
const (
	vocabTabStats  = study.TabStats
	vocabTabExport = study.TabExport
)

// vocabStats is the study stats dialog: open, at tab, with release giving back the window's modal flag it holds while it is (holdModal); the export's options, and the file the last export wrote or why it failed.
type vocabStats struct {
	open     bool
	release  func()
	tab      string
	options  saved.VocabOptions
	exported study.VocabExported
}

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
}

// CloseStats closes the dialog, as Done, Escape and closing it do, and gives back the window's modal flag.
func (a *app) CloseStats() {
	a.vocab.stats.open = false
	if a.vocab.stats.release != nil {
		a.vocab.stats.release()
		a.vocab.stats.release = nil
	}
}

// StudyStats is what the open library's study record says of the learner's study now.
func (a *app) StudyStats() saved.Stats { return a.savedBook().Stats(a.saved.now()) }

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
	a.pickFolder(func(dir string) {
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
