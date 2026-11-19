package track

import (
	"cmp"
	"slices"
	"time"
)

// Doc is a film's document as its logs hold it: every device's records (Read).
type Doc struct {
	Events []Event
	// Newer is true when a newer Kando wrote one of the logs, whose records this one may not understand: the document is shown as far as it can be, and not written to (ErrNewer).
	Newer bool
}

// Text is what a side of a line shows: the revision that won it (View).
type Text struct {
	Text   string
	Layer  Layer
	Source string
}

// Line is a line of the film as it shows: when, and the winning revision of each side, the zero Text for a side no revision was written of, as a line not translated yet.
type Line struct {
	ID          string
	Start, End  time.Duration
	Original    Text
	Translation Text
}

// View is the film's lines as they show, in the order they show: those of the generation the film shows (Shown), as Lines reads them.
func (d Doc) View() []Line {
	return d.Lines(d.Shown())
}

// Lines are the lines of generation gen in the order they show, less the ones taken out of it, each side the revision of the highest layer written of it, and of two in one layer the later (Event.after); none for "".
func (d Doc) Lines(gen string) []Line {
	if gen == "" {
		return nil
	}
	lines := map[string]*Line{}
	written := map[string]map[Side]Event{}
	var gone []string
	for _, e := range d.Events {
		switch {
		case e.Kind == KindLine && e.Gen == gen:
			if _, ok := lines[e.Line]; !ok {
				start, end := e.Span()
				lines[e.Line] = &Line{ID: e.Line, Start: start, End: end}
			}
		case e.Kind == KindGone:
			gone = append(gone, e.Line)
		}
	}
	for _, id := range gone {
		delete(lines, id)
	}
	for _, e := range d.Events {
		if e.Kind != KindRev || lines[e.Line] == nil || (e.Side != Original && e.Side != Translation) {
			continue
		}
		sides := written[e.Line]
		if sides == nil {
			sides = map[Side]Event{}
			written[e.Line] = sides
		}
		if was, ok := sides[e.Side]; !ok || wins(e, was) {
			sides[e.Side] = e
		}
	}
	out := make([]Line, 0, len(lines))
	for id, l := range lines {
		if o, ok := written[id][Original]; ok {
			l.Original = Text{Text: o.Text, Layer: o.Layer, Source: o.Source}
		}
		if t, ok := written[id][Translation]; ok {
			l.Translation = Text{Text: t.Text, Layer: t.Layer, Source: t.Source}
		}
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Line) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// wins reports whether revision e shows over was: a higher layer, or the same layer written later.
func wins(e, was Event) bool {
	if e.Layer != was.Layer {
		return e.Layer > was.Layer
	}
	return e.after(was)
}

// Shown is the generation the film shows: the one done last (KindDone), so a redo shows the film as it was until it is done, and one begun earlier that is done after another, as a redo stopped and run again later, shows once it is; or, while none is done, the first one begun, which a film made for the first time shows as its lines come, and which a redo begun before that first making was done does not take the place of, so a redo that fails or is stopped never leaves the film showing the part it made. "" for a document without one.
func (d Doc) Shown() string {
	var first, latestDone Event
	for _, e := range d.Events {
		switch {
		case e.Kind == KindGen && (first.Gen == "" || first.after(e)):
			first = e
		case e.Kind == KindDone && (latestDone.Gen == "" || e.after(latestDone)):
			latestDone = e
		}
	}
	if latestDone.Gen != "" {
		return latestDone.Gen
	}
	return first.Gen
}

// Plus is d with events added as device's, stamped after every record d holds, without writing them: the document as it would read had Append written them, as a read takes in what a subtitle file adds without writing it (media.ReadTrack).
func (d Doc) Plus(device string, events ...Event) Doc {
	at := int64(0)
	for _, e := range d.Events {
		at = max(at, e.At)
	}
	out := Doc{Events: slices.Clone(d.Events), Newer: d.Newer}
	for _, e := range events {
		at++
		e.At, e.Device = at, device
		out.Events = append(out.Events, e)
	}
	return out
}

// File is the latest record of the subtitle file named name that the document took in or wrote out (KindFile), on any device; false when it has none.
func (d Doc) File(name string) (Event, bool) {
	var last Event
	found := false
	for _, e := range d.Events {
		if e.Kind == KindFile && e.Source == name && (!found || e.after(last)) {
			last, found = e, true
		}
	}
	return last, found
}

// Done reports whether gen holds every line of the film.
func (d Doc) Done(gen string) bool {
	return slices.ContainsFunc(d.Events, func(e Event) bool { return e.Kind == KindDone && e.Gen == gen })
}
