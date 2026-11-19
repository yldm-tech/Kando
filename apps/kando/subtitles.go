package main

import (
	"errors"
	"os"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"
)

// The open film's subtitle file, read off the engine's goroutine.

// What went wrong opening the film, as the player's state keeps it (subtitleView.err, playErr): an ID, which ErrText puts in the interface's language when the page shows it.
const (
	playerErrSubsMissing    = "subtitle-missing"
	playerErrSubsUnreadable = "subtitle-unreadable"
	playerErrUnplayable     = "video-unplayable"
	playerErrPlugins        = film.ErrMissingPlugins
	// A video moved to a storage location (playerStored): the location is no longer in the settings, could not be reached, no longer has the video, or the system's player cannot play it from there.
	playerErrStoredGone        = film.ErrStoredGone
	playerErrStoredUnreachable = film.ErrStoredUnreachable
	playerErrStoredMissing     = film.ErrStoredMissing
	playerErrStoredUnplayable  = film.ErrStoredUnplayable
)

// playerErrTextIn is what the page says for one of the errors above in the language of m, as the page says it (ErrText) and the feedback issue says it in the language it is written in; any other text shows as it is.
func playerErrTextIn(m locales.Messages, id string) string {
	switch id {
	case playerErrSubsMissing:
		return m.PlayerSubsMissing()
	case playerErrSubsUnreadable:
		return m.PlayerSubsUnreadable()
	case playerErrUnplayable:
		return m.PlayerUnplayable()
	case playerErrPlugins:
		return m.PlayerMissingPlugins()
	case playerErrStoredGone:
		return m.PlayerStoredGone()
	case playerErrStoredUnreachable:
		return m.PlayerStoredUnreachable()
	case playerErrStoredMissing:
		return m.PlayerStoredMissing()
	case playerErrStoredUnplayable:
		return m.PlayerStoredUnplayable()
	}
	return id
}

// subtitleView is the subtitle file open on the film page. err is one of the player's error IDs, or "".
type subtitleView struct {
	key  string
	path string
	// machine is the file's cues as the pipeline wrote them, and edits the user's corrections of them, kept in the film's corrections file (media.CorrectionsPath); editsErr is set when that file could not be read, which is then not written over. cues are the lines as they show, the corrections laid over the machine's and those marked as no speech left out (core.CorrectSegments), and from the corrected line each of them comes from, index for index.
	machine  []core.Segment
	edits    []core.Correction
	editsErr bool
	cues     []core.Segment
	from     []core.Corrected
	err      string
	ready    bool
	mod      time.Time
	size     int64
	seen     bool
}

// loadSubtitles reads path off the engine's goroutine, with the user's corrections of the film (media.LoadCorrections), and the frames of the 片段 its lines cut the film into once read (loadClipFrames). An empty path means this film has no subtitle yet.
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
		edits, editsErr := media.LoadCorrections(media.CorrectionsPath(path))
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
			a.player.subs.machine = cues
			a.player.subs.edits, a.player.subs.editsErr = edits, editsErr != nil
			a.playerCorrectSubs()
			a.player.subs.err = msg
			a.player.subs.ready = true
			if statErr == nil {
				a.player.subs.mod = info.ModTime()
				a.player.subs.size = info.Size()
				a.player.subs.seen = true
			}
			a.loadClipFrames()
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

// playerCorrectSubs lays the user's corrections of the open film over the lines the machine wrote, as the picture, the subtitle list and the keys take them (subtitleView.cues).
func (a *app) playerCorrectSubs() {
	subs := &a.player.subs
	subs.cues, subs.from = core.CorrectSegments(subs.machine, subs.edits)
	if len(subs.machine) == 0 {
		subs.cues, subs.from = nil, nil
	}
}
