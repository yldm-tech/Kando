package study

import (
	"encoding/json"
	"strings"
	"time"

	"kando/packages/sync"
)

// The corrections of a film's lines (sync.KindCorrection): a learner fixes a line's translation on the phone as on the computer, and the computer lays the fix over the machine's line wherever the line is read. A correction is named by the machine's line it fixes (sync.CorrectionID), and may move the line's start and end; the lines the computer serves are the corrected ones, so the phone finds the correction of a line it shows by where the line shows (Correction.Start, End).

// Correction is a correction of film's machine line, as the phone shows it: ID names the record, Start and End are where the line shows once the correction moves it, and Original and Translation are the corrected texts, nil to keep the machine's.
type Correction struct {
	ID                    string
	Start, End            time.Duration
	Original, Translation *string
}

// Corrections are the corrections of film, the phone's changes not yet taken by the computer laid over the computer's, those that change neither text left out.
func Corrections(l Library, film string) []Correction {
	var out []Correction
	for _, id := range IDs(l, sync.KindCorrection) {
		r, ok := View(l, sync.KindCorrection, id)
		if !ok {
			continue
		}
		f, startMS, endMS, ok := sync.ParseCorrectionID(id)
		if !ok || f != film {
			continue
		}
		c := Correction{ID: id, Start: time.Duration(startMS) * time.Millisecond, End: time.Duration(endMS) * time.Millisecond}
		var shift float64
		if read(r, sync.FieldStartShiftMS, &shift); shift != 0 {
			c.Start += time.Duration(shift) * time.Millisecond
		}
		shift = 0
		if read(r, sync.FieldEndShiftMS, &shift); shift != 0 {
			c.End += time.Duration(shift) * time.Millisecond
		}
		c.Original, c.Translation = text(r, sync.FieldOriginal), text(r, sync.FieldTranslation)
		if c.Original != nil || c.Translation != nil {
			out = append(out, c)
		}
	}
	return out
}

// text is field name of r as a text, nil for none or null.
func text(r sync.Record, name string) *string {
	f, ok := r.Fields[name]
	if !ok || sync.IsNull(f.Value) {
		return nil
	}
	var s string
	if json.Unmarshal(f.Value, &s) != nil {
		return nil
	}
	return &s
}

// near reports whether a and b are one line's time, as the computer's study record takes them (SameStart).
func near(a, b time.Duration) bool { d := a - b; return d < SameStart && d > -SameStart }

// CorrectionAt is the correction of film whose line shows from start to end, false for none.
func CorrectionAt(l Library, film string, start, end time.Duration) (Correction, bool) {
	for _, c := range Corrections(l, film) {
		if near(c.Start, start) && near(c.End, end) {
			return c, true
		}
	}
	return Correction{}, false
}

// Correct is the change of the translation of c, a line of film as the phone shows it, to translation, trimmed, at now: the correction of the line the computer has, else a new one of the machine's line c is, made from the value the computer has (Based), so a fix made meanwhile on another device is recorded as a conflict rather than lost.
func Correct(l Library, film string, c Cue, translation string, now time.Time) sync.Record {
	id := sync.CorrectionID(film, c.Start.Milliseconds(), c.End.Milliseconds())
	if had, ok := CorrectionAt(l, film, c.Start, c.End); ok {
		id = had.ID
	}
	change := sync.Record{Kind: sync.KindCorrection, ID: id, Film: film, Fields: map[string]sync.Field{
		sync.FieldTranslation: {Value: sync.Value(strings.TrimSpace(translation)), Stamp: sync.Stamp{At: now}},
	}}
	return Based(l, change)
}
