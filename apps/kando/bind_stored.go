package main

import (
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// StoredMenu is the items of the menu of a done film, or of the films selected, that move their videos to a storage location or back (films.App), as storedItems makes them.
func (a *app) StoredMenu(cx *ns.Scope, items ...films.Film) []nsdesktop.MenuItem {
	return a.storedItems(cx, items, func(fn func()) func() { return live.Do(a, fn) })
}

// storedItems are the items that move the videos of items, a film or several selected, to a storage location or back: 移动视频到 ▸ with each location the settings have (storedTargets), while one of those whose videos are in the library's folder can move (storedCan), and 将视频移回资料库 while one's video is on a location, disabled while none of their locations is still in the settings. Both are disabled while another video is being moved. wrap makes an item's OnSelect from what it does: live.Do for the films' menus, menuSelect for the menu bar's.
func (a *app) storedItems(cx *ns.Scope, items []films.Film, wrap func(func()) func()) []nsdesktop.MenuItem {
	var out []nsdesktop.MenuItem
	if places := a.storedPlaces(cx, items, wrap); len(places) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.StoredMoveTo(cx), Items: places})
	}
	if back := a.storedOn(items); len(back) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.StoredMoveBack(cx), Disabled: !a.storedBackReady(back), OnSelect: wrap(func() { a.MoveVideosBack(back) })})
	}
	// An indexed film's video is copied from its media source to be kept in the library (state_mediasource_import.go).
	if linked := a.linkedOn(items); len(linked) > 0 {
		out = append(out, nsdesktop.MenuItem{Label: locales.MediaSourceCopyToLibrary(cx), Disabled: !a.linkedCopyReady(linked), OnSelect: wrap(func() { a.CopyLinked(linked) })})
	}
	return out
}

// storedPlaces are the items of 移动视频到 ▸ for items: each location a video can be moved to, the object store disabled while offline mode is on; none while no film of items whose video is in the library's folder can move, or there is no location.
func (a *app) storedPlaces(cx *ns.Scope, items []films.Film, wrap func(func()) func()) []nsdesktop.MenuItem {
	targets := a.storedTargets()
	if len(targets) == 0 {
		return nil
	}
	m := locales.For(cx)
	var movable []films.Film
	can := false
	for _, item := range items {
		// An indexed film's video stays on its media source.
		if _, stored := a.storedOf(item.VideoPath); stored || item.Linked != "" {
			continue
		}
		movable = append(movable, item)
		can = can || a.storedCan(m, item) == nil
	}
	if !can {
		return nil
	}
	var places []nsdesktop.MenuItem
	for _, t := range targets {
		// An object store is reached over the internet, which offline mode keeps closed.
		offline := t.Store != nil && a.privacy.offline
		places = append(places, nsdesktop.MenuItem{Label: a.storedName(m, t.ID), Disabled: a.stored.run != nil || offline, OnSelect: wrap(func() { a.MoveVideosTo(movable, t.ID) })})
	}
	return places
}

// storedBackReady reports whether the videos of items, on storage locations, can be moved back now: no other video is being moved, and the location of one of them is still in the settings.
func (a *app) storedBackReady(items []films.Film) bool {
	if a.stored.run != nil {
		return false
	}
	for _, item := range items {
		if s, ok := a.storedOf(item.VideoPath); ok {
			if _, there := a.storedTargetOf(s.Location); there {
				return true
			}
		}
	}
	return false
}

// storedKeys are the File menu's rows that move the videos of the films it acts on (libraryTargets: the film open, or those selected on the library page) to a storage location or back, as their menus do (storedItems): 移动视频到 ▸, a submenu of the locations, and 将视频移回资料库, neither with a key.
func (a *app) storedKeys(cx *ns.Scope) []shortcut {
	plain := func(fn func()) func() { return fn }
	return []shortcut{
		{Group: groupFile, Label: locales.StoredMoveTo(cx),
			When: func() bool { return len(a.storedPlaces(cx, a.libraryTargets(), plain)) > 0 },
			Items: func() []nsdesktop.MenuItem {
				return a.storedPlaces(cx, a.libraryTargets(), func(fn func()) func() { return a.menuSelect(false, fn) })
			}},
		{Group: groupFile, Label: locales.StoredMoveBack(cx),
			When: func() bool { return a.storedBackReady(a.storedOn(a.libraryTargets())) },
			Run:  func() { a.MoveVideosBack(a.libraryTargets()) }},
	}
}

// StoredOpenMenu is StoredMenu for the open film, for the film page's ⋯ (film.App).
func (a *app) StoredOpenMenu(cx *ns.Scope) []nsdesktop.MenuItem {
	item, ok := a.openFilm()
	if !ok {
		return nil
	}
	return a.StoredMenu(cx, item)
}

// StoredPlace names the storage location item's video is on, in the interface's language, "" for a video in the library's folder (films.App).
func (a *app) StoredPlace(item films.Film) string {
	if item.Stored == "" {
		return ""
	}
	return strings.TrimSpace(a.storedName(a.messages(), item.Stored))
}

// StoredMark is what the mark on the poster and list row of a film whose video is on a storage location says (films.App): the location's name in m's language, whether it is an object store rather than a NAS location, and whether it is no longer in the settings (storedTargetOf). It only reads the settings and never reaches the location. All zero for a video in the library's folder.
func (a *app) StoredMark(m locales.Messages, item films.Film) (place string, cloud, gone bool) {
	if item.Stored == "" {
		return "", false, false
	}
	cloud = strings.HasPrefix(item.Stored, storedStorePrefix)
	if _, there := a.storedTargetOf(item.Stored); !there {
		return "", cloud, true
	}
	return strings.TrimSpace(a.storedName(m, item.Stored)), cloud, false
}
