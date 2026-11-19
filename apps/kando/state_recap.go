package main

import (
	"context"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	"kando/apps/kando/recap"

	"github.com/nsgo-dev/nsgo/core"
)

// recapUI is the 前情提要 card's (film.Recapper): the card of the film opened where it stopped last time (card), the playback it was weighed for, so each film opening is weighed once (seen), and the recaps the AI service wrote since the app started (cache), so opening the film again before more of it was watched shows the same one without asking again.
type recapUI struct {
	card  recapCard
	seen  *filmPlay
	cache map[recapKey][]recap.Item
	// gen numbers the recaps asked for, so one for a card closed since is dropped; cancel stops the one on its way.
	gen    int
	cancel context.CancelFunc
}

// recapCard is the card: the film's video and where it stopped last time (at); pending until the film's lines are read, then the lines watched sum up into items, busy while the AI service is asked, or are the lines watched themselves (excerpts) when no AI service can, failed when it could not; the model that wrote them, and whether it ran on this computer.
type recapCard struct {
	open, pending  bool
	video          string
	at             time.Duration
	items          []recap.Item
	busy, excerpts bool
	failed         bool
	model          string
	local          bool
}

// recapKey is what a recap is cached by: the film's video, how far it had been watched, the language and the model it was written in and by.
type recapKey struct {
	video, language, model string
	horizon                time.Duration
}

// recapTimeout is how long the recap may take.
const recapTimeout = 90 * time.Second

func newRecapUI() recapUI { return recapUI{cache: map[recapKey][]recap.Item{}} }

// recapHooks: the card opens as a film opens held where it stopped last time and fills in once its lines are read; it closes as the film plays or moves, another opens, or another card takes its place; the recap on its way stops as the app quits.
func (a *app) recapHooks() areaHooks {
	return areaHooks{
		afterChange: a.recapFollow,
		quitting:    a.recapStop,
	}
}

// recapFollow brings the card up to date with the player and reports whether it changed it. A film opened held where it stopped last time, as from its poster, the hero's card or the menu bar, which the player opens unplayed at its resume position (films.ResumeFrom), opens the card; a film opened playing, as by 继续 or a line of the palette, does not. The card waits for the film's lines and shows only when some of them were watched (the palette's furthest point, which a seek does not move), then sums them up. It closes once the film plays or is moved, once it is not the film open, and while the word looked up or the line selected for correcting shows its own card.
func (a *app) recapFollow() bool {
	r := &a.recap
	changed := false
	if p := a.player.play; p != r.seen {
		r.seen = p
		if r.card.open {
			a.recapClose()
			changed = true
		}
		if p != nil && !p.played && p.head > 0 && a.openKey != "" {
			r.card = recapCard{open: true, pending: true, video: p.path, at: p.head}
			changed = true
		}
	}
	c := &r.card
	if !c.open {
		return changed
	}
	p := a.player.play
	if p == nil || p.played || p.Playing() || a.player.film.video != c.video || a.vocab.lookup.open || a.correctSelecting() {
		a.recapClose()
		return true
	}
	if !c.pending {
		return changed
	}
	subs := a.player.subs
	if subs.key != a.openKey || !subs.ready {
		return changed
	}
	c.pending = false
	seen := a.recapSeen()
	if len(seen) == 0 {
		a.recapClose()
		return true
	}
	a.recapSum(seen)
	return true
}

// recapSeen are the open film's lines watched: those starting by the furthest point its video played to, as the palette counts them.
func (a *app) recapSeen() []lines.Line {
	horizon := a.palette.furthest[a.recap.card.video]
	if horizon <= 0 {
		return nil
	}
	var out []lines.Line
	for _, cue := range a.playerCues() {
		if !lines.Seen(cue.Start, horizon) {
			break
		}
		translation, original := lines.Split(cue.Text)
		out = append(out, lines.New(cue.Start, translation, original))
	}
	return out
}

// recapSum fills the card with what was watched of the film, seen: as the AI service set in 「AI 服务」 sums it up (recap.Prompt), asked off the engine's goroutine, or as it summed it up before for as much of it; without an AI service that can answer, as while offline mode is on and it is not on this computer, or when it fails, the first, middle and last lines watched (recap.Excerpts), which the footnote says are not a summary.
func (a *app) recapSum(seen []lines.Line) {
	c := &a.recap.card
	t, why := a.paletteTranslator(locales.RecapExcerptNote)
	if why != nil {
		c.items, c.excerpts = recap.Excerpts(seen), true
		return
	}
	item, _ := a.openFilm()
	language := paletteAnswerLanguage(item, a.prefs.language)
	c.model, c.local = strings.TrimSpace(t.Model), t.Local()
	key := recapKey{video: c.video, language: language, model: c.model, horizon: seen[len(seen)-1].Start}
	if items, ok := a.recap.cache[key]; ok {
		c.items = items
		return
	}
	c.busy = true
	a.recap.gen++
	gen := a.recap.gen
	system, user := recap.Prompt(language), recap.Request(item.Name, seen)
	if item.Name == "" {
		user = recap.Request(item.Title, seen)
	}
	ask := func(ctx context.Context) ([]recap.Item, error) {
		reply, err := t.Chat(ctx, system, user)
		if err != nil {
			return nil, err
		}
		return recap.Parse(reply, seen)
	}
	a.recap.cancel = aiAsk(a, recapTimeout, ask, func() bool { return gen == a.recap.gen && a.recap.card.open }, func(items []recap.Item, err error) {
		c := &a.recap.card
		a.recap.cancel = nil
		c.busy = false
		if err != nil {
			c.items, c.excerpts, c.failed = recap.Excerpts(seen), true, true
			return
		}
		c.items = items
		a.recap.cache[key] = items
	})
}

// recapStop stops the recap on its way, whose answer is then dropped.
func (a *app) recapStop() {
	a.recap.gen++
	if a.recap.cancel != nil {
		a.recap.cancel()
		a.recap.cancel = nil
	}
}

// recapClose closes the card.
func (a *app) recapClose() {
	a.recapStop()
	a.recap.card = recapCard{}
}

// Recap is the card as the player shows it: none until the film's lines are read and some were watched, nor while the command palette shows over it.
func (a *app) Recap() film.Recap {
	c := a.recap.card
	if !c.open || c.pending || a.palette.open {
		return film.Recap{}
	}
	out := film.Recap{Open: true, At: c.at, Busy: c.busy, Excerpts: c.excerpts}
	for _, it := range c.items {
		out.Items = append(out.Items, film.RecapItem{At: it.At, Text: it.Text})
	}
	model, local, failed := c.model, c.local, c.failed
	switch {
	case c.busy:
	case failed:
		out.Note = locales.RecapFailedNote
	case c.excerpts:
		out.Note = locales.RecapExcerptNote
	case local:
		out.Note = func(cx *core.Scope) string { return locales.RecapNoteLocal(cx, model) }
	default:
		out.Note = func(cx *core.Scope) string { return locales.RecapNote(cx, model) }
	}
	return out
}

// CloseRecap is the card's × and Escape: the film stays held where it stopped.
func (a *app) CloseRecap() { a.recapClose() }

// ContinueRecap is 从 X 继续: the film plays on from where it stopped.
func (a *app) ContinueRecap() { a.recapPlayFrom(a.recap.card.at) }

// StartOver is 从头看: the film plays from its start.
func (a *app) StartOver() { a.recapPlayFrom(0) }

// RecapJump plays the film from the moment of item i, where the picture shows the line it starts at, with the film's subtitle delay.
func (a *app) RecapJump(i int) {
	items := a.recap.card.items
	if i < 0 || i >= len(items) {
		return
	}
	a.recapPlayFrom(max(items[i].At+a.player.film.delay, 0))
}

// recapPlayFrom closes the card and plays the open film from at, as the hero's 继续 and 从头看 do (OpenAt).
func (a *app) recapPlayFrom(at time.Duration) {
	open := a.recap.card.open
	a.recapClose()
	item, ok := a.openFilm()
	if !open || !ok {
		return
	}
	a.OpenAt(item, at)
}
