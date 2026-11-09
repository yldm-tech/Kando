package main

import (
	"errors"
	"os"
	"time"

	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// The open film's subtitle file, read off the UI thread.

// What went wrong opening the film, as the player's state keeps it (subtitleView.err, playErr): an ID, which playerErrText puts in the interface's language when the page shows it.
const (
	playerErrSubsMissing    = "subtitle-missing"
	playerErrSubsUnreadable = "subtitle-unreadable"
	playerErrUnplayable     = "video-unplayable"
)

// playerErrText is what the page says for one of the errors above; any other text shows as it is.
func playerErrText(id string) string { return playerErrTextIn(locales.In(i18n.Locale()), id) }

// playerErrTextIn is playerErrText in the language of m, as the feedback issue says it in the language it is written in.
func playerErrTextIn(m locales.Messages, id string) string {
	switch id {
	case playerErrSubsMissing:
		return m.PlayerSubsMissing()
	case playerErrSubsUnreadable:
		return m.PlayerSubsUnreadable()
	case playerErrUnplayable:
		return m.PlayerUnplayable()
	}
	return id
}

// subtitleView is the subtitle file open on the film page. list keeps the scroll position across frames. err is one of the player's error IDs, or "".
type subtitleView struct {
	key   string
	path  string
	cues  []core.Segment
	err   string
	ready bool
	list  ui.ListState
	clips ui.ListState
	mod   time.Time
	size  int64
	seen  bool
}

// loadSubtitles reads path off the UI thread. An empty path means this film has no subtitle yet.
func (a *app) loadSubtitles(key, path string) {
	if a.player.subs.key == key && a.player.subs.path == path {
		return
	}
	a.player.subs = subtitleView{key: key, path: path}
	if path == "" {
		a.player.subs.ready = true
		return
	}
	update := a.update
	if update == nil {
		a.player.subs.err = playerErrSubsUnreadable
		a.player.subs.ready = true
		return
	}
	go func() {
		data, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		var cues []core.Segment
		msg := ""
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				msg = playerErrSubsMissing
			} else {
				msg = playerErrSubsUnreadable
			}
		} else if cues, err = media.ParseSRT(data); err != nil {
			msg = playerErrSubsUnreadable
			cues = nil
		}
		update(func() {
			if a.player.subs.key != key || a.player.subs.path != path {
				return
			}
			a.player.subs.cues = cues
			a.player.subs.err = msg
			a.player.subs.ready = true
			if statErr == nil {
				a.player.subs.mod = info.ModTime()
				a.player.subs.size = info.Size()
				a.player.subs.seen = true
			}
		})
	}()
}

// reloadOpenSubtitles reads the open subtitle again when the file on disk was rewritten or removed.
func (a *app) reloadOpenSubtitles() {
	if a.player.subs.key == "" || a.player.subs.path == "" {
		return
	}
	info, err := os.Stat(a.player.subs.path)
	if err != nil {
		if !a.player.subs.ready || a.player.subs.err != "" {
			return
		}
	} else if a.player.subs.seen && a.player.subs.size == info.Size() && a.player.subs.mod.Equal(info.ModTime()) {
		return
	}
	key, path := a.player.subs.key, a.player.subs.path
	a.player.subs.path = ""
	a.loadSubtitles(key, path)
}
