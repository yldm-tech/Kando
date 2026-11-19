package main

import (
	"kando/apps/kando/locales"
	"kando/packages/core"
)

// The 术语 tab's edit log (编辑记录): each change the tab makes to the open film's glossary, a term added, moved to another scope, locked or unlocked, removed, or a suggested one added, is a step of the same Undo and Redo as the corrections of the film's lines (correctStep), so ⌘Z takes back the last change whichever it was. The tab lists the last few steps, the latest first, with 撤销最近一项, which is Undo.

// The kinds of correctStep that change the glossary alone.
const (
	correctKindTermAdd    = "term-add"
	correctKindTermScope  = "term-scope"
	correctKindTermLock   = "term-lock"
	correctKindTermUnlock = "term-unlock"
	correctKindTermRemove = "term-remove"
	correctKindTermAccept = "term-accept"
)

// glossaryEditLogLength is how many of the last steps the 术语 tab lists.
const glossaryEditLogLength = 5

// correctTermStep reports whether step changed the glossary alone, not the corrections.
func correctTermStep(step correctStep) bool {
	switch step.kind {
	case correctKindTermAdd, correctKindTermScope, correctKindTermLock, correctKindTermUnlock, correctKindTermRemove, correctKindTermAccept:
		return true
	}
	return false
}

// glossaryChange puts t in the open film's glossary (glossaryPut) and keeps the change as a step of kind for Undo and Redo, which put back the term as it was before, or take it out when there was none; a t without a target takes the term out. The step is kept only once the film's subtitles are read (correctLoaded), as the steps are the open film's. It reports whether t was put.
func (a *app) glossaryChange(kind string, t core.Term) bool {
	before, had := a.glossaryTerm(t.Source)
	if !had {
		before = core.Term{Source: t.Source}
	}
	heldBefore := a.glossaryHeldOf(t.Source)
	if !a.glossaryPut(t) {
		return false
	}
	heldAfter := a.glossaryHeldOf(t.Source)
	if !a.correctLoaded() {
		return true
	}
	after := t
	name := after.Target
	if name == "" {
		name = before.Target
	}
	edits := a.player.subs.edits
	step := correctStep{before: edits, after: edits, kind: kind, from: t.Source, to: name, scope: t.Scope, termBefore: &before, termAfter: &after, heldBefore: &heldBefore, heldAfter: &heldAfter}
	a.correct.film = a.player.subs.path
	a.correct.undo = append(a.correct.undo, step)
	if len(a.correct.undo) > correctUndoDepth {
		a.correct.undo = a.correct.undo[len(a.correct.undo)-correctUndoDepth:]
	}
	a.correct.redo = nil
	return true
}

// glossaryStepName is what step, a change of the glossary alone, was, in the language of m.
func glossaryStepName(m locales.Messages, step correctStep) string {
	switch step.kind {
	case correctKindTermAdd:
		return m.GlossaryStepAdd(step.to)
	case correctKindTermScope:
		return m.GlossaryStepScope(step.to, glossaryScopeName(m, step.scope))
	case correctKindTermLock:
		return m.GlossaryStepLock(step.to)
	case correctKindTermUnlock:
		return m.GlossaryStepUnlock(step.to)
	case correctKindTermRemove:
		return m.GlossaryStepRemove(step.to)
	}
	return m.GlossaryStepAccept(step.to)
}

// glossaryScopeName is what the 术语 tab calls scope, in the language of m.
func glossaryScopeName(m locales.Messages, scope core.TermScope) string {
	switch scope {
	case core.ScopeSeries:
		return m.GlossaryScopeSeries()
	case core.ScopeGlobal:
		return m.GlossaryScopeGlobal()
	}
	return m.GlossaryScopeFilm()
}

// EditLog is what the last steps Undo can take back were, the latest first, at most glossaryEditLogLength: the 术语 tab's edit log.
func (a *app) EditLog() []string {
	if !a.correctLoaded() {
		return nil
	}
	m := a.messages()
	undo := a.correct.undo
	out := make([]string, 0, min(len(undo), glossaryEditLogLength))
	for i := len(undo) - 1; i >= 0 && len(out) < glossaryEditLogLength; i-- {
		out = append(out, a.correctStepName(m, undo[i]))
	}
	return out
}

// CanUndoEdit reports whether there is a step to take back now; UndoEdit takes back the last one, as Edit › Undo does.
func (a *app) CanUndoEdit() bool { return a.correctCanUndo() }

func (a *app) UndoEdit() { a.correctUndo() }
