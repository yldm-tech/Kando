package track

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"
)

// Cell is how much of the film one unit of work takes: the speech recognition hears, and the draft translates, the film cell by cell, as the speech recognition hears 30 seconds at a time. A line belongs to the cell its middle falls in.
const Cell = 30 * time.Second

// Attempts is how many times a unit is tried with the same inputs before it is given up, its cell then needing the user's attention rather than being tried again and again.
const Attempts = 3

// Ahead is how far past the playhead the window the film is made in first reaches; Behind how far before it, so the line just said shows too.
const (
	Ahead  = 90 * time.Second
	Behind = 2 * time.Second
)

// Work is the kind of a unit: hearing a cell's speech, or making a draft of its lines.
type Work string

const (
	WorkHear  Work = "hear"
	WorkDraft Work = "draft"
	// WorkFinal is the final proofread of a block of cells (Block), once every cell of it is drafted: a unit of it has the block's first cell as its Cell.
	WorkFinal Work = "final"
)

// Block is how many cells the final proofread reads at once: five minutes, about as many lines as a batch of the final proofread holds, so it sees how the film writes a name across them.
const Block = 10

// Unit is a piece of work on generation Gen: Work on cell Cell, from the inputs Key names. Two units with the same fields are the same work, which is made once, or given up once tried Attempts times.
type Unit struct {
	Work Work
	Gen  string
	Cell int
	Key  string
}

// Span is the part of the film the unit's cell covers.
func (u Unit) Span() (time.Duration, time.Duration) {
	return time.Duration(u.Cell) * Cell, time.Duration(u.Cell+1) * Cell
}

// Made is the record that unit u was made: what it wrote, as its lines and revisions, is already in the log before it.
func (u Unit) Made() Event {
	start, end := u.Span()
	return Event{Kind: KindUnit, Source: string(u.Work), Gen: u.Gen, Start: start.Milliseconds(), End: end.Milliseconds(), Key: u.Key}
}

// Failed is the record of a failed attempt at unit u, and why.
func (u Unit) Failed(reason string) Event {
	e := u.Made()
	e.Kind, e.Text = KindAttempt, reason
	return e
}

// unitOf is the unit e, a record of one made or tried, is of.
func unitOf(e Event) Unit {
	return Unit{Work: Work(e.Source), Gen: e.Gen, Cell: int(e.Start / Cell.Milliseconds()), Key: e.Key}
}

// Making is what is being made of a film, and from where it is watched: what Plan plans the work for.
type Making struct {
	// Gen is the generation being made.
	Gen string
	// Length is how long the film is; its cells cover it. A film of unknown length has none planned.
	Length time.Duration
	// Playhead is where the film is watched, or will be: the work nearest it after it comes first.
	Playhead time.Duration
	// Hearing names how the speech is heard, as the language it is heard in: a cell heard another way is heard again.
	Hearing string
	// Language is what the lines are translated into, "" when they are not translated, as a film heard in the language its subtitles are for.
	Language string
	// Final, when set, has each block of the film proofread as a whole once its cells are drafted (WorkFinal), and names what else its final proofread is made from besides its lines as heard, given those lines: the film's terms found in them, so a term found later in the film does not have the blocks it is not in proofread again. Nil makes no final proofread, as for a film not translated or a user who turned it off.
	Final func(heard []string) string
}

// Plan is the work left on a film, each kind in the order it is to be done.
type Plan struct {
	// Hear and Draft are the units needed: the cells not heard yet, and the heard cells whose lines have no draft, nearest the playhead first, then on to the end of the film, then from its start.
	Hear, Draft []Unit
	// GivenUp are the units tried Attempts times without being made, whose cells need the user's attention.
	GivenUp []Unit
	// Cells is how many cells the film has; Heard and Drafted how many are done, made or given up.
	Cells, Heard, Drafted int
	// Final are the blocks whose final proofread is needed, every cell of them drafted, nearest the playhead first. Blocks is how many blocks the film has when it is proofread as a whole, 0 when not; Finished the blocks whose final proofread is made from what they hold now, whose lines are final; Finaled how many blocks are done, Finished or given up.
	Final           []Unit
	Blocks, Finaled int
	Finished        map[int]bool
}

// Done reports whether nothing is left to make: every cell heard and drafted, and every block proofread as a whole when the film is, or given up.
func (p Plan) Done() bool {
	return p.Cells > 0 && len(p.Hear) == 0 && len(p.Draft) == 0 && len(p.Final) == 0 && p.Heard == p.Cells && p.Drafted == p.Cells && p.Finaled == p.Blocks
}

// Plan is the work left on making m of the film: every unit whose record is not in the document, made or given up. It is derived from the document alone, and nothing of it is kept, so a film run again, after a crash or a quit, goes on where it was.
func (d Doc) Plan(m Making) Plan {
	if m.Length <= 0 || m.Gen == "" {
		return Plan{}
	}
	cells := int((m.Length + Cell - 1) / Cell)
	made := map[Unit]bool{}
	tried := map[Unit]int{}
	heard := map[string]string{}
	var lines []Event
	for _, e := range d.Events {
		switch {
		case e.Kind == KindUnit && e.Gen == m.Gen:
			made[unitOf(e)] = true
		case e.Kind == KindAttempt && e.Gen == m.Gen:
			tried[unitOf(e)]++
		case e.Kind == KindLine && e.Gen == m.Gen:
			lines = append(lines, e)
		case e.Kind == KindRev && e.Side == Original && e.Layer == LayerHeard:
			heard[e.Line] = e.Text
		}
	}
	// A cell's lines, in the order they show, which its draft translates: their text as heard is what the draft's key is made from, so a draft that corrects them does not make itself needed again.
	inCell := make([][]Event, cells)
	gone := d.gone()
	for _, l := range lines {
		if gone[l.Line] {
			continue
		}
		if c := int((l.Start + l.End) / 2 / Cell.Milliseconds()); c >= 0 && c < cells {
			inCell[c] = append(inCell[c], l)
		}
	}
	p := Plan{Cells: cells}
	done := make([]bool, cells)
	for _, c := range order(cells, m.Playhead) {
		hear := Unit{Work: WorkHear, Gen: m.Gen, Cell: c, Key: m.Hearing}
		switch {
		case made[hear]:
			p.Heard++
		case tried[hear] >= Attempts:
			p.Heard++
			p.GivenUp = append(p.GivenUp, hear)
			// A cell not heard has no lines to draft.
			p.Drafted++
			done[c] = true
			continue
		default:
			p.Hear = append(p.Hear, hear)
			continue
		}
		if m.Language == "" || len(inCell[c]) == 0 {
			p.Drafted++
			done[c] = true
			continue
		}
		draft := Unit{Work: WorkDraft, Gen: m.Gen, Cell: c, Key: draftKey(m.Language, inCell[c], heard)}
		switch {
		case made[draft]:
			p.Drafted++
			done[c] = true
		case tried[draft] >= Attempts:
			p.Drafted++
			p.GivenUp = append(p.GivenUp, draft)
			done[c] = true
		default:
			p.Draft = append(p.Draft, draft)
		}
	}
	if m.Final == nil || m.Language == "" {
		return p
	}
	// A block is proofread as a whole once every cell of it is drafted, nearest the playhead first; a block without lines has nothing to proofread.
	p.Blocks = (cells + Block - 1) / Block
	p.Finished = map[int]bool{}
	for _, b := range rotate(p.Blocks, int(max(m.Playhead-Behind, 0)/(Cell*Block))) {
		first, last := b*Block, min((b+1)*Block, cells)
		var block []Event
		ready := true
		for c := first; c < last; c++ {
			ready = ready && done[c]
			block = append(block, inCell[c]...)
		}
		switch {
		case !ready:
			continue
		case len(block) == 0:
			p.Finaled++
			p.Finished[b] = true
			continue
		}
		texts := make([]string, len(block))
		for i, l := range block {
			texts[i] = heard[l.Line]
		}
		final := Unit{Work: WorkFinal, Gen: m.Gen, Cell: first, Key: draftKey(m.Language+"\x00"+m.Final(texts), block, heard)}
		switch {
		case made[final]:
			p.Finaled++
			p.Finished[b] = true
		case tried[final] >= Attempts:
			p.Finaled++
			p.GivenUp = append(p.GivenUp, final)
		default:
			p.Final = append(p.Final, final)
		}
	}
	return p
}

// GivenUp reports whether a unit of generation gen, of one of works (any work when none is named), was tried Attempts times without being made.
func (d Doc) GivenUp(gen string, works ...Work) bool {
	made := map[Unit]bool{}
	tried := map[Unit]int{}
	for _, e := range d.Events {
		switch {
		case e.Kind == KindUnit && e.Gen == gen:
			made[unitOf(e)] = true
		case e.Kind == KindAttempt && e.Gen == gen:
			tried[unitOf(e)]++
		}
	}
	for u, n := range tried {
		if n >= Attempts && !made[u] && (len(works) == 0 || slices.Contains(works, u.Work)) {
			return true
		}
	}
	return false
}

// gone is the lines taken out of their generation.
func (d Doc) gone() map[string]bool {
	out := map[string]bool{}
	for _, e := range d.Events {
		if e.Kind == KindGone {
			out[e.Line] = true
		}
	}
	return out
}

// draftKey names the inputs of a cell's draft: the language it is made in and its lines as they were heard.
func draftKey(language string, lines []Event, heard map[string]string) string {
	sorted := slices.Clone(lines)
	slices.SortFunc(sorted, func(a, b Event) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End), cmp.Compare(a.Line, b.Line))
	})
	h := sha256.New()
	h.Write([]byte(language))
	for _, l := range sorted {
		h.Write([]byte{0})
		h.Write([]byte(l.Line))
		h.Write([]byte{1})
		h.Write([]byte(heard[l.Line]))
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// order is the film's cells in the order they are made: from the one the window before the playhead starts in to the end of the film, then from its start.
func order(cells int, playhead time.Duration) []int {
	return rotate(cells, int(max(playhead-Behind, 0)/Cell))
}

// rotate is 0 to n-1 from first to the end, then from 0.
func rotate(n, first int) []int {
	first = min(first, n-1)
	out := make([]int, 0, n)
	for c := first; c < n; c++ {
		out = append(out, c)
	}
	for c := 0; c < first; c++ {
		out = append(out, c)
	}
	return out
}

// Final reports which lines of generation gen are final: those whose block's final proofread was made from what the block holds now (WorkFinal, its inputs named by language and final as Making names them), and those whose texts were written final already, by the user or taken in from a file made before the document (Source "file"). Every other line is a draft, a line of a block to be proofread again among them, whatever layer its texts are.
func (d Doc) Final(gen, language string, final func(heard []string) string) map[string]bool {
	out := map[string]bool{}
	heard := map[string]string{}
	made := map[Unit]bool{}
	for _, e := range d.Events {
		switch {
		case e.Kind == KindRev && e.Side == Original && e.Layer == LayerHeard:
			heard[e.Line] = e.Text
		case e.Kind == KindUnit && e.Gen == gen && Work(e.Source) == WorkFinal:
			made[unitOf(e)] = true
		}
	}
	blocks := map[int][]Line{}
	for _, l := range d.Lines(gen) {
		shown := l.Translation
		if shown.Text == "" {
			shown = l.Original
		}
		if shown.Layer == LayerUser || shown.Layer == LayerFinal && shown.Source == "file" {
			out[l.ID] = true
		}
		b := int((l.Start + l.End) / 2 / (Cell * Block))
		blocks[b] = append(blocks[b], l)
	}
	if final == nil || language == "" {
		return out
	}
	for b, lines := range blocks {
		events := make([]Event, len(lines))
		texts := make([]string, len(lines))
		for i, l := range lines {
			events[i] = Event{Line: l.ID, Start: l.Start.Milliseconds(), End: l.End.Milliseconds()}
			texts[i] = heard[l.ID]
		}
		if made[Unit{Work: WorkFinal, Gen: gen, Cell: b * Block, Key: draftKey(language+"\x00"+final(texts), events, heard)}] {
			for _, l := range lines {
				out[l.ID] = true
			}
		}
	}
	return out
}
