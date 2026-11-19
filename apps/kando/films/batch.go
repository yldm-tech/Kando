package films

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// menuOf is the menu a right click or ⋯ on item opens among done, the films the page shows: the selection's (SelectionMenu) when item is one of two or more films selected, as Finder's acts on every selected item; otherwise the film's own (filmMenu), the selection becoming item alone in the list, as a right click selects the row it is on, and nothing in the posters, where a click opens a film rather than selecting it. It runs in the handler that opens the menu, so it tells the revision itself.
func menuOf(cx *core.Scope, a App, item Film, done []Film, list bool) []nsdesktop.MenuItem {
	sel := a.Selection()
	if sel.Len() > 1 && sel.Has(item.Key) {
		return SelectionMenu(cx, a, sel.Of(done))
	}
	live.Do(a, func() {
		if list {
			sel.Only(item.Key)
		} else {
			sel.Clear()
		}
	})()
	return filmMenu(cx, a, item)
}

// SelectionMenu is what can be done with several done films at once, items, from a right click or ⋯ on one of them while they are selected, and the File menu's rows (libraryKeys): how many it acts on, dimmed, then 标记为已看完 and 标记为未看 (selectionWatchMenu), 重新处理, 重新翻译为 ▸ for those a job made, 移到资料库 ▸ while Kando knows another library, 移动视频到 ▸ and 将视频移回资料库 while their videos can move to a storage location or back (App.StoredMenu), and 从资料库移除. What only makes sense for one film, playing, renaming and showing it in Finder, is left out. It is built as the menu opens, so it says what is true then.
func SelectionMenu(cx *core.Scope, a App, items []Film) []nsdesktop.MenuItem {
	do := func(fn func(...Film)) func() { return live.Do(a, func() { fn(items...) }) }
	out := []nsdesktop.MenuItem{{Label: locales.FilmsSelectedCount(cx, len(items)), Disabled: true}, {Separator: true}}
	out = append(out, selectionWatchMenu(cx, a, items)...)
	out = append(out, nsdesktop.MenuItem{Label: locales.FilmsReprocess(cx), OnSelect: do(a.AskReprocess)})
	if into := SelectionTranslateMenu(cx, a, items, func(fn func()) func() { return live.Do(a, fn) }); len(into) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.FilmsTranslateAgain(cx), Items: into})
	}
	if move := a.MoveFilmMenu(cx, items...); len(move) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.FilmsMoveToLibrary(cx), Items: move})
	}
	out = append(out, a.StoredMenu(cx, items...)...)
	return append(out, nsdesktop.MenuItem{Separator: true}, nsdesktop.MenuItem{Label: locales.FilmsRemoveFromLibrary(cx), OnSelect: do(a.AskRemove)})
}

// SelectionMarks reports which of 标记为已看完 and 标记为未看 apply to items, as watchMenu decides for one film: watched while any of them can be marked watched, and ready while one of those has a known length, which marking it watched needs; unwatched while any was played.
func SelectionMarks(p Progress, items []Film) (watched, ready, unwatched bool) {
	for _, item := range items {
		if item.VideoPath == "" {
			continue
		}
		state := WatchState(p, item)
		if _, resumable := ResumeFrom(p, item); state != sidebar.WatchWatched || resumable {
			watched = true
			ready = ready || p.Duration(item.VideoPath) > 0
		}
		if state != sidebar.WatchUnwatched {
			unwatched = true
		}
	}
	return watched, ready, unwatched
}

// selectionWatchMenu is 标记为已看完 and 标记为未看 for several films, whichever applies to any of them (SelectionMarks). Marking them watched marks those whose length is known and leaves the others as they were.
func selectionWatchMenu(cx *core.Scope, a App, items []Film) []nsdesktop.MenuItem {
	watched, ready, unwatched := SelectionMarks(a, items)
	var out []nsdesktop.MenuItem
	if watched {
		out = append(out, nsdesktop.MenuItem{Label: locales.FilmsMarkWatched(cx), Disabled: !ready, OnSelect: live.Do(a, func() { a.MarkWatched(items...) })})
	}
	if unwatched {
		out = append(out, nsdesktop.MenuItem{Label: locales.FilmsMarkUnwatched(cx), OnSelect: live.Do(a, func() { a.MarkUnwatched(items...) })})
	}
	return out
}

// SelectionTranslateMenu is the submenu 重新翻译为 ▸ of several films: the languages to translate those of items a job made into (Translatable), the one they all are in now checked, none when no job made any of them. wrap makes an item's OnSelect from what it does: live.Do for the films' menus, the main package's menuSelect for the menu bar's.
func SelectionTranslateMenu(cx *core.Scope, a App, items []Film, wrap func(func()) func()) []nsdesktop.MenuItem {
	jobs := Translatable(items)
	if len(jobs) == 0 {
		return nil
	}
	current := translatedInto(jobs[0])
	for _, item := range jobs[1:] {
		if translatedInto(item) != current {
			current = ""
		}
	}
	return subtitle.TranslateIntoMenu(cx, a.TargetLanguage(), current, func(language string) { wrap(func() { a.TranslateFilmsAgain(jobs, language) })() })
}

// Translatable is the films of items that 重新翻译为 can translate again: those a job made, whose transcript it kept.
func Translatable(items []Film) []Film {
	var out []Film
	for _, item := range items {
		if item.JobID != "" {
			out = append(out, item)
		}
	}
	return out
}

// translatedInto is the language a done film's subtitles are translated into, "" for one that was not translated at all, whatever its job was asked to translate it into.
func translatedInto(item Film) string {
	if item.TranslationErr != "" {
		return ""
	}
	return item.Language
}
