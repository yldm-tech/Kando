package main

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/explain"
	"kando/apps/kando/films"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	"kando/apps/kando/lookup"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// phoneStudy is the lookups and explanations a paired phone asks for over protocol v2 (psync.StudyService): the AI service set in 「AI 服务」 answers them as it answers the word looked up on the picture (LookUpWord) and Explain Line, in the language the film's subtitles are translated into, so the service's key stays on this computer; a word goes into the same cache as the picture's, so a word looked up on either answers again at once on both.
type phoneStudy struct{ a *app }

// phoneStudyNoAI is what a phone is told, in English for its logs, when no AI service can answer: it tells its user in its own words (psync.CodeNoAI).
const phoneStudyNoAI = "no AI service set in Settings › AI Service can answer"

// film is the film of library the call names, as the app files it, for its title and languages; a film the desktop does not serve, a locked library's among them, is none, and the call is asked without them.
func (s phoneStudy) film(ctx context.Context, library, film string) films.Film {
	src := phoneSource{s.a}
	libs, err := src.Libraries(ctx)
	if err != nil {
		return films.Film{}
	}
	i := slices.IndexFunc(libs, func(l phonesync.Library) bool { return l.ID == library })
	if i < 0 {
		return films.Film{}
	}
	list, _, err := src.items(ctx, libs[i])
	if err != nil {
		return films.Film{}
	}
	if j := slices.IndexFunc(list, func(item films.Film) bool { return phoneFilmID(item) == film }); j >= 0 {
		return list[j]
	}
	return films.Film{}
}

// phoneStudyAsk is what a call asks the AI service set with, read on the engine's goroutine: the service, and the language it answers in, as the picture's lookup and Explain Line answer about the film (paletteAnswerLanguage).
type phoneStudyAsk struct {
	t        media.ChatTranslator
	language string
}

// ask is the AI service set and the language it answers about item in, or psync.CodeNoAI when none can answer. UI thread only.
func (s phoneStudy) ask(item films.Film) (phoneStudyAsk, error) {
	a := s.a
	t, why := a.paletteTranslator(locales.VocabLookupNoAI)
	if why != nil {
		return phoneStudyAsk{}, psync.Errorf(psync.CodeNoAI, "%s", phoneStudyNoAI)
	}
	return phoneStudyAsk{t: t, language: paletteAnswerLanguage(item, a.prefs.language)}, nil
}

// LookUp answers what the word means in the line, from the words looked up on this computer, or else as the AI service set says, which it then keeps among them, unless the phone is read-only (psync.Caller.ReadOnly): a read-only phone reads the words looked up and asks the AI service, but changes nothing on this computer, the words looked up among it.
func (s phoneStudy) LookUp(ctx context.Context, caller psync.Caller, req psync.WordLookup) (psync.WordEntry, error) {
	a := s.a
	f := s.film(ctx, req.Library, req.Film)
	title := remoteFilmTitle(f)
	word, line := strings.TrimSpace(req.Word), strings.TrimSpace(req.Line)
	type found struct {
		q      lookup.Query
		entry  lookup.Entry
		cached bool
		ask    phoneStudyAsk
	}
	got, err := remote.OnUI(ctx, a.update, func() (found, error) {
		// The same query as the picture's lookup of the word (vocabQuery), so a word looked up on either answers again at once on both.
		q := vocabQuery(f, a.prefs.language, word, line)
		if e, ok := a.vocabCache().Get(q); ok {
			return found{q: q, entry: e, cached: true}, nil
		}
		ask, err := s.ask(f)
		return found{q: q, ask: ask}, err
	})
	if err != nil {
		return psync.WordEntry{}, err
	}
	if !got.cached {
		ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		defer cancel()
		reply, err := got.ask.t.Chat(ctx, lookup.Prompt(got.q.Language), lookup.Request(word, line, title))
		if err == nil {
			got.entry, err = lookup.Parse(word, reply)
		}
		if err != nil {
			return psync.WordEntry{}, err
		}
		if !caller.ReadOnly {
			_, _ = remote.OnUI(ctx, a.update, func() (struct{}, error) {
				cache := a.vocabCache()
				cache.Put(got.q, got.entry, a.saved.now())
				return struct{}{}, cache.Save()
			})
		}
	}
	return psync.WordEntry{Word: cmp.Or(got.entry.Word, word), Reading: got.entry.Reading, Meaning: got.entry.Meaning, Language: got.q.Language}, nil
}

// Explain answers the AI service's explanation of the line, as Explain Line asks for it on this computer (explain.Prompt): from the line and the lines before it, at most explainBefore of them, so it gives nothing away of what comes after; the lines after it a phone sends are left out. Its sections are the explanation's paragraphs, in the order the side panel shows them.
func (s phoneStudy) Explain(ctx context.Context, _ psync.Caller, req psync.LineExplain) (psync.Explanation, error) {
	a := s.a
	f := s.film(ctx, req.Library, req.Film)
	ask, err := remote.OnUI(ctx, a.update, func() (phoneStudyAsk, error) { return s.ask(f) })
	if err != nil {
		return psync.Explanation{}, err
	}
	cue := func(c psync.Cue) lines.Line {
		return lines.New(time.Duration(c.StartMS)*time.Millisecond, strings.TrimSpace(c.Translation), strings.TrimSpace(c.Original))
	}
	var before []lines.Line
	for _, c := range req.Lines[max(req.Index-explainBefore, 0):req.Index] {
		before = append(before, cue(c))
	}
	line := cue(req.Lines[req.Index])
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()
	reply, err := ask.t.Chat(ctx, explain.Prompt(ask.language, line.Translation != "" && line.Original != ""), explain.Request(remoteFilmTitle(f), before, line))
	var sections explain.Sections
	if err == nil {
		sections, err = explain.Parse(reply)
	}
	if err != nil {
		return psync.Explanation{}, err
	}
	var paragraphs []string
	for _, p := range []string{sections.Meaning, sections.Breakdown, sections.Translation, sections.Tone, sections.Related} {
		if p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	return psync.Explanation{Text: strings.Join(paragraphs, "\n\n"), Language: ask.language, Model: strings.TrimSpace(ask.t.Model)}, nil
}
