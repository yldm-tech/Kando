package main

import (
	"context"
	"maps"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	"kando/apps/kando/shelf"

	"github.com/nsgo-dev/nsgo/core"
)

// paletteUI is the state of the command palette that ⌘K opens (palette_app.go draws it with package palette).
type paletteUI struct {
	open  bool
	query string
	// sel is the highlighted row, which Return runs, counting the row that asks the open film first when there is one (paletteAsks), then the lines, then the commands. It goes back to the first row as the query changes. The palette brings the highlighted row into view itself as ↑ and ↓ move it.
	sel int
	// furthest is how far each video has been played on from its start, by path (palettePlaying), kept in library.json with the shelf (shelf.Library.Furthest). The palette finds and sends only lines up to it, so it gives nothing away of what has not been watched. dirty is set while it holds more than library.json does.
	furthest map[string]time.Duration
	dirty    bool
	// watching is the playback paletteNoteAt saw at the player's last progress report, and watchedAt where its picture was then.
	watching  palettePlayback
	watchedAt time.Duration
	// files are the library's subtitle files the palette has read, by path; reading is set while a read started by paletteRead runs.
	files   map[string]paletteFile
	reading bool
	readGen int
	// hits are the lines the query found in the films in scope, before the furthest point played narrows them; hitsFor says which query, read and scope they were found for, so they are searched again only when one of those changed.
	hits    []paletteHit
	hitsFor string
	// answer is the card above the results: the answer to the question asked last, or what stands in for it. askGen numbers the questions, so a reply to one asked before the last is dropped, and cancel stops the one on its way.
	answer *paletteAnswer
	askGen int
	cancel context.CancelFunc
	// release gives back the window's modal flag the palette holds while it is open, so the menu bar's commands wait behind it as behind any modal dialog; nil while it is closed.
	release func()
}

// palettePlayback is a playback of the open video as the palette tells them apart: the player's playback and its generation (app.player.playGen), which opening the video and every seek move on, so where a seek lands is never taken as played. The zero value is none.
type palettePlayback struct {
	play *filmPlay
	gen  int
}

// paletteFile is a subtitle file as the palette read it: its cues, and the size and time it had, so it is read again only once it changed.
type paletteFile struct {
	cues []lines.Line
	mod  time.Time
	size int64
}

// paletteHit is a line the query found, in the film it belongs to.
type paletteHit struct {
	film films.Film
	cue  lines.Line
}

// paletteAnswer is the answer card. busy is set while the question is on its way; needsAI when no AI service is set or the one set could not answer, so the card offers to open its settings; failed when no answer came.
type paletteAnswer struct {
	busy    bool
	needsAI bool
	failed  bool
	// text is the model's answer. say, when set, writes what stands in for one instead (the question on its way, why there is none), in the language the app shows under the card's scope.
	text string
	say  func(cx *core.Scope) string
	// refs are the moments of the film the answer points at, each the start of a line watched.
	refs []time.Duration
	// horizon is how far the film had been watched when it was asked, and model the model that answered.
	horizon time.Duration
	model   string
	// explained is the original of the line Explain Line explained, which the card shows over the explanation; "" for an answer to a question.
	explained string
	film      string
}

func newPaletteUI() paletteUI { return paletteUI{} }

// paletteHooks: the palette reads how far each video was played with the shelf and saves it with the player's resume positions and keys it by the new paths of videos that moved, and notes how far the open video has played as the player reports its progress. When the shelf's films change while it is open, it reads their subtitle files again.
func (a *app) paletteHooks() areaHooks {
	return areaHooks{
		shelfLoaded:      func() { a.palette.furthest = maps.Clone(a.library.Furthest) },
		positionsChanged: a.paletteSaveFurthest,
		playing:          a.palettePlaying,
		videosMoved:      func(move func(string) (string, bool)) { shelf.MoveKeys(a.palette.furthest, move) },
		shelfChanged: func() {
			if a.palette.open {
				a.paletteRead()
			}
		},
	}
}

// paletteKeys are the palette's shortcuts: ⌘K opens it, and closes it again, anywhere in the window. It is the prototype's 学习 › 问这部影片…, which the menu bar puts before 窗口; over the library, without a film, it finds commands and lines, so its label names both. The cheat sheet lists it under 窗口, as the prototype's sheet does (cheatsheetLayout). E has the AI service explain the subtitle line playing in the palette (ExplainLine), while the open film has lines.
func (a *app) paletteKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupStudy, Label: locales.PaletteMenuItem(cx), Key: paletteKey, Run: a.togglePalette},
		{Group: groupStudy, Label: locales.PaletteExplainLine(cx), Key: "e", When: func() bool { return len(a.playerCues()) > 0 }, Run: a.paletteExplainPlaying},
	}
}

// paletteKey is ⌘K, which opens the palette and closes it again; the palette leaves its own row out of the commands it lists.
const paletteKey = "mod+k"
