package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/hotkey"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	cmdpalette "kando/apps/kando/palette"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// The command palette's behavior: what it finds for the query, what choosing a row does, and asking the AI service about the open film. package palette draws it.

const (
	// paletteFirstCommands is how many commands the palette lists before anything is typed, as the prototype's.
	paletteFirstCommands = 5
	// paletteFilms is how many films it lists whose names hold the query.
	paletteFilms = 5
	// paletteFilmLines and paletteLibraryLines are how many lines it lists for a query: the prototype's four from the open film, and more over the library, whose lines come from every film in it.
	paletteFilmLines    = 4
	paletteLibraryLines = 8
	// paletteSlack is how far past the furthest point played a line may start and still count as seen (lines.Slack).
	paletteSlack = lines.Slack
	// paletteContextRunes caps the subtitles sent with a question; past it the earliest lines are left out, so a long film still fits a small model's context.
	paletteContextRunes = 40000
	// paletteAskTimeout is how long a question may take.
	paletteAskTimeout = 90 * time.Second
)

// paletteCommand is a row of the palette's 命令 group: what it says, the title of the menu it is in ("" for none), its shortcut outside the palette in NSGo's form ("" for none), and what it does.
type paletteCommand struct {
	label string
	group string
	key   string
	run   func()
}

// togglePalette is ⌘K: it opens the palette with an empty query, or closes it again.
func (a *app) togglePalette() {
	if a.palette.open {
		a.paletteClose()
		return
	}
	a.paletteShow()
}

// paletteShow opens the palette with an empty query, holding the window's modal flag, and reads the subtitle files it searches.
func (a *app) paletteShow() {
	a.palette.open = true
	a.palette.query = ""
	a.palette.sel = 0
	a.palette.answer = nil
	a.palette.hitsFor = ""
	if a.palette.release == nil {
		a.palette.release = a.holdModal()
	}
	a.paletteRead()
}

// paletteClose closes the palette, however it was closed: by ⌘K, Escape, a click outside it or a row run. It stops a question on its way and gives the window's modal flag back.
func (a *app) paletteClose() {
	a.palette.open = false
	if a.palette.cancel != nil {
		a.palette.cancel()
		a.palette.cancel = nil
	}
	if a.palette.release != nil {
		a.palette.release()
		a.palette.release = nil
	}
}

// palettePlaying notes how far the open video has played each time the player reports its progress (areaHooks.playing), about four times a second while it plays, and reports whether the open palette now lists other lines (paletteProgress). It reads the playhead the report just set, so the furthest point follows the picture as MyGo's per-frame note did.
func (a *app) palettePlaying() bool {
	play := a.player.play
	if play == nil || play.path == "" {
		a.palette.watching = palettePlayback{}
		return false
	}
	return a.paletteProgress(palettePlayback{play: play, gen: a.player.playGen}, play.path, play.playhead(), play.Playing())
}

// paletteProgress notes that the video at path, open in the player as playback seen, is at at, playing or paused, as paletteNoteAt says, and reports whether that changed the lines the open palette lists: only when the furthest point passed the start of a line the query finds in that film, so a film playing under the open palette does not make the app follow its every progress report, nor does one playing with the palette closed.
func (a *app) paletteProgress(seen palettePlayback, path string, at time.Duration, playing bool) bool {
	from := a.palette.furthest[path]
	if !a.paletteNoteAt(seen, path, at, playing) || !a.palette.open {
		return false
	}
	to := a.palette.furthest[path]
	query := strings.ToLower(strings.TrimSpace(a.palette.query))
	for _, hit := range a.paletteHits(query) {
		if hit.film.VideoPath == path && hit.cue.Start > from+paletteSlack && hit.cue.Start <= to+paletteSlack {
			return true
		}
	}
	return false
}

// paletteNoteAt moves the furthest point played of the video at path, open in the player as playback seen with its picture at at, but only while it plays on from what was watched already: from where it was noted before, in the same playback, and from no further than the furthest point played. A seek starts another playback (seekPlayback moves app.player.playGen on), so where it lands is not taken as played, nor is a picture that moved while paused; and playing on from a seek past the furthest point does not count either, so a seek to the end does not give the film away. The prototype likewise moves seen only in its playback tick. It reports whether the furthest point moved.
func (a *app) paletteNoteAt(seen palettePlayback, path string, at time.Duration, playing bool) bool {
	prev, same := a.palette.watchedAt, seen != palettePlayback{} && a.palette.watching == seen
	a.palette.watching, a.palette.watchedAt = seen, at
	if !same || !playing || prev > a.palette.furthest[path]+paletteSlack {
		return false
	}
	return a.paletteNoteFurthest(path, at)
}

// paletteNoteFurthest keeps at as how far the video at path has played, when it is further than before, and reports whether it was.
func (a *app) paletteNoteFurthest(path string, at time.Duration) bool {
	if path == "" || at <= a.palette.furthest[path] {
		return false
	}
	if a.palette.furthest == nil {
		a.palette.furthest = map[string]time.Duration{}
	}
	a.palette.furthest[path] = at
	a.palette.dirty = true
	return true
}

// paletteSaveFurthest writes how far each video has played into library.json, beside the resume positions the queue saved just before.
func (a *app) paletteSaveFurthest() {
	if !a.palette.dirty {
		return
	}
	a.palette.dirty = false
	a.library.Furthest = make(map[string]time.Duration, len(a.palette.furthest))
	for path, at := range a.palette.furthest {
		a.library.Furthest[path] = at
	}
	if err := a.library.Save(); err != nil {
		a.saveErr = a.shelfSaveErr(err, a.messages().PaletteLibrarySaveFailed)
	}
}

// paletteFilm is the film of the open library with key id.
func (a *app) paletteFilm(id string) (films.Film, bool) { return films.Find(a.films(), id) }

// paletteOpenFilm is the film open in the player, when one is.
func (a *app) paletteOpenFilm() (films.Film, bool) {
	if a.openKey == "" {
		return films.Film{}, false
	}
	return a.paletteFilm(a.openKey)
}

// paletteHorizon is how far the film has been seen: the furthest point its video played to from its start. A picture a seek moved further, which paletteNoteAt does not count, does not move it, so the palette neither lists nor sends a line the seek passed.
func (a *app) paletteHorizon(item films.Film) time.Duration {
	return a.palette.furthest[item.VideoPath]
}

// paletteScope is the films whose lines the palette searches: the open film in the player, every film of the library over it.
func (a *app) paletteScope() []films.Film {
	if item, ok := a.paletteOpenFilm(); ok {
		return []films.Film{item}
	}
	return a.films()
}

// paletteRead reads the subtitle files of the films in the palette's scope off the UI thread, each only when it is new or changed since it was read.
func (a *app) paletteRead() {
	if a.update == nil {
		return
	}
	type want struct {
		path string
		mod  time.Time
		size int64
	}
	var todo []want
	for _, item := range a.paletteScope() {
		if item.SubtitlePath == "" {
			continue
		}
		info, err := os.Stat(item.SubtitlePath)
		if err != nil {
			continue
		}
		if f, ok := a.palette.files[item.SubtitlePath]; ok && f.size == info.Size() && f.mod.Equal(info.ModTime()) {
			continue
		}
		todo = append(todo, want{item.SubtitlePath, info.ModTime(), info.Size()})
	}
	if len(todo) == 0 {
		return
	}
	a.palette.readGen++
	gen := a.palette.readGen
	a.palette.reading = true
	update := a.update
	go func() {
		read := make(map[string]paletteFile, len(todo))
		for _, w := range todo {
			cues, err := lines.Read(w.path)
			if err != nil {
				continue
			}
			read[w.path] = paletteFile{cues: cues, mod: w.mod, size: w.size}
		}
		update(func() {
			if a.palette.files == nil {
				a.palette.files = map[string]paletteFile{}
			}
			for path, f := range read {
				a.palette.files[path] = f
			}
			a.palette.hitsFor = ""
			if gen == a.palette.readGen {
				a.palette.reading = false
			}
		})
	}()
}

// paletteHits are the lines of the films in scope that hold the query, whether seen yet or not, in the order of the films and of their lines.
func (a *app) paletteHits(query string) []paletteHit {
	scope := a.paletteScope()
	key := fmt.Sprintf("%s\x00%d\x00%s\x00%d", query, a.palette.readGen, a.openKey, len(scope))
	if a.palette.hitsFor == key {
		return a.palette.hits
	}
	a.palette.hitsFor = key
	a.palette.hits = nil
	if query == "" {
		return nil
	}
	for _, item := range scope {
		f, ok := a.palette.files[item.SubtitlePath]
		if !ok || item.SubtitlePath == "" {
			continue
		}
		for _, cue := range f.cues {
			if cue.Holds(query) {
				a.palette.hits = append(a.palette.hits, paletteHit{film: item, cue: cue})
			}
		}
	}
	return a.palette.hits
}

// paletteLines are the lines the palette lists for the query: those of the films in scope that hold it and start no later than the furthest point their film was played to, so the palette gives nothing away of what comes after. Over the library a film never played lists none.
func (a *app) paletteLines() []paletteHit {
	query := strings.ToLower(strings.TrimSpace(a.palette.query))
	limit := paletteLibraryLines
	_, inFilm := a.paletteOpenFilm()
	if inFilm {
		limit = paletteFilmLines
	}
	horizons := map[string]time.Duration{}
	var out []paletteHit
	for _, hit := range a.paletteHits(query) {
		h, ok := horizons[hit.film.Key]
		if !ok {
			h = a.paletteHorizon(hit.film)
			horizons[hit.film.Key] = h
		}
		if (!inFilm && h <= 0) || hit.cue.Start > h+paletteSlack {
			continue
		}
		out = append(out, hit)
		if len(out) == limit {
			break
		}
	}
	return out
}

// paletteCommands are the commands the palette offers now, their labels in the language the app shows under cx: in the player, the subtitle modes it is not in; then every row of the shortcut table that applies now but the palette's own and the screen the window shows already, the player's first while a film is open; then a row for each settings section.
func (a *app) paletteCommands(cx *core.Scope) []paletteCommand {
	var out []paletteCommand
	_, inFilm := a.paletteOpenFilm()
	if inFilm {
		for _, mode := range []string{film.SubTranslation, film.SubBilingual, film.SubOriginal} {
			if mode == a.player.subMode {
				continue
			}
			out = append(out, paletteCommand{label: locales.PaletteSwitchTo(cx, film.ModeName(cx, mode)), group: groupTitle(cx, groupSubtitles), run: func() {
				a.SetSubMode(mode)
				a.flash(a.messages().PaletteSubtitleMode(film.ModeName(cx, mode)))
			}})
		}
	}
	rows := slices.Clone(a.shortcuts(cx))
	if inFilm {
		rank := func(group string) int {
			switch group {
			case groupSubtitles:
				return 0
			case groupView:
				return 1
			case groupPlayback:
				return 2
			}
			return 3
		}
		slices.SortStableFunc(rows, func(x, y shortcut) int { return rank(x.Group) - rank(y.Group) })
	}
	for _, s := range rows {
		if s.Run == nil || (s.When != nil && !s.When()) {
			continue
		}
		if s.Key == paletteKey {
			continue
		}
		if s.Group == groupWindow && s.Checked != nil && s.Checked() {
			continue
		}
		out = append(out, paletteCommand{label: s.Label, group: groupTitle(cx, s.Group), key: s.Key, run: s.guarded()})
	}
	for _, id := range settings.IDs() {
		out = append(out, paletteCommand{label: locales.PaletteSettingsSection(cx, settings.Title(cx, id)), run: func() { a.OpenSettings(id) }})
	}
	return out
}

// paletteFound is what the palette lists for the query, the commands' labels in the language the app shows under cx: its lines and the commands it finds as Settings › 快捷键 finds its rows (hotkey.Match), those whose label starts with it first; or, while nothing is typed, the first commands, the shortcut cheat sheet before them, so the list of shortcuts is a press away where no menu bar shows it.
func (a *app) paletteFound(cx *core.Scope) ([]paletteHit, []paletteCommand) {
	query := strings.TrimSpace(a.palette.query)
	cmds := a.paletteCommands(cx)
	if query == "" {
		if i := slices.IndexFunc(cmds, func(c paletteCommand) bool { return c.key == cheatsheetKey }); i > 0 {
			cmds = slices.Concat(cmds[i:i+1], cmds[:i], cmds[i+1:])
		}
		return nil, cmds[:min(len(cmds), paletteFirstCommands)]
	}
	type ranked struct {
		cmd  paletteCommand
		rank int
	}
	var found []ranked
	for _, c := range cmds {
		if rank, ok := hotkey.Match(query, c.label, c.group, hotkey.Label(cx, c.key)); ok {
			found = append(found, ranked{c, rank})
		}
	}
	slices.SortStableFunc(found, func(x, y ranked) int { return x.rank - y.rank })
	out := make([]paletteCommand, len(found))
	for i, f := range found {
		out[i] = f.cmd
	}
	return a.paletteLines(), out
}

// paletteFilmsFound are the films the palette lists for the query: the done films of the open library whose names hold it, as the sidebar's search finds them (films.MatchesTitle), in the shelf's order, the open film left out. While nothing is typed it lists none.
func (a *app) paletteFilmsFound() []films.Film {
	query := strings.TrimSpace(a.palette.query)
	if query == "" {
		return nil
	}
	var out []films.Film
	for _, item := range a.films() {
		if films.InQueue(item) || item.VideoPath == "" || item.Key == a.openKey || !films.MatchesTitle(item, query) {
			continue
		}
		out = append(out, item)
		if len(out) == paletteFilms {
			break
		}
	}
	return out
}

// paletteAsks reports whether the palette offers to ask the open film what was typed: in the player, once something is. Its row comes first, before the lines and the commands, and is the one highlighted until ↑ or ↓ moves on, so Return asks only while the row that says so is highlighted.
func (a *app) paletteAsks() bool {
	_, inFilm := a.paletteOpenFilm()
	return inFilm && strings.TrimSpace(a.palette.query) != ""
}

// PaletteOpen reports whether the palette shows (cmdpalette.App).
func (a *app) PaletteOpen() bool { return a.palette.open }

// PaletteQuery is the palette's query, which its field edits.
func (a *app) PaletteQuery() *string { return &a.palette.query }

// PaletteView is what the palette shows now, its words in the language the app shows under cx.
func (a *app) PaletteView(cx *core.Scope) cmdpalette.View {
	lines, cmds := a.paletteFound(cx)
	_, inFilm := a.paletteOpenFilm()
	v := cmdpalette.View{
		Label:       locales.PaletteFieldLabel(cx),
		Placeholder: locales.PalettePlaceholder(cx),
		Footer:      locales.PaletteFooter(cx, desktop.Platform(cx)),
		Selected:    -1,
	}
	// In the player the palette also asks the open film, which its field's name and its footer say.
	if inFilm {
		v.Label = locales.PaletteFieldLabelFilm(cx)
		v.Placeholder = locales.PalettePlaceholderFilm(cx)
		v.Footer = locales.PaletteFooterFilm(cx, desktop.Platform(cx))
	}
	asks := 0
	if a.paletteAsks() {
		v.Ask = locales.PaletteAskRow(cx, strings.TrimSpace(a.palette.query))
		asks = 1
	}
	for _, item := range a.paletteFilmsFound() {
		v.Films = append(v.Films, item.Name)
	}
	for _, hit := range lines {
		l := cmdpalette.Line{At: cmdpalette.ClockTenths(hit.cue.Start), Translation: hit.cue.Translation, Original: hit.cue.Original}
		if l.Translation == "" {
			l.Translation, l.Original = l.Original, ""
		}
		if !inFilm {
			l.Film = hit.film.Title
		}
		v.Lines = append(v.Lines, l)
	}
	for _, c := range cmds {
		v.Commands = append(v.Commands, cmdpalette.Command{Label: c.label, Key: c.key})
	}
	found := len(v.Films) + len(v.Lines) + len(v.Commands)
	if total := asks + found; total > 0 {
		v.Selected = min(a.palette.sel, total-1)
	}
	if found == 0 && a.palette.answer == nil && strings.TrimSpace(a.palette.query) != "" {
		v.Empty = locales.PaletteNothingFound(cx)
	}
	if ans := a.palette.answer; ans != nil {
		v.Answer = paletteAnswerView(cx, ans)
	}
	return v
}

// paletteAnswerView is the answer card as palette draws it, in the language the app shows under cx.
func paletteAnswerView(cx *core.Scope, ans *paletteAnswer) *cmdpalette.Answer {
	out := &cmdpalette.Answer{Busy: ans.busy, Text: ans.text, Explain: ans.explain}
	if ans.say != nil {
		out.Text = ans.say(cx)
	}
	if ans.needsAI {
		out.Action = locales.PaletteNoAIAction(cx)
	}
	switch {
	case ans.busy || ans.needsAI || ans.failed || ans.model == "":
	case ans.explained != "":
		out.Note = locales.PaletteExplainNote(cx, ans.model)
	default:
		out.Note = locales.PaletteAnswerNote(cx, cmdpalette.Clock(ans.horizon), ans.model)
	}
	for _, ref := range ans.refs {
		out.Refs = append(out.Refs, cmdpalette.Clock(ref))
	}
	return out
}

// paletteOverlay is the palette over the window while it is open (state_overlay.go).
func (a *app) paletteOverlay(*core.Scope) core.Node { return cmdpalette.Palette{App: a} }

// PaletteEvent does what was typed, pressed and clicked in the palette. It runs in the palette's handlers, on the engine's goroutine; the palette tells the revision after it.
func (a *app) PaletteEvent(ev cmdpalette.Event) {
	if !a.palette.open || a.cx == nil {
		return
	}
	v := a.PaletteView(a.cx)
	total := len(v.Films) + len(v.Lines) + len(v.Commands)
	if v.Ask != "" {
		total++
	}
	switch {
	case ev.Close, ev.Toggle:
		a.paletteClose()
	case ev.Typed:
		a.palette.sel = 0
		a.paletteDropAnswer()
	case ev.Move != 0:
		if total > 0 {
			a.palette.sel = min(max(v.Selected+ev.Move, 0), total-1)
		}
	case ev.Submit:
		a.paletteRun(v.Selected)
	case ev.Pick >= 0:
		a.paletteRun(ev.Pick)
	case ev.Ref >= 0:
		if ans := a.palette.answer; ans != nil && ev.Ref < len(ans.refs) {
			if item, ok := a.paletteFilm(ans.film); ok {
				a.paletteClose()
				a.paletteJump(item, ans.refs[ev.Ref])
			}
		}
	case ev.Action:
		a.paletteClose()
		a.OpenSettings(settings.SectionAI)
	}
}

// paletteRun runs row i, as Return runs the highlighted row and a click the row clicked, counting the row that asks the film first when there is one (paletteAsks), then the films, the lines and the commands: the first asks the film what was typed, and keeps the palette open for the answer; a film closes it and plays the film as its poster does; a line closes it and opens its film at its start, a command closes it and runs.
func (a *app) paletteRun(i int) {
	if a.paletteAsks() {
		if i == 0 {
			a.paletteAsk()
			return
		}
		i--
	}
	found := a.paletteFilmsFound()
	if i >= 0 && i < len(found) {
		a.paletteClose()
		if a.page != pageFilms {
			a.Select(pageFilms)
		}
		a.Open(found[i])
		return
	}
	i -= len(found)
	lines, cmds := a.paletteFound(a.cx)
	switch {
	case i < 0:
		return
	case i < len(lines):
		a.paletteClose()
		a.paletteJump(lines[i].film, lines[i].cue.Start)
	case i < len(lines)+len(cmds):
		a.paletteClose()
		cmds[i-len(lines)].run()
	}
}

// paletteJump opens item playing from at, the start of one of its lines; in the open film, at is where the line shows with the film's subtitle delay.
func (a *app) paletteJump(item films.Film, at time.Duration) {
	if item.Key == a.openKey {
		at += a.player.film.delay
	} else if a.page != pageFilms {
		a.Select(pageFilms)
	}
	a.OpenAt(item, max(at, 0))
}

// paletteDropAnswer forgets the answer card and stops the question on its way, as the query changed.
func (a *app) paletteDropAnswer() {
	a.palette.answer = nil
	if a.palette.cancel != nil {
		a.palette.cancel()
		a.palette.cancel = nil
	}
}

// paletteTranslator is the AI service set in 「AI 服务」, when one is set that can answer: a model at an endpoint, with a key, or through the EveryAPI CLI unless the sidebar's check found it signed out. When it cannot, why says so, in the language the app shows under the scope it is given: noAI when no service is set, which names what was asked of it (a question, or Explain Line), otherwise the words the sidebar's status line uses for the same reason. why is nil when it can.
func (a *app) paletteTranslator(noAI func(*core.Scope) string) (t media.ChatTranslator, why func(*core.Scope) string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return media.ChatTranslator{}, noAI
	}
	t = tools.Translator
	if strings.TrimSpace(t.Model) == "" || strings.TrimSpace(t.BaseURL) == "" {
		return t, noAI
	}
	if everyAPICLIMissing(t) {
		return t, everyAPIMissing
	}
	if t.Credential != nil && a.side.ready.signInDone && a.side.ready.signInErr != nil {
		return t, everyAPISignInFailed(a.side.ready.signInErr)
	}
	return t, nil
}

// paletteAsk asks the AI service the query about the open film, sending its lines up to the furthest point played and nothing after them.
func (a *app) paletteAsk() {
	item, ok := a.paletteOpenFilm()
	if !ok {
		return
	}
	question := strings.TrimSpace(a.palette.query)
	a.paletteDropAnswer()
	t, why := a.paletteTranslator(locales.PaletteNoAI)
	if why != nil {
		a.palette.answer = &paletteAnswer{needsAI: true, say: why}
		return
	}
	f, ok := a.palette.files[item.SubtitlePath]
	if !ok || item.SubtitlePath == "" || len(f.cues) == 0 {
		a.palette.answer = &paletteAnswer{failed: true, say: locales.PaletteNoSubtitles}
		return
	}
	horizon := a.paletteHorizon(item)
	var seen []lines.Line
	for _, cue := range f.cues {
		if cue.Start <= horizon+paletteSlack {
			seen = append(seen, cue)
		}
	}
	if len(seen) == 0 {
		a.palette.answer = &paletteAnswer{failed: true, say: locales.PaletteNotWatched}
		return
	}
	a.paletteSend(t, item, palettePrompt(paletteAnswerLanguage(item, a.prefs.language), horizon), paletteContext(item.Name, seen)+"\n\n问题："+question, seen, paletteAnswer{horizon: horizon})
}

// paletteSend sends user, with the instruction system, to t, the AI service set, about item, and shows its reply in the answer card once it comes: the answer and the lines of seen it rests on (paletteParseReply). Meanwhile the card says the question, or the line to explain, is on its way; when the service fails it says why and offers the service's settings. done is the card the answer fills in: how far the film had been watched, or the line it explains (done.explain), and the chips it has before those the reply names.
func (a *app) paletteSend(t media.ChatTranslator, item films.Film, system, user string, seen []lines.Line, done paletteAnswer) {
	model := strings.TrimSpace(t.Model)
	a.palette.askGen++
	gen := a.palette.askGen
	done.model, done.film = model, item.Key
	busy, failed := locales.PaletteAsking, locales.PaletteAskFailed
	if done.explain {
		busy, failed = locales.PaletteExplaining, locales.PaletteExplainFailed
	}
	a.palette.answer = &paletteAnswer{busy: true, explain: done.explain, say: func(cx *core.Scope) string { return busy(cx, model) }, horizon: done.horizon, model: model, film: item.Key}
	ctx, cancel := context.WithTimeout(context.Background(), paletteAskTimeout)
	a.palette.cancel = cancel
	update := a.update
	go func() {
		defer cancel()
		reply, err := t.Chat(ctx, system, user)
		update(func() {
			if gen != a.palette.askGen || a.palette.answer == nil {
				return
			}
			a.palette.cancel = nil
			if err != nil {
				// The service's settings are the likely fix for a refused key, a wrong model or a signed-out CLI, so the card offers them as it does when none is set.
				a.palette.answer = &paletteAnswer{failed: true, needsAI: true, explain: done.explain, say: func(cx *core.Scope) string { return failed(cx, err.Error()) }}
				return
			}
			text, refs := paletteParseReply(reply, seen)
			for _, ref := range refs {
				if !slices.Contains(done.refs, ref) {
					done.refs = append(done.refs, ref)
				}
			}
			done.text = text
			if done.explained != "" {
				done.text = done.explained + "\n\n" + text
			}
			a.palette.answer = &done
		})
	}()
}

// paletteExplainBefore and paletteExplainAfter are how many lines before the line Explain Line explains, and after it, go with it as its context.
const (
	paletteExplainBefore = 2
	paletteExplainAfter  = 1
)

// ExplainLine is Explain Line on line i of the open film's subtitles, from the subtitle list's right-click menu or E (paletteExplainPlaying): the palette opens with the AI service set explaining the line, its meaning, its key words and its grammar, from the line and the lines around it, with no question to type. The line goes even when it is past the furthest point played, as the subtitle list shows every line. The card starts with the line's original and its first chip plays the line. Without an AI service the card offers its settings, as for a question (paletteAsk).
func (a *app) ExplainLine(i int) {
	item, ok := a.paletteOpenFilm()
	cues := a.playerCues()
	if !ok || i < 0 || i >= len(cues) {
		return
	}
	if !a.palette.open {
		a.paletteShow()
	}
	a.palette.query = ""
	a.paletteDropAnswer()
	t, why := a.paletteTranslator(locales.PaletteExplainNoAI)
	if why != nil {
		a.palette.answer = &paletteAnswer{needsAI: true, explain: true, say: why}
		return
	}
	from, to := max(i-paletteExplainBefore, 0), min(i+paletteExplainAfter+1, len(cues))
	around := make([]lines.Line, 0, to-from)
	for _, cue := range cues[from:to] {
		translation, original := lines.Split(cue.Text)
		around = append(around, lines.New(cue.Start, translation, original))
	}
	line := around[i-from]
	quoted := cmp.Or(line.Original, line.Translation)
	user := paletteContext(item.Name, around) + "\n\n要讲解的一句：[" + cmdpalette.Clock(line.Start) + "] " + quoted
	a.paletteSend(t, item, paletteExplainPrompt(paletteAnswerLanguage(item, a.prefs.language)), user, around, paletteAnswer{explain: true, explained: quoted, refs: []time.Duration{line.Start}})
}

// paletteExplainPlaying is E: Explain Line on the line playing, or between lines on the line just heard, or before the first on the line coming, as L picks the line it loops.
func (a *app) paletteExplainPlaying() {
	prev, cur, next := film.CueNear(a.playerCues(), a.playerLineAt())
	for _, i := range []int{cur, prev, next} {
		if i >= 0 {
			a.ExplainLine(i)
			return
		}
	}
}

// paletteAnswerLanguage is the language the AI service answers about item in: the one its subtitles are translated into, which may not be the one the settings name now, or, for a film translated into none, fallback, the settings' target language.
func paletteAnswerLanguage(item films.Film, fallback string) string {
	if item.TranslationErr == "" {
		if language := strings.TrimSpace(item.Language); language != "" {
			return language
		}
	}
	return fallback
}

// paletteExplainPrompt is the instruction Explain Line is sent with: explain the original of the line named last, in the language subtitles are translated into, as JSON in the form palettePrompt asks for.
func paletteExplainPrompt(language string) string {
	if strings.TrimSpace(language) == "" {
		language = "简体中文"
	}
	return "你在帮用户借助影片学外语。用户消息里是一部影片的几行字幕，每行以 [分:秒] 开头，后面是译文和原文，最后一行是要讲解的那一句。" +
		"用" + language + "讲解这一句的原文：先说整句的意思，再挑出关键的词或短语说明词义和用法，最后讲值得注意的语法或口语说法；前后几行只用来理解语境。" +
		"简短，每部分一两句，分行写。" +
		`只输出一个 JSON 对象：{"answer": "讲解", "refs": ["分:秒"]}，refs 是讲解提到的字幕行开头的时间，照字幕里写的抄，最多 3 个。`
}

// palettePrompt is the instruction a question about a film is sent with: answer from the subtitles seen so far only, in the language subtitles are translated into, as JSON with the moments the answer rests on.
func palettePrompt(language string, horizon time.Duration) string {
	if strings.TrimSpace(language) == "" {
		language = "简体中文"
	}
	return "你在帮用户看懂一部影片。用户消息里是这部影片从开头到 " + cmdpalette.Clock(horizon) + " 的字幕，每行以 [分:秒] 开头，后面是译文和原文。" +
		"只根据这些字幕回答用户的问题；字幕里没讲到的就直说没讲到，不要猜测或透露之后的剧情。" +
		"用" + language + "回答，简短，两三句话以内。" +
		`只输出一个 JSON 对象：{"answer": "回答", "refs": ["分:秒"]}，refs 是回答所依据的字幕行开头的时间，照字幕里写的抄，最多 3 个。`
}

// paletteContext writes the lines seen of the film named name, one a line as "[m:ss] translation / original", leaving out the earliest past paletteContextRunes.
func paletteContext(name string, seen []lines.Line) string {
	lines := make([]string, len(seen))
	for i, cue := range seen {
		text := cue.Translation
		switch {
		case text == "":
			text = cue.Original
		case cue.Original != "":
			text += " / " + cue.Original
		}
		lines[i] = "[" + cmdpalette.Clock(cue.Start) + "] " + text
	}
	size, from := 0, len(lines)
	for from > 0 {
		n := len([]rune(lines[from-1])) + 1
		if size+n > paletteContextRunes {
			break
		}
		size += n
		from--
	}
	return "影片：" + name + "\n字幕：\n" + strings.Join(lines[from:], "\n")
}

// paletteClockRef matches a moment as the prompt asks the model to write it, "1:05" or "1:02:03".
var paletteClockRef = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{2})$`)

// paletteParseReply reads the model's reply: the answer and the lines it rests on, each the seen line that starts at the moment it named. A reply that is not the JSON asked for is shown as it is, without chips; a moment that is not the start of a line seen is dropped, so a chip never jumps past what was watched.
func paletteParseReply(reply string, seen []lines.Line) (string, []time.Duration) {
	var parsed struct {
		Answer string   `json:"answer"`
		Refs   []string `json:"refs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(reply)), &parsed); err != nil || strings.TrimSpace(parsed.Answer) == "" {
		return strings.TrimSpace(reply), nil
	}
	var refs []time.Duration
	for _, ref := range parsed.Refs {
		m := paletteClockRef.FindStringSubmatch(strings.TrimSpace(ref))
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		ss, _ := strconv.Atoi(m[3])
		label := cmdpalette.Clock(time.Duration(h*3600+mm*60+ss) * time.Second)
		for _, cue := range seen {
			if cmdpalette.Clock(cue.Start) == label {
				if !slices.Contains(refs, cue.Start) {
					refs = append(refs, cue.Start)
				}
				break
			}
		}
	}
	return strings.TrimSpace(parsed.Answer), refs
}

// Furthest is how far the video at path has ever played on from its start (films.App), the furthest point the palette keeps.
func (a *app) Furthest(path string) time.Duration { return a.palette.furthest[path] }
