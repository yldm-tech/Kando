package main

import (
	"errors"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// Correcting the open film's lines: the user selects a line on the picture (E), edits its translation and its original, has the AI service translate it again and picks one of its translations (这句不对, ⇧E), changes words a fix changed in the film's other lines as well, moves its start and end by a tenth of a second, or marks it as no speech, and undoes and redoes those changes. The changes are kept in the film's corrections file (media.SaveCorrections), apart from the subtitle files the pipeline writes, so processing or translating the film again keeps them, and every reader of the lines lays them over the machine's.

// correctUI is the state of correcting the open film's lines.
type correctUI struct {
	// on is set while a line is selected, and sel is the span of the machine's line it is (core.Corrected.Machine), which keeps the selection on that line as the corrections move it among the others. film is the subtitle file the selection and the steps are of (subtitleView.path); another film open drops both (correctFollow).
	on   bool
	sel  core.Span
	film string
	// editing shows the fields that edit the selected line, holding translation and original as typed.
	editing     bool
	translation string
	original    string
	// undo and redo are the changes made to the film's corrections, the last at the end: what each changed them from and to, and what it was, for the Edit menu to name.
	undo, redo []correctStep
	// pick is the other translations of the selected line while they show (这句不对, state_correct_candidates.go), pickGen counting the requests for them, so a reply to one made before is dropped.
	pick    *correctPick
	pickGen int
	// spread is the fix the bar over the picture asks about spreading to the film's other lines, spreadTimer putting the bar away unanswered (state_correct_spread.go).
	spread      *correctSpread
	spreadTimer *film.Timer
}

func newCorrectUI() correctUI { return correctUI{} }

// correctStep is one change of the open film's corrections: the corrections before and after it, and what it was (one of the correctKind values), of which line, by its number from 1, or of how many lines; a fix spread to the film's lines is named by its words, from and to, and keeps the glossary term it changed, before and after it, for Undo and Redo to put back.
type correctStep struct {
	before, after         []core.Correction
	kind                  string
	line                  int
	from, to              string
	termBefore, termAfter *core.Term
}

// What a change of the corrections was, for the Edit menu's Undo and Redo to name it.
const (
	correctKindEdit      = "edit"
	correctKindStart     = "start"
	correctKindEnd       = "end"
	correctKindNoSpeech  = "no-speech"
	correctKindRestore   = "restore"
	correctKindCandidate = "candidate"
	correctKindSpread    = "spread"
)

// correctNudgeShortest is how short a line's start and end moves may make it.
const correctNudgeShortest = 300 * time.Millisecond

// correctNudge is how far one press of the keys that move a line's start or end moves it.
const correctNudge = 100 * time.Millisecond

// correctHooks: after every change, a selection or steps left from a film no longer open go (correctFollow).
func (a *app) correctHooks() areaHooks {
	return areaHooks{afterChange: a.correctFollow}
}

// correctFollow drops the selection and the steps once their film is no longer the one open, or the selected line is gone, and reports whether it dropped anything.
func (a *app) correctFollow() bool {
	c := &a.correct
	subs := a.player.subs
	if c.film != "" && (a.openKey == "" || subs.key != a.openKey || subs.path != c.film) {
		a.CloseCandidates()
		*c = correctUI{pickGen: c.pickGen, spreadTimer: c.spreadTimer}
		return true
	}
	if c.on && a.correctSelected() < 0 {
		a.CloseCandidates()
		c.on, c.editing = false, false
		return true
	}
	return false
}

// correctLines are the open film's lines as they show and the corrected lines they come from, index for index, none while its subtitles are not read.
func (a *app) correctLines() ([]core.Segment, []core.Corrected) {
	cues := a.playerCues()
	if len(cues) != len(a.player.subs.from) {
		return nil, nil
	}
	return cues, a.player.subs.from
}

// correctSelected is the index of the selected line among the open film's lines, -1 while none is.
func (a *app) correctSelected() int {
	if !a.correct.on || a.correct.film != a.player.subs.path {
		return -1
	}
	_, from := a.correctLines()
	return slices.IndexFunc(from, func(l core.Corrected) bool { return l.Machine == a.correct.sel })
}

// Selected is the index of the line selected for correcting among the open film's lines (film.Corrector).
func (a *app) Selected() int { return a.correctSelected() }

// Editing reports whether the fields that edit the selected line show (film.Corrector).
func (a *app) Editing() bool { return a.correct.editing && a.correctSelected() >= 0 }

func (a *app) EditTranslation() *string { return &a.correct.translation }

func (a *app) EditOriginal() *string { return &a.correct.original }

// correctCanSelect reports whether the open film has lines the user can select.
func (a *app) correctCanSelect() bool {
	cues, _ := a.correctLines()
	return a.openKey != "" && len(cues) > 0
}

// correctSelecting reports whether a line is selected and its fields do not show, while the keys of the selection work.
func (a *app) correctSelecting() bool {
	return a.correctSelected() >= 0 && !a.correct.editing && a.correct.pick == nil
}

// correctSelectPlaying is E on the picture: it selects the line playing, or between lines the one just heard, or before the first the one coming, as L loops it.
func (a *app) correctSelectPlaying() {
	cues, _ := a.correctLines()
	if len(cues) == 0 {
		a.flash(a.messages().FilmNoLines())
		return
	}
	prev, cur, next := film.CueNear(cues, a.playerLineAt())
	switch {
	case cur >= 0:
	case prev >= 0:
		cur = prev
	default:
		cur = next
	}
	a.SelectLine(cur)
}

// SelectLine selects line i of the open film for correcting: the film holds at its start, so the picture shows it while it is corrected (film.Corrector).
func (a *app) SelectLine(i int) {
	cues, from := a.correctLines()
	if i < 0 || i >= len(cues) {
		return
	}
	a.CloseCandidates()
	a.correct.on, a.correct.editing, a.correct.spread = true, false, nil
	a.correct.sel, a.correct.film = from[i].Machine, a.player.subs.path
	a.correctHoldAt(cues[i].Start)
}

// correctHoldAt moves the picture to at, in the subtitles' time, and holds it there, as a seek plays on.
func (a *app) correctHoldAt(at time.Duration) {
	a.playerSeek(max(at+a.player.film.delay, 0))
	if play := a.player.play; play != nil && play.Playing() {
		play.toggle()
	}
}

// StepSelection selects the line d after the one selected, d before for a negative d (film.Corrector).
func (a *app) StepSelection(d int) {
	i := a.correctSelected()
	cues, _ := a.correctLines()
	if i < 0 {
		return
	}
	j := i + d
	if j < 0 || j >= len(cues) {
		if d < 0 {
			a.flash(a.messages().FilmNoEarlierLine())
		} else {
			a.flash(a.messages().FilmNoLaterLine())
		}
		return
	}
	a.SelectLine(j)
}

// EndSelection lets go of the selected line (film.Corrector).
func (a *app) EndSelection() {
	a.CloseCandidates()
	a.correct.on, a.correct.editing = false, false
}

// EditLine shows the fields that edit the selected line, holding its translation and its original as they show (film.Corrector).
func (a *app) EditLine() {
	i := a.correctSelected()
	if i < 0 {
		return
	}
	_, from := a.correctLines()
	a.correct.editing = true
	a.correct.translation, a.correct.original = from[i].Translation, from[i].Original
}

// CancelEdit puts the fields away without changing the line, which stays selected (film.Corrector).
func (a *app) CancelEdit() { a.correct.editing = false }

// SubmitEdit keeps what the fields hold as the selected line's translation and original, and plays on from the line (film.Corrector); when the edit changed words the film's other lines hold too, a bar asks whether to change them as well (correctOffer). A text back to the machine's drops the correction of it, so a later translation again reaches it. An empty original is not kept, as a cue holding a translation alone reads back as an original (core.SplitCue); a line that is no speech is marked so (MarkNoSpeech).
func (a *app) SubmitEdit() {
	i := a.correctSelected()
	if i < 0 {
		return
	}
	_, from := a.correctLines()
	line := from[i]
	translation, original := trimLine(a.correct.translation), trimLine(a.correct.original)
	if translation == line.Translation && original == line.Original {
		a.correct.editing = false
		return
	}
	if original == "" {
		a.flash(a.messages().CorrectOriginalEmpty())
		return
	}
	machineTranslation, machineOriginal := a.correctMachineText(line.Machine)
	c := a.correctionOf(line)
	c.Translation, c.Original = textOverride(translation, machineTranslation), textOverride(original, machineOriginal)
	if !a.correctSet(line, c, correctStep{kind: correctKindEdit, line: i + 1}) {
		return
	}
	a.correct.on, a.correct.editing = false, false
	if !a.correctOffer(line.Machine, line.Translation, line.Original) {
		a.flash(a.messages().CorrectSaved())
	}
	if play := a.player.play; play != nil && !play.Playing() {
		play.toggle()
	}
}

// NudgeLine moves the selected line's start, or with end its end, by d, keeping it at least correctNudgeShortest long and its start in the video, and says where it is now on the picture (film.Corrector). A press that would make the line shorter than that, or one already shorter, moves it no further than it may, and never the other way.
func (a *app) NudgeLine(end bool, d time.Duration) {
	i := a.correctSelected()
	if i < 0 {
		return
	}
	_, from := a.correctLines()
	line := from[i]
	c := a.correctionOf(line)
	step := correctStep{kind: correctKindStart, line: i + 1}
	if end {
		at := line.End + d
		if d < 0 {
			at = min(max(at, line.Start+correctNudgeShortest), line.End)
		}
		c.EndShift += at - line.End
		step.kind = correctKindEnd
	} else {
		at := max(line.Start+d, 0)
		if d > 0 {
			at = max(min(at, line.End-correctNudgeShortest), line.Start)
		}
		c.StartShift += at - line.Start
	}
	if c.StartShift == a.correctionOf(line).StartShift && c.EndShift == a.correctionOf(line).EndShift {
		return
	}
	if !a.correctSet(line, c, step) {
		return
	}
	cues, _ := a.correctLines()
	if j := a.correctSelected(); j >= 0 {
		if end {
			a.flash(a.messages().CorrectEndAt(film.LineClock(cues[j].End)))
		} else {
			a.correctHoldAt(cues[j].Start)
			a.flash(a.messages().CorrectStartAt(film.LineClock(cues[j].Start)))
		}
	}
}

// MarkNoSpeech leaves the selected line out of the film, as one the machine heard in music or noise, and lets go of it; Undo, or 恢复标为无语音的句子, brings it back (film.Corrector).
func (a *app) MarkNoSpeech() {
	i := a.correctSelected()
	if i < 0 {
		return
	}
	_, from := a.correctLines()
	c := a.correctionOf(from[i])
	c.NoSpeech = true
	if !a.correctSet(from[i], c, correctStep{kind: correctKindNoSpeech, line: i + 1}) {
		return
	}
	a.correct.on, a.correct.editing = false, false
	a.flash(a.messages().CorrectMarkedNoSpeech())
}

// correctNoSpeechCount is how many lines of the open film the user marked as no speech.
func (a *app) correctNoSpeechCount() int {
	if a.openKey == "" || a.player.subs.key != a.openKey {
		return 0
	}
	n := 0
	for _, c := range a.player.subs.edits {
		if c.NoSpeech {
			n++
		}
	}
	return n
}

// correctRestoreNoSpeech brings back every line of the open film the user marked as no speech.
func (a *app) correctRestoreNoSpeech() {
	n := a.correctNoSpeechCount()
	if n == 0 {
		return
	}
	edits := slices.Clone(a.player.subs.edits)
	for i := range edits {
		edits[i].NoSpeech = false
	}
	edits = slices.DeleteFunc(edits, core.Correction.Empty)
	if a.correctSave(edits, correctStep{kind: correctKindRestore, line: n}) {
		a.flash(a.messages().CorrectRestored(n))
	}
}

// correctionOf is the correction laid over line now, or a new one of its machine's line.
func (a *app) correctionOf(line core.Corrected) core.Correction {
	if line.Edit >= 0 && line.Edit < len(a.player.subs.edits) {
		c := a.player.subs.edits[line.Edit]
		// A correction that followed its line after a redo moved it is kept by the line's span from now on.
		c.Span = line.Machine
		return c
	}
	return core.Correction{Span: line.Machine}
}

// correctMachineText is the translation and the original of the machine's line at span.
func (a *app) correctMachineText(span core.Span) (translation, original string) {
	for _, cue := range a.player.subs.machine {
		if cue.Start == span.Start && cue.End == span.End {
			return core.SplitCue(cue.Text)
		}
	}
	return "", ""
}

// correctSet puts c in place of the correction laid over line, or adds it, and keeps the change as step (correctSave).
func (a *app) correctSet(line core.Corrected, c core.Correction, step correctStep) bool {
	return a.correctSave(core.SetCorrection(a.player.subs.edits, line.Edit, c), step)
}

// correctSave writes edits as the open film's corrections and shows the lines with them, keeping the change as step for Undo, or says on the picture why it could not. It reports whether it did.
func (a *app) correctSave(edits []core.Correction, step correctStep) bool {
	step.before, step.after = a.player.subs.edits, edits
	if !a.correctWrite(edits) {
		return false
	}
	a.correct.undo = append(a.correct.undo, step)
	if len(a.correct.undo) > correctUndoDepth {
		a.correct.undo = a.correct.undo[len(a.correct.undo)-correctUndoDepth:]
	}
	a.correct.redo = nil
	return true
}

// correctUndoDepth is how many changes Undo can take back.
const correctUndoDepth = 50

// correctWrite writes edits to the open film's corrections file and shows its lines with them, and has the library hero read its first line again (loadQuote), or says on the picture why it could not, when the file could not be read, as it would then lose what it held, or written. It writes nothing while the subtitles are being read (correctLoaded).
func (a *app) correctWrite(edits []core.Correction) bool {
	subs := &a.player.subs
	if !a.correctLoaded() {
		return false
	}
	if subs.editsErr {
		a.flash(a.messages().CorrectUnreadable())
		return false
	}
	if err := media.SaveCorrections(media.CorrectionsPath(subs.path), edits); err != nil {
		if errors.Is(err, media.ErrCorrectionsNewer) {
			a.flash(a.messages().CorrectNewer())
		} else {
			a.flash(a.messages().CorrectSaveFailed())
		}
		return false
	}
	a.correct.film = subs.path
	subs.edits = edits
	a.playerCorrectSubs()
	a.loadClipFrames()
	delete(a.lib.quotes, subs.path)
	a.libraryHeroQuote()
	return true
}

// correctLoaded reports whether the open film's subtitles and its corrections are read: while they are being read again (reloadOpenSubtitles), the corrections the read brings back would put back a change written meanwhile, and whether the file can be read is not yet known.
func (a *app) correctLoaded() bool {
	subs := a.player.subs
	return subs.path != "" && subs.key == a.openKey && subs.ready && subs.err == ""
}

// correctCanUndo and correctCanRedo report whether there is a change of the open film's corrections to undo or redo, once its subtitles are read (correctLoaded). While a text field of the film page has the keyboard focus, the line's edit fields or the subtitle search, they leave Undo and Redo to it, as its typing is what the user would take back.
func (a *app) correctCanUndo() bool { return a.correctUndoable() && len(a.correct.undo) > 0 }

func (a *app) correctCanRedo() bool { return a.correctUndoable() && len(a.correct.redo) > 0 }

func (a *app) correctUndoable() bool {
	// TODO(nsgo#671): drop the focus checks (a.correct.editing, a.player.film.searchFocused) once a MenuAuto Undo or Redo item hands ⌘Z and ⇧⌘Z to the focused field's undo: and redo: on macOS, so the corrections' items can stay in the Edit menu while a field has the focus; until then the system's items have to be the Edit menu's then, or typing in the field could not be undone https://github.com/nsgo-dev/nsgo/issues/671
	return a.openKey != "" && a.correctLoaded() && !a.correct.editing && !a.player.film.searchFocused
}

// correctUndo takes back the last change of the open film's corrections and says which on the picture.
func (a *app) correctUndo() {
	if !a.correctCanUndo() {
		return
	}
	step := a.correct.undo[len(a.correct.undo)-1]
	if !a.correctWrite(step.before) {
		return
	}
	a.correct.undo = a.correct.undo[:len(a.correct.undo)-1]
	a.correct.redo = append(a.correct.redo, step)
	a.correct.editing, a.correct.spread = false, nil
	if step.termBefore != nil {
		a.glossaryPut(*step.termBefore)
	}
	a.flash(a.messages().CorrectUndone(a.correctStepName(a.messages(), step)))
}

// correctRedo makes the last change Undo took back again and says which on the picture.
func (a *app) correctRedo() {
	if !a.correctCanRedo() {
		return
	}
	step := a.correct.redo[len(a.correct.redo)-1]
	if !a.correctWrite(step.after) {
		return
	}
	a.correct.redo = a.correct.redo[:len(a.correct.redo)-1]
	a.correct.undo = append(a.correct.undo, step)
	a.correct.editing, a.correct.spread = false, nil
	if step.termAfter != nil {
		a.glossaryPut(*step.termAfter)
	}
	a.flash(a.messages().CorrectRedone(a.correctStepName(a.messages(), step)))
}

// correctStepName is what step was, in the language of m, for the Edit menu and the picture to name it.
func (a *app) correctStepName(m locales.Messages, step correctStep) string {
	switch step.kind {
	case correctKindStart:
		return m.CorrectStepStart(step.line)
	case correctKindEnd:
		return m.CorrectStepEnd(step.line)
	case correctKindNoSpeech:
		return m.CorrectStepNoSpeech(step.line)
	case correctKindRestore:
		return m.CorrectStepRestore(step.line)
	case correctKindCandidate:
		return m.CorrectStepCandidate(step.line)
	case correctKindSpread:
		return m.CorrectStepSpread(step.from, step.to)
	}
	return m.CorrectStepEdit(step.line)
}

// correctKeys are the shortcuts of correcting the open film's lines. E selects the line playing and ⇧E asks the AI service for other translations of it (这句不对), of which ↑ and ↓ highlight one, Return keeps it and Escape puts them away; while a line is selected, Shift+← and Shift+→ select the one before or after, Return edits it, ⌃⌥← and ⌃⌥→ move its start and with Shift its end, ⌘⌫ marks it as no speech and Escape lets go of it, for which the player's Escape, which leaves the film, waits (playerKeys): these rows come after the player's, and the rows of the other translations and of spreading a fix after the selection's, as the first row of a key names it on the cheat sheet, Settings › 快捷键 and the menu bar. Undo and Redo take back and make again the last change, in the Edit menu with what it was, where they stand in for the system's while there is one (shortcut.Role); the fields that edit a line keep ⌘Z for their text.
func (a *app) correctKeys(cx *ns.Scope) []shortcut {
	can := func() bool { return a.correctCanSelect() && !a.correct.editing && a.correctSelected() < 0 }
	selecting := a.correctSelecting
	picking := a.correctPicking
	rows := []shortcut{
		{Group: groupSubtitles, Label: locales.CorrectKeySelect(cx), Key: "e", When: can, Run: a.correctSelectPlaying},
		{Group: groupSubtitles, Label: locales.CorrectKeyCandidates(cx), Key: "shift+e", When: a.correctCanPick, Run: a.StartCandidates},
		{Group: groupSubtitles, Label: locales.CorrectKeyEdit(cx), Key: "enter", When: selecting, Run: a.EditLine},
		{Group: groupSubtitles, Label: locales.CorrectKeyPrev(cx), Key: "shift+left", Repeat: true, When: selecting, Run: func() { a.StepSelection(-1) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyNext(cx), Key: "shift+right", Repeat: true, When: selecting, Run: func() { a.StepSelection(1) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyStartEarlier(cx), Key: "ctrl+alt+left", Repeat: true, When: selecting, Run: func() { a.NudgeLine(false, -correctNudge) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyStartLater(cx), Key: "ctrl+alt+right", Repeat: true, When: selecting, Run: func() { a.NudgeLine(false, correctNudge) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyEndEarlier(cx), Key: "ctrl+alt+shift+left", Repeat: true, When: selecting, Run: func() { a.NudgeLine(true, -correctNudge) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyEndLater(cx), Key: "ctrl+alt+shift+right", Repeat: true, When: selecting, Run: func() { a.NudgeLine(true, correctNudge) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyNoSpeech(cx), Key: "mod+backspace", When: selecting, Run: a.MarkNoSpeech},
		{Group: groupSubtitles, Label: locales.CorrectKeyDone(cx), Key: "escape", When: selecting, Run: a.EndSelection},
		{Group: groupSubtitles, Label: locales.CorrectKeyRestore(cx), When: func() bool { return a.correctNoSpeechCount() > 0 }, Run: a.correctRestoreNoSpeech},
		{Group: groupSubtitles, Label: locales.CorrectKeyPickUp(cx), Key: "up", Repeat: true, When: picking, Run: func() { a.correctMovePick(-1) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyPickDown(cx), Key: "down", Repeat: true, When: picking, Run: func() { a.correctMovePick(1) }},
		{Group: groupSubtitles, Label: locales.CorrectKeyPick(cx), Key: "enter", When: picking, Run: a.correctPickHighlighted},
		{Group: groupSubtitles, Label: locales.CorrectKeyPickCancel(cx), Key: "escape", When: picking, Run: a.CloseCandidates},
		{Group: groupSubtitles, Label: locales.CorrectKeySpreadAll(cx), Key: "mod+enter", When: a.correctSpreading, Run: a.SpreadAll},
		{Group: groupSubtitles, Label: locales.CorrectKeySpreadView(cx), Key: "mod+shift+enter", When: a.correctSpreading, Run: a.SpreadView},
		{Group: groupSubtitles, Label: locales.CorrectKeySpreadOnly(cx), Key: "escape", When: a.correctSpreading, Run: a.SpreadOnly},
	}
	// Undo and Redo are rows only while there is a change to undo or redo, so the system's own items, which the text fields use, are the Edit menu's otherwise (shortcut.Role).
	if a.correctCanUndo() {
		rows = append(rows, shortcut{Group: groupEdit, Label: locales.CorrectUndo(cx, a.correctStepName(a.messages(), a.correct.undo[len(a.correct.undo)-1])), Key: "mod+z", Role: nsdesktop.MenuUndo, When: a.correctCanUndo, Run: a.correctUndo})
	}
	if a.correctCanRedo() {
		rows = append(rows, shortcut{Group: groupEdit, Label: locales.CorrectRedo(cx, a.correctStepName(a.messages(), a.correct.redo[len(a.correct.redo)-1])), Key: "mod+shift+z", Role: nsdesktop.MenuRedo, When: a.correctCanRedo, Run: a.correctRedo})
	}
	return rows
}

// trimLine is a line as typed, without the spaces around it, its line breaks made spaces, as a cue holds its translation on one line and its original under it.
func trimLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// textOverride is the correction of a text that is now text and was machine as the pipeline wrote it: none when they are the same.
func textOverride(text, machine string) *string {
	if text == machine {
		return nil
	}
	return &text
}
