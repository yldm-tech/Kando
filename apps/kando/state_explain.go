package main

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"time"

	"kando/apps/kando/explain"
	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// explainUI is Explain Line's: the line the side panel explains (explainLine), and what the AI service said of the lines explained since the app started (cache), so going back to a line shows its explanation again without asking for it again.
type explainUI struct {
	line  explainLine
	cache map[explainKey]explain.Sections
	// gen numbers the explanations and questions asked, so an answer to one the learner left is dropped; cancel stops those on their way.
	gen    int
	cancel []context.CancelFunc
}

// explainLine is the line explained: the film's video, name and the language it is explained in, the line's index among the film's lines and where it starts, its original and translation, and the lines before it, which go with it as its context; busy while the AI service is asked, failed when no explanation came, with say saying why and needsAI set when the AI service's settings are what helps; the model that explained it, and whether it ran on this computer; the explanation, more once 展开 was pressed, the questions asked and the one being typed.
type explainLine struct {
	open                  bool
	video, film, language string
	index                 int
	start                 time.Duration
	line                  lines.Line
	before                []lines.Line
	busy, failed, needsAI bool
	say                   func(*core.Scope) string
	model                 string
	local                 bool
	sections              explain.Sections
	more                  bool
	asked                 []explainQA
	question              string
}

// explainQA is a follow-up question: its answer once it came, busy while it is asked, failed with say saying why when none came.
type explainQA struct {
	question, answer string
	busy, failed     bool
	say              func(*core.Scope) string
}

// explainKey is what an explanation is cached by: the line of a video, the language it is explained in and the model that explained it.
type explainKey struct {
	video, language, model string
	start                  time.Duration
}

const (
	// explainBefore is how many lines before the line explained go with it as its context; none after it goes, so the explanation gives nothing away.
	explainBefore = 3
	// explainTimeout is how long an explanation or a follow-up question may take.
	explainTimeout = 90 * time.Second
)

func newExplainUI() explainUI { return explainUI{cache: map[explainKey]explain.Sections{}} }

// explainHooks: the explanation goes once its film is not the one open, and stops what it asks as the app quits.
func (a *app) explainHooks() areaHooks {
	return areaHooks{
		afterChange: a.explainFollowFilm,
		quitting:    a.explainStop,
	}
}

// explainFollowFilm closes the explanation once its film is not the one open, the side panel going back to the subtitle list, and reports whether it did. Closing the film counts, its video kept until another opens (closeFilm), so opening the same film again does not bring back the explanation or keep what it asked on its way.
func (a *app) explainFollowFilm() bool {
	l := a.explain.line
	if !l.open || (a.openKey != "" && a.player.play != nil && a.player.film.video == l.video) {
		return false
	}
	a.explainClose()
	return true
}

// explainStop stops the explanation and the questions on their way, whose answers are then dropped.
func (a *app) explainStop() {
	a.explain.gen++
	for _, cancel := range a.explain.cancel {
		cancel()
	}
	a.explain.cancel = nil
}

// explainClose forgets the line explained and shows the subtitle list again where the panel showed it.
func (a *app) explainClose() {
	a.explainStop()
	a.explain.line = explainLine{}
	if a.player.filmSide == film.SideExplain {
		a.player.filmSide = ""
	}
}

// ExplainLine is Explain Line on line i of the open film's lines, from the subtitle list's right-click menu, the held line's card, the looked-up word's card, the correct bar and I (explainPlaying): the film holds, and the side panel, shown if it was hidden, explains the line in place of its tabs' content, with what the AI service set in 「AI 服务」 says of it from the line and the lines before it (explain.Prompt), asked off the engine's goroutine, or said before for the same line. Without an AI service that can answer, as while offline mode is on and it is not on this computer, the panel says why and offers its settings.
func (a *app) ExplainLine(i int) {
	cues := a.playerCues()
	item, ok := a.openFilm()
	if !ok || i < 0 || i >= len(cues) || a.player.film.video == "" {
		return
	}
	if p := a.Play(); p != nil && p.Playing() {
		p.Toggle()
	}
	a.explainStop()
	from := max(i-explainBefore, 0)
	before := make([]lines.Line, 0, i-from)
	for _, cue := range cues[from:i] {
		translation, original := lines.Split(cue.Text)
		before = append(before, lines.New(cue.Start, translation, original))
	}
	translation, original := lines.Split(cues[i].Text)
	a.explain.line = explainLine{open: true, video: a.player.film.video, film: cmp.Or(item.Name, item.Title), language: paletteAnswerLanguage(item, a.prefs.language),
		index: i, start: cues[i].Start, line: lines.New(cues[i].Start, translation, original), before: before}
	a.player.film.styleView = false
	a.player.panel = true
	a.player.filmSide = film.SideExplain
	t, why := a.paletteTranslator(locales.ExplainNoAI)
	l := &a.explain.line
	if why != nil {
		l.failed, l.needsAI, l.say = true, true, why
		return
	}
	l.model, l.local = strings.TrimSpace(t.Model), t.Local()
	key := explainKey{video: l.video, language: l.language, model: l.model, start: l.start}
	if s, ok := a.explain.cache[key]; ok {
		l.sections = s
		return
	}
	model := l.model
	l.busy, l.say = true, func(cx *core.Scope) string { return locales.ExplainBusy(cx, model) }
	system, user := explain.Prompt(l.language, translation != "" && original != ""), explain.Request(l.film, before, l.line)
	a.explainAsk(t, system, user, func(reply string, err error) {
		l := &a.explain.line
		l.busy, l.say = false, nil
		var s explain.Sections
		if err == nil {
			s, err = explain.Parse(reply)
		}
		if err != nil {
			msg := err.Error()
			l.failed, l.needsAI = true, !errors.Is(err, explain.ErrEmpty)
			l.say = func(cx *core.Scope) string { return locales.ExplainFailed(cx, msg) }
			return
		}
		l.sections = s
		a.explain.cache[key] = s
	})
}

// explainAsk sends user, with the instruction system, to t off the engine's goroutine, and hands its reply to done back on it, unless the line explained changed or was closed meanwhile (explainStop).
func (a *app) explainAsk(t media.ChatTranslator, system, user string, done func(reply string, err error)) {
	gen := a.explain.gen
	ctx, cancel := context.WithTimeout(context.Background(), explainTimeout)
	a.explain.cancel = append(a.explain.cancel, cancel)
	update := a.update
	go func() {
		defer cancel()
		reply, err := t.Chat(ctx, system, user)
		update(func() {
			if gen != a.explain.gen || !a.explain.line.open {
				return
			}
			done(reply, err)
		})
	}()
}

// explainPlaying is I: Explain Line on the line selected for correcting (E), or with none the line playing, or between lines on the line just heard, or before the first on the line coming, as L picks the line it loops.
func (a *app) explainPlaying() {
	if i := a.correctSelected(); i >= 0 {
		a.ExplainLine(i)
		return
	}
	prev, cur, next := film.CueNear(a.playerCues(), a.playerLineAt())
	for _, i := range []int{cur, prev, next} {
		if i >= 0 {
			a.ExplainLine(i)
			return
		}
	}
}

// Explanation is the line explained, as the side panel shows it.
func (a *app) Explanation() film.Explanation {
	l := a.explain.line
	if !l.open {
		return film.Explanation{}
	}
	out := film.Explanation{Open: true, At: l.start, Original: cmp.Or(l.line.Original, l.line.Translation), Busy: l.busy, Failed: l.failed, NeedsAI: l.needsAI, Say: l.say,
		Saved: a.LineSaved(l.index)}
	if l.line.Original != "" {
		out.Translation = l.line.Translation
	}
	add := func(kind, body string) {
		if body = strings.TrimSpace(body); body != "" {
			out.Sections = append(out.Sections, film.ExplainSection{Kind: kind, Body: body})
		}
	}
	if !l.busy && !l.failed {
		s := l.sections
		add(film.ExplainMeaning, s.Meaning)
		add(film.ExplainBreakdown, s.Breakdown)
		add(film.ExplainTranslation, s.Translation)
		add(film.ExplainTerms, strings.Join(explain.Terms(l.line.Original, a.FilmTerms()), "\n"))
		if l.more {
			add(film.ExplainTone, s.Tone)
			add(film.ExplainRelated, s.Related)
		}
		out.More = !l.more && s.HasMore()
		model, local := l.model, l.local
		out.Note = func(cx *core.Scope) string {
			if local {
				return locales.ExplainNoteLocal(cx, model)
			}
			return locales.ExplainNote(cx, model)
		}
	}
	for _, q := range l.asked {
		qa := film.ExplainQA{Question: q.question, Busy: q.busy, Failed: q.failed}
		switch answer, say := q.answer, q.say; {
		case say != nil:
			qa.Answer = say
		default:
			qa.Answer = func(*core.Scope) string { return answer }
		}
		out.Asked = append(out.Asked, qa)
	}
	return out
}

// CloseExplain is ‹ 字幕: the side panel goes back to the subtitle list.
func (a *app) CloseExplain() { a.explainClose() }

// ExpandExplain is 展开: the explanation shows its tone and related expressions too.
func (a *app) ExpandExplain() { a.explain.line.more = true }

// ExplainSettings is the panel's Open AI Service Settings, when no AI service could explain the line.
func (a *app) ExplainSettings() { a.OpenSettings(settings.SectionAI) }

// ExplainQuestion is the follow-up question being typed.
func (a *app) ExplainQuestion() *string { return &a.explain.line.question }

// AskExplain asks the AI service the follow-up question typed about the line explained, with the line, the lines before it, what was explained of it and the questions asked before (explain.FollowRequest), and empties the field. The question shows under the explanation at once, its answer once it comes. Without an AI service that can answer, it says why there instead.
func (a *app) AskExplain() {
	l := &a.explain.line
	question := strings.TrimSpace(l.question)
	if !l.open || question == "" {
		return
	}
	l.question = ""
	var asked []explain.QA
	for _, q := range l.asked {
		if !q.busy && !q.failed {
			asked = append(asked, explain.QA{Question: q.question, Answer: q.answer})
		}
	}
	n := len(l.asked)
	t, why := a.paletteTranslator(locales.ExplainNoAI)
	if why != nil {
		l.asked = append(l.asked, explainQA{question: question, failed: true, say: why})
		return
	}
	l.asked = append(l.asked, explainQA{question: question, busy: true, say: locales.ExplainAnswering})
	system, user := explain.FollowPrompt(l.language), explain.FollowRequest(l.film, l.before, l.line, l.sections, asked, question)
	a.explainAsk(t, system, user, func(reply string, err error) {
		l := &a.explain.line
		if n >= len(l.asked) {
			return
		}
		q := &l.asked[n]
		q.busy = false
		reply = strings.TrimSpace(reply)
		if err == nil && reply == "" {
			err = explain.ErrEmpty
		}
		if err != nil {
			msg := err.Error()
			q.failed, q.say = true, func(cx *core.Scope) string { return locales.ExplainAskFailed(cx, msg) }
			return
		}
		q.answer, q.say = reply, nil
	})
}

// SaveExplained is 存这句到笔记: the line explained goes into the saved lines, which the daily review asks, with what the explanation says it means as its note, and the picture says so. A line saved already keeps its note, or takes the explanation's when it has none.
func (a *app) SaveExplained() {
	l := a.explain.line
	cues := a.playerCues()
	if !l.open || l.index >= len(cues) || !saved.SameStart(cues[l.index].Start, l.start) {
		return
	}
	a.savedSync()
	book := &a.saved.book
	note := ""
	if !l.busy && !l.failed {
		note = l.sections.Meaning
	}
	if i := book.Find(book.Key(l.video), l.start); i >= 0 {
		if note != "" && book.Lines[i].Note == "" && book.SetNote(book.Lines[i].ID, note) {
			a.flash(a.messages().ExplainOsdNoted())
			a.savedWrite()
			return
		}
		a.flash(a.messages().ExplainOsdInNotes())
		return
	}
	book.Toggle(saved.Line{Video: book.Key(l.video), Start: l.start, End: cues[l.index].End, Original: l.line.Original, Translation: l.line.Translation, Note: note, Film: l.film, SavedAt: a.saved.now()})
	a.flash(a.messages().ExplainOsdNoted())
	a.savedWrite()
}
