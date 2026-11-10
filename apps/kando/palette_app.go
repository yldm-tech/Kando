package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	cmdpalette "kando/apps/kando/palette"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// The command palette's behavior: what it finds for the query, what choosing a row does, and asking the AI service about the open film. package palette draws it.

const (
	// paletteFirstCommands is how many commands the palette lists before anything is typed, as the prototype's.
	paletteFirstCommands = 5
	// paletteFilmLines and paletteLibraryLines are how many lines it lists for a query: the prototype's four from the open film, and more over the library, whose lines come from every film in it.
	paletteFilmLines    = 4
	paletteLibraryLines = 8
	// paletteSlack is how far past the furthest point played a line may start and still count as seen, as the prototype's half second.
	paletteSlack = 500 * time.Millisecond
	// paletteContextRunes caps the subtitles sent with a question; past it the earliest lines are left out, so a long film still fits a small model's context.
	paletteContextRunes = 40000
	// paletteAskTimeout is how long a question may take.
	paletteAskTimeout = 90 * time.Second
)

// paletteCommand is a row of the palette's 命令 group: what it says, its shortcut outside the palette, and what it does.
type paletteCommand struct {
	label string
	mods  ui.Modifiers
	key   ui.Key
	run   func()
}

// togglePalette is ⌘K: it opens the palette with an empty query, or closes it again.
func (a *app) togglePalette() {
	if a.palette.open {
		a.paletteClose()
		return
	}
	a.palette.open = true
	a.palette.query = ""
	a.palette.sel = 0
	a.palette.moved = false
	a.palette.answer = nil
	a.palette.hitsFor = ""
	if a.palette.release == nil {
		a.palette.release = a.holdModal()
	}
	a.paletteRead()
}

// paletteClose closes the palette, stops a question on its way and gives the window's modal flag back.
func (a *app) paletteClose() {
	a.palette.open = false
	a.paletteSettle()
}

// paletteSettle lets go of what the palette holds once it is closed, however it was closed: by paletteClose, or by Escape or a click outside it, which palette.Show reports by clearing open.
func (a *app) paletteSettle() {
	if a.palette.open {
		return
	}
	if a.palette.cancel != nil {
		a.palette.cancel()
		a.palette.cancel = nil
	}
	if a.palette.release != nil {
		a.palette.release()
		a.palette.release = nil
	}
}

// paletteAfterFrame notes how far the open video has played and lets go of what a palette closed in this frame held.
func (a *app) paletteAfterFrame() {
	a.paletteNotePlayed(a.player.play)
	a.paletteSettle()
}

// paletteNotePlayed moves the furthest point played of the open video with its picture, but only while it plays on from what was watched already: from the frame before, in the same playback, and from no further than the furthest point played. A seek opens the video again as another playback (seekPlayback), so where it lands is not taken as played, nor is a picture that moved while paused; and playing on from a seek past the furthest point does not count either, so a seek to the end does not give the film away. The prototype likewise moves seen only in its playback tick.
func (a *app) paletteNotePlayed(play *filmPlay) {
	if play == nil || play.path == "" {
		a.palette.watching = nil
		return
	}
	at := play.playhead()
	prev, same := a.palette.watchedAt, a.palette.watching == play
	a.palette.watching, a.palette.watchedAt = play, at
	if !same || !play.Playing() || prev > a.palette.furthest[play.path]+paletteSlack {
		return
	}
	a.paletteNoteFurthest(play.path, at)
}

// paletteNoteFurthest keeps at as how far the video at path has played, when it is further than before.
func (a *app) paletteNoteFurthest(path string, at time.Duration) {
	if path == "" || at <= a.palette.furthest[path] {
		return
	}
	if a.palette.furthest == nil {
		a.palette.furthest = map[string]time.Duration{}
	}
	a.palette.furthest[path] = at
	a.palette.dirty = true
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
		a.saveErr = shelfSaveErr(err, locales.PaletteLibrarySaveFailed)
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

// paletteHorizon is how far the film has been seen: the furthest point its video played to from its start. A picture a seek moved further, which paletteNotePlayed does not count, does not move it, so the palette neither lists nor sends a line the seek passed.
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
			data, err := os.ReadFile(w.path)
			if err != nil {
				continue
			}
			segments, err := media.ParseSRT(data)
			if err != nil {
				continue
			}
			cues := make([]paletteCue, len(segments))
			for i, seg := range segments {
				translation, original := paletteSplit(seg.Text)
				cues[i] = paletteCue{start: seg.Start, translation: translation, original: original, folded: strings.ToLower(translation + "\n" + original)}
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

// paletteSplit is a cue of an exported subtitle file as its translation and its original: the pipeline writes the translation on the first line and the original under it, and a cue of one line is an original without a translation.
func paletteSplit(text string) (translation, original string) {
	text = strings.TrimSpace(text)
	first, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return "", text
	}
	return strings.TrimSpace(first), strings.TrimSpace(rest)
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
			if strings.Contains(cue.folded, query) {
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
		if (!inFilm && h <= 0) || hit.cue.start > h+paletteSlack {
			continue
		}
		out = append(out, hit)
		if len(out) == limit {
			break
		}
	}
	return out
}

// paletteCommands are the commands the palette offers now: in the player, the subtitle modes it is not in; then every row of the shortcut table that applies now but the palette's own and the screen the window shows already, the player's first while a film is open; then a row for each settings section.
func (a *app) paletteCommands() []paletteCommand {
	var out []paletteCommand
	_, inFilm := a.paletteOpenFilm()
	if inFilm {
		for _, mode := range []string{film.SubTranslation, film.SubBilingual, film.SubOriginal} {
			if mode == a.player.subMode {
				continue
			}
			out = append(out, paletteCommand{label: locales.PaletteSwitchTo(film.ModeName(mode)), run: func() {
				a.SetSubMode(mode)
				a.flash(locales.PaletteSubtitleMode(film.ModeName(mode)))
			}})
		}
	}
	rows := slices.Clone(a.shortcuts())
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
		if s.Mods == ui.Cmd && s.Key == ui.KeyK {
			continue
		}
		if s.Group == groupWindow && s.Checked != nil && s.Checked() {
			continue
		}
		out = append(out, paletteCommand{label: s.Label, mods: s.Mods, key: s.Key, run: s.guarded()})
	}
	for _, id := range settings.IDs() {
		out = append(out, paletteCommand{label: locales.PaletteSettingsSection(settings.Title(id)), run: func() { a.OpenSettings(id) }})
	}
	return out
}

// paletteFound is what the palette lists for the query: its lines and the commands whose names hold it, or the first commands while nothing is typed.
func (a *app) paletteFound() ([]paletteHit, []paletteCommand) {
	query := strings.ToLower(strings.TrimSpace(a.palette.query))
	cmds := a.paletteCommands()
	if query == "" {
		return nil, cmds[:min(len(cmds), paletteFirstCommands)]
	}
	var found []paletteCommand
	for _, c := range cmds {
		if strings.Contains(strings.ToLower(c.label), query) {
			found = append(found, c)
		}
	}
	return a.paletteLines(), found
}

// paletteView is what the palette shows now.
func (a *app) paletteView() cmdpalette.View {
	lines, cmds := a.paletteFound()
	_, inFilm := a.paletteOpenFilm()
	v := cmdpalette.View{
		Query:       &a.palette.query,
		Placeholder: locales.PalettePlaceholder(),
		Footer:      locales.PaletteFooter(),
		Selected:    -1,
		Reveal:      a.palette.reveal,
	}
	if inFilm {
		v.Placeholder = locales.PalettePlaceholderFilm()
		v.Footer = locales.PaletteFooterFilm()
	}
	for _, hit := range lines {
		l := cmdpalette.Line{At: cmdpalette.ClockTenths(hit.cue.start), Translation: hit.cue.translation, Original: hit.cue.original}
		if l.Translation == "" {
			l.Translation, l.Original = l.Original, ""
		}
		if !inFilm {
			l.Film = hit.film.Title
		}
		v.Lines = append(v.Lines, l)
	}
	for _, c := range cmds {
		v.Commands = append(v.Commands, cmdpalette.Command{Label: c.label, Mods: c.mods, Key: c.key})
	}
	total := len(v.Lines) + len(v.Commands)
	if total > 0 {
		v.Selected = min(a.palette.sel, total-1)
	}
	if total == 0 && a.palette.answer == nil && strings.TrimSpace(a.palette.query) != "" {
		// Over the library the note says nothing was found; in the player it says Return asks the query instead.
		v.Empty = locales.PaletteNothingFound()
		if inFilm {
			v.Empty = locales.PaletteNothingFoundAsk()
		}
	}
	if ans := a.palette.answer; ans != nil {
		v.Answer = a.paletteAnswerView(ans)
	}
	return v
}

// paletteAnswerView is the answer card as palette draws it.
func (a *app) paletteAnswerView(ans *paletteAnswer) *cmdpalette.Answer {
	out := &cmdpalette.Answer{Busy: ans.busy, Text: ans.text}
	if ans.say != nil {
		out.Text = ans.say()
	}
	if ans.needsAI {
		out.Action = locales.PaletteNoAIAction()
	}
	if !ans.busy && !ans.needsAI && !ans.failed && ans.model != "" {
		out.Note = locales.PaletteAnswerNote(cmdpalette.Clock(ans.horizon), ans.model)
	}
	for _, ref := range ans.refs {
		out.Refs = append(out.Refs, cmdpalette.Clock(ref))
	}
	return out
}

// paletteOverlay draws the palette while it is open and does what was typed, pressed and clicked in it.
func (a *app) paletteOverlay(c *ui.Context, dark *ui.Theme) {
	if !a.palette.open {
		return
	}
	v := a.paletteView()
	a.palette.reveal = false
	ev := cmdpalette.Show(c, dark, &a.palette.open, v)
	if !a.palette.open {
		return
	}
	total := len(v.Lines) + len(v.Commands)
	switch {
	case ev.Toggle:
		a.paletteClose()
	case ev.Typed:
		a.palette.sel = 0
		a.palette.moved = false
		a.paletteDropAnswer()
	case ev.Move != 0:
		if total > 0 {
			a.palette.sel = min(max(v.Selected+ev.Move, 0), total-1)
			a.palette.moved = true
			a.palette.reveal = true
		}
	case ev.Submit:
		a.paletteSubmit(v)
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

// paletteSubmit is Return in the field. In the player, a question typed and not yet moved off with the arrow keys is asked of the film, as the prototype's footer says; otherwise the highlighted row runs.
func (a *app) paletteSubmit(v cmdpalette.View) {
	_, inFilm := a.paletteOpenFilm()
	if inFilm && strings.TrimSpace(a.palette.query) != "" && !a.palette.moved {
		a.paletteAsk()
		return
	}
	if v.Selected >= 0 {
		a.paletteRun(v.Selected)
	}
}

// paletteRun closes the palette and runs row i, counting the lines first and the commands after them: a line opens its film at its start, a command runs.
func (a *app) paletteRun(i int) {
	lines, cmds := a.paletteFound()
	switch {
	case i < 0:
		return
	case i < len(lines):
		a.paletteClose()
		a.paletteJump(lines[i].film, lines[i].cue.start)
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

// paletteTranslator is the AI service set in 「AI 服务」, when one is set that can answer: a model at an endpoint, with a key, or through the EveryAPI CLI unless the sidebar's check found it signed out.
func (a *app) paletteTranslator() (media.ChatTranslator, bool) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return media.ChatTranslator{}, false
	}
	t := tools.Translator
	if strings.TrimSpace(t.Model) == "" || strings.TrimSpace(t.BaseURL) == "" {
		return t, false
	}
	if t.Credential == nil && t.BaseURL == media.EveryAPIBaseURL && t.APIKey == "" {
		return t, false
	}
	if t.Credential != nil && a.side.ready.signInDone && a.side.ready.signInErr != nil {
		return t, false
	}
	return t, true
}

// paletteAsk asks the AI service the query about the open film, sending its lines up to the furthest point played and nothing after them.
func (a *app) paletteAsk() {
	item, ok := a.paletteOpenFilm()
	if !ok {
		return
	}
	question := strings.TrimSpace(a.palette.query)
	a.paletteDropAnswer()
	t, ok := a.paletteTranslator()
	if !ok {
		a.palette.answer = &paletteAnswer{needsAI: true, say: locales.PaletteNoAI}
		return
	}
	f, ok := a.palette.files[item.SubtitlePath]
	if !ok || item.SubtitlePath == "" || len(f.cues) == 0 {
		a.palette.answer = &paletteAnswer{failed: true, say: locales.PaletteNoSubtitles}
		return
	}
	horizon := a.paletteHorizon(item)
	var seen []paletteCue
	for _, cue := range f.cues {
		if cue.start <= horizon+paletteSlack {
			seen = append(seen, cue)
		}
	}
	if len(seen) == 0 {
		a.palette.answer = &paletteAnswer{failed: true, say: locales.PaletteNotWatched}
		return
	}
	model := strings.TrimSpace(t.Model)
	a.palette.askGen++
	gen := a.palette.askGen
	a.palette.answer = &paletteAnswer{busy: true, say: func() string { return locales.PaletteAsking(model) }, horizon: horizon, model: model, film: item.Key}
	ctx, cancel := context.WithTimeout(context.Background(), paletteAskTimeout)
	a.palette.cancel = cancel
	system := palettePrompt(a.prefs.language, horizon)
	user := paletteContext(item.Name, seen) + "\n\n问题：" + question
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
				a.palette.answer = &paletteAnswer{failed: true, needsAI: true, say: func() string { return locales.PaletteAskFailed(err.Error()) }}
				return
			}
			text, refs := paletteParseReply(reply, seen)
			a.palette.answer = &paletteAnswer{text: text, refs: refs, horizon: horizon, model: model, film: item.Key}
		})
	}()
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
func paletteContext(name string, seen []paletteCue) string {
	lines := make([]string, len(seen))
	for i, cue := range seen {
		text := cue.translation
		switch {
		case text == "":
			text = cue.original
		case cue.original != "":
			text += " / " + cue.original
		}
		lines[i] = "[" + cmdpalette.Clock(cue.start) + "] " + text
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
func paletteParseReply(reply string, seen []paletteCue) (string, []time.Duration) {
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
			if cmdpalette.Clock(cue.start) == label {
				if !slices.Contains(refs, cue.start) {
					refs = append(refs, cue.start)
				}
				break
			}
		}
	}
	return strings.TrimSpace(parsed.Answer), refs
}
