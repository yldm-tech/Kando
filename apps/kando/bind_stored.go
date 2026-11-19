package main

import (
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// StoredMenu is the items of a done film's menu that move its video to a storage location or back (films.App): 移动视频到 ▸ with each location the settings have (storedTargets), or, for a film whose video is on one, 将视频移回资料库, disabled while that location is no longer in the settings. Both are disabled while another video is being moved. None for a film whose video cannot move (storedCan), or when there is no location to move it to.
func (a *app) StoredMenu(cx *ns.Scope, item films.Film) []nsdesktop.MenuItem {
	if s, ok := a.storedOf(item.VideoPath); ok {
		_, there := a.storedTargetOf(s.Location)
		return []nsdesktop.MenuItem{{Label: locales.StoredMoveBack(cx), Disabled: !there || a.stored.run != nil, OnSelect: live.Do(a, func() { a.MoveVideoBack(item) })}}
	}
	targets := a.storedTargets()
	if len(targets) == 0 {
		return nil
	}
	m := locales.For(cx)
	if a.storedCan(m, item) != nil {
		return nil
	}
	var places []nsdesktop.MenuItem
	for _, t := range targets {
		// An object store is reached over the internet, which offline mode keeps closed.
		offline := t.Store != nil && a.privacy.offline
		places = append(places, nsdesktop.MenuItem{Label: a.storedName(m, t.ID), Disabled: a.stored.run != nil || offline, OnSelect: live.Do(a, func() { a.MoveVideoTo(item, t.ID) })})
	}
	return []nsdesktop.MenuItem{{Label: locales.StoredMoveTo(cx), Items: places}}
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

// MoveOpenVideoBack moves the open film's video back to the library, as its menu's 将视频移回资料库 does (MoveVideoBack); nothing for a video that was not moved.
func (a *app) MoveOpenVideoBack() {
	if item, ok := a.storedFilm(a.player.playPath); ok {
		a.MoveVideoBack(item)
	}
}
