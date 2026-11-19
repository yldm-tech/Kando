package main

import (
	"context"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// 这句不对 (⇧E): the translation source chosen in Settings › 翻译 translates the selected line again from the lines around it and the film's glossary (media.Tools.Candidates): a script plugin's translator whose plugin exports candidates (state_plugins_translate.go), else the AI service set (media.ChatTranslator.Candidates). The user picks one of its translations, or keeps the line as it is. The one picked is kept as the user's correction of the line, which no machine pass writes over.

// correctPick is the other translations of the selected line while they show. busy is set while they are on their way, gen telling their reply from one of a request made before; options are the translation source's, best first, and at the one highlighted, 0 for the line as it is. say, when set, says why there are none, and needsAI that the AI service's settings are the fix; a script plugin's translator that failed is not the AI service's to fix.
type correctPick struct {
	busy    bool
	gen     int
	cancel  context.CancelFunc
	options []string
	at      int
	say     func(cx *ns.Scope) string
	needsAI bool
}

// correctPickBefore and correctPickAfter are how many lines before and after the selected one go with it as its context: more than Explain Line sends, as the line is to be translated again, and a line before often names who or what it speaks of.
const (
	correctPickBefore = 6
	correctPickAfter  = 3
)

// correctPickTimeout bounds how long the other translations of a line are waited for.
const correctPickTimeout = 90 * time.Second

// correctPicking reports whether the other translations of the selected line show, while their keys work (correctKeys) and the player leaves Space, Escape and the arrows to them (playerKeys).
func (a *app) correctPicking() bool { return a.correct.pick != nil && a.correctSelected() >= 0 }

// correctCanPick reports whether ⇧E can ask for other translations now: the film has lines, none is being edited, and none is being asked for.
func (a *app) correctCanPick() bool {
	return a.correctCanSelect() && !a.correct.editing && a.correct.pick == nil
}

// StartCandidates is 这句不对 (⇧E): the selected line, or with none the line playing (correctSelectPlaying), held on the picture while the translation source translates it again, its translations to pick from showing over it (film.Corrector): the script plugin's translator chosen when its plugin offers them, else the AI service. Without an AI service, where it is the one asked, they say why and offer its settings.
func (a *app) StartCandidates() {
	if !a.correctCanPick() {
		return
	}
	if a.correctSelected() < 0 {
		a.correctSelectPlaying()
	}
	i := a.correctSelected()
	if i < 0 {
		return
	}
	a.correct.spread = nil
	a.correct.pickGen++
	pick := &correctPick{gen: a.correct.pickGen}
	a.correct.pick = pick
	item, open := a.paletteOpenFilm()
	tools, _ := a.pipeline.(media.Tools)
	tools = a.pluginsTranslateTools(tools, item.Spoken)
	if _, offers := tools.Engine.(pluginsCandidater); !offers {
		t, why := a.paletteTranslator(locales.CorrectCandidatesNoAI)
		if why != nil {
			pick.say, pick.needsAI = why, true
			return
		}
		tools.Translator = t
	}
	_, from := a.correctLines()
	req := media.CandidateRequest{Line: from[i].Line, Terms: a.FilmTerms()}
	for _, l := range from[max(i-correctPickBefore, 0):i] {
		req.Before = append(req.Before, l.Line)
	}
	for _, l := range from[i+1 : min(i+1+correctPickAfter, len(from))] {
		req.After = append(req.After, l.Line)
	}
	req.Language = a.prefs.language
	if open {
		req.Language = paletteAnswerLanguage(item, a.prefs.language)
	}
	pick.busy = true
	gen := pick.gen
	ask := func(ctx context.Context) ([]string, error) { return tools.Candidates(ctx, req) }
	pick.cancel = aiAsk(a, correctPickTimeout, ask, func() bool { return a.correct.pick != nil && a.correct.pick.gen == gen }, func(options []string, err error) {
		p := a.correct.pick
		p.busy, p.cancel = false, nil
		source, bySource := media.EngineOf(err)
		switch {
		case err != nil && bySource:
			// A script plugin's translator failed: its card's words, as a job's card says them (jobfailure.SourceHint), and not the AI service's settings.
			detail := err.Error()
			p.say = func(cx *ns.Scope) string {
				reason, _ := jobfailure.SourceHint(locales.For(cx), source, detail)
				return reason
			}
		case err != nil:
			// The service's settings are the likely fix for a refused key, a wrong model or a signed-out CLI, as for Explain Line.
			p.say = func(cx *ns.Scope) string { return locales.CorrectCandidatesFailed(cx, err.Error()) }
			p.needsAI = true
		case len(options) == 0:
			p.say = locales.CorrectCandidatesNone
		default:
			p.options, p.at = options, 1
		}
	})
}

// Candidates is the other translations of the selected line as they show (film.Corrector).
func (a *app) Candidates() film.Candidates {
	p := a.correct.pick
	i := a.correctSelected()
	if p == nil || i < 0 {
		return film.Candidates{}
	}
	_, from := a.correctLines()
	return film.Candidates{Open: true, Busy: p.busy, Original: from[i].Original, Current: from[i].Translation, Options: p.options, At: p.at, Say: p.say, NeedsAI: p.needsAI}
}

// correctMovePick highlights the translation d after the one highlighted, d before for a negative d, the line as it is first.
func (a *app) correctMovePick(d int) {
	p := a.correct.pick
	if p == nil || p.busy || len(p.options) == 0 {
		return
	}
	p.at = min(max(p.at+d, 0), len(p.options))
}

// PickCandidate keeps translation i of the selected line, 0 being the line as it is, and puts the translations away, the line staying selected (film.Corrector). The one kept is the user's correction of the line, which Undo takes back.
func (a *app) PickCandidate(i int) {
	p := a.correct.pick
	if p == nil || p.busy || i < 0 || i > len(p.options) {
		return
	}
	line := a.correctSelected()
	if i == 0 || line < 0 {
		a.CloseCandidates()
		return
	}
	_, from := a.correctLines()
	machineTranslation, _ := a.correctMachineText(from[line].Machine)
	c := a.correctionOf(from[line])
	c.Translation = textOverride(p.options[i-1], machineTranslation)
	if !a.correctSet(from[line], c, correctStep{kind: correctKindCandidate, line: line + 1}) {
		return
	}
	a.CloseCandidates()
	a.flash(a.messages().CorrectCandidateKept())
}

// correctPickHighlighted keeps the translation highlighted (Return).
func (a *app) correctPickHighlighted() {
	if p := a.correct.pick; p != nil {
		if p.say != nil {
			a.CloseCandidates()
			return
		}
		a.PickCandidate(p.at)
	}
}

// CloseCandidates puts the other translations away, and stops asking for them, the line staying selected (film.Corrector).
func (a *app) CloseCandidates() {
	if p := a.correct.pick; p != nil && p.cancel != nil {
		p.cancel()
	}
	a.correct.pick = nil
}

// CandidateSettings opens the AI service's settings, when no AI service could give translations (film.Corrector).
func (a *app) CandidateSettings() {
	a.CloseCandidates()
	a.OpenSettings(settings.SectionAI)
}
