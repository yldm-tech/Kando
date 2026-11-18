package sync

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// The learning state (yldm-tech/Kando#217) is what a learner makes of a library's films on any of their devices: where each film was left (KindPosition), the lines saved with their notes and review schedule (KindLine), the corrections of the machine's lines (KindCorrection) and the vocabulary (KindWord). It travels as records over the same calls as the jobs (OpStateGet, OpStatePut, OpStateResolve), so the storage relay (yldm-tech/Kando#219) carries it as it carries them.
//
// Records. A record is a kind, an ID unique among the library's records of its kind, the film it belongs to and its fields. Each field carries its value as JSON and a Stamp, when and on which device it last changed, so two devices' records merge field by field, the later change winning (MergeRecord). A line or a word removed leaves its record behind with Removed set, a tombstone, which wins over any change of the record: the record is gone on every device for good, and saving the line again makes a record of another ID. A position or a correction is never removed; its fields are set to null instead, which is how a correction taken back or a film watched to its end reads, so a later change can set them again.
//
// Revisions. The desktop numbers every change of a library's records with a revision, in an epoch named when the library's state is first kept and kept with it (unlike the jobs' epoch, which is named afresh at each start). A phone asks for the changes after the revision it has (StateGet); one with another epoch, or none, gets them all with Reset set, tombstones among them, and replaces what it has.
//
// Conflicts. A change a phone puts names, for each field, the stamp of the value it was changed from (Field.Base). When the desktop's value has changed since on another device, and both values have text in them, the later still wins but the other is not lost: the desktop records a Conflict, which every device lists until one resolves it (OpStateResolve) with the value to keep, one of the two or both joined, which then travels as any change does. Only fields a learner writes are watched so (Conflictable): a line's original, translation and note, a correction's original and translation, and a word's meaning and note; positions, schedules and the rest merge silently.
//
// A locked library's state is never served nor written: to a phone it is not found.

// The operations of the learning state, which Hello lists among its Ops when the desktop serves it.
const (
	// OpStateGet takes StateGet and answers StateChanges.
	OpStateGet = "state.get"
	// OpStatePut takes StatePut and answers StatePutResult.
	OpStatePut = "state.put"
	// OpStateResolve takes ConflictResolve and answers StatePutResult, the record as resolved and the conflict closed.
	OpStateResolve = "state.resolve"
)

// The kinds of Record.
const (
	KindPosition   = "position"
	KindLine       = "line"
	KindCorrection = "correction"
	KindWord       = "word"
)

// The fields of a KindPosition record, whose ID is its film's ID (Film.ID).
const (
	// FieldPositionMS is where the film was left, in milliseconds from its start, as a number; null for a film to start from the beginning.
	FieldPositionMS = "position_ms"
)

// The fields of a KindLine record: a saved line, whose ID is the line's own (study.Line.ID) and Film the film it was saved from. Its start, end, title and when it was saved do not change after it is saved.
const (
	// FieldStartMS and FieldEndMS are the line's times in its film's subtitles, in milliseconds, as numbers.
	FieldStartMS = "start_ms"
	FieldEndMS   = "end_ms"
	// FieldOriginal is what is said and FieldTranslation its translation, strings. In a KindCorrection record they are the corrected texts, or null to keep the machine's.
	FieldOriginal    = "original"
	FieldTranslation = "translation"
	// FieldNote is the learner's note, a string, "" for none. A KindWord record has one too.
	FieldNote = "note"
	// FieldReview is the line's review schedule, a Review.
	FieldReview = "review"
	// FieldTitle is the film's title when the line was saved, a string.
	FieldTitle = "title"
	// FieldSavedAt is when the line was saved, a time.
	FieldSavedAt = "saved_at"
)

// The fields of a KindCorrection record: a correction of the machine's line of film Film that showed from start to end (CorrectionID), besides FieldOriginal and FieldTranslation.
const (
	// FieldStartShiftMS and FieldEndShiftMS move the line's start and end, in milliseconds, as numbers.
	FieldStartShiftMS = "start_shift_ms"
	FieldEndShiftMS   = "end_shift_ms"
	// FieldNoSpeech, true, leaves the line out as one that is not speech.
	FieldNoSpeech = "no_speech"
)

// The fields of a KindWord record: a word of the vocabulary (yldm-tech/Kando#214), whose ID is the word's own (study.Word.ID) and Film the film of the line it was looked up in, besides FieldReview, its schedule, FieldTitle, that film's title, FieldStartMS, the line's start, and FieldSavedAt, when it was added. The word, the language it is spoken in, the line and the line's translation do not change after it is added; its reading, meaning and the meaning's language change together.
const (
	// FieldWord is the word as the vocabulary lists it, its dictionary form, and FieldSpoken the language of the film it was looked up in, as whisper's code ("en"), "" for one not known: strings.
	FieldWord   = "word"
	FieldSpoken = "spoken"
	// FieldReading is how it is said, FieldMeaning what it means and FieldLanguage the language the meaning is written in: strings.
	FieldReading  = "reading"
	FieldMeaning  = "meaning"
	FieldLanguage = "language"
	// FieldContext is the line it was looked up in and FieldContextTranslation that line's translation: strings.
	FieldContext            = "context"
	FieldContextTranslation = "context_translation"
)

// Review is a line's or a word's review schedule (FieldReview): its box, from 0, and when it is due.
type Review struct {
	Box int       `json:"box"`
	Due time.Time `json:"due"`
}

// Stamp is when a field changed and on which device, by the device's ID: the paired device's (PairResponse.DeviceID) for a phone, the desktop's own (StateChanges.Devices) for the desktop. Of two changes the later wins; of two at the same time, the one of the device whose ID sorts last, so every device picks the same one.
type Stamp struct {
	At     time.Time `json:"at"`
	Device string    `json:"device"`
}

// After reports whether s is a later change than o.
func (s Stamp) After(o Stamp) bool {
	if !s.At.Equal(o.At) {
		return s.At.After(o.At)
	}
	return s.Device > o.Device
}

// Equal reports whether s and o are the same change.
func (s Stamp) Equal(o Stamp) bool { return s.At.Equal(o.At) && s.Device == o.Device }

// IsZero reports whether s names no change.
func (s Stamp) IsZero() bool { return s.At.IsZero() && s.Device == "" }

// Field is a field's value, as JSON, with when and where it last changed.
type Field struct {
	Value json.RawMessage `json:"value"`
	Stamp
	// Base, in a StatePut, is the stamp of the value the change was made from, nil for a field the device had no value of; the desktop records a Conflict when its value has changed since (Conflictable). The desktop never sends it.
	Base *Stamp `json:"base,omitempty"`
}

// Record is a record of a library's learning state.
type Record struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Film is the ID of the film the record belongs to (Film.ID), "" for one of none, as a word, or a line whose film the desktop no longer serves.
	Film   string           `json:"film,omitempty"`
	Fields map[string]Field `json:"fields,omitempty"`
	// Removed is the record's tombstone: when and where it was removed, nil for a record that is there.
	Removed *Stamp `json:"removed,omitempty"`
	// Rev is the revision at which the record last changed on the desktop; what a device puts carries none.
	Rev int64 `json:"rev,omitempty"`
}

// Conflict is two changes of one field made apart, of which the later was kept (Kept) and the other set aside (Lost), until a device resolves it.
type Conflict struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Record string `json:"record"`
	Film   string `json:"film,omitempty"`
	Field  string `json:"field"`
	Kept   Field  `json:"kept"`
	Lost   Field  `json:"lost"`
	// Resolved is when and where it was resolved, nil while it is open. A resolved conflict is told once so devices close it, then forgotten.
	Resolved *Stamp `json:"resolved,omitempty"`
	Rev      int64  `json:"rev"`
}

// StateGet asks for the changes of library Library's learning state after revision Since of epoch Epoch; "" asks for all of it.
type StateGet struct {
	Library string `json:"library"`
	Epoch   string `json:"epoch"`
	Since   int64  `json:"since"`
}

// StateChanges is the answer to StateGet: the records and conflicts that changed after Since, in the order of their revisions, and Rev, the revision to ask after next. With Reset the epoch was not the desktop's, or Since was not one it can count from, and they are all the library's records, tombstones among them, and its open conflicts, which replace what the device has.
type StateChanges struct {
	Library   string     `json:"library"`
	Epoch     string     `json:"epoch"`
	Rev       int64      `json:"rev"`
	Reset     bool       `json:"reset,omitempty"`
	Records   []Record   `json:"records"`
	Conflicts []Conflict `json:"conflicts"`
	// Devices names the devices the stamps name, by their IDs, the desktop's own among them, so a phone can say where a film was left.
	Devices map[string]string `json:"devices,omitempty"`
}

// StatePut is a device's changes of library Library's records: for each, the fields that changed, each with its stamp and the stamp it was changed from, or the record's tombstone. The desktop stamps each with the calling device, whatever it names, and takes a time later than its own clock as its own clock's.
type StatePut struct {
	Library string   `json:"library"`
	Records []Record `json:"records"`
}

// StatePutResult is the answer to StatePut and ConflictResolve: the records put, as they are on the desktop once merged, which the device keeps in place of its own; the conflicts the put recorded or the resolve closed; and the revision the library's state is at.
type StatePutResult struct {
	Epoch     string     `json:"epoch"`
	Rev       int64      `json:"rev"`
	Records   []Record   `json:"records"`
	Conflicts []Conflict `json:"conflicts"`
}

// ConflictResolve resolves conflict ID of library Library with Value, the value to keep: the kept one, the lost one, or one made of both.
type ConflictResolve struct {
	Library string          `json:"library"`
	ID      string          `json:"id"`
	Value   json.RawMessage `json:"value"`
}

// EventState is an Event that library ID's learning state changed: the phone asks for the changes after the revision it has (OpStateGet).
const EventState = "state"

// Tombstoned reports whether records of kind are removed with a tombstone, which is final, rather than by setting their fields to null.
func Tombstoned(kind string) bool { return kind == KindLine || kind == KindWord }

// Conflictable reports whether two changes of field of a record of kind made apart are recorded as a Conflict: the fields a learner writes.
func Conflictable(kind, field string) bool {
	switch kind {
	case KindLine:
		return field == FieldOriginal || field == FieldTranslation || field == FieldNote
	case KindCorrection:
		return field == FieldOriginal || field == FieldTranslation
	case KindWord:
		return field == FieldMeaning || field == FieldNote
	}
	return false
}

// CorrectionID is the ID of the correction of film's machine line from startMS to endMS.
func CorrectionID(film string, startMS, endMS int64) string {
	return film + "/" + strconv.FormatInt(startMS, 10) + "-" + strconv.FormatInt(endMS, 10)
}

// ParseCorrectionID is the film and the machine line's span a CorrectionID names.
func ParseCorrectionID(id string) (film string, startMS, endMS int64, ok bool) {
	film, span, found := strings.Cut(id, "/")
	from, to, found2 := strings.Cut(span, "-")
	if !found || !found2 || film == "" {
		return "", 0, 0, false
	}
	start, err1 := strconv.ParseInt(from, 10, 64)
	end, err2 := strconv.ParseInt(to, 10, 64)
	if err1 != nil || err2 != nil || start < 0 || end < start {
		return "", 0, 0, false
	}
	return film, start, end, true
}

// Value is v as a field's value; a v that cannot be written as JSON is null.
func Value(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}

// Null is the value of a field that holds none.
var Null = json.RawMessage("null")

// IsNull reports whether v holds no value: null, or nothing.
func IsNull(v json.RawMessage) bool {
	v = bytes.TrimSpace(v)
	return len(v) == 0 || bytes.Equal(v, Null)
}

// SameValue reports whether a and b are the same value, whatever their spacing.
func SameValue(a, b json.RawMessage) bool {
	if IsNull(a) || IsNull(b) {
		return IsNull(a) == IsNull(b)
	}
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

// empty reports whether v has nothing a learner would lose: null, "", false or 0.
func empty(v json.RawMessage) bool {
	if IsNull(v) {
		return true
	}
	var x any
	if json.Unmarshal(v, &x) != nil {
		return false
	}
	switch x := x.(type) {
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case float64:
		return x == 0
	}
	return false
}

// MergeRecord is have, the desktop's copy of a record (Kind "" for none yet), with in, a device's change of it, taken in field by field, the later change of each winning, and whether that changed have. A change made from the value have holds (its Base is have's stamp) is later than it, and is stamped so when the device's clock says otherwise. A tombstone on either side wins over every field. It returns, for each field where two changes made apart of a Conflictable field both had text and the one set aside was not the device's own earlier change, the field kept and the one set aside, for the caller to record as a Conflict.
func MergeRecord(have, in Record) (out Record, changed bool, clashes []Clash) {
	if have.Kind == "" {
		out = Record{Kind: in.Kind, ID: in.ID, Film: in.Film, Removed: cloneStamp(in.Removed)}
		if in.Removed == nil {
			out.Fields = map[string]Field{}
			for name, f := range in.Fields {
				out.Fields[name] = Field{Value: f.Value, Stamp: f.Stamp}
			}
		}
		return out, true, nil
	}
	out = have.clone()
	if Tombstoned(have.Kind) && (have.Removed != nil || in.Removed != nil) {
		switch {
		case have.Removed == nil:
			out.Removed, out.Fields = cloneStamp(in.Removed), nil
			return out, true, nil
		case in.Removed != nil && in.Removed.After(*have.Removed):
			// The later of two removals is kept, as the study record keeps it, so every device ends with the same tombstone.
			out.Removed = cloneStamp(in.Removed)
			return out, true, nil
		}
		return out, false, nil
	}
	if out.Film == "" && in.Film != "" {
		out.Film, changed = in.Film, true
	}
	if out.Fields == nil {
		out.Fields = map[string]Field{}
	}
	for name, f := range in.Fields {
		h, ok := out.Fields[name]
		if !ok {
			out.Fields[name] = Field{Value: f.Value, Stamp: f.Stamp}
			changed = true
			continue
		}
		if SameValue(h.Value, f.Value) {
			if f.Stamp.After(h.Stamp) {
				out.Fields[name] = Field{Value: f.Value, Stamp: f.Stamp}
				changed = true
			}
			continue
		}
		if f.Base != nil && f.Base.Equal(h.Stamp) && !f.Stamp.After(h.Stamp) {
			// The device changed the value the desktop has, so its change came after it, whatever the device's clock says: one running behind the desktop's would otherwise lose the change without a conflict. It is made just after, as Ledger.Resolve makes a resolution.
			f.Stamp.At = h.At.Add(time.Millisecond)
		}
		kept, lost := h, Field{Value: f.Value, Stamp: f.Stamp}
		if f.Stamp.After(h.Stamp) {
			kept, lost = lost, kept
			out.Fields[name] = kept
			changed = true
		}
		// The device changed the value it had; the desktop's changed meanwhile on another device. A value with nothing in it loses nothing, so there is nothing to choose.
		apart := !h.Stamp.IsZero() && h.Device != f.Device && (f.Base == nil || !f.Base.Equal(h.Stamp))
		if apart && Conflictable(have.Kind, name) && !empty(kept.Value) && !empty(lost.Value) {
			clashes = append(clashes, Clash{Field: name, Kept: kept, Lost: lost})
		}
	}
	return out, changed, clashes
}

// Clash is a field of which MergeRecord kept one change and set another aside.
type Clash struct {
	Field      string
	Kept, Lost Field
}

// clone is r with fields of its own.
func (r Record) clone() Record {
	out := r
	out.Removed = cloneStamp(r.Removed)
	if r.Fields != nil {
		out.Fields = make(map[string]Field, len(r.Fields))
		for name, f := range r.Fields {
			f.Base = nil
			out.Fields[name] = f
		}
	}
	return out
}

func cloneStamp(s *Stamp) *Stamp {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}
