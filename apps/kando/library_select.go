package main

import (
	"errors"
	"slices"
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// The library page's selection of several done films (films.Selection) and what is done with them at once: the selection's menu (films.SelectionMenu) and the File menu's rows act on them, each film's work done or queued as for one film, one after another, and the message bar says once how it went. The selection is the app's, not the page's, so the menu bar acts on it, and it is kept to the films the page shows (libraryFollowSelection).

// Selection is the done films selected on the library page (films.App).
func (a *app) Selection() *films.Selection { return &a.lib.selection }

// libraryShownFilms is the done films the library page shows, as its grid or its list does (films.Shown): from YouTube on YouTube's page, else from the source the sidebar chose, heard in the language and watched as far as it chose, whose names hold the search.
func (a *app) libraryShownFilms() []films.Film {
	source := a.side.filter
	if a.page == pageYouTube {
		source = sidebar.SourceYouTube
	}
	return films.Shown(a.films(), a, source, a.SpokenFilter(), a.side.watch, a.side.query)
}

// libraryFollowSelection leaves out of the selection the films the library page no longer shows, as when a filter or the search hides them, they went into 处理中 to be processed again, or they left the library, as Finder lets go of what leaves a window; libraryHooks empties it as another library opens. It runs after every change (libraryAfterChange) and reports whether it changed it.
func (a *app) libraryFollowSelection() bool {
	if a.lib.selection.Len() == 0 {
		return false
	}
	return a.lib.selection.Keep(films.Keys(a.libraryShownFilms()))
}

// librarySelected is the films selected on the library page, in the order it lists them, none while another page or a film shows.
func (a *app) librarySelected() []films.Film {
	if a.lib.selection.Len() == 0 || !a.libraryOwnsTitleBar() {
		return nil
	}
	return a.lib.selection.Of(a.libraryShownFilms())
}

// libraryTargets is what File › 移到资料库 acts on (libraryMoveRow): the film open, as before films could be selected, or else those selected on the library page.
func (a *app) libraryTargets() []films.Film {
	if item, ok := a.libraryMoveOpenFilm(); ok {
		return []films.Film{item}
	}
	return a.librarySelected()
}

// librarySelectionKeys are the File menu's rows for the films selected on the library page (librarySelected), without keys, as the selection's menu has them (films.SelectionMenu): 标记为已看完, 标记为未看, 重新处理, 重新翻译为 ▸ and 从资料库移除, dimmed while none is selected. The film page's ⋯ has them for the film open. 移到资料库 ▸ is libraryMoveRow. Edit › 全选 stands in for the system's item while the library's films show, so chosen with the pointer it selects them all once no text field takes it (films.SelectAll), as Finder's does; ⌘A is the page's own (films.selectionKeys).
func (a *app) librarySelectionKeys(cx *ns.Scope) []shortcut {
	marks := func() (watched, unwatched bool) {
		w, ready, u := films.SelectionMarks(a, a.librarySelected())
		return w && ready, u
	}
	rows := []shortcut{
		{Group: groupFile, Label: locales.FilmsMarkWatched(cx), When: func() bool { w, _ := marks(); return w }, Run: func() { a.MarkWatched(a.librarySelected()...) }},
		{Group: groupFile, Label: locales.FilmsMarkUnwatched(cx), When: func() bool { _, u := marks(); return u }, Run: func() { a.MarkUnwatched(a.librarySelected()...) }},
		{Group: groupFile, Label: locales.FilmsReprocess(cx), When: func() bool { return len(a.librarySelected()) > 0 }, Run: func() { a.AskReprocess(a.librarySelected()...) }},
		{Group: groupFile, Label: locales.FilmsTranslateAgain(cx), When: func() bool { return len(films.Translatable(a.librarySelected())) > 0 }, Items: func() []nsdesktop.MenuItem {
			return films.SelectionTranslateMenu(cx, a, a.librarySelected(), func(fn func()) func() { return a.menuSelect(false, fn) })
		}},
		{Group: groupFile, Label: locales.FilmsRemoveFromLibrary(cx), When: func() bool { return len(a.librarySelected()) > 0 }, Run: func() { a.AskRemove(a.librarySelected()...) }},
	}
	if a.libraryOwnsTitleBar() && a.libraryShown() {
		rows = append(rows, shortcut{Group: groupEdit, Label: locales.MenuSelectAll(cx), Key: "mod+a", Role: nsdesktop.MenuSelectAll, Run: func() { films.SelectAll(cx, a) }})
	}
	return rows
}

// librarySay says in the message bar once how work on total films went: all of done when every film was done, else how many of how many and reason, why the first that was not done was not, a sentence of its own, with section the settings section that fixes it; reason alone when none was done. A film skipped without a reason, as one already waiting in the queue, counts as done.
func (a *app) librarySay(total, done int, reason, section string, all func(n int) string, some func(done, total int, reason string) string) {
	switch {
	case reason == "" || done == total:
		a.tell(all(total))
	case done == 0:
		a.warn(reason, section)
	default:
		a.warn(some(done, total, reason), section)
	}
}

// TrashFilm takes a film off the shelf, as TrashFilms does.
func (a *app) TrashFilm(item films.Film) { a.TrashFilms(item) }

// TrashFilms takes items, a film or several selected, off the shelf, as 从资料库移除 confirms, one after another (films.Units): a film inside the library goes to the Trash with its folder, or with only its video and subtitles when that folder holds other videos or folders too; several films go with their folder when they are every video in it. A local video kept outside the library stays where it is and only leaves the shelf. One that cannot go to the Trash stays, and the others go on; the message bar says once how it went, and why the first that stayed did.
func (a *app) TrashFilms(items ...films.Film) {
	a.libraryCloseRemove()
	if len(items) == 0 {
		return
	}
	m, root := a.messages(), a.snap().LibraryDir
	a.libraryTrashIn(root, items, m, func(gone []films.Film, trashed bool, reason string) {
		defer a.Changed()
		a.libraryForget(root, gone)
		switch {
		case len(items) == 1 && len(gone) == 0:
			a.warn(reason, "")
		case len(items) == 1 && trashed:
			a.tell(m.LibraryTrashed(films.Name(items[0])))
		case len(items) == 1:
			a.tell(m.LibraryRemoved(films.Name(items[0])))
		case trashed:
			a.librarySay(len(items), len(gone), reason, "", m.LibraryTrashedMany, m.LibraryTrashedSome)
		default:
			a.librarySay(len(items), len(gone), reason, "", m.LibraryRemovedMany, m.LibraryTrashedSome)
		}
	})
}

// libraryTrashIn moves the files of items, films of the library at root, to the Trash, one unit after another (films.Units), as TrashFilms does, and once the last was tried calls done, on the engine's goroutine, with the films whose files went, or that had none to go, for libraryForget to take off the shelf; whether any file went; and why the first film that stayed did, in the language of m. It neither takes them off the shelf nor says anything.
func (a *app) libraryTrashIn(root string, items []films.Film, m locales.Messages, done func(gone []films.Film, trashed bool, reason string)) {
	units := films.Units(root, items)
	var gone []films.Film
	trashed, reason := false, ""
	var next func(i int)
	next = func(i int) {
		if i == len(units) {
			done(gone, trashed, reason)
			return
		}
		unit := units[i]
		files := []string{unit.Folder}
		if unit.Folder == "" {
			files = filmFiles(root, unit.Items[0])
		}
		a.libraryTrash(files, func(err error) {
			switch {
			case err == nil:
				trashed = trashed || len(files) > 0
				gone = append(gone, unit.Items...)
			case reason == "":
				name := films.Name(unit.Items[0])
				// desktop.Trash fails with desktop.ErrNotTrashed when the item could not go there, and with errors.ErrUnsupported after desktop.Headless; it logs what the system said, so the bar says each in a sentence of its own.
				reason = m.LibraryTrashFailed(name)
				if errors.Is(err, errors.ErrUnsupported) {
					reason = m.LibraryTrashUnsupported(name)
				}
			}
			next(i + 1)
		})
	}
	next(0)
}

// libraryTrash moves files to the Trash one after another, stopping at the first that cannot go, and calls done with its error, or nil once all went.
func (a *app) libraryTrash(files []string, done func(err error)) {
	if len(files) == 0 {
		done(nil)
		return
	}
	trashItem(a.cx, files[0], func(err error) {
		if err != nil {
			done(err)
			return
		}
		a.libraryTrash(files[1:], done)
	})
}

// libraryForget takes items, films of the library at root, off the shelf once their files went to the Trash, or for a video outside the library without them: an open one closes, the title given to it in Kando goes, and its job, or for a video added without a job its place among the library's (shelf.Library.Added), its file staying where it is; then the library's folder is read again, once.
func (a *app) libraryForget(root string, items []films.Film) {
	if len(items) == 0 {
		return
	}
	named := false
	for _, item := range items {
		if a.openKey == item.Key {
			a.openKey = ""
			a.stopPlayback()
		}
		// The film leaves the shelf, and the title given to it in Kando with it; remove saves library.json for a job, and a video no job processed saves it here.
		named = a.library.Rename(item.VideoPath, "") || named
		// A video added without a job leaves the library and stays where it is.
		named = a.library.Unadd(root, item.VideoPath) || named
		a.storedForget(item)
		if item.JobID != "" {
			a.remove(item.JobID)
		}
	}
	if named {
		if err := a.library.Save(); err != nil {
			a.saveErr = a.messages().LibraryShelfSaveFailed(err.Error())
		}
	}
	a.refreshLibrary()
}

// ReprocessFilms processes items again, as 重新处理 confirms: a film alone as Reprocess does; several one after another, each into the language it was translated into (queueReprocessLanguage) or, for a video no job has processed, the one the settings name, behind the jobs waiting, without leaving the page, and the message bar says once how many were queued, and why the first that was not was not.
func (a *app) ReprocessFilms(items ...films.Film) {
	if len(items) == 1 {
		a.Reprocess(items[0])
		return
	}
	done, reason, section := 0, "", ""
	for _, item := range items {
		if msg, sec := a.libraryReprocessOne(item); msg != "" {
			if reason == "" {
				reason, section = msg, sec
			}
			continue
		}
		done++
	}
	m := a.messages()
	a.librarySay(len(items), done, reason, section, m.LibraryReprocessingMany, m.LibraryReprocessingSome)
}

// libraryReprocessOne queues item to be processed again, as Reprocess does, without a word in the message bar and leaving the page and the card selected as they are, and returns why it could not, with the settings section that fixes it: a job's film again from its source (queueRetryable), a video no job processed from its file (queueFromFolderChecked). A job already waiting or running is left as it is.
func (a *app) libraryReprocessOne(item films.Film) (msg, section string) {
	delete(a.lib.quotes, item.SubtitlePath)
	if item.JobID == "" {
		_, msg, section = a.queueFromFolderChecked(item)
		return msg, section
	}
	job, ok := a.library.Find(item.JobID)
	if !ok {
		return "", ""
	}
	ready, err := a.queueRetryable(job)
	var stored *storedRefusal
	switch {
	case errors.Is(err, errInQueue):
		return "", ""
	case errors.As(err, &stored):
		return err.Error(), ""
	case err != nil:
		return err.Error(), settings.SectionTranslate
	}
	a.queueAdd(ready)
	return "", ""
}

// TranslateFilmsAgain translates every line of each done film of items that a job made (films.Translatable) again into language, as TranslateAgain does one (films.App): a film alone as TranslateAgain does; several once the user confirms replacing translations in another language, when any is (films.Replacing, RetranslateDialog), then one after another (libraryRetranslateFilms).
func (a *app) TranslateFilmsAgain(items []films.Film, language string) {
	language = strings.TrimSpace(language)
	items = films.Translatable(items)
	switch {
	case len(items) == 0:
	case len(items) == 1:
		a.TranslateAgain(items[0], language)
	case len(films.Replacing(items, language)) == 0:
		a.libraryRetranslateFilms(items, language)
	default:
		a.libraryAskRetranslate(slices.Clone(items), language)
	}
}

// libraryRetranslateFilms queues items, done films a job made, to be translated again into language, behind the jobs waiting, without leaving the page (queueRetranslatable), and says in the message bar once how many were queued, and why the first that was not was not. A film being processed again is left as it is.
func (a *app) libraryRetranslateFilms(items []films.Film, language string) {
	done, reason := 0, ""
	for _, item := range items {
		job, ok := a.library.Find(item.JobID)
		if !ok || job.Stage != core.StageComplete || job.ID == a.queue.active {
			done++
			continue
		}
		ready, err := a.queueRetranslatable(job, core.RedoTranslation, language)
		if err != nil {
			if reason == "" {
				reason = err.Error()
			}
			continue
		}
		delete(a.lib.quotes, item.SubtitlePath)
		a.queueAdd(ready)
		done++
	}
	m := a.messages()
	name := subtitle.TargetLanguageNameIn(m, language)
	a.librarySay(len(items), done, reason, settings.SectionTranslate,
		func(n int) string { return m.LibraryRetranslatingMany(n, name) },
		func(done, total int, reason string) string {
			return m.LibraryRetranslatingSome(done, total, name, reason)
		})
}
