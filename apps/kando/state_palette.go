package main

import (
	"context"
	"maps"
	"time"

	"kando/apps/kando/films"

	"github.com/egoist/mygo/ui"
)

// paletteUI is the state of the command palette that ⌘K opens (palette_app.go draws it with package palette).
type paletteUI struct {
	open  bool
	query string
	// sel is the highlighted row, counting the lines first and the commands after them. moved is set once ↑ or ↓ moved it since the query last changed: until then Return with a query asks the open film rather than running the row, as the prototype's footer says. reveal scrolls the highlighted row into view in the next frame.
	sel    int
	moved  bool
	reveal bool
	// furthest is how far each video has been played on from its start, by path (paletteNotePlayed), kept in library.json with the shelf (shelf.Library.Furthest). The palette finds and sends only lines up to it, so it gives nothing away of what has not been watched. dirty is set while it holds more than library.json does.
	furthest map[string]time.Duration
	dirty    bool
	// watching is the playback paletteNotePlayed saw in the frame before, and watchedAt where its picture was then.
	watching  *filmPlay
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

// paletteFile is a subtitle file as the palette read it: its cues, and the size and time it had, so it is read again only once it changed.
type paletteFile struct {
	cues []paletteCue
	mod  time.Time
	size int64
}

// paletteCue is a line of a subtitle file: where it starts, its translation and its original, and both in lower case for the search.
type paletteCue struct {
	start                 time.Duration
	translation, original string
	folded                string
}

// paletteHit is a line the query found, in the film it belongs to.
type paletteHit struct {
	film films.Film
	cue  paletteCue
}

// paletteAnswer is the answer card. busy is set while the question is on its way; needsAI when no AI service is set or the one set could not answer, so the card offers to open its settings; failed when no answer came.
type paletteAnswer struct {
	busy    bool
	needsAI bool
	failed  bool
	text    string
	// refs are the moments of the film the answer points at, each the start of a line watched.
	refs []time.Duration
	// horizon is how far the film had been watched when it was asked, and model the model that answered.
	horizon time.Duration
	model   string
	film    string
}

func newPaletteUI() paletteUI { return paletteUI{} }

// paletteHooks: the palette reads how far each video was played with the shelf and saves it with the player's resume positions, notes after each frame how far the open video has played, and holds the window's modal flag while it is open. When the shelf's films change while it is open, it reads their subtitle files again.
func (a *app) paletteHooks() areaHooks {
	return areaHooks{
		shelfLoaded:      func() { a.palette.furthest = maps.Clone(a.library.Furthest) },
		positionsChanged: a.paletteSaveFurthest,
		afterFrame:       a.paletteAfterFrame,
		shelfChanged: func() {
			if a.palette.open {
				a.paletteRead()
			}
		},
	}
}

// paletteKeys are the palette's shortcuts: ⌘K opens it, and closes it again, anywhere in the window. It is the prototype's 学习 › 问这部影片…, which the menu bar puts before 窗口; over the library, without a film, it finds commands and lines, so its label names both. The cheat sheet lists it under 窗口, as the prototype's sheet does (cheatsheetLayout).
func (a *app) paletteKeys() []shortcut {
	return []shortcut{
		{Group: "学习", Label: "问这部影片 / 命令…", Mods: ui.Cmd, Key: ui.KeyK, Run: a.togglePalette},
	}
}
