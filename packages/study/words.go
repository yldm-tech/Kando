package study

import (
	"slices"
	"strings"
	"time"
)

// Word is a word the learner put in the vocabulary from a line of a film: the word as its dictionary form gives it, how it is read and what it means, in the language the meaning is written in, and the line it was looked up in, so the review and the export can show it in use. Like a Line it is a record built to be synced: its id, the device that added it, and stamps for the fields that change after it was added (WordStamps).
type Word struct {
	// ID names the word on every device; Device is the device that added it.
	ID, Device string
	// Text is the word as the vocabulary lists it, its dictionary form, and Spoken the language of the film it was looked up in, as whisper's code (en, de), "" when it was not known; the two name it in the book (WordIndex), so a word is there once whatever its case, and "die" of a German film is not "die" of an English one.
	Text, Spoken string
	// Reading is how it is said, as a phonetic transcription or a reading in kana, "" for none; Meaning is what it means, written in Language, the language the subtitles are translated into.
	Reading, Meaning, Language string
	// Context is the line it was looked up in, and ContextTranslation that line's translation, "" for none.
	Context, ContextTranslation string
	// Video, Start, End and Film are where the line is: its video's path as Key gives it, the line's start and end, End 0 when it is not known, as for a word added on a device that did not keep it, and the film's title.
	Video      string
	Start, End time.Duration
	Film       string
	// SavedAt is when it was added.
	SavedAt time.Time
	// Box and Due are its review schedule, as a Line's (Intervals).
	Box int
	Due time.Time
	// Stamps is when each field that changes last changed, for MergeWords.
	Stamps WordStamps
}

// WordStamps is when the fields of a Word that change after it was added last changed: Meaning is Reading, Meaning and Language's, which change together, and Review Box and Due's.
type WordStamps struct {
	Meaning, Review Stamp
}

// MasteredBox is the box from which a word or a line counts as mastered: reached by knowing it five times in a row, after which it is next due a month later, more than three weeks apart.
const MasteredBox = 5

// WordKey is how the book names the word text: trimmed and in lower case, so "River" and "river" are one word.
func WordKey(text string) string { return strings.ToLower(strings.TrimSpace(text)) }

// spokenKey is how the book compares the languages words are spoken in: trimmed and in lower case.
func spokenKey(spoken string) string { return strings.ToLower(strings.TrimSpace(spoken)) }

// WordIndex is the index of the word text of the language spoken in the vocabulary, or -1.
func (b Book) WordIndex(text, spoken string) int {
	key, lang := WordKey(text), spokenKey(spoken)
	if key == "" {
		return -1
	}
	return slices.IndexFunc(b.Words, func(w Word) bool { return WordKey(w.Text) == key && spokenKey(w.Spoken) == lang })
}

// WordByID is the index of the word id, or -1.
func (b Book) WordByID(id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(b.Words, func(w Word) bool { return w.ID == id })
}

// AddWord puts w in the vocabulary, first and due at once, and reports whether it did: a word the vocabulary has already, in any case, of the same language spoken, stays as it is. It gets an id when it has none, the book's device, and its fields are stamped now, when it was added unless SavedAt says.
func (b *Book) AddWord(w Word) bool {
	w.Text, w.Spoken = strings.TrimSpace(w.Text), strings.TrimSpace(w.Spoken)
	if w.Text == "" || b.WordIndex(w.Text, w.Spoken) >= 0 {
		return false
	}
	at := b.stamp()
	if w.SavedAt.IsZero() {
		w.SavedAt = at.At
	}
	at.At = w.SavedAt
	if w.ID == "" || b.WordByID(w.ID) >= 0 {
		w.ID = NewID()
	}
	if w.Device == "" {
		w.Device = b.Device
	}
	w.Box, w.Due = 0, w.SavedAt
	w.Stamps = WordStamps{Meaning: at, Review: at}
	b.Words = slices.Insert(b.Words, 0, w)
	return true
}

// RemoveWord takes the word text of the language spoken out of the vocabulary, leaving a Removed behind that names it by its id and its key (Removed.Video), and reports whether there was one.
func (b *Book) RemoveWord(text, spoken string) bool {
	i := b.WordIndex(text, spoken)
	if i < 0 {
		return false
	}
	w := b.Words[i]
	b.Words = slices.Delete(b.Words, i, i+1)
	b.RemovedWords = append(b.RemovedWords, Removed{ID: w.ID, Video: WordKey(w.Text), At: b.stamp()})
	return true
}

// SetWordMeaning writes reading and meaning as those of the word text of the language spoken, as a lookup made again does, and reports whether that changed the word.
func (b *Book) SetWordMeaning(text, spoken, reading, meaning string) bool {
	i := b.WordIndex(text, spoken)
	reading, meaning = strings.TrimSpace(reading), strings.TrimSpace(meaning)
	if i < 0 || (b.Words[i].Reading == reading && b.Words[i].Meaning == meaning) {
		return false
	}
	b.Words[i].Reading, b.Words[i].Meaning = reading, meaning
	b.Words[i].Stamps.Meaning = b.stamp()
	return true
}

// GradeWord schedules the word text of the language spoken after a review at now, as Grade does a line: good moves it up a box, else back to box 0, due at once. It reports whether there was such a word.
func (b *Book) GradeWord(text, spoken string, good bool, now time.Time) bool {
	i := b.WordIndex(text, spoken)
	if i < 0 {
		return false
	}
	w := &b.Words[i]
	w.Stamps.Review = Stamp{At: now, Device: b.Device}
	w.Box, w.Due = schedule(w.Box, good, now)
	return true
}

// DueWords is the indexes of the words due for review at now, the one due longest first, as Due is of the lines.
func (b Book) DueWords(now time.Time) []int {
	var out []int
	for i, w := range b.Words {
		if !w.Due.After(now) {
			out = append(out, i)
		}
	}
	slices.SortStableFunc(out, func(x, y int) int { return b.Words[x].Due.Compare(b.Words[y].Due) })
	return out
}

// schedule is the box and the due date of something in box after a review at now (Intervals).
func schedule(box int, good bool, now time.Time) (int, time.Time) {
	if !good {
		return 0, now
	}
	box = min(box+1, len(Intervals))
	return box, now.Add(Intervals[box-1])
}

// MergeWords takes into the vocabulary another device's words and the words it removed, as Merge does lines, and reports whether the vocabulary changed. A word removed on either device is removed on both; of a word both have, each field is the one changed later; a word both devices added on their own, of the same language spoken, becomes one, the one with the smaller id, and the other is removed. It is the same whichever book merges into which, and merging again changes nothing.
func (b *Book) MergeWords(words []Word, removed []Removed) bool {
	changed := false
	for _, r := range removed {
		changed = b.forgetWord(r) || changed
	}
	for _, in := range words {
		if in.ID == "" || slices.ContainsFunc(b.RemovedWords, func(r Removed) bool { return r.ID == in.ID }) {
			continue
		}
		i := b.WordByID(in.ID)
		if i < 0 {
			i = b.WordIndex(in.Text, in.Spoken)
		}
		if i < 0 {
			b.insertWord(in)
			changed = true
			continue
		}
		have := b.Words[i]
		merged := mergeWord(have, in)
		if have.ID != in.ID {
			lost := have
			if strings.Compare(in.ID, have.ID) > 0 {
				lost = in
			}
			b.Words = slices.Delete(b.Words, i, i+1)
			b.forgetWord(Removed{ID: lost.ID, Video: WordKey(lost.Text), At: Stamp{At: lost.SavedAt, Device: lost.Device}})
			b.insertWord(merged)
			changed = true
			continue
		}
		if !sameWord(have, merged) {
			b.Words[i] = merged
			changed = true
		}
	}
	return changed
}

// forgetWord records r, the later of two removals of one word, and takes the word out of the vocabulary; it reports whether that changed the book.
func (b *Book) forgetWord(r Removed) bool {
	if r.ID == "" {
		return false
	}
	changed := false
	if k := slices.IndexFunc(b.RemovedWords, func(have Removed) bool { return have.ID == r.ID }); k < 0 {
		b.RemovedWords = append(b.RemovedWords, r)
		changed = true
	} else if r.At.After(b.RemovedWords[k].At) {
		b.RemovedWords[k] = r
		changed = true
	}
	if i := b.WordByID(r.ID); i >= 0 {
		b.Words = slices.Delete(b.Words, i, i+1)
		changed = true
	}
	return changed
}

// insertWord puts w among the words by when it was added, newest first, after the words added at the same time.
func (b *Book) insertWord(w Word) {
	at, _ := slices.BinarySearchFunc(b.Words, w, func(have, w Word) int {
		if c := w.SavedAt.Compare(have.SavedAt); c != 0 {
			return c
		}
		return -1
	})
	b.Words = slices.Insert(b.Words, at, w)
}

// mergeWord is a and b, two copies of one word, field by field, as mergeLine is of a line.
func mergeWord(a, b Word) Word {
	out, other := a, b
	if strings.Compare(b.ID, a.ID) < 0 {
		out, other = b, a
	}
	if out.SavedAt.IsZero() || (!other.SavedAt.IsZero() && other.SavedAt.Before(out.SavedAt)) {
		out.SavedAt = other.SavedAt
	}
	if out.End <= out.Start && other.End > other.Start {
		out.End = other.End
	}
	if other.Stamps.Meaning.After(out.Stamps.Meaning) {
		out.Reading, out.Meaning, out.Language, out.Stamps.Meaning = other.Reading, other.Meaning, other.Language, other.Stamps.Meaning
	}
	if other.Stamps.Review.After(out.Stamps.Review) {
		out.Box, out.Due, out.Stamps.Review = other.Box, other.Due, other.Stamps.Review
	}
	return out
}

// sameWord reports whether a and b hold the same of every field MergeWords changes.
func sameWord(a, b Word) bool {
	same := func(x, y Stamp) bool { return x.At.Equal(y.At) && x.Device == y.Device }
	return a.ID == b.ID && a.Reading == b.Reading && a.Meaning == b.Meaning && a.Language == b.Language &&
		a.End == b.End && a.Box == b.Box && a.Due.Equal(b.Due) && a.SavedAt.Equal(b.SavedAt) && same(a.Stamps.Meaning, b.Stamps.Meaning) && same(a.Stamps.Review, b.Stamps.Review)
}
