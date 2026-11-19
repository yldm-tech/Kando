package main

import (
	"context"
	"kando/packages/media"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/apps/kando/saved"
	kcore "kando/packages/core"
	"kando/packages/study"

	"github.com/nsgo-dev/nsgo/core"
)

// savedUI is the lines the learner saved (package saved): the open library's book, read from its folder.
type savedUI struct {
	book saved.Book
	// device is this computer's id in the study record, which the lines saved and changed here are stamped with (study.Line.Device), kept in settings.json; "" until the first book is read (savedLoad).
	device string
	// note is the saved line whose note the sheet is editing, by its id, "" while none is, and draft the note as typed so far (bind_saved.go).
	note, draft string
	// now is the clock the saved lines are stamped and scheduled by; tests set it.
	now func() time.Time
	// notesWords is whether the player's 笔记 tab lists the words rather than the saved lines.
	notesWords bool
	// only is whether the player's subtitle list shows the saved lines alone, for the film onlyFor, the open key it was picked in; another film opens showing them all.
	only    bool
	onlyFor string
	// sheet is whether the Saved Lines sheet is open (bind_saved.go); release gives back the window's modal flag it holds while it is (holdModal).
	sheet   bool
	release func()
	// export is the export of the saved lines for Anki running or done last (savedExport).
	export savedExport
	// review is the review of the lines due open now (saved_review.go).
	review savedReview
	// keeping is the saved lines and words whose sound is being cut to be kept in the library, by id (saved_sounds.go).
	keeping map[string]bool
	// filled is the open film's subtitle lines the saved lines last followed (savedFollowSubtitles), so each reading of them is followed once.
	filled []kcore.Segment
}

// savedExport is an export of the saved lines (saved.Export): busy while it runs, with how many lines it has done of total; once done, what it wrote, or why it failed, context.Canceled once stopped. noFFmpeg is set when it ran without ffmpeg, so without the lines' audio. cancel stops the one running. gen numbers the exports, so a report of one that another took over from is dropped.
type savedExport struct {
	busy        bool
	done, total int
	result      saved.Exported
	err         error
	finished    bool
	noFFmpeg    bool
	cancel      context.CancelFunc
	gen         int
}

func newSavedUI() savedUI { return savedUI{now: time.Now, keeping: map[string]bool{}} }

// savedStudy is the saved lines' part of settings.json: this computer's id in the study record (savedUI.device).
type savedStudy struct {
	StudyDevice string `json:"studyDevice,omitempty"`
}

func (s *savedStudy) clean() { s.StudyDevice = strings.TrimSpace(s.StudyDevice) }

// savedHooks: the open library's saved lines are read once its films are, and again when another library opens; lines of videos that moved follow them; the open film's lines saved without a translation take the one its subtitles have, each time they are read.
func (a *app) savedHooks() areaHooks {
	return areaHooks{
		save:           func(saved *savedSettings) { saved.savedStudy = savedStudy{StudyDevice: a.saved.device} },
		restore:        func(saved savedSettings) { a.saved.device = saved.StudyDevice },
		shelfChanged:   a.savedSync,
		libraryChanged: a.savedSync,
		afterChange: func() bool {
			followed := a.savedFollowFilm()
			return a.savedFollowSubtitles() || followed
		},
		quitting: func() {
			a.savedStopExport()
			a.CloseReview()
		},
		videosMoved: func(move func(string) (string, bool)) {
			a.savedSync()
			if a.saved.book.MoveVideos(move) {
				a.savedWrite()
			}
		},
	}
}

// savedKeys is B, which saves the line playing, or removes it from the saved lines, in the 学习 menu with a check mark while it is saved, and the menu's Saved Lines…, without a key, which lists the library's saved lines, and Review Saved Lines, N, which reviews the ones due; and the 文件 menu's Export for Anki…, which exports them.
func (a *app) savedKeys(cx *core.Scope) []shortcut {
	line := func() bool { return a.openKey != "" && a.savedPlayingLine() >= 0 }
	return []shortcut{
		{Group: groupStudy, Label: locales.SavedKeySaveLine(cx), Key: "b", When: line, Checked: func() bool { return a.LineSaved(a.savedPlayingLine()) }, Run: func() { a.ToggleSaveLine(a.savedPlayingLine()) }},
		{Group: groupStudy, Label: locales.SavedMenuItem(cx), Run: a.OpenSavedSheet},
		{Group: groupStudy, Label: locales.SavedReviewMenuItem(cx), Key: "n", Run: a.StartReview},
		{Group: groupFile, Label: locales.SavedExport(cx), When: func() bool { return !a.saved.export.busy && len(a.savedBook().Lines) > 0 }, Run: a.ExportSaved},
	}
}

// savedSync reads the saved lines of the library open now, unless they are read already. Only handlers and hooks call it, as it reads the disk.
func (a *app) savedSync() {
	dir := a.snap().LibraryDir
	if a.saved.book.Dir() == dir && dir != "" {
		return
	}
	book, err := a.savedLoad(dir)
	a.saved.book = book
	if err != nil {
		a.saveErr = a.messages().SavedReadFailed()
	}
	if a.saved.book.Migrated() {
		a.savedWrite()
	}
}

// savedLoad reads the saved lines of the library at dir (saved.Load), to be changed on this computer by the saved lines' clock (savedOwn).
func (a *app) savedLoad(dir string) (saved.Book, error) {
	book, err := saved.Load(dir)
	return a.savedOwn(book), err
}

// savedOwn is book, read from its folder (saved.Load), to be changed on this computer by the saved lines' clock. The first time, this computer is given its id in the study record, which settings.json keeps from then on. UI thread only.
func (a *app) savedOwn(book saved.Book) saved.Book {
	if a.saved.device == "" {
		a.saved.device = study.NewID()
		_ = a.saveSettingsChanged()
	}
	book.Device = a.saved.device
	book.Now = func() time.Time { return a.saved.now() }
	return book
}

// savedBook is the saved lines of the library open now, none while they are not read yet (savedSync) or while the library is locked (lockShut), for the bindings.
func (a *app) savedBook() saved.Book {
	if a.saved.book.Dir() != a.snap().LibraryDir || a.lockShut() {
		return saved.Book{}
	}
	return a.saved.book
}

// savedWrite keeps the saved lines on disk, saying so on the picture when they cannot be kept.
func (a *app) savedWrite() {
	if err := a.saved.book.Save(); err != nil {
		a.flash(a.messages().SavedWriteFailed())
	}
}

// savedPlayingLine is the index of the open film's line at the playhead, or between lines the one just heard, -1 with none.
func (a *app) savedPlayingLine() int {
	cues := a.playerCues()
	if len(cues) == 0 {
		return -1
	}
	prev, cur, _ := film.CueNear(cues, a.playerLineAt())
	if cur < 0 {
		return prev
	}
	return cur
}

// savedFollowSubtitles keeps the open film's saved lines as its subtitles say them now (saved.Book.FollowSubtitles): those that have no translation, as lines saved while it was still being translated, take the ones it has now, and those saved before its transcript was proofread take the corrected originals. It runs once each time the film's subtitles are read, as when its translation finishes and the file is written again, and reports whether it changed a saved line.
func (a *app) savedFollowSubtitles() bool {
	cues, video := a.playerCues(), a.player.film.video
	if len(cues) == 0 || video == "" {
		return false
	}
	if last := a.saved.filled; len(last) == len(cues) && &last[0] == &cues[0] {
		return false
	}
	a.saved.filled = cues
	a.savedSync()
	book := &a.saved.book
	filled := book.FollowSubtitles(book.Key(video), func(start time.Duration) (string, string, bool) {
		for _, cue := range cues {
			if saved.SameStart(cue.Start, start) {
				translation, original := film.SplitCue(cue.Text)
				return translation, original, true
			}
		}
		return "", "", false
	}, media.Proofreads)
	if filled {
		a.savedWrite()
	}
	return filled
}

// savedFollowFilm has the subtitle list of a film opened since the Saved chip was picked show all its lines again, and reports whether it changed that.
func (a *app) savedFollowFilm() bool {
	if a.saved.onlyFor == a.openKey {
		return false
	}
	a.saved.onlyFor = a.openKey
	if !a.saved.only {
		return false
	}
	a.saved.only = false
	return true
}

// SavedOnly is whether the player's subtitle list shows the saved lines alone (the Saved chip).
func (a *app) SavedOnly() *bool { return &a.saved.only }

// SavedLines is the indexes of the open film's lines that are saved, in order.
func (a *app) SavedLines() []int {
	cues := a.playerCues()
	if len(cues) == 0 || a.player.film.video == "" {
		return nil
	}
	book := a.savedBook()
	key := book.Key(a.player.film.video)
	var out []int
	for _, l := range book.Lines {
		if l.Video != key {
			continue
		}
		if i := sort.Search(len(cues), func(i int) bool { return cues[i].Start > l.Start-time.Millisecond }); i < len(cues) && cues[i].Start < l.Start+time.Millisecond {
			out = append(out, i)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// LineSaved reports whether line i of the open film is among the saved lines.
func (a *app) LineSaved(i int) bool {
	cues := a.playerCues()
	if i < 0 || i >= len(cues) || a.player.film.video == "" {
		return false
	}
	book := a.savedBook()
	return book.Find(book.Key(a.player.film.video), cues[i].Start) >= 0
}

// ToggleSaveLine is B, the Save Line of a line's right-click menu and of the card of the line the film is held on: line i of the open film goes into the saved lines with its two lines as they read now, its sound cut to be kept with it (savedKeepSound), or out of them with its sound, and the picture says which.
func (a *app) ToggleSaveLine(i int) {
	cues := a.playerCues()
	if i < 0 || i >= len(cues) || a.player.film.video == "" {
		return
	}
	a.savedSync()
	cue := cues[i]
	translation, original := film.SplitCue(cue.Text)
	title := ""
	if item, ok := a.openFilm(); ok {
		title = item.Name
		if title == "" {
			title = item.Title
		}
	}
	book := &a.saved.book
	line := saved.Line{Video: book.Key(a.player.film.video), Start: cue.Start, End: cue.End, Original: original, Translation: translation, Film: title, SavedAt: a.saved.now()}
	if book.Toggle(line) {
		a.flash(a.messages().SavedOsdSaved())
		a.savedWrite()
		a.savedKeepSound(book.Lines[0])
		return
	}
	a.flash(a.messages().SavedOsdRemoved())
	a.savedWrite()
	if n := len(book.Removed); n > 0 {
		a.savedDropSound(book.Removed[n-1].ID)
	}
}

// ExportSaved is Export for Anki…: in the folder the user picks, the saved lines' notes and their sound (saved.Export), cut with ffmpeg off the engine's goroutine. The Saved Lines sheet opens to say how it goes and what it wrote.
func (a *app) ExportSaved() {
	if a.saved.export.busy {
		return
	}
	a.savedSync()
	a.OpenSavedSheet()
	a.pickFolder(core.PickDirectoriesOptions{}, func(dir string) {
		a.savedStopExport()
		ctx, cancel := context.WithCancel(context.Background())
		a.saved.export.gen++
		gen := a.saved.export.gen
		book := a.saved.book
		book.Lines = slices.Clone(book.Lines)
		delays := maps.Clone(a.player.delays)
		ffmpeg, update := a.ffmpegPath(), a.update
		a.saved.export = savedExport{busy: true, total: len(book.Lines), noFFmpeg: ffmpeg == "", cancel: cancel, gen: gen}
		report := func(fn func(e *savedExport)) {
			update(func() {
				if a.saved.export.gen == gen {
					fn(&a.saved.export)
				}
			})
		}
		go func() {
			result, err := saved.Export(ctx, ffmpeg, book, func(video string) time.Duration { return delays[video] }, dir, func(done, total int) {
				report(func(e *savedExport) { e.done, e.total = done, total })
			})
			report(func(e *savedExport) {
				cancel()
				*e = savedExport{finished: true, result: result, err: err, noFFmpeg: e.noFFmpeg, gen: gen, total: e.total, done: e.done}
			})
		}()
	})
}

// savedStopExport stops the export running, as the user cancels it, the app quits or another starts.
func (a *app) savedStopExport() {
	if a.saved.export.cancel != nil {
		a.saved.export.cancel()
		a.saved.export.cancel = nil
	}
}
