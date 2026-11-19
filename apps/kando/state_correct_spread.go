package main

import (
	"slices"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/live"
	"kando/packages/core"
)

// Spreading a fix: once an edit of a line changed words the film's other lines hold too (core.FixOf), a bar over the picture asks whether to change them all (⌘↩), shows them in the subtitle list (⇧⌘↩), or leaves them (Escape). Changing them all is one change of the corrections, which Undo takes back whole; a fix of a translation that a glossary term wrote also changes that term and locks it, so the film translated again writes the fix too.

// correctSpread is the fix the bar asks about: in the translations or, with original, in the originals; line is the machine's line the user edited, lines the machine's other lines the fix can change, by span (core.Corrected.Machine), and edited how many of them the user had corrected in that text.
type correctSpread struct {
	fix      core.Fix
	original bool
	line     core.Span
	lines    []core.Span
	edited   int
}

// correctSpreadFor is how long the bar waits for an answer before it goes, the line edited keeping the fix alone.
const correctSpreadFor = 8 * time.Second

// correctOffer asks about spreading the fix the edit of line made, from translation and original to what the line shows now, when the film's other lines hold it: the translation's first, else the original's. It reports whether it asks.
func (a *app) correctOffer(line core.Span, translation, original string) bool {
	_, from := a.correctLines()
	edited := slices.IndexFunc(from, func(l core.Corrected) bool { return l.Machine == line })
	if edited < 0 {
		return false
	}
	now := from[edited]
	for _, field := range []struct {
		before, after string
		original      bool
	}{{translation, now.Translation, false}, {original, now.Original, true}} {
		fix, ok := core.FixOf(field.before, field.after)
		if !ok {
			continue
		}
		s := &correctSpread{fix: fix, original: field.original, line: line}
		for j, l := range from {
			if j == edited {
				continue
			}
			if _, ok := fix.OnLine(l.Line, field.original); !ok {
				continue
			}
			s.lines = append(s.lines, l.Machine)
			if c := a.correctEditOf(l); c != nil && (field.original && c.Original != nil || !field.original && c.Translation != nil) {
				s.edited++
			}
		}
		if len(s.lines) > 0 {
			a.correctAsk(s)
			return true
		}
	}
	return false
}

// correctAsk shows the bar asking about s, until it is answered or correctSpreadFor passes.
func (a *app) correctAsk(s *correctSpread) {
	a.correct.spread = s
	if a.cx == nil {
		return
	}
	if a.correct.spreadTimer == nil {
		a.correct.spreadTimer = film.NewTimer(a.cx)
	}
	a.correct.spreadTimer.Set(correctSpreadFor, live.Do(a, func() {
		if a.correct.spread == s {
			a.correct.spread = nil
		}
	}))
}

// correctEditOf is the correction laid over l, nil for none.
func (a *app) correctEditOf(l core.Corrected) *core.Correction {
	if l.Edit < 0 || l.Edit >= len(a.player.subs.edits) {
		return nil
	}
	return &a.player.subs.edits[l.Edit]
}

// correctSpreading reports whether the bar asks about spreading a fix, while its keys work and the player leaves Escape to it.
func (a *app) correctSpreading() bool { return a.correct.spread != nil && a.correctLoaded() }

// Spread is the fix the bar asks about, while it shows (film.Corrector).
func (a *app) Spread() film.Spread {
	s := a.correct.spread
	if s == nil || !a.correctLoaded() {
		return film.Spread{}
	}
	return film.Spread{Open: true, From: s.fix.From, To: s.fix.To, Lines: len(s.lines), Edited: s.edited}
}

// SpreadAll changes the fix in every other line that holds it, as one change Undo takes back, and in the glossary term that wrote it (film.Corrector).
func (a *app) SpreadAll() {
	s := a.correct.spread
	if s == nil || !a.correctLoaded() {
		return
	}
	a.correct.spread = nil
	_, from := a.correctLines()
	edits := slices.Clone(a.player.subs.edits)
	n := 0
	for _, l := range from {
		if !slices.Contains(s.lines, l.Machine) {
			continue
		}
		text, ok := s.fix.OnLine(l.Line, s.original)
		if !ok {
			continue
		}
		machineTranslation, machineOriginal := a.correctMachineText(l.Machine)
		c := a.correctionOf(l)
		if s.original {
			c.Original = textOverride(text, machineOriginal)
		} else {
			c.Translation = textOverride(text, machineTranslation)
		}
		// Corrections are changed in place and laid over the lines without one appended, so the indexes the lines hold (core.Corrected.Edit) stay right; those left empty go after.
		if l.Edit >= 0 && l.Edit < len(edits) {
			edits[l.Edit] = c
		} else {
			edits = append(edits, c)
		}
		n++
	}
	if n == 0 {
		return
	}
	edits = slices.DeleteFunc(edits, core.Correction.Empty)
	step := correctStep{kind: correctKindSpread, line: n, from: s.fix.From, to: s.fix.To}
	if before, after, ok := a.correctSpreadTerm(s); ok {
		step.termBefore, step.termAfter = &before, &after
	}
	var heldBefore glossaryHeld
	if step.termAfter != nil {
		heldBefore = a.glossaryHeldOf(step.termAfter.Source)
	}
	if !a.correctSave(edits, step) {
		return
	}
	termed := step.termAfter != nil && a.glossaryPut(*step.termAfter)
	// The step correctSave kept, with the corrections before and after it.
	kept := &a.correct.undo[len(a.correct.undo)-1]
	if termed {
		heldAfter := a.glossaryHeldOf(step.termAfter.Source)
		kept.heldBefore, kept.heldAfter = &heldBefore, &heldAfter
	} else {
		kept.termBefore, kept.termAfter = nil, nil
	}
	m := a.messages()
	switch {
	case termed:
		a.flash(m.CorrectSpreadDoneTerm(n))
	default:
		a.flash(m.CorrectSpreadDone(n))
	}
}

// correctSpreadTerm is the open film's glossary term whose translation the fix changes, before and after it: a fix of a translation that a term wrote is the user's word on that term, which the change locks. A term wrote the words only if its source is in the original of the line edited or of one the fix changes, so a series or global term that translates other words the same way is left alone. It reports false for a fix of an original, one that takes the words out, or one no term wrote.
func (a *app) correctSpreadTerm(s *correctSpread) (before, after core.Term, ok bool) {
	if s.original || s.fix.To == "" || !a.TermsWritable() {
		return core.Term{}, core.Term{}, false
	}
	_, from := a.correctLines()
	var originals []string
	for _, l := range from {
		if l.Machine == s.line || slices.Contains(s.lines, l.Machine) {
			originals = append(originals, l.Original)
		}
	}
	for _, t := range a.FilmTerms() {
		if t.Target == s.fix.From && slices.ContainsFunc(originals, func(o string) bool { return core.TermIn(o, t.Source) }) {
			after = t
			after.Target, after.Locked, after.Origin = s.fix.To, true, core.OriginUser
			return t, after, true
		}
	}
	return core.Term{}, core.Term{}, false
}

// SpreadView shows the lines that hold the fix in the side panel's subtitle list, its search holding the words, and keeps the bar up a while longer (film.Corrector).
func (a *app) SpreadView() {
	s := a.correct.spread
	if s == nil {
		return
	}
	a.searchSubtitles()
	a.player.film.search = s.fix.From
	a.correctAsk(s)
}

// SpreadOnly leaves the other lines as they are, the edited line alone keeping the fix (film.Corrector).
func (a *app) SpreadOnly() {
	if a.correct.spread == nil {
		return
	}
	a.correct.spread = nil
	a.flash(a.messages().CorrectSpreadOnly())
}
