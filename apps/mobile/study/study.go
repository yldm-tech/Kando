// Package study is the phone's copy of a library's learning state (kando/packages/sync, state.go): the records as the computer last answered them, the conflicts open, and the phone's own changes the computer has not taken yet, which the session sends until it does. It makes the phone's changes, a line saved or taken back and a note written, as records, and reads the saved lines of a film from the records with the changes laid over them. Nothing here touches the interface or the disk.
package study

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"kando/packages/sync"
)

// Library is a library's learning state on the phone: the records as the computer last answered them, at revision Rev of epoch Epoch, its open conflicts, Pending, the phone's changes not yet taken by the computer, newest last, and Sent, those of them handed to the computer at least once, which it may have taken though it did not answer. Devices names the devices the stamps name, by their IDs, as the computer last named them (sync.StateChanges.Devices).
type Library struct {
	Epoch     string            `json:"epoch"`
	Rev       int64             `json:"rev"`
	Records   []sync.Record     `json:"records"`
	Conflicts []sync.Conflict   `json:"conflicts"`
	Pending   []sync.Record     `json:"pending"`
	Sent      []Ref             `json:"sent,omitempty"`
	Devices   map[string]string `json:"devices,omitempty"`
}

// Ref names a record by its kind and ID.
type Ref struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Sending is l once sent, pending changes, are handed to the computer: from then on it may have them, so taking one back has to tell it (Change).
func Sending(l Library, sent []sync.Record) Library {
	out := l
	out.Sent = slices.Clone(l.Sent)
	for _, r := range sent {
		if ref := (Ref{r.Kind, r.ID}); !slices.Contains(out.Sent, ref) {
			out.Sent = append(out.Sent, ref)
		}
	}
	return out
}

// Batches are records cut, in order, into puts the computer takes: each at most limit bytes of records, but for a record longer than that, which goes alone.
func Batches(records []sync.Record, limit int) [][]sync.Record {
	var out [][]sync.Record
	var cur []sync.Record
	size := 0
	for _, r := range records {
		data, _ := json.Marshal(r)
		n := len(data) + 1
		if len(cur) > 0 && size+n > limit {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, r)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// Line is a saved line of a film, as the player shows it.
type Line struct {
	ID                    string
	Start, End            time.Duration
	Original, Translation string
	Note                  string
	SavedAt               time.Time
}

// Cue is a line of a film's subtitles, to save.
type Cue struct {
	Start, End            time.Duration
	Original, Translation string
}

// find is the index of the record kind id among records, or -1.
func find(records []sync.Record, kind, id string) int {
	return slices.IndexFunc(records, func(r sync.Record) bool { return r.Kind == kind && r.ID == id })
}

// clone is r with fields and a tombstone of its own.
func clone(r sync.Record) sync.Record {
	if r.Fields != nil {
		fields := make(map[string]sync.Field, len(r.Fields))
		for k, f := range r.Fields {
			fields[k] = f
		}
		r.Fields = fields
	}
	if r.Removed != nil {
		at := *r.Removed
		r.Removed = &at
	}
	return r
}

// Apply is l with the changes the computer answered taken in: all of its records and conflicts in place of l's when they are a reset, else the records and conflicts that changed in place of l's, resolved conflicts closed, and the names of the devices it names. The phone's pending changes stay.
func Apply(l Library, ch sync.StateChanges) Library {
	out := l
	out.Epoch, out.Rev = ch.Epoch, ch.Rev
	if ch.Reset {
		out.Records, out.Conflicts = nil, nil
	} else {
		out.Records, out.Conflicts = slices.Clone(l.Records), slices.Clone(l.Conflicts)
	}
	for _, r := range ch.Records {
		out.Records = upsert(out.Records, r)
	}
	if len(ch.Devices) > 0 {
		out.Devices = maps.Clone(l.Devices)
		if out.Devices == nil {
			out.Devices = map[string]string{}
		}
		maps.Copy(out.Devices, ch.Devices)
	}
	for _, c := range ch.Conflicts {
		i := slices.IndexFunc(out.Conflicts, func(x sync.Conflict) bool { return x.ID == c.ID })
		switch {
		case c.Resolved != nil && i >= 0:
			out.Conflicts = slices.Delete(out.Conflicts, i, i+1)
		case c.Resolved != nil:
		case i >= 0:
			out.Conflicts[i] = c
		default:
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	return out
}

// Took is l once the computer took sent, the pending changes the phone put, answering res: its records as merged in place of l's, its conflicts among l's, and the changes sent no longer pending, but for those changed again meanwhile, which go with the next put. A put the computer refused for good, as one of a film it no longer serves (rejected), drops sent as well, as it would refuse them again; the session puts a refused batch again a record at a time, so only the records refused are dropped.
func Took(l Library, sent []sync.Record, res sync.StatePutResult, rejected bool) Library {
	out := l
	out.Pending = slices.DeleteFunc(slices.Clone(l.Pending), func(p sync.Record) bool {
		i := find(sent, p.Kind, p.ID)
		return i >= 0 && same(sent[i], p)
	})
	out.Sent = slices.DeleteFunc(slices.Clone(l.Sent), func(ref Ref) bool { return find(out.Pending, ref.Kind, ref.ID) < 0 })
	if rejected {
		return out
	}
	out.Records = slices.Clone(l.Records)
	for _, r := range res.Records {
		out.Records = upsert(out.Records, r)
	}
	out.Conflicts = slices.Clone(l.Conflicts)
	for _, c := range res.Conflicts {
		if c.Resolved == nil && !slices.ContainsFunc(out.Conflicts, func(x sync.Conflict) bool { return x.ID == c.ID }) {
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	// The revision stays: the changes of other devices up to res.Rev are not here yet, and the next StateGet asks for them.
	return out
}

// same reports whether a and b are the same change.
func same(a, b sync.Record) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}

// upsert is records with r in place of the record of its kind and ID, or added.
func upsert(records []sync.Record, r sync.Record) []sync.Record {
	if i := find(records, r.Kind, r.ID); i >= 0 {
		records[i] = clone(r)
		return records
	}
	return append(records, clone(r))
}

// Change is l with change pending: merged into the change of the same record not yet taken, its fields over that one's, its tombstone over everything; a tombstone of a record the computer never had, nor was ever handed (Sending), takes back the change that made it.
func Change(l Library, change sync.Record) Library {
	out := l
	out.Pending = slices.Clone(l.Pending)
	i := find(out.Pending, change.Kind, change.ID)
	if i < 0 {
		out.Pending = append(out.Pending, clone(change))
		return out
	}
	p := clone(out.Pending[i])
	if change.Removed != nil && find(l.Records, change.Kind, change.ID) < 0 && !slices.Contains(l.Sent, Ref{change.Kind, change.ID}) {
		// A record the computer was never told of goes without a word; one it was handed may be there though it did not answer, so the tombstone goes to it.
		out.Pending = slices.Delete(out.Pending, i, i+1)
		return out
	}
	switch {
	case change.Removed != nil:
		p.Removed, p.Fields = change.Removed, nil
	case p.Removed == nil:
		if p.Fields == nil {
			p.Fields = map[string]sync.Field{}
		}
		for name, f := range change.Fields {
			if had, ok := p.Fields[name]; ok && had.Base != nil {
				// The change is still made from the value the computer had when the first one was.
				f.Base = had.Base
			}
			p.Fields[name] = f
		}
	}
	if p.Film == "" {
		p.Film = change.Film
	}
	out.Pending[i] = p
	return out
}

// View is the record kind id as the phone shows it: the computer's with the phone's pending change laid over it, false for none or one removed.
func View(l Library, kind, id string) (sync.Record, bool) {
	var r sync.Record
	if i := find(l.Records, kind, id); i >= 0 {
		r = clone(l.Records[i])
	}
	if i := find(l.Pending, kind, id); i >= 0 {
		p := l.Pending[i]
		if r.Kind == "" {
			r = sync.Record{Kind: p.Kind, ID: p.ID, Film: p.Film, Fields: map[string]sync.Field{}}
		}
		if p.Removed != nil {
			r.Removed = p.Removed
		}
		if r.Fields == nil {
			r.Fields = map[string]sync.Field{}
		}
		for name, f := range p.Fields {
			f.Base = nil
			r.Fields[name] = f
		}
	}
	return r, r.Kind != "" && r.Removed == nil
}

// Lines are the saved lines of film, in the order they play.
func Lines(l Library, film string) []Line {
	var ids []string
	for _, rs := range [][]sync.Record{l.Records, l.Pending} {
		for _, r := range rs {
			if r.Kind == sync.KindLine && r.Film == film && !slices.Contains(ids, r.ID) {
				ids = append(ids, r.ID)
			}
		}
	}
	var out []Line
	for _, id := range ids {
		r, ok := View(l, sync.KindLine, id)
		if !ok {
			continue
		}
		line := Line{ID: id}
		read(r, sync.FieldOriginal, &line.Original)
		read(r, sync.FieldTranslation, &line.Translation)
		read(r, sync.FieldNote, &line.Note)
		read(r, sync.FieldSavedAt, &line.SavedAt)
		var start, end float64
		read(r, sync.FieldStartMS, &start)
		read(r, sync.FieldEndMS, &end)
		line.Start, line.End = time.Duration(start)*time.Millisecond, time.Duration(end)*time.Millisecond
		out = append(out, line)
	}
	slices.SortStableFunc(out, func(a, b Line) int { return int((a.Start - b.Start) / time.Millisecond) })
	return out
}

// read reads field name of r into v, leaving it as it is for none or null.
func read(r sync.Record, name string, v any) {
	f, ok := r.Fields[name]
	if !ok || sync.IsNull(f.Value) {
		return
	}
	_ = json.Unmarshal(f.Value, v)
}

// SameStart is how far apart two starts may be and still be one line's, as the computer's study record takes them.
const SameStart = 50 * time.Millisecond

// At is the saved line of film that starts at start, false for none.
func At(l Library, film string, start time.Duration) (Line, bool) {
	for _, line := range Lines(l, film) {
		if d := line.Start - start; d < SameStart && d > -SameStart {
			return line, true
		}
	}
	return Line{}, false
}

// Save is the record of c saved from film titled title, at now, with note, under the new ID id.
func Save(id, film, title string, c Cue, note string, now time.Time) sync.Record {
	at := sync.Stamp{At: now}
	field := func(v any) sync.Field { return sync.Field{Value: sync.Value(v), Stamp: at} }
	return sync.Record{Kind: sync.KindLine, ID: id, Film: film, Fields: map[string]sync.Field{
		sync.FieldStartMS: field(c.Start.Milliseconds()), sync.FieldEndMS: field(c.End.Milliseconds()),
		sync.FieldOriginal: field(c.Original), sync.FieldTranslation: field(c.Translation),
		sync.FieldTitle: field(title), sync.FieldSavedAt: field(now),
		sync.FieldNote: field(strings.TrimSpace(note)), sync.FieldReview: field(sync.Review{Due: now}),
	}}
}

// Remove is the tombstone of the saved line id of film, at now.
func Remove(id, film string, now time.Time) sync.Record {
	return sync.Record{Kind: sync.KindLine, ID: id, Film: film, Removed: &sync.Stamp{At: now}}
}

// AddWord is the record of word put in the vocabulary at now under the new ID id, as e, the computer's lookup of it, gives it: its dictionary form, else word as tapped, with its reading and meaning; looked up in c, a line of film titled title, whose language spoken is as whisper names it. It is due for review at once, as the computer's own words are when added.
func AddWord(id, film, title, spoken, word string, e sync.WordEntry, c Cue, now time.Time) sync.Record {
	at := sync.Stamp{At: now}
	field := func(v any) sync.Field { return sync.Field{Value: sync.Value(v), Stamp: at} }
	text := strings.TrimSpace(e.Word)
	if text == "" {
		text = strings.TrimSpace(word)
	}
	return sync.Record{Kind: sync.KindWord, ID: id, Film: film, Fields: map[string]sync.Field{
		sync.FieldWord: field(text), sync.FieldSpoken: field(spoken),
		sync.FieldReading: field(strings.TrimSpace(e.Reading)), sync.FieldMeaning: field(strings.TrimSpace(e.Meaning)), sync.FieldLanguage: field(e.Language),
		sync.FieldContext: field(c.Original), sync.FieldContextTranslation: field(c.Translation),
		sync.FieldTitle: field(title), sync.FieldStartMS: field(c.Start.Milliseconds()),
		sync.FieldSavedAt: field(now), sync.FieldReview: field(sync.Review{Due: now}),
	}}
}

// RemoveWord is the tombstone of the word id of the vocabulary, looked up in film, at now.
func RemoveWord(id, film string, now time.Time) sync.Record {
	return sync.Record{Kind: sync.KindWord, ID: id, Film: film, Removed: &sync.Stamp{At: now}}
}

// Note is the change of the note of the saved line id of film to note, trimmed, at now, made from the note the computer has (Field.Base), so a note written meanwhile on another device is not lost but recorded as a conflict.
func Note(l Library, id, film, note string, now time.Time) sync.Record {
	f := sync.Field{Value: sync.Value(strings.TrimSpace(note)), Stamp: sync.Stamp{At: now}}
	if i := find(l.Records, sync.KindLine, id); i >= 0 {
		if had, ok := l.Records[i].Fields[sync.FieldNote]; ok {
			base := had.Stamp
			f.Base = &base
		}
	}
	return sync.Record{Kind: sync.KindLine, ID: id, Film: film, Fields: map[string]sync.Field{sync.FieldNote: f}}
}

// IDs are the IDs of l's records of kind as the phone shows them, those the computer has first, then those only the phone made; removed ones among them (View tells them apart).
func IDs(l Library, kind string) []string {
	var ids []string
	for _, rs := range [][]sync.Record{l.Records, l.Pending} {
		for _, r := range rs {
			if r.Kind == kind && !slices.Contains(ids, r.ID) {
				ids = append(ids, r.ID)
			}
		}
	}
	return ids
}

// Based is change with each field that names no Base made from the value the computer has (Field.Base), so a change of it made meanwhile on another device is merged as a conflict rather than lost; a record or field the computer does not have yet names none.
func Based(l Library, change sync.Record) sync.Record {
	i := find(l.Records, change.Kind, change.ID)
	if i < 0 || change.Fields == nil {
		return change
	}
	out := clone(change)
	for name, f := range out.Fields {
		if had, ok := l.Records[i].Fields[name]; ok && f.Base == nil && !had.Stamp.IsZero() {
			base := had.Stamp
			f.Base = &base
			out.Fields[name] = f
		}
	}
	return out
}

// Position is the change of where film was left to at, at now, null for a film to start from its beginning (at 0), as the computer keeps a film watched to its end.
func Position(film string, at time.Duration, now time.Time) sync.Record {
	v := sync.Null
	if at > 0 {
		v = sync.Value(at.Milliseconds())
	}
	return sync.Record{Kind: sync.KindPosition, ID: film, Film: film, Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: v, Stamp: sync.Stamp{At: now}}}}
}
