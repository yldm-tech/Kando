package main

import (
	"context"
	"errors"
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
	// copying is the move being copied to another volume, nil while none is.
	copying *libraryMoveCopy
	// undo is the move the message bar's Undo puts back, while the bar offers it.
	undo libraryMoveBack
}

// libraryMoveCopy is a move being copied to another volume: the film's name and the other library's, for the message bar, the libraries it goes from and to, whose encrypted images stay attached meanwhile (lockNeeds), how far the copy has gone, in percent, and cancel, which stops it.
type libraryMoveCopy struct {
	name, library string
	from, to      string
	percent       int
	cancel        context.CancelFunc
}

// libraryMoveBack is a move done: the moved film's key in the library it went to, and the libraries it went from and to.
type libraryMoveBack struct {
	key, from, to string
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

// libraryMoveLook reads, off the UI thread, which known libraries' folders are there, so 移到资料库 lists one on a drive that is not plugged in without letting it be chosen. libraryHooks runs it once the window exists, when another library opens, when the app comes to the front, as after a drive was plugged in, and after each move.
func (a *app) libraryMoveLook() {
	if a.update == nil {
		return
	}
	dirs := a.knownLibraries()
	a.lib.move.looking++
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

// libraryMoveItems are the items of the submenu 移到资料库 for item, a film of the open library: each other library Kando knows, named as File › 切换资料库 names them, disabled while its folder is not there or another film is being copied. wrap makes an item's OnSelect from what it does: live.Do for the film's menus, menuSelect for the menu bar's. It is none for a film without a video, or when Kando knows no other library.
func (a *app) libraryMoveItems(cx *ns.Scope, item films.Film, wrap func(func()) func()) []nsdesktop.MenuItem {
	if item.VideoPath == "" {
		return nil
	}
	open := filepath.Clean(a.snap().LibraryDir)
	dirs := a.knownLibraries()
	labels := films.LibraryLabels(locales.For(cx), dirs)
	var items []nsdesktop.MenuItem
	for i, dir := range dirs {
		if filepath.Clean(dir) == open {
			continue
		}
		reach, looked := a.lib.move.reach[filepath.Clean(dir)]
		items = append(items, nsdesktop.MenuItem{
			Label:    a.lockLibraryLabel(cx, dir, labels[i]),
			Disabled: (looked && !reach) || a.lib.move.copying != nil,
			OnSelect: wrap(func() { a.MoveFilm(item, dir) }),
		})
	}
	return items
}

// MoveFilmMenu is the submenu 移到资料库 of a film's ⋯ on the library page (films.App).
func (a *app) MoveFilmMenu(cx *ns.Scope, item films.Film) []nsdesktop.MenuItem {
	return a.libraryMoveItems(cx, item, func(fn func()) func() { return live.Do(a, fn) })
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

// libraryMoveRow is the File menu's 移到资料库 while a film of the open library is open, a submenu of the other libraries, without a key.
func (a *app) libraryMoveRow(cx *ns.Scope) shortcut {
	return shortcut{Group: groupFile, Label: locales.FilmsMoveToLibrary(cx),
		When: func() bool {
			item, ok := a.libraryMoveOpenFilm()
			return ok && len(a.libraryMoveItems(cx, item, func(fn func()) func() { return fn })) > 0
		},
		Items: func() []nsdesktop.MenuItem {
			item, ok := a.libraryMoveOpenFilm()
			if !ok {
				return nil
			}
			return a.libraryMoveItems(cx, item, func(fn func()) func() { return a.menuSelect(false, fn) })
		}}
}

// MoveFilm moves item, a film of the open library, to the library at dir, and says in the message bar why it cannot, how far a copy to another volume has gone and how it ended, offering to open dir and to undo the move once it is done. A locked dir asks for its password first (lockThen), as switching to it does.
func (a *app) MoveFilm(item films.Film, dir string) {
	a.lockThen(dir, func() {
		if err := a.libraryMoveFilm(a.messages(), item, a.snap().LibraryDir, dir, true, nil); err != nil {
			a.warn(err.Error(), "")
		}
	})
}

// UndoMove moves the film the message bar says was moved back to the library it came from, as the bar's Undo does. When the library it went to is locked, its password is asked for first (lockThen).
func (a *app) UndoMove() {
	back := a.lib.move.undo
	a.lib.move.undo = libraryMoveBack{}
	a.lib.notice.Open = false
	if back.key == "" {
		return
	}
	a.lockThen(back.to, func() {
		item, ok := films.Find(a.libraryMoveFilmsOf(back.to), back.key)
		if !ok {
			a.warn(a.messages().LibraryMoveGone(), "")
			return
		}
		if err := a.libraryMoveFilm(a.messages(), item, back.to, back.from, false, nil); err != nil {
			a.warn(err.Error(), "")
		}
	})
}

// CancelMove stops the copy of a film to another volume, as the message bar's 取消 does; its files stay where they were.
func (a *app) CancelMove() {
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

// libraryMoveFilm moves item, a film of the library at from, to the library at to, and returns why it cannot, in the words of m, before anything moved: another film is being copied, to is not a library Kando knows, from or to is locked and its password was not typed for this move (lockBars), to is from itself or its folder is not there, the film is being processed, or library.json or either library's saved lines could not be read, so they could not be written without losing what is in them. Within one volume it moves at once; across volumes it copies the film off the UI thread, saying in the message bar how far it has gone, and moves it once the copy is whole. The message bar says how it ended, in the interface's language, offering to open to and, with undo, to move the film back; done, when it is not nil, hears the film's key in to, or why it did not move, in the words of m. An open film it moves is closed first.
func (a *app) libraryMoveFilm(m locales.Messages, item films.Film, from, to string, undo bool, done func(key string, err error)) error {
	from, to = filepath.Clean(from), filepath.Clean(to)
	name := item.Name
	if name == "" {
		name = item.Title
	}
	library := a.libraryMoveLabel(m, to)
	refuse := func(code string, say func(locales.Messages) string) error { return a.libraryMoveRefusal(m, code, say) }
	switch {
	case a.lib.move.copying != nil:
		return refuse(remote.CodeNotReady, locales.Messages.LibraryMoveBusy)
	case !slices.ContainsFunc(a.knownLibraries(), func(dir string) bool { return filepath.Clean(dir) == to }):
		return refuse(remote.CodeNotFound, func(m locales.Messages) string { return m.LibraryMoveUnknown(to) })
	case a.lockBars(from):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LockLibraryIsLocked(sidebar.Title(from)) })
	case a.lockBars(to):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LockLibraryIsLocked(sidebar.Title(to)) })
	case to == from:
		return refuse(remote.CodeInvalid, func(m locales.Messages) string { return m.LibraryMoveSame(name, a.libraryMoveLabel(m, to)) })
	case item.VideoPath == "":
		return refuse(remote.CodeInvalid, func(m locales.Messages) string { return m.LibraryMoveNoVideo(name) })
	case a.libraryMoveBusy(item):
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveProcessing(name) })
	case a.library.Unread() != nil:
		return refuse(remote.CodeFailed, locales.Messages.LibraryShelfNotSaved)
	}
	if info, err := os.Stat(to); err != nil || !info.IsDir() {
		return refuse(remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveUnreachable(a.libraryMoveLabel(m, to)) })
	}
	source, err := a.libraryMoveBook(from)
	if err != nil {
		return refuse(remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, from)) })
	}
	dest, err := a.libraryMoveBook(to)
	if err != nil {
		return refuse(remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, to)) })
	}
	var plan *media.FilmMove
	if films.InDir(from, item.VideoPath) {
		if _, err := os.Stat(item.VideoPath); err != nil {
			return refuse(remote.CodeFailed, locales.Messages.LibraryFileMissing)
		}
		p := libraryMovePlan(from, to, item)
		plan = &p
	}
	if a.openKey == item.Key {
		a.closeFilm()
	}
	finish := func(key string, left bool, err error) {
		a.libraryMoveEnded(m, name, library, from, to, key, left, undo, err, done)
	}
	if plan == nil || libraryMoveRename(*plan) == nil {
		key, _ := a.libraryMoveCommit(item, from, to, plan, false, source, dest)
		finish(key, false, nil)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	copying := &libraryMoveCopy{name: name, library: library, from: from, to: to, cancel: cancel}
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
			if err != nil {
				finish("", false, err)
				return
			}
			// While the film was copied it may have started being processed, or a library's saved lines may have become unreadable; the move then stops and its copy goes.
			source, err := a.libraryMoveBook(from)
			var dest *saved.Book
			if err == nil {
				dest, err = a.libraryMoveBook(to)
			}
			switch {
			case err != nil:
				err = a.libraryMoveRefusal(m, remote.CodeFailed, func(m locales.Messages) string { return m.LibraryMoveLinesUnread(a.libraryMoveLabel(m, to)) })
			case a.libraryMoveBusy(item):
				err = a.libraryMoveRefusal(m, remote.CodeNotReady, func(m locales.Messages) string { return m.LibraryMoveProcessing(name) })
			}
			if err != nil {
				move.RemoveCopy()
				finish("", false, err)
				return
			}
			key, left := a.libraryMoveCommit(item, from, to, &move, true, source, dest)
			finish(key, left, nil)
		})
	}()
	return nil
}

// libraryMovePlan is how item's files move from the library at from to the one at to: its folder, when it is the only video in a folder of its own (aloneIn, as 从资料库移除 decides), else its video, its subtitles and the files beside it that are its (media.Companions), into a new folder in to.
func libraryMovePlan(from, to string, item films.Film) media.FilmMove {
	video := item.VideoPath
	if dir := filepath.Dir(video); films.InDir(from, dir) && aloneIn(dir, video) {
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
	book, err := saved.Load(dir)
	return &book, err
}

// libraryMoveCommit points what Kando keeps of item at where its files are now in to, once plan moved or copied (copied) them, plan being nil for a film whose video is outside from, which moves without its files, and saves it: its saved lines go from source, from's book, to dest, to's (libraryMoveLines), its job takes the new paths and to as its library, and what library.json and the areas keep by its video's path follows it (moveVideos). After a copy it then removes the files from from, and reports left when some could not be. It returns the film's key in to.
func (a *app) libraryMoveCommit(item films.Film, from, to string, plan *media.FilmMove, copied bool, source, dest *saved.Book) (key string, left bool) {
	move := func(path string) (string, bool) {
		if plan == nil {
			return "", false
		}
		return plan.Path(path)
	}
	a.libraryMoveLines(source, dest, item.VideoPath, move)
	a.moveVideos(move)
	key = item.Key
	if next, ok := move(item.VideoPath); ok && item.JobID == "" {
		key = next
	}
	for i, job := range a.library.Jobs {
		if item.JobID == "" || job.ID != item.JobID {
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
	}
	if open := filepath.Clean(a.snap().LibraryDir); open == from || open == to {
		a.refreshLibrary()
	}
	a.countLibraries()
	return key, left
}

// libraryMoveLines moves the saved lines of the video at video from source to dest, keyed in dest by its path once moved (move), and saves both books, dest first, so a line is never in neither. When dest cannot be saved the lines stay in source, and the sidebar says so.
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
}

// libraryMoveEnded says in the message bar how the move of the film called name to library ended: moved, with 打开 library and, with undo, Undo; moved with some of its files left in from (left); stopped; or why it failed, its files where they were. done, when set, hears it too.
func (a *app) libraryMoveEnded(m locales.Messages, name, library, from, to, key string, left, undo bool, err error, done func(string, error)) {
	ui := a.messages()
	a.libraryMoveLook()
	switch {
	case errors.Is(err, context.Canceled):
		a.tell(ui.LibraryMoveStopped(name))
		err = &libraryMoveErr{code: remote.CodeFailed, text: m.LibraryMoveStopped(name), err: err}
	case err != nil:
		var refused *libraryMoveErr
		if errors.As(err, &refused) {
			a.warn(refused.ui, "")
		} else {
			a.warn(ui.LibraryMoveFailed(name, a.libraryMoveLabel(ui, to), err.Error()), "")
			err = &libraryMoveErr{code: remote.CodeFailed, text: m.LibraryMoveFailed(name, library, err.Error()), err: err}
		}
	default:
		text := ui.LibraryMoved(name, a.libraryMoveLabel(ui, to))
		kind := "success"
		if left {
			text, kind = ui.LibraryMovedLeft(name, a.libraryMoveLabel(ui, to), a.libraryMoveLabel(ui, from)), "warning"
			undo = false
		}
		n := films.Notice{Kind: kind, Text: text, Undo: undo, Open: true}
		if to != filepath.Clean(a.snap().LibraryDir) {
			n.Library = to
		}
		a.libraryShow(n)
		a.lib.move.undo = libraryMoveBack{}
		if undo {
			a.lib.move.undo = libraryMoveBack{key: key, from: from, to: to}
		}
	}
	if done != nil {
		done(key, err)
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
	if n.Open && n.Stop {
		n.Text = text
		return
	}
	// A bar the user closed while the copy goes on stays closed until the copy ends.
	if c.percent == 0 {
		a.libraryShow(films.Notice{Kind: "success", Text: text, Stop: true, Open: true})
	}
}

// libraryMoveQuit stops a copy still going as the app quits, which removes what it copied.
func (a *app) libraryMoveQuit() {
	if c := a.lib.move.copying; c != nil {
		c.cancel()
	}
}

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
