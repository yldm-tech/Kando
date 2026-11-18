package phonesync

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"kando/packages/core"
	"kando/packages/study"
	psync "kando/packages/sync"
)

// The learning state as protocol v2 carries it (psync.Record), made from what the desktop keeps where it kept it before: the saved lines of a library's study record (study.Book), the resume positions, and each film's corrections (core.Correction). The ledger that stamps the changes of what keeps no stamps and gives a device's changes back to be kept is kept in the library folder (LedgerFile), so it is locked, moved and encrypted with the library, and nothing of a locked library's state is left outside it.

// LedgerFile is the file in a library folder that keeps the library's learning state as phones were last told it (psync.Ledger). The library lists only videos, so it is not taken for one.
const LedgerFile = ".kando-state.json"

// LedgerPath is the ledger of the library at dir.
func LedgerPath(dir string) string { return filepath.Join(dir, LedgerFile) }

// StateKinds are the kinds of record the desktop syncs, which Hello lists.
var StateKinds = []string{psync.KindPosition, psync.KindLine, psync.KindCorrection, psync.KindWord}

// stampOf is a study record's stamp as the protocol carries it.
func stampOf(s study.Stamp) psync.Stamp { return psync.Stamp{At: s.At, Device: s.Device} }

// studyStamp is a protocol stamp as the study record keeps it.
func studyStamp(s psync.Stamp) study.Stamp { return study.Stamp{At: s.At, Device: s.Device} }

// LineRecords are book's lines as records, and the lines removed from it as tombstones. film is the ID of the film of a line's video, "" for one the desktop does not serve.
func LineRecords(book study.Book, film func(video string) string) []psync.Record {
	out := make([]psync.Record, 0, len(book.Lines)+len(book.Removed))
	for _, l := range book.Lines {
		saved := psync.Stamp{At: l.SavedAt, Device: l.Device}
		out = append(out, psync.Record{Kind: psync.KindLine, ID: l.ID, Film: film(l.Video), Fields: map[string]psync.Field{
			psync.FieldStartMS:     {Value: psync.Value(l.Start.Milliseconds()), Stamp: saved},
			psync.FieldEndMS:       {Value: psync.Value(l.End.Milliseconds()), Stamp: saved},
			psync.FieldTitle:       {Value: psync.Value(l.Film), Stamp: saved},
			psync.FieldSavedAt:     {Value: psync.Value(l.SavedAt), Stamp: saved},
			psync.FieldOriginal:    {Value: psync.Value(l.Original), Stamp: stampOf(l.Stamps.Original)},
			psync.FieldTranslation: {Value: psync.Value(l.Translation), Stamp: stampOf(l.Stamps.Translation)},
			psync.FieldNote:        {Value: psync.Value(l.Note), Stamp: stampOf(l.Stamps.Note)},
			psync.FieldReview:      {Value: psync.Value(psync.Review{Box: l.Box, Due: l.Due}), Stamp: stampOf(l.Stamps.Review)},
		}})
	}
	for _, r := range book.Removed {
		at := stampOf(r.At)
		out = append(out, psync.Record{Kind: psync.KindLine, ID: r.ID, Film: film(r.Video), Removed: &at})
	}
	return out
}

// LinesOf are line records, as the ledger has them once merged, as book takes them in (study.Book.Merge): the lines there and the lines removed. video is the path, as the book keys it (study.Book.Key), of the video of film, ok false for a film the desktop does not serve; a line of such a film that the book has keeps its video, and one it does not have is left out (StateAccepts refuses it).
func LinesOf(book study.Book, records []psync.Record, video func(film string) (string, bool)) (lines []study.Line, removed []study.Removed) {
	for _, r := range records {
		if r.Kind != psync.KindLine {
			continue
		}
		var have study.Line
		if i := book.Index(r.ID); i >= 0 {
			have = book.Lines[i]
		}
		if r.Removed != nil {
			removed = append(removed, study.Removed{ID: r.ID, Video: have.Video, Start: have.Start, At: studyStamp(*r.Removed)})
			continue
		}
		l := have
		if l.ID == "" {
			key, ok := video(r.Film)
			if !ok {
				continue
			}
			l = study.Line{ID: r.ID, Video: key}
		}
		if f, ok := r.Fields[psync.FieldSavedAt]; ok {
			l.Device = f.Device
			readJSON(f.Value, &l.SavedAt)
		}
		if f, ok := r.Fields[psync.FieldStartMS]; ok {
			l.Start = max(msField(f), 0)
		}
		if f, ok := r.Fields[psync.FieldEndMS]; ok {
			l.End = max(msField(f), 0)
		}
		if f, ok := r.Fields[psync.FieldTitle]; ok {
			readJSON(f.Value, &l.Film)
		}
		if f, ok := r.Fields[psync.FieldOriginal]; ok {
			l.Original = ""
			readJSON(f.Value, &l.Original)
			l.Stamps.Original = studyStamp(f.Stamp)
		}
		if f, ok := r.Fields[psync.FieldTranslation]; ok {
			l.Translation = ""
			readJSON(f.Value, &l.Translation)
			l.Stamps.Translation = studyStamp(f.Stamp)
		}
		if f, ok := r.Fields[psync.FieldNote]; ok {
			l.Note = ""
			readJSON(f.Value, &l.Note)
			l.Stamps.Note = studyStamp(f.Stamp)
		}
		if f, ok := r.Fields[psync.FieldReview]; ok {
			var rv psync.Review
			readJSON(f.Value, &rv)
			l.Box, l.Due = max(rv.Box, 0), rv.Due
			l.Stamps.Review = studyStamp(f.Stamp)
		}
		if l.Stamps.Video == (study.Stamp{}) {
			l.Stamps.Video = study.Stamp{At: l.SavedAt, Device: l.Device}
		}
		lines = append(lines, l)
	}
	return lines, removed
}

// PositionRecord is where film was left, at, as a record, null for a film to start from its beginning (at 0). Its stamp is left for the ledger to set, as the desktop keeps none.
func PositionRecord(film string, at time.Duration) psync.Record {
	v := psync.Null
	if at > 0 {
		v = psync.Value(at.Milliseconds())
	}
	return psync.Record{Kind: psync.KindPosition, ID: film, Film: film, Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: v}}}
}

// PositionOf is where a position record says its film was left, 0 for its beginning.
func PositionOf(r psync.Record) time.Duration {
	return max(msField(r.Fields[psync.FieldPositionMS]), 0)
}

// CorrectionRecords are film's corrections as records, their stamps left for the ledger to set, as the corrections file keeps none.
func CorrectionRecords(film string, edits []core.Correction) []psync.Record {
	out := make([]psync.Record, 0, len(edits))
	for _, c := range edits {
		if c.Empty() {
			continue
		}
		text := func(s *string) json.RawMessage {
			if s == nil {
				return psync.Null
			}
			return psync.Value(*s)
		}
		ms := func(d time.Duration) json.RawMessage {
			if d == 0 {
				return psync.Null
			}
			return psync.Value(d.Milliseconds())
		}
		noSpeech := psync.Null
		if c.NoSpeech {
			noSpeech = psync.Value(true)
		}
		out = append(out, psync.Record{Kind: psync.KindCorrection, ID: psync.CorrectionID(film, c.Span.Start.Milliseconds(), c.Span.End.Milliseconds()), Film: film, Fields: map[string]psync.Field{
			psync.FieldOriginal:     {Value: text(c.Original)},
			psync.FieldTranslation:  {Value: text(c.Translation)},
			psync.FieldStartShiftMS: {Value: ms(c.StartShift)},
			psync.FieldEndShiftMS:   {Value: ms(c.EndShift)},
			psync.FieldNoSpeech:     {Value: noSpeech},
		}})
	}
	return out
}

// CorrectionOf is the correction a record is, of the machine's line its ID names, ok false for an ID that names none. A correction whose fields are all null changes nothing (core.Correction.Empty).
func CorrectionOf(r psync.Record) (core.Correction, bool) {
	_, start, end, ok := psync.ParseCorrectionID(r.ID)
	if !ok {
		return core.Correction{}, false
	}
	c := core.Correction{Span: core.Span{Start: time.Duration(start) * time.Millisecond, End: time.Duration(end) * time.Millisecond}}
	text := func(name string) *string {
		f, ok := r.Fields[name]
		if !ok || psync.IsNull(f.Value) {
			return nil
		}
		var s string
		if json.Unmarshal(f.Value, &s) != nil {
			return nil
		}
		return &s
	}
	c.Original, c.Translation = text(psync.FieldOriginal), text(psync.FieldTranslation)
	c.StartShift, c.EndShift = msField(r.Fields[psync.FieldStartShiftMS]), msField(r.Fields[psync.FieldEndShiftMS])
	readJSON(r.Fields[psync.FieldNoSpeech].Value, &c.NoSpeech)
	return c, true
}

// SetCorrection is edits with c in place of the correction of the same machine's line, or added, and without it when it changes nothing. edits is not changed.
func SetCorrection(edits []core.Correction, c core.Correction) []core.Correction {
	at := -1
	for i, e := range edits {
		if e.Span == c.Span {
			at = i
			break
		}
	}
	return core.SetCorrection(edits, at, c)
}

// StateAccepts reports why records a device put cannot be kept, "" when they all can: a record of a kind the desktop does not sync, a field whose value is not of its type (FieldValid), a correction whose ID names no line, a new word without its word, or a record of a film the desktop does not serve (known), unless the ledger has it already (have), as a line whose film has left the library; a word may be of any film.
func StateAccepts(records []psync.Record, known func(film string) bool, have func(kind, id string) bool) string {
	for _, r := range records {
		switch r.Kind {
		case psync.KindPosition, psync.KindLine, psync.KindCorrection, psync.KindWord:
		default:
			return "this desktop does not sync records of kind " + r.Kind
		}
		for name, f := range r.Fields {
			if !FieldValid(r.Kind, name, f.Value) {
				return "field " + name + " of " + r.Kind + " " + r.ID + " is not of its type"
			}
		}
		if r.Kind == psync.KindCorrection {
			film, _, _, ok := psync.ParseCorrectionID(r.ID)
			if !ok || film != r.Film {
				return "correction " + r.ID + " names no line of film " + r.Film
			}
		}
		if r.Kind == psync.KindPosition && r.ID != r.Film {
			return "position " + r.ID + " is not of film " + r.Film
		}
		if have(r.Kind, r.ID) || r.Removed != nil {
			continue
		}
		if r.Kind == psync.KindWord {
			// A word is the vocabulary's, whatever film it was looked up in: one of a film the library no longer has keeps its line's text.
			if _, ok := r.Fields[psync.FieldWord]; !ok {
				return "word " + r.ID + " names no word"
			}
			continue
		}
		if !known(r.Film) {
			return r.Kind + " " + r.ID + " is of no film this library has"
		}
		if r.Kind == psync.KindLine {
			if _, ok := r.Fields[psync.FieldStartMS]; !ok {
				return "line " + r.ID + " names no start"
			}
		}
	}
	return ""
}

// FieldValid reports whether v is a value field name of a record of kind may hold, as state.go types it, so a value of another type is refused rather than kept as nothing over the desktop's. A field the desktop does not know, as one a later protocol adds, holds anything. Milliseconds are any number, a phone's Double among them, rounded when read (msField).
func FieldValid(kind, name string, v json.RawMessage) bool {
	var x any
	if json.Unmarshal(v, &x) != nil {
		return false
	}
	isNull := x == nil
	_, isString := x.(string)
	_, isNumber := x.(float64)
	_, isBool := x.(bool)
	switch kind {
	case psync.KindPosition:
		if name == psync.FieldPositionMS {
			return isNull || isNumber
		}
	case psync.KindLine:
		switch name {
		case psync.FieldStartMS, psync.FieldEndMS:
			return isNumber
		case psync.FieldOriginal, psync.FieldTranslation, psync.FieldNote, psync.FieldTitle:
			return isString
		case psync.FieldSavedAt:
			var at time.Time
			return isString && json.Unmarshal(v, &at) == nil
		case psync.FieldReview:
			var r psync.Review
			_, isObject := x.(map[string]any)
			return isObject && json.Unmarshal(v, &r) == nil
		}
	case psync.KindCorrection:
		switch name {
		case psync.FieldOriginal, psync.FieldTranslation:
			return isNull || isString
		case psync.FieldStartShiftMS, psync.FieldEndShiftMS:
			return isNull || isNumber
		case psync.FieldNoSpeech:
			return isNull || isBool
		}
	case psync.KindWord:
		return wordFieldValid(name, v, x)
	}
	return true
}

// msField is a field's value read as milliseconds, rounded, 0 for null or anything else; a shift may be less than 0.
func msField(f psync.Field) time.Duration {
	var ms float64
	readJSON(f.Value, &ms)
	return time.Duration(math.Round(ms)) * time.Millisecond
}

// readJSON reads v from data, leaving it as it is for null or what it cannot read.
func readJSON(data json.RawMessage, v any) {
	if psync.IsNull(data) {
		return
	}
	_ = json.Unmarshal(data, v)
}

// LineSign is what a phone is told of l that the desktop changes, as text, for the desktop to sum up and see when it changed.
func LineSign(l study.Line) []string {
	s := l.Stamps
	return []string{l.ID, l.Video, stampSign(s.Original), stampSign(s.Translation), stampSign(s.Note), stampSign(s.Review), stampSign(s.Video)}
}

// CorrectionSign is what a phone is told of c, as text, for the desktop to sum up and see when it changed.
func CorrectionSign(c core.Correction) []string {
	return []string{strconv.FormatInt(int64(c.Span.Start), 10), strconv.FormatInt(int64(c.Span.End), 10), textSign(c.Original), textSign(c.Translation), strconv.FormatInt(int64(c.StartShift), 10), strconv.FormatInt(int64(c.EndShift), 10), strconv.FormatBool(c.NoSpeech)}
}

// stampSign is when and where s was stamped, as text.
func stampSign(s study.Stamp) string { return strconv.FormatInt(s.At.UnixNano(), 36) + s.Device }

// textSign is s, or a mark of none for nil.
func textSign(s *string) string {
	if s == nil {
		return "\x00nil"
	}
	return *s
}
