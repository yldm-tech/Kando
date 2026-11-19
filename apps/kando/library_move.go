package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/remote"
	"kando/apps/kando/saved"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// 移到资料库 moves a film to another library Kando knows, from the film's ⋯ on its card and on its page, from the menu bar while it is open, and over the socket (films.move). Its files go from the library's folder into a folder of their own in the other's (media.FilmMove): renamed within one volume, copied and then removed from where they were across volumes, with the copy's progress in the message bar. Everything Kando keeps of the film by its video's path goes with it: its job, where it stopped, how far it was played, when it last played, its subtitle delay and place, the title given to it in Kando (all in library.json, which every library shares) and its saved lines, which each library keeps in its own folder (saved.FileName). A film whose video is kept outside the library, as a local video picked where it was, moves without its files. The message bar then offers to open the other library and to undo the move.

// libraryMoving is the state of 移到资料库.
type libraryMoving struct {
	// reach is whether each known library's folder was there when last looked (libraryMoveLook), by its cleaned path; one not looked at yet counts as there. looking numbers the looks, so one that ends after a later one is dropped.
	reach   map[string]bool
	looking int
	// run is the films being moved, one after another, nil while none is; copying is the one being copied to another volume, nil while none is.
	run     *libraryMoveRun
	copying *libraryMoveCopy
	// undo is the move the message bar's Undo puts back, while the bar offers it.
	undo libraryMoveBack
}

// libraryMoveCopy is a move being copied to another volume: the film's name and the other library's, for the message bar, which film of how many of its run it is (at, total), the libraries it goes from and to, whose encrypted images stay attached meanwhile (lockNeeds), how far the copy has gone, in percent, and cancel, which stops it.
type libraryMoveCopy struct {
	name, library string
	from, to      string
	at, total     int
	percent       int
	cancel        context.CancelFunc
}

// libraryMoveBack is a move done: the moved films' keys in the library they went to, and the libraries they went from and to.
type libraryMoveBack struct {
	keys     []string
	from, to string
}

// libraryMoveRun is the move of a film, or of several selected, from the library at from to the one at to, a unit after another (films.Unit), as libraryMoveFilms starts it: m is the language its errors are said in, undo whether the message bar offers to undo it, done what hears how it ended, when it is not nil. next is the unit to move next and at how many films were begun; moved are the keys in to of the films moved, left whether some files of one could not be removed from from, failed how many did not move and err why the first of them did not, stopped whether the user stopped it, and synced whether a unit moved without the library page reading its folder again since, which the run's end does.
type libraryMoveRun struct {
	m        locales.Messages
	from, to string
	library  string
	units    []films.Unit
	total    int
	undo     bool
	done     func(keys []string, err error)
	next, at int
	moved    []string
	left     bool
	failed   int
	err      error
	stopped  bool
	synced   bool
}

// libraryMoveErr is why a film did not move: text in the words of the messages it was asked in, ui in the interface's language, for the message bar, with the socket's code for it (remote.Code…).
type libraryMoveErr struct {
	code, text, ui string
	err            error
}

// libraryMoveRefusal is the libraryMoveErr with code that say words, in m and in the interface's language.
func (a *app) libraryMoveRefusal(m locales.Messages, code string, say func(locales.Messages) string) *libraryMoveErr {
	return &libraryMoveErr{code: code, text: say(m), ui: say(a.messages())}
}

func (e *libraryMoveErr) Error() string { return e.text }

func (e *libraryMoveErr) Unwrap() error { return e.err }

// libraryMoveRename moves a film's files within one volume (media.FilmMove.Rename), and libraryMoveCopyFiles copies them to another (media.FilmMove.Copy). Tests replace them, to take the copy that moves the files across volumes and to hold it until it is stopped.
var (
	libraryMoveRename    = media.FilmMove.Rename
	libraryMoveCopyFiles = media.FilmMove.Copy
)

// libraryMoveLook reads, off the UI thread, which known libraries' folders are there, so 移到资料库 lists one on a drive that is not plugged in without letting it be chosen. libraryHooks runs it once the window exists, when another library opens, when the app comes to the front, as after a drive was plugged in, and after each move. The open library is not looked at, as 移到资料库 never lists it (libraryMoveItems), and with no other library there is nothing to look at and nothing is posted through update: at startup the report that adds the files opened with Kando (libraryPostOpened) is then the only one ready posts.
func (a *app) libraryMoveLook() {
	if a.update == nil {
		return
	}
	open := filepath.Clean(a.snap().LibraryDir)
	dirs := slices.DeleteFunc(a.knownLibraries(), func(dir string) bool { return filepath.Clean(dir) == open })
	a.lib.move.looking++
	if len(dirs) == 0 {
		return
	}
	run := a.lib.move.looking
	update := a.update
	go func() {
		reach := make(map[string]bool, len(dirs))
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			reach[filepath.Clean(dir)] = err == nil && info.IsDir()
		}
		update(func() {
			if run == a.lib.move.looking {
				a.lib.move.reach = reach
			}
		})
	}()
}

// libraryMoveItems are the items of the submenu 移到资料库 for items, a film of the open library or several selected: each other library Kando knows, named as File › 切换资料库 names them, disabled while its folder is not there or another move is going on. wrap makes an item's OnSelect from what it does: live.Do for the film's menus, menuSelect for the menu bar's. It is none for films without a video, or when Kando knows no other library.
func (a *app) libraryMoveItems(cx *ns.Scope, items []films.Film, wrap func(func()) func()) []nsdesktop.MenuItem {
	if !slices.ContainsFunc(items, func(item films.Film) bool { return item.VideoPath != "" }) {
		return nil
	}
	open := filepath.Clean(a.snap().LibraryDir)
	dirs := a.knownLibraries()
	labels := films.LibraryLabels(locales.For(cx), dirs)
	var out []nsdesktop.MenuItem
	for i, dir := range dirs {
		if filepath.Clean(dir) == open {
			continue
		}
		reach, looked := a.lib.move.reach[filepath.Clean(dir)]
		out = append(out, nsdesktop.MenuItem{
			Label:    a.lockLibraryLabel(cx, dir, labels[i]),
			Disabled: (looked && !reach) || a.lib.move.run != nil,
			OnSelect: wrap(func() { a.MoveFilms(items, dir) }),
		})
	}
	return out
}

// MoveFilmMenu is the submenu 移到资料库 of a film's ⋯ on the library page, or of the menu of the films selected (films.App).
func (a *app) MoveFilmMenu(cx *ns.Scope, items ...films.Film) []nsdesktop.MenuItem {
	return a.libraryMoveItems(cx, items, func(fn func()) func() { return live.Do(a, fn) })
}

// MoveOpenFilmMenu is the submenu 移到资料库 of the film page's ⋯, for the open film (film.App).
func (a *app) MoveOpenFilmMenu(cx *ns.Scope) []nsdesktop.MenuItem {
	item, ok := a.libraryMoveOpenFilm()
	if !ok {
		return nil
	}
	return a.MoveFilmMenu(cx, item)
}

// libraryMoveOpenFilm is the open film, when it is a film of the open library's.
func (a *app) libraryMoveOpenFilm() (films.Film, bool) {
	if a.openKey == "" {
		return films.Film{}, false
	}
	return films.Find(a.films(), a.openKey)
}

// libraryMoveRow is the File menu's 移到资料库 for the films it acts on (libraryTargets): the film open, or those selected on the library page; a submenu of the other libraries, without a key.
func (a *app) libraryMoveRow(cx *ns.Scope) shortcut {
	return shortcut{Group: groupFile, Label: locales.FilmsMoveToLibrary(cx),
		When: func() bool {
			return len(a.libraryMoveItems(cx, a.libraryTargets(), func(fn func()) func() { return fn })) > 0
		},
		Items: func() []nsdesktop.MenuItem {
			return a.libraryMoveItems(cx, a.libraryTargets(), func(fn func()) func() { return a.menuSelect(false, fn) })
		}}
}

// MoveFilm moves item, a film of the open library, to the library at dir, as MoveFilms does.
func (a *app) MoveFilm(item films.Film, dir string) { a.MoveFilms([]films.Film{item}, dir) }

// MoveFilms moves items, films of the open library, to the library at dir, one after another, and says in the message bar why it cannot, how far a copy to another volume has gone and how it ended, once for them all, offering to open dir and to undo the move once it is done. A locked dir asks for its password first (lockThen), as switching to it does, once for them all.
func (a *app) MoveFilms(items []films.Film, dir string) {
	a.lockThen(dir, func() {
		if err := a.libraryMoveFilms(a.messages(), items, a.snap().LibraryDir, dir, true, nil); err != nil {
			a.warn(err.Error(), "")
		}
	})
}

// UndoMove moves the films the message bar says were moved back to the library they came from, as the bar's Undo does. When the library they went to is locked, its password is asked for first (lockThen). A film no longer there is left out, and the bar says so when none is.
func (a *app) UndoMove() {
	back := a.lib.move.undo
	a.lib.move.undo = libraryMoveBack{}
	a.lib.notice.Open = false
	if len(back.keys) == 0 {
		return
	}
	a.lockThen(back.to, func() {
		there := a.libraryMoveFilmsOf(back.to)
		var items []films.Film
		for _, key := range back.keys {
			if item, ok := films.Find(there, key); ok {
				items = append(items, item)
			}
		}
		if len(items) == 0 {
			a.warn(a.messages().LibraryMoveGone(), "")
			return
		}
		if err := a.libraryMoveFilms(a.messages(), items, back.to, back.from, false, nil); err != nil {
			a.warn(err.Error(), "")
		}
	})
}

// CancelMove stops the move going on, as the message bar's 取消 does: the copy of a film to another volume, whose files stay where they were, and the films after it. The films moved before it stay moved.
func (a *app) CancelMove() {
	if r := a.lib.move.run; r != nil {
		r.stopped = true
	}
	if c := a.lib.move.copying; c != nil {
		c.cancel()
	}
}

// libraryMoveFilmsOf is the films of the library at dir as its shelf lists them: the open library's, or another's from its folder, read here.
func (a *app) libraryMoveFilmsOf(dir string) []films.Film {
	if filepath.Clean(dir) == filepath.Clean(a.snap().LibraryDir) {
		return a.films()
	}
	videos, _ := media.ListVideos(dir)
	return films.List(dir, a.library.Jobs, videos, a.library.Names, jobName, a.statusOf)
}

// libraryMoveLabel is the name the menus give the library at dir among the libraries Kando knows (films.LibraryLabels), in the language of m.
func (a *app) libraryMoveLabel(m locales.Messages, dir string) string {
	dirs := a.knownLibraries()
	labels := films.LibraryLabels(m, dirs)
	for i, d := range dirs {
		if filepath.Clean(d) == filepath.Clean(dir) {
			return labels[i]
		}
	}
	return sidebar.Title(dir)
}

// libraryMoveFilm moves item, a film of the library at from, to the library at to, as libraryMoveFilms does a film alone; done, when it is not nil, hears the film's key in to, or why it did not move.
func (a *app) libraryMoveFilm(m locales.Messages, item films.Film, from, to string, undo bool, done func(key string, err error)) error {
	var heard func([]string, error)
	if done != nil {
		heard = func(keys []string, err error) {
			key := ""
			if len(keys) > 0 {
				key = keys[0]
			}
			done(key, err)
		}
	}
	return a.libraryMoveFilms(m, []films.Film{item}, from, to, undo, heard)
}

// libraryMoveFilms moves items, films of the library at from, to the library at to, one after another, and returns why it cannot, in the words of m, before anything moved: another move is going on, to is not a library Kando knows, from or to is locked and its password was not typed for this move (lockBars), to is from itself or its folder is not there, or library.json could not be read, so it could not be written without losing what is in it. The locks are passed once, as the move starts. A film that cannot move then, as one being processed, one without its video, or one whose library's saved lines could not be read, is left where it is and the others move on (libraryMoveUnitStart). The films of a folder whose every video goes with them move with the folder (films.Units). Within one volume each moves at once; across volumes each is copied off the UI thread, the message bar saying which of how many and how far it has gone, and moved once the copy is whole. The message bar says how it ended once, in the interface's language, offering to open to and, with undo, to move back the films moved (libraryMoveEnded); done, when it is not nil, hears their keys in to, or why the first that did not move did not, in the words of m. An open film it moves is closed first.
func (a *app) libraryMoveFilms(m locales.Messages, items []films.Film, from, to string, undo bool, done func(keys []string, err error)) error {
	from, to = filepath.Clean(from), filepath.Clean(to)
	refuse := func(code string, say func(locales.Messages) string) error { return a.libraryMoveRefusal(m, code, say) }
	switch {
	case len(items) == 0:
		return nil
	case a.lib.move.run != nil || a.lib.move.copying != nil:
		return refuse(remote.CodeNotReady, locales.Messages.LibraryMoveBusy)
	case !slices.ContainsFunc(a.knownLibraries(), func(dir string) bool { return filepath.Clean(dir) == to }):
		return refuse(remote.CodeNotFound, func(m locales.Messages) string { return m.LibraryMoveUnknown(to) })
	case a.lockBars(from):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LockLibraryIsLocked(sidebar.Title(from)) })
	case a.lockBars(to):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LockLibraryIsLocked(sidebar.Title(to)) })
	case to == from && len(items) == 1:
		name := films.Name(items[0])
		return refuse(remote.CodeInvalid, func(m locales.Messages) string { return m.LibraryMoveSame(name, a.libraryMoveLabel(m, to)) })
	case to == from:
		return nil
	case a.library.Unread() != nil:
		return refuse(remote.CodeFailed, locales.Messages.LibraryShelfNotSaved)
	}
	if info, err := os.Stat(to); err != nil || !info.IsDir() {
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveUnreachable(a.libraryMoveLabel(m, to)) })
	}
	// A single film is refused at once, as before anything moved; of several, those that cannot move are counted and the others move on.
	if len(items) == 1 {
		if err := a.libraryMoveCheck(m, items[0], from); err != nil {
			return err
		}
	}
	run := &libraryMoveRun{m: m, from: from, to: to, library: a.libraryMoveLabel(m, to), units: films.Units(from, items), total: len(items), undo: undo, done: done}
	a.lib.move.run = run
	a.libraryMoveStep(run)
	return nil
}

// libraryMoveCheck is why item, a film of the library at from, cannot move now, in the words of m, nil when it can: it has no video, it is being processed, or its video is no longer where it was.
func (a *app) libraryMoveCheck(m locales.Messages, item films.Film, from string) error {
	name := films.Name(item)
	refuse := func(code string, say func(locales.Messages) string) error { return a.libraryMoveRefusal(m, code, say) }
	switch {
	case item.VideoPath == "":
		return refuse(remote.CodeInvalid, func(m locales.Messages) string { return m.LibraryMoveNoVideo(name) })
	case a.libraryMoveBusy(item):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveProcessing(name) })
	}
	// A film that keeps no video (core.Job.SubtitlesOnly) is on disk as its subtitles, whose folder moves.
	kept := item.VideoPath
	if item.SubtitlesOnly && item.SubtitlePath != "" {
		kept = item.SubtitlePath
	}
	if films.InDir(from, item.VideoPath) {
		if _, err := os.Stat(kept); err != nil {
			return refuse(remote.CodeFailed, locales.Messages.LibraryFileMissing)
		}
	}
	return nil
}

// libraryMoveStep moves the run's units from the next on, until one is being copied to another volume, which takes the run on once its copy ends, or there are none left or the run was stopped, when it ends (libraryMoveEnded). A unit that cannot move is counted and the next goes on.
func (a *app) libraryMoveStep(run *libraryMoveRun) {
	for run.next < len(run.units) && !run.stopped {
		unit := run.units[run.next]
		run.next++
		copying, err := a.libraryMoveUnitStart(run, unit)
		if err != nil {
			run.fail(len(unit.Items), err)
			continue
		}
		if copying {
			return
		}
	}
	a.libraryMoveEnded(run)
}

// fail counts n films of the run that did not move, keeping why the first did not.
func (r *libraryMoveRun) fail(n int, err error) {
	r.failed += n
	if r.err == nil {
		r.err = err
	}
}

// libraryMoveUnitStart moves unit, films of the run, and reports whether it is being copied to another volume, which goes on the run from where it ends, or why it could not move, before anything moved: one of its films cannot move now (libraryMoveCheck), or a library's saved lines could not be read, so they could not be written without losing what is in them.
func (a *app) libraryMoveUnitStart(run *libraryMoveRun, unit films.Unit) (bool, error) {
	m, from, to := run.m, run.from, run.to
	first := unit.Items[0]
	name := films.Name(first)
	run.at += len(unit.Items)
	for _, item := range unit.Items {
		if err := a.libraryMoveCheck(m, item, from); err != nil {
			return false, err
		}
	}
	source, err := a.libraryMoveBook(from)
	if err != nil {
		return false, a.libraryMoveRefusal(m, remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, from)) })
	}
	dest, err := a.libraryMoveBook(to)
	if err != nil {
		return false, a.libraryMoveRefusal(m, remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, to)) })
	}
	var plan *media.FilmMove
	switch {
	case unit.Folder != "":
		p := media.PlanFilmMove([]string{unit.Folder}, true, to)
		plan = &p
	case films.InDir(from, first.VideoPath):
		p := libraryMovePlan(from, to, first)
		plan = &p
	}
	for _, item := range unit.Items {
		if a.openKey == item.Key {
			a.closeFilm()
		}
	}
	if plan == nil || libraryMoveRename(*plan) == nil {
		keys, _ := a.libraryMoveCommit(unit.Items, from, to, plan, false, source, dest)
		run.moved = append(run.moved, keys...)
		run.synced = true
		return false, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	copying := &libraryMoveCopy{name: name, library: a.libraryMoveLabel(a.messages(), to), from: from, to: to, at: run.at, total: run.total, cancel: cancel}
	a.lib.move.copying = copying
	a.libraryMoveShowCopy()
	update, move := a.update, *plan
	go func() {
		defer cancel()
		shown := 0
		err := libraryMoveCopyFiles(move, ctx, func(n, total int64) {
			if total <= 0 {
				return
			}
			if percent := int(n * 100 / total); percent > shown {
				shown = percent
				update(func() {
					if a.lib.move.copying == copying {
						copying.percent = percent
						a.libraryMoveShowCopy()
					}
				})
			}
		})
		update(func() {
			a.lib.move.copying = nil
			if err == nil {
				err = a.libraryMoveCopied(run, unit, move)
			}
			switch {
			case errors.Is(err, context.Canceled):
				run.stopped = true
			case err != nil:
				var refused *libraryMoveErr
				if !errors.As(err, &refused) {
					err = &libraryMoveErr{code: remote.CodeFailed, text: m.LibraryMoveFailed(name, run.library, err.Error()), ui: a.messages().LibraryMoveFailed(name, a.libraryMoveLabel(a.messages(), to), err.Error()), err: err}
				}
				run.fail(len(unit.Items), err)
			}
			a.libraryMoveStep(run)
		})
	}()
	return true, nil
}

// libraryMoveCopied moves unit, whose files move has copied to the run's library, there, and removes them from where they were; or, when while it was copied one of its films started being processed or a library's saved lines became unreadable, stops its move and removes its copy, and returns why.
func (a *app) libraryMoveCopied(run *libraryMoveRun, unit films.Unit, move media.FilmMove) error {
	m, from, to := run.m, run.from, run.to
	source, err := a.libraryMoveBook(from)
	var dest *saved.Book
	if err == nil {
		dest, err = a.libraryMoveBook(to)
	}
	if err != nil {
		err = a.libraryMoveRefusal(m, remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, to)) })
	}
	for _, item := range unit.Items {
		if err == nil && a.libraryMoveBusy(item) {
			name := films.Name(item)
			err = a.libraryMoveRefusal(m, remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveProcessing(name) })
		}
	}
	if err != nil {
		move.RemoveCopy()
		return err
	}
	keys, left := a.libraryMoveCommit(unit.Items, from, to, &move, true, source, dest)
	run.moved = append(run.moved, keys...)
	run.left = run.left || left
	return nil
}

// libraryMovePlan is how item's files move from the library at from to the one at to: its folder, when it is the only video in a folder of its own (films.AloneIn, as 从资料库移除 decides), else its video, its subtitles and the files beside it that are its (media.Companions), into a new folder in to.
func libraryMovePlan(from, to string, item films.Film) media.FilmMove {
	video := item.VideoPath
	if dir := filepath.Dir(video); films.InDir(from, dir) && films.AloneIn(dir, video) {
		return media.PlanFilmMove([]string{dir}, true, to)
	}
	sources := append([]string{video}, media.Companions(video)...)
	if sub := item.SubtitlePath; sub != "" && filepath.Dir(sub) == filepath.Dir(video) && fileExists(sub) && !slices.Contains(sources, sub) {
		sources = append(sources, sub)
	}
	return media.PlanFilmMove(sources, false, to)
}

// libraryMoveBusy reports whether item is being processed: its job, or another job of its video, is waiting or running.
func (a *app) libraryMoveBusy(item films.Film) bool {
	return slices.ContainsFunc(a.library.Jobs, func(job core.Job) bool {
		return !job.Stage.Done() && (job.ID == item.JobID || (job.VideoPath != "" && filepath.Clean(job.VideoPath) == filepath.Clean(item.VideoPath)))
	})
}

// libraryMoveBook is the saved lines of the library at dir: the open library's (savedSync), or another's read from its folder. It fails when they could not be read, as they could then not be written without losing what is in them.
func (a *app) libraryMoveBook(dir string) (*saved.Book, error) {
	if filepath.Clean(dir) == filepath.Clean(a.snap().LibraryDir) {
		a.savedSync()
		return &a.saved.book, a.saved.book.Unread()
	}
	book, err := a.savedLoad(dir)
	return &book, err
}

// libraryMoveCommit points what Kando keeps of items at where their files are now in to, once plan moved or copied (copied) them, plan being nil for a film whose video is outside from, which moves without its files, and saves it: their saved lines go from source, from's book, to dest, to's (libraryMoveLines), their jobs take the new paths and to as their library, and what library.json and the areas keep by their videos' paths follows them (moveVideos). After a copy it then removes the files from from, and reports left when some could not be, and the library page reads its folder again; after a rename the run's end does that once for all its films (libraryMoveEnded). It returns the films' keys in to.
func (a *app) libraryMoveCommit(items []films.Film, from, to string, plan *media.FilmMove, copied bool, source, dest *saved.Book) (keys []string, left bool) {
	move := func(path string) (string, bool) {
		if plan == nil {
			return "", false
		}
		return plan.Path(path)
	}
	jobs := map[string]bool{}
	for _, item := range items {
		a.libraryMoveLines(source, dest, item.VideoPath, move)
		key := item.Key
		if next, ok := move(item.VideoPath); ok && item.JobID == "" {
			key = next
		}
		keys = append(keys, key)
		if item.JobID != "" {
			jobs[item.JobID] = true
		}
	}
	a.moveVideos(move)
	for i, job := range a.library.Jobs {
		if !jobs[job.ID] {
			continue
		}
		if next, ok := move(job.VideoPath); ok {
			a.library.Jobs[i].VideoPath = next
		}
		if next, ok := move(job.SubtitlePath); ok {
			a.library.Jobs[i].SubtitlePath = next
		}
		a.library.Jobs[i].LibraryDir = to
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
	}
	if copied {
		left = plan.RemoveSources() != nil
		a.libraryMoveRefresh(from, to)
	}
	return keys, left
}

// libraryMoveRefresh reads the open library's folder again when films moved out of it or into it, and counts the films of the others.
func (a *app) libraryMoveRefresh(from, to string) {
	if open := filepath.Clean(a.snap().LibraryDir); open == from || open == to {
		a.refreshLibrary()
	}
	a.countLibraries()
}

// libraryMoveLines moves the saved lines of the video at video from source to dest, keyed in dest by its path once moved (move), and saves both books, dest first, so a line is never in neither, and then the sound kept of each line goes with it. When dest cannot be saved the lines stay in source, and the sidebar says so.
func (a *app) libraryMoveLines(source, dest *saved.Book, video string, move func(string) (string, bool)) {
	next, ok := move(video)
	if !ok {
		next = video
	}
	before := slices.Clone(source.Lines)
	lines := source.TakeVideo(video)
	if len(lines) == 0 {
		return
	}
	dest.AddVideo(next, lines)
	if err := dest.Save(); err != nil {
		source.Lines = before
		dest.TakeVideo(next)
		a.saveErr = a.messages().SavedWriteFailed()
		return
	}
	if err := source.Save(); err != nil {
		a.saveErr = a.messages().SavedWriteFailed()
	}
	// The sound kept of each line goes with it (saved.StoredSound), under the id it has in dest.
	key := dest.Key(next)
	for _, l := range lines {
		if i := dest.Find(key, l.Start); i >= 0 {
			if err := saved.MoveSound(*source, l.ID, *dest, dest.Lines[i].ID); err != nil {
				log.Printf("library: the sound of the line %s did not move: %v", l.ID, err)
			}
		}
	}
}

// libraryMoveEnded ends run: the library page reads its folders again if a film moved at once since it last did, and the message bar says how the run ended, in the interface's language, offering to open its library and, with undo, Undo, which moves back the films it moved (libraryMoveSay). done, when set, hears the keys of the films moved, or why the first that did not move did not, or that it was stopped, in the words of the run's messages.
func (a *app) libraryMoveEnded(run *libraryMoveRun) {
	if a.lib.move.run != run {
		return
	}
	a.lib.move.run = nil
	if run.synced {
		a.libraryMoveRefresh(run.from, run.to)
	}
	a.libraryMoveLook()
	err := run.err
	if run.stopped && len(run.moved) == 0 && run.total == 1 {
		name := films.Name(run.units[0].Items[0])
		err = &libraryMoveErr{code: remote.CodeFailed, text: run.m.LibraryMoveStopped(name), err: context.Canceled}
	}
	a.libraryMoveSay(run)
	if run.done != nil {
		if len(run.moved) > 0 {
			err = nil
		}
		run.done(run.moved, err)
	}
}

// libraryMoveSay says in the message bar how run ended, in the interface's language. A single film: moved, with 打开 its library and, with undo, Undo; moved with some of its files left behind; stopped; or why it did not move, its files where they were. Several: how many moved, or how many of how many and why the first that did not move did not, or that it was stopped and how many had moved, with 打开 and Undo for those, or else that some of their files were left behind.
func (a *app) libraryMoveSay(run *libraryMoveRun) {
	ui := a.messages()
	library, from := a.libraryMoveLabel(ui, run.to), a.libraryMoveLabel(ui, run.from)
	reason := ""
	if run.err != nil {
		reason = run.err.Error()
		var refused *libraryMoveErr
		if errors.As(run.err, &refused) && refused.ui != "" {
			reason = refused.ui
		}
	}
	moved := len(run.moved)
	single := run.total == 1
	var text, kind string
	switch {
	case moved == 0 && run.stopped && single:
		a.tell(ui.LibraryMoveStopped(films.Name(run.units[0].Items[0])))
		return
	case moved == 0 && run.stopped:
		text, kind = ui.LibraryMoveStoppedMany(0, library), "success"
	case moved == 0:
		a.warn(reason, "")
		return
	case run.left && single:
		text, kind = ui.LibraryMovedLeft(films.Name(run.units[0].Items[0]), library, from), "warning"
	case run.left:
		text, kind = ui.LibraryMovedManyLeft(moved, library, from), "warning"
	case single:
		text, kind = ui.LibraryMoved(films.Name(run.units[0].Items[0]), library), "success"
	case run.stopped:
		text, kind = ui.LibraryMoveStoppedMany(moved, library), "success"
	case run.failed > 0:
		text, kind = ui.LibraryMovedSome(moved, run.total, library, reason), "warning"
	default:
		text, kind = ui.LibraryMovedMany(moved, library), "success"
	}
	undo := run.undo && !run.left && moved > 0
	n := films.Notice{Kind: kind, Text: text, Undo: undo, Open: true}
	if moved > 0 && run.to != filepath.Clean(a.snap().LibraryDir) {
		n.Library = run.to
	}
	a.libraryShow(n)
	a.lib.move.undo = libraryMoveBack{}
	if undo {
		a.lib.move.undo = libraryMoveBack{keys: slices.Clone(run.moved), from: run.from, to: run.to}
	}
}

// libraryMoveShowCopy says in the message bar how far the copy of a film to another volume has gone, with 取消, which stops it.
func (a *app) libraryMoveShowCopy() {
	c := a.lib.move.copying
	if c == nil {
		return
	}
	n := &a.lib.notice
	text := a.messages().LibraryMoving(c.name, c.library, c.percent)
	if c.total > 1 {
		text = a.messages().LibraryMovingMany(c.at, c.total, c.library, c.percent)
	}
	if n.Open && n.Stop {
		n.Text = text
		return
	}
	// A bar the user closed while the copy goes on stays closed until the copy ends; the copy of the next film of a run shows it again.
	if c.percent == 0 {
		a.libraryShow(films.Notice{Kind: "success", Text: text, Stop: true, Open: true})
	}
}

// libraryMoveQuit stops a move still going as the app quits: the copy, which removes what it copied, and the films after it.
func (a *app) libraryMoveQuit() { a.CancelMove() }

// remoteMove moves film p.ID of the open library to the library p.To names, by its path or its name, as 移到资料库 does, and answers once it moved, which across volumes takes as long as copying the film does.
func (a *app) remoteMove(ctx context.Context, p remote.MoveParams) (remote.Moved, error) {
	type ended struct {
		key string
		err error
	}
	end := make(chan ended, 1)
	to, err := remote.OnUI(ctx, a.update, func() (string, error) {
		item, err := a.remoteFind(p.ID)
		if err != nil {
			return "", err
		}
		to, err := a.remoteLibrary(p.To)
		if err != nil {
			return "", err
		}
		err = a.libraryMoveFilm(remoteMessages(), item, a.snap().LibraryDir, to, true, func(key string, err error) { end <- ended{key, err} })
		return to, remoteMoveErr(err)
	})
	if err != nil {
		return remote.Moved{}, err
	}
	select {
	case e := <-end:
		if e.err != nil {
			return remote.Moved{}, remoteMoveErr(e.err)
		}
		return remote.Moved{ID: e.key, Library: to, Name: sidebar.Title(to)}, nil
	case <-ctx.Done():
		return remote.Moved{}, remote.Errorf(remote.CodeFailed, "Kando is still moving the film; its window shows how far it has gone")
	}
}

// remoteMoveErr is err, a libraryMoveErr or nil, as the socket answers it.
func remoteMoveErr(err error) error {
	var e *libraryMoveErr
	if errors.As(err, &e) {
		return remote.Errorf(e.code, "%s", e.text)
	}
	return err
}

// remoteLibrary is the library Kando knows that given names: by its path, or by its name as the window shows it, ignoring case. UI thread only.
func (a *app) remoteLibrary(given string) (string, error) {
	given = strings.TrimSpace(given)
	if given == "" {
		return "", remote.Errorf(remote.CodeInvalid, "name the library to move the film to")
	}
	dirs := a.knownLibraries()
	labels := films.LibraryLabels(remoteMessages(), dirs)
	var found []string
	for i, dir := range dirs {
		switch {
		case filepath.IsAbs(given) && filepath.Clean(dir) == filepath.Clean(given):
			return dir, nil
		case strings.EqualFold(labels[i], given) || strings.EqualFold(sidebar.Title(dir), given):
			found = append(found, dir)
		}
	}
	switch len(found) {
	case 0:
		return "", remote.Errorf(remote.CodeNotFound, "no library %q; kando status lists the libraries", given)
	case 1:
		return found[0], nil
	}
	return "", remote.Errorf(remote.CodeInvalid, "more than one library is called %q; name it by its path", given)
}
