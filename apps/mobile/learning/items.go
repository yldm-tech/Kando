// Package learning is what the Learn tab makes of the learner's study, with nothing of the interface in it: the words and saved lines of the paired computers' libraries' learning state (kept and synced as package study keeps it, the records of protocol v2, kando/packages/sync, state.go) as the tab lists and reviews them, and their review schedule; how long films played on the phone each day; the export of the vocabulary and the saved lines; and when to remind the learner to review. The session reads and writes it off the engine's goroutine.
package learning

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"

	phonestudy "kando/apps/mobile/study"
	"kando/packages/study"
	"kando/packages/sync"
)

// Kind is what an Item is.
type Kind uint8

const (
	// Word is a word of the vocabulary.
	Word Kind = iota
	// Line is a saved line.
	Line
)

// Item is a word or a saved line of a library, as the Learn tab lists and reviews it.
type Item struct {
	Kind Kind
	// Desktop and Library are the computer and library whose learning state it is in; ID its record's and Film the film's it was saved or looked up in, "" for none.
	Desktop, Library, ID, Film string
	// Text is the word, or the line's original; Meaning the word's meaning, or the line's translation.
	Text, Meaning string
	// Reading is how a word is said, Spoken the language of its film, as whisper's code.
	Reading, Spoken string
	// Context is the line a word was looked up in and ContextTranslation that line's translation.
	Context, ContextTranslation string
	// Note is the learner's note of a line.
	Note string
	// Title is the film's title, Start and End where the line is in it.
	Title      string
	Start, End time.Duration
	// SavedAt is when it was saved, and Device the device that saved it, by its ID.
	SavedAt time.Time
	Device  string
	// Box and Due are its review schedule (study.Intervals), Review the stamp of the schedule.
	Box    int
	Due    time.Time
	Review sync.Stamp
}

// Key names the item on the phone.
func (it Item) Key() string {
	return strings.Join([]string{it.Desktop, it.Library, it.kind(), it.ID}, "/")
}

func (it Item) kind() string {
	if it.Kind == Word {
		return sync.KindWord
	}
	return sync.KindLine
}

// IsDue reports whether it is due for review at now.
func (it Item) IsDue(now time.Time) bool { return !it.Due.After(now) }

// Mastered reports whether it is known well enough to count as mastered (study.MasteredBox).
func (it Item) Mastered() bool { return it.Box >= study.MasteredBox }

// Items are the words and saved lines of l, library lib of computer desktop's learning state as the phone shows it, the phone's changes not yet sent laid over the computer's, those removed left out, the latest saved first.
func Items(desktop, lib string, l phonestudy.Library) []Item {
	var out []Item
	var records []sync.Record
	for _, kind := range []string{sync.KindWord, sync.KindLine} {
		for _, id := range phonestudy.IDs(l, kind) {
			if r, ok := phonestudy.View(l, kind, id); ok {
				records = append(records, r)
			}
		}
	}
	for _, r := range records {
		it := Item{Desktop: desktop, Library: lib, ID: r.ID, Film: r.Film, Kind: Line}
		text := func(name string, to *string) { read(r.Fields[name].Value, to) }
		if r.Kind == sync.KindWord {
			it.Kind = Word
			text(sync.FieldWord, &it.Text)
			text(sync.FieldMeaning, &it.Meaning)
			text(sync.FieldReading, &it.Reading)
			text(sync.FieldSpoken, &it.Spoken)
			text(sync.FieldContext, &it.Context)
			text(sync.FieldContextTranslation, &it.ContextTranslation)
		} else {
			text(sync.FieldOriginal, &it.Text)
			text(sync.FieldTranslation, &it.Meaning)
			text(sync.FieldNote, &it.Note)
		}
		if it.Text == "" {
			continue
		}
		text(sync.FieldTitle, &it.Title)
		it.Start, it.End = ms(r.Fields[sync.FieldStartMS]), ms(r.Fields[sync.FieldEndMS])
		saved := r.Fields[sync.FieldSavedAt]
		read(saved.Value, &it.SavedAt)
		it.Device = saved.Device
		review := r.Fields[sync.FieldReview]
		var rv sync.Review
		read(review.Value, &rv)
		it.Box, it.Due, it.Review = max(rv.Box, 0), rv.Due, review.Stamp
		out = append(out, it)
	}
	slices.SortStableFunc(out, func(a, b Item) int { return cmp.Or(b.SavedAt.Compare(a.SavedAt), cmp.Compare(a.ID, b.ID)) })
	return out
}

// Due are the items due at now, the one due longest first, words before lines due at the same time.
func Due(items []Item, now time.Time) []Item {
	var out []Item
	for _, it := range items {
		if it.IsDue(now) {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(a, b Item) int { return cmp.Or(a.Due.Compare(b.Due), cmp.Compare(a.Kind, b.Kind)) })
	return out
}

// NextIn is how many days after now's the first item not due at now comes due, 0 for later the same day, by the calendar of now's location; false while none is waiting.
func NextIn(items []Item, now time.Time) (int, bool) {
	var next time.Time
	for _, it := range items {
		if !it.IsDue(now) && (next.IsZero() || it.Due.Before(next)) {
			next = it.Due
		}
	}
	if next.IsZero() {
		return 0, false
	}
	next = next.In(now.Location())
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	to := time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, time.UTC)
	return int(to.Sub(from) / (24 * time.Hour)), true
}

// Grade is the change of it's schedule after a review at now on device, as the computer's own review makes it (study.Intervals): good, known, moves it up a box and makes it due the box's interval later; not known puts it back in box 0, due at once. The session makes it from the schedule the computer has (phonestudy.Based).
func Grade(it Item, good bool, now time.Time, device string) sync.Record {
	box, due := 0, now
	if good {
		box = min(it.Box+1, len(study.Intervals))
		due = now.Add(study.Intervals[box-1])
	}
	return sync.Record{Kind: it.kind(), ID: it.ID, Film: it.Film, Fields: map[string]sync.Field{
		sync.FieldReview: {Value: sync.Value(sync.Review{Box: box, Due: due}), Stamp: sync.Stamp{At: now, Device: device}},
	}}
}

// Removal is the removal of it at now on device, a tombstone: it is gone on every device for good.
func Removal(it Item, now time.Time, device string) sync.Record {
	return sync.Record{Kind: it.kind(), ID: it.ID, Film: it.Film, Removed: &sync.Stamp{At: now, Device: device}}
}

// read reads v from data, leaving it as it is for null or what it cannot read.
func read(data json.RawMessage, v any) {
	if sync.IsNull(data) {
		return
	}
	_ = json.Unmarshal(data, v)
}

// ms is a field's value read as milliseconds, 0 for none.
func ms(f sync.Field) time.Duration {
	var v float64
	read(f.Value, &v)
	return max(time.Duration(math.Round(v))*time.Millisecond, 0)
}
