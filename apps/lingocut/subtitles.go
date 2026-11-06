package main

import (
	"errors"
	"os"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// The open film's subtitle file, read off the UI thread.

// subtitleView is the subtitle file open on the film page. list keeps the scroll position across frames.
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
		a.player.subs.err = "字幕无法读取"
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
				msg = "字幕文件不在了"
			} else {
				msg = "字幕无法读取"
			}
		} else if cues, err = media.ParseSRT(data); err != nil {
			msg = "字幕无法读取"
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
