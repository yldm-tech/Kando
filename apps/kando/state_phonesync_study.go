package main

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"kando/apps/kando/explain"
	"kando/apps/kando/films"
	"kando/apps/kando/lines"
	"kando/apps/kando/locales"
	"kando/apps/kando/lookup"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/recap"
	"kando/apps/kando/remote"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// phoneStudy is the lookups, explanations, follow-up questions and recaps a paired phone asks for over protocol v2 (psync.StudyService): the AI service set in 「AI 服务」 answers them as it answers the word looked up on the picture (LookUpWord), Explain Line, its question field and the 前情提要 card, a lookup the dictionary packs turned on first, in the language the film's subtitles are translated into, so the service's key stays on this computer; a word goes into the same cache as the picture's, so a word looked up on either answers again at once on both.
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

// LookUp answers what the word means in the line as the picture's lookup does (vocabResolve): from the words looked up on this computer, else from the dictionary packs turned on, offline and without an AI service, else as the AI service set says, whose answer it then keeps among the words looked up, unless the phone is read-only (psync.Caller.ReadOnly): a read-only phone reads the words looked up, the dictionaries and asks the AI service, but changes nothing on this computer, the words looked up among it. Only when no dictionary has the word and no AI service can answer is it psync.CodeNoAI.
func (s phoneStudy) LookUp(ctx context.Context, caller psync.Caller, req psync.WordLookup) (psync.WordEntry, error) {
	a := s.a
	f := s.film(ctx, req.Library, req.Film)
	word, line := strings.TrimSpace(req.Word), strings.TrimSpace(req.Line)
	ask, err := remote.OnUI(ctx, a.update, func() (vocabAsk, error) {
		// The same query as the picture's lookup of the word (vocabQuery), so a word looked up on either answers again at once on both.
		return a.vocabResolve(vocabQuery(f, a.prefs.language, word, line), remoteFilmTitle(f)), nil
	})
	if err != nil {
		return psync.WordEntry{}, err
	}
	entry := ask.entry
	if !ask.cached {
		ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		defer cancel()
		var source string
		entry, source, err = ask.answer(ctx)
		if errors.Is(err, lookup.ErrNoAnswer) {
			return psync.WordEntry{}, psync.Errorf(psync.CodeNoAI, "%s", phoneStudyNoAI)
		}
		if err != nil {
			return psync.WordEntry{}, err
		}
		if source == "" && !caller.ReadOnly {
			_, _ = remote.OnUI(ctx, a.update, func() (struct{}, error) {
				cache := a.vocabCache()
				cache.Put(ask.ask.Query, entry, a.saved.now())
				return struct{}{}, cache.Save()
			})
		}
	}
	return psync.WordEntry{Word: cmp.Or(entry.Word, word), Reading: entry.Reading, Meaning: entry.Meaning, Language: ask.ask.Query.Language}, nil
}

// Explain answers the AI service's explanation of the line, as Explain Line asks for it on this computer (explain.Prompt): from the line and the lines before it, at most explainBefore of them, so it gives nothing away of what comes after; the lines after it a phone sends are left out. Its sections are those the side panel shows, in its order, each with its kind, and its text their paragraphs, for a phone that shows no headings.
func (s phoneStudy) Explain(ctx context.Context, _ psync.Caller, req psync.LineExplain) (psync.Explanation, error) {
	f := s.film(ctx, req.Library, req.Film)
	ask, err := remote.OnUI(ctx, s.a.update, func() (phoneStudyAsk, error) { return s.ask(f) })
	if err != nil {
		return psync.Explanation{}, err
	}
	before, line := phonesync.ExplainLines(req, explainBefore)
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
	out := psync.Explanation{Language: ask.language, Model: strings.TrimSpace(ask.t.Model), Local: ask.t.Local()}
	var paragraphs []string
	for _, sec := range []psync.ExplainSection{
		{Kind: psync.ExplainMeaning, Text: sections.Meaning},
		{Kind: psync.ExplainBreakdown, Text: sections.Breakdown},
		{Kind: psync.ExplainTranslation, Text: sections.Translation},
		{Kind: psync.ExplainTone, Text: sections.Tone},
		{Kind: psync.ExplainRelated, Text: sections.Related},
	} {
		if sec.Text != "" {
			out.Sections = append(out.Sections, sec)
			paragraphs = append(paragraphs, sec.Text)
		}
	}
	out.Text = strings.Join(paragraphs, "\n\n")
	return out, nil
}

// Ask answers a follow-up question about the line, as the side panel's question field asks it (explain.FollowRequest): with the line, the lines before it, what was explained of it and the questions asked before, never a line after it.
func (s phoneStudy) Ask(ctx context.Context, _ psync.Caller, req psync.ExplainAsk) (psync.ExplainAnswer, error) {
	f := s.film(ctx, req.Library, req.Film)
	ask, err := remote.OnUI(ctx, s.a.update, func() (phoneStudyAsk, error) { return s.ask(f) })
	if err != nil {
		return psync.ExplainAnswer{}, err
	}
	before, line := phonesync.ExplainLines(req.LineExplain, explainBefore)
	var explained explain.Sections
	for _, sec := range req.Explained {
		switch sec.Kind {
		case psync.ExplainMeaning:
			explained.Meaning = strings.TrimSpace(sec.Text)
		case psync.ExplainBreakdown:
			explained.Breakdown = strings.TrimSpace(sec.Text)
		}
	}
	var asked []explain.QA
	for _, qa := range req.Asked {
		if q, a := strings.TrimSpace(qa.Question), strings.TrimSpace(qa.Answer); q != "" && a != "" {
			asked = append(asked, explain.QA{Question: q, Answer: a})
		}
	}
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()
	reply, err := ask.t.Chat(ctx, explain.FollowPrompt(ask.language), explain.FollowRequest(remoteFilmTitle(f), before, line, explained, asked, req.Question))
	reply = strings.TrimSpace(reply)
	if err == nil && reply == "" {
		err = explain.ErrEmpty
	}
	if err != nil {
		return psync.ExplainAnswer{}, err
	}
	return psync.ExplainAnswer{Text: reply, Language: ask.language, Model: strings.TrimSpace(ask.t.Model), Local: ask.t.Local()}, nil
}

// Recap answers what happened in the part of the film the phone watched, as the 前情提要 card sums it up on this computer (recap.Prompt): from the lines the phone sends alone, each item at the start of one of them.
func (s phoneStudy) Recap(ctx context.Context, _ psync.Caller, req psync.FilmRecap) (psync.Recap, error) {
	f := s.film(ctx, req.Library, req.Film)
	ask, err := remote.OnUI(ctx, s.a.update, func() (phoneStudyAsk, error) { return s.ask(f) })
	if err != nil {
		return psync.Recap{}, err
	}
	var seen []lines.Line
	for _, c := range req.Lines {
		if l := phonesync.Line(c); l.Original != "" || l.Translation != "" {
			seen = append(seen, l)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, recapTimeout)
	defer cancel()
	reply, err := ask.t.Chat(ctx, recap.Prompt(ask.language), recap.Request(remoteFilmTitle(f), seen))
	var items []recap.Item
	if err == nil {
		items, err = recap.Parse(reply, seen)
	}
	if err != nil {
		return psync.Recap{}, err
	}
	out := psync.Recap{Language: ask.language, Model: strings.TrimSpace(ask.t.Model), Local: ask.t.Local()}
	for _, it := range items {
		out.Items = append(out.Items, psync.RecapItem{StartMS: it.At.Milliseconds(), Text: it.Text})
	}
	return out, nil
}
