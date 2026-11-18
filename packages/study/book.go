// Package study is the learner's study record, shared by Kando on the computer and on the phone: the subtitle lines the learner saved while watching, each with a note and its review schedule, the words looked up and put in the vocabulary (Word), and how long films played on each day and device (Watched), which the stats are made from (Stats), all in one Book kept in one file in the library folder (FileName). It knows nothing of the interface.
//
// Every line is a record built to be synced between devices: it has an id of its own, the device that saved it, and for each field that can change after it was saved, when and on which device it last changed (Stamps), so two devices' books merge field by field, the later change winning (Merge). A removed line leaves a Removed behind, so a device that still has it does not bring it back.
//
// Before this package Kando kept the saved lines in saved-lines.json (LegacyFileName). Load reads that file into the book of a library that has none yet, and again whenever an older Kando has written it since, and leaves the file as it is.
package study

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// FileName is the file the study record is kept in, at the top of the library folder, beside the films' folders. The library lists only videos, so it is not taken for one.
const FileName = "study.json"

// Version is the version of FileName this package writes. A file of a later version is not read, nor written over (ErrUnread). Version 2 added the vocabulary and the time watched, which a Kando that writes version 1 would pass over and then write the file without.
const Version = 2

// Stamp is when a field last changed and on which device. Of two changes the later wins; of two at the same time, the one of the device whose id sorts last, so every device picks the same one.
type Stamp struct {
	At     time.Time
	Device string
}

// After reports whether s is a later change than o.
func (s Stamp) After(o Stamp) bool {
	if !s.At.Equal(o.At) {
		return s.At.After(o.At)
	}
	return s.Device > o.Device
}

// Stamps is when each field of a Line that can change after it was saved last changed. The other fields, its start and end, its film, when and on which device it was saved, stay as they were saved.
type Stamps struct {
	Video, Original, Translation, Note Stamp
	// Review is Box and Due's, which change together.
	Review Stamp
}

// Line is a subtitle line the learner saved: where it is in which video, its two lines as they were when it was saved, so it stays as it was when the film is translated again, the learner's note on it and its review schedule.
type Line struct {
	// ID names the line on every device; Device is the device that saved it, "" for a line saved before lines had one.
	ID, Device string
	// Video is the path of the line's video under the library folder, with forward slashes, so the file stays right when the library folder is renamed or moved; a video kept outside the library has its full path (Key).
	Video string
	// Start and End are the line's times in its subtitles, before the subtitle delay the user set for the video moves them against the picture.
	Start, End time.Duration
	// Original is what is said, and Translation its translation, "" for a line that has none.
	Original, Translation string
	// Film is the film's title when the line was saved, for the list and the export.
	Film string
	// Note is what the learner wrote about the line, "" for none.
	Note    string
	SavedAt time.Time
	// Box and Due are the line's review schedule (Grade): a line starts in box 0, due at once.
	Box int
	Due time.Time
	// Stamps is when each field last changed, for Merge.
	Stamps Stamps
}

// Removed is a line taken out of the book: its id, its video and start, and when and on which device it was removed, so Merge does not take it back from a book that still has it, nor Load from saved-lines.json.
type Removed struct {
	ID    string
	Video string
	Start time.Duration
	At    Stamp
}

// Book is the study record of one library: its saved lines, newest first, and the lines removed from it; its vocabulary, newest first, and the words removed from it; and how long films played on each day and device.
type Book struct {
	dir          string
	Lines        []Line
	Removed      []Removed
	Words        []Word
	RemovedWords []Removed
	Watched      []Watched
	// Device is the id of the device the book's changes are made on, which they are stamped with; Now is the clock they are stamped by, time.Now when nil.
	Device string
	Now    func() time.Time
	// imported is the SHA-256 of saved-lines.json as the book last took its lines, so Load takes them again only once an older Kando has written it since. taken is the lines that file held then (legacyKey), which Load does not take again, wherever they went in the book since. migrated is set when Load took lines from it, so the book needs saving.
	imported string
	taken    map[string]took
	migrated bool
	// unread is why Load could not read the file. While it is set Save does not write, as the file may hold lines this book lacks.
	unread error
}

// ErrUnread is what Save returns for a book whose file Load could not read, so it is not written over.
var ErrUnread = errors.New("the study record could not be read, so it is not written over")

// Load reads the study record of the library folder dir. A missing file is a book without lines, which takes the lines of saved-lines.json when the library has one (legacy.go). A file that is there but is not one Load can read, or one a later version wrote, gives a book without lines and the error, and Save then leaves the file as it is (ErrUnread). Fields a later version of the same file version adds are passed over, and a line without a video, a time or an id is dropped.
func Load(dir string) (Book, error) {
	b := Book{dir: dir}
	if dir == "" {
		return b, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	found := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		b.unread = err
		return b, err
	}
	if found {
		if err := b.decode(data); err != nil {
			b = Book{dir: dir, unread: err}
			return b, err
		}
	}
	if err := b.importLegacy(found); err != nil {
		b = Book{dir: dir, unread: err}
		return b, err
	}
	return b, nil
}

// Dir is the library folder the book belongs to.
func (b Book) Dir() string { return b.dir }

// Migrated reports whether Load took lines from saved-lines.json that the book's file does not have yet, so the book needs saving.
func (b Book) Migrated() bool { return b.migrated }

// Save writes the book atomically. A book without a folder is kept in memory only, as tests use it. A book with nothing in it and no file leaves no file behind.
//
// The file may have changed since Load read it, as when another computer that opens the same library folder, shared or synced, saved to it: Save first merges what the file holds now into the book (Merge, MergeWords, MergeWatched), so the other's changes are kept rather than written over. A file that cannot be read now, or that a later version wrote since, is left as it is (ErrUnread), and the book is not written from then on.
func (b *Book) Save() error {
	if b.dir == "" {
		return nil
	}
	if b.unread != nil {
		return fmt.Errorf("%w: %v", ErrUnread, b.unread)
	}
	path := filepath.Join(b.dir, FileName)
	if err := b.mergeFile(path); err != nil {
		return err
	}
	if len(b.Lines) == 0 && len(b.Removed) == 0 && len(b.Words) == 0 && len(b.RemovedWords) == 0 && len(b.Watched) == 0 && b.imported == "" {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	data, err := json.MarshalIndent(b.encode(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	b.migrated = false
	return nil
}

// mergeFile merges the lines, the words and the time watched that the file at path holds now into the book, as another device's (Save). A file that is not there holds nothing; one that cannot be read or decoded marks the book unread.
func (b *Book) mergeFile(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		var disk Book
		if err = disk.decode(data); err == nil {
			b.Merge(disk.Lines, disk.Removed)
			b.MergeWords(disk.Words, disk.RemovedWords)
			b.MergeWatched(disk.Watched)
			return nil
		}
		b.unread = err
	}
	return fmt.Errorf("%w: %v", ErrUnread, err)
}

// stamp is a change made now on the book's device.
func (b Book) stamp() Stamp {
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	return Stamp{At: now(), Device: b.Device}
}

// Key is how a Line names the video at path: its path under the book's library folder, with forward slashes, or for a video kept outside the library its full path as it is. A relative path, as a book with no folder is given, is a path under the library too, so it also takes forward slashes, as Abs reads it.
func (b Book) Key(path string) string {
	if path == "" {
		return ""
	}
	if b.dir != "" {
		rel, err := filepath.Rel(filepath.Clean(b.dir), filepath.Clean(path))
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return filepath.ToSlash(rel)
		}
	}
	if path = filepath.Clean(path); !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	return path
}

// Abs is the path on disk of the video of l.
func (b Book) Abs(l Line) string {
	if filepath.IsAbs(l.Video) {
		return l.Video
	}
	return filepath.Join(b.dir, filepath.FromSlash(l.Video))
}

// sameStart is how far apart two starts may be and still be one line's: the subtitle file writes times to the millisecond, and the book keeps them in seconds.
const sameStart = time.Millisecond

// Find is the index of the line of video, a path as Key gives it, that starts at start, or -1.
func (b Book) Find(video string, start time.Duration) int {
	return slices.IndexFunc(b.Lines, func(l Line) bool { return l.Video == video && SameStart(l.Start, start) })
}

// Index is the index of the line id, or -1.
func (b Book) Index(id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(b.Lines, func(l Line) bool { return l.ID == id })
}

// Toggle saves l, first and due at once, or removes the line of its video and start when it was saved, and reports whether it is saved now. A line saved gets an id when it has none and the book's device, and its fields are stamped when it was saved, SavedAt, or now when that is not set.
func (b *Book) Toggle(l Line) bool {
	if i := b.Find(l.Video, l.Start); i >= 0 {
		b.removeAt(i)
		return false
	}
	at := b.stamp()
	if l.SavedAt.IsZero() {
		l.SavedAt = at.At
	}
	at.At = l.SavedAt
	if l.ID == "" {
		l.ID = NewID()
	}
	if l.Device == "" {
		l.Device = b.Device
	}
	l.Note = strings.TrimSpace(l.Note)
	l.Box = 0
	l.Due = l.SavedAt
	l.Stamps = Stamps{Video: at, Original: at, Translation: at, Note: at, Review: at}
	b.Lines = slices.Insert(b.Lines, 0, l)
	return true
}

// removeAt takes line i out of the book, leaving a Removed behind.
func (b *Book) removeAt(i int) {
	l := b.Lines[i]
	b.Lines = slices.Delete(b.Lines, i, i+1)
	b.Removed = append(b.Removed, Removed{ID: l.ID, Video: l.Video, Start: l.Start, At: b.stamp()})
}

// SetNote writes note, trimmed, as the note of the line id, and reports whether that changed the line; "" takes the note away.
func (b *Book) SetNote(id, note string) bool {
	i := b.Index(id)
	note = strings.TrimSpace(note)
	if i < 0 || b.Lines[i].Note == note {
		return false
	}
	b.Lines[i].Note = note
	b.Lines[i].Stamps.Note = b.stamp()
	return true
}

// FollowSubtitles keeps each line of video, a path as Key gives it, as the film's subtitles say it now, at giving the translation and the original of the subtitle line starting at a line's start, ok false with none. A line whose original the film's now proofreads (proofreads, media.Proofreads), as a film whose transcript was proofread after the line was saved, takes the corrected original; one whose original the film no longer says, as a film made again with other lines, is left alone. A line saved without a translation, as one saved while its film was still being translated, takes the translation the film has now; one that has a translation keeps it as it was saved. It reports whether any line changed.
func (b *Book) FollowSubtitles(video string, at func(start time.Duration) (translation, original string, ok bool), proofreads func(heard, now string) bool) bool {
	changed := false
	for i, l := range b.Lines {
		if l.Video != video {
			continue
		}
		translation, original, ok := at(l.Start)
		original = strings.TrimSpace(original)
		if !ok || original == "" {
			continue
		}
		if original != strings.TrimSpace(l.Original) {
			if !proofreads(l.Original, original) {
				continue
			}
			b.Lines[i].Original = original
			b.Lines[i].Stamps.Original = b.stamp()
			changed = true
		}
		if translation = strings.TrimSpace(translation); l.Translation == "" && translation != "" {
			b.Lines[i].Translation = translation
			b.Lines[i].Stamps.Translation = b.stamp()
			changed = true
		}
	}
	return changed
}

// SameStart reports whether a and b are one line's start, as Find takes them.
func SameStart(a, b time.Duration) bool {
	d := a - b
	return d < sameStart && d > -sameStart
}

// Remove drops the line of video that starts at start, and reports whether there was one.
func (b *Book) Remove(video string, start time.Duration) bool {
	i := b.Find(video, start)
	if i < 0 {
		return false
	}
	b.removeAt(i)
	return true
}

// MoveVideos keys the lines of videos that moved on disk by their new paths: move gives a video's new path, a full one, for its old one, and reports false for one that did not move. It reports whether any moved.
func (b *Book) MoveVideos(move func(path string) (string, bool)) bool {
	moved := false
	for i, l := range b.Lines {
		next, ok := move(b.Abs(l))
		if !ok {
			continue
		}
		if key := b.Key(next); key != l.Video {
			b.Lines[i].Video = key
			b.Lines[i].Stamps.Video = b.stamp()
			moved = true
		}
	}
	return moved
}

// Unread is why Load could not read the book's file, nil when it read it or there was none; while it is set Save leaves the file as it is (ErrUnread).
func (b Book) Unread() error { return b.unread }

// TakeVideo takes the lines of the video at path out of the book and returns them, newest first, as when the video moves to another library (AddVideo there).
func (b *Book) TakeVideo(path string) []Line {
	key := b.Key(path)
	var taken []Line
	for i := 0; i < len(b.Lines); {
		if b.Lines[i].Video != key {
			i++
			continue
		}
		taken = append(taken, b.Lines[i])
		b.removeAt(i)
	}
	return taken
}

// AddVideo puts lines, the lines another book kept of a video (TakeVideo), in this book as the lines of the video at path, each with its id, its note, its times and schedule, among the others by when it was saved, newest first. A line this book has already, at the same start of that video, stays as it is.
//
// A line whose id this book removed, as one that comes back when a move to another library is undone, takes a new id and the removal stays: a device that has the removal takes the line out by its id when the books merge (Merge), so the line under its old id would go there and then here.
func (b *Book) AddVideo(path string, lines []Line) {
	key := b.Key(path)
	for _, l := range lines {
		if b.Find(key, l.Start) >= 0 {
			continue
		}
		if l.ID == "" || b.Index(l.ID) >= 0 || slices.ContainsFunc(b.Removed, func(r Removed) bool { return r.ID == l.ID }) {
			l.ID = NewID()
		}
		if l.Video != key {
			l.Video = key
			l.Stamps.Video = b.stamp()
		}
		b.insert(l)
	}
}

// insert puts l among the lines by when it was saved, newest first, after the lines saved at the same time.
func (b *Book) insert(l Line) {
	at, _ := slices.BinarySearchFunc(b.Lines, l, func(have, l Line) int {
		if c := l.SavedAt.Compare(have.SavedAt); c != 0 {
			return c
		}
		return -1
	})
	b.Lines = slices.Insert(b.Lines, at, l)
}

// Counts is how many lines each video has saved, by its path as Key gives it.
func (b Book) Counts() map[string]int {
	out := map[string]int{}
	for _, l := range b.Lines {
		out[l.Video]++
	}
	return out
}

// NewID is a new line's id: 16 random hex digits.
func NewID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// storedBook is FileName. Times in a line are in seconds.
type storedBook struct {
	Version      int             `json:"version"`
	Imported     string          `json:"imported,omitempty"`
	Taken        []storedTook    `json:"taken,omitempty"`
	Lines        []storedLine    `json:"lines"`
	Removed      []storedGone    `json:"removed,omitempty"`
	Words        []storedWord    `json:"words,omitempty"`
	RemovedWords []storedGone    `json:"removedWords,omitempty"`
	Watched      []storedWatched `json:"watched,omitempty"`
}

// storedWord is a Word; its start is in seconds.
type storedWord struct {
	ID                 string           `json:"id"`
	Device             string           `json:"device,omitempty"`
	Text               string           `json:"text"`
	Spoken             string           `json:"spoken,omitempty"`
	Reading            string           `json:"reading,omitempty"`
	Meaning            string           `json:"meaning,omitempty"`
	Language           string           `json:"language,omitempty"`
	Context            string           `json:"context,omitempty"`
	ContextTranslation string           `json:"contextTranslation,omitempty"`
	Video              string           `json:"video,omitempty"`
	Start              float64          `json:"start,omitempty"`
	End                float64          `json:"end,omitempty"`
	Film               string           `json:"film,omitempty"`
	SavedAt            time.Time        `json:"savedAt,omitzero"`
	Box                int              `json:"box"`
	Due                time.Time        `json:"due,omitzero"`
	Stamps             storedWordStamps `json:"stamps"`
}

type storedWordStamps struct {
	Meaning storedStamp `json:"meaning,omitzero"`
	Review  storedStamp `json:"review,omitzero"`
}

// storedWatched is a Watched, its time in seconds.
type storedWatched struct {
	Day     string  `json:"day"`
	Device  string  `json:"device,omitempty"`
	Seconds float64 `json:"seconds"`
}

type storedStamp struct {
	At     time.Time `json:"at,omitzero"`
	Device string    `json:"device,omitempty"`
}

type storedStamps struct {
	Video       storedStamp `json:"video,omitzero"`
	Original    storedStamp `json:"original,omitzero"`
	Translation storedStamp `json:"translation,omitzero"`
	Note        storedStamp `json:"note,omitzero"`
	Review      storedStamp `json:"review,omitzero"`
}

type storedLine struct {
	ID          string       `json:"id"`
	Device      string       `json:"device,omitempty"`
	Video       string       `json:"video"`
	Start       float64      `json:"start"`
	End         float64      `json:"end,omitempty"`
	Original    string       `json:"original"`
	Translation string       `json:"translation,omitempty"`
	Film        string       `json:"film,omitempty"`
	Note        string       `json:"note,omitempty"`
	SavedAt     time.Time    `json:"savedAt,omitzero"`
	Box         int          `json:"box"`
	Due         time.Time    `json:"due,omitzero"`
	Stamps      storedStamps `json:"stamps"`
}

// storedTook is a line of saved-lines.json the book took already (Book.taken): its video and start, and when it was saved, which name it there.
type storedTook struct {
	Video   string    `json:"video"`
	Start   float64   `json:"start"`
	SavedAt time.Time `json:"savedAt,omitzero"`
}

type storedGone struct {
	ID    string      `json:"id"`
	Video string      `json:"video"`
	Start float64     `json:"start"`
	At    storedStamp `json:"at"`
}

func toStored(s Stamp) storedStamp   { return storedStamp(s) }
func fromStored(s storedStamp) Stamp { return Stamp(s) }

func (b Book) encode() storedBook {
	out := storedBook{Version: Version, Imported: b.imported, Lines: make([]storedLine, len(b.Lines))}
	for i, l := range b.Lines {
		out.Lines[i] = storedLine{
			ID: l.ID, Device: l.Device,
			Video: l.Video, Start: l.Start.Seconds(), End: l.End.Seconds(),
			Original: l.Original, Translation: l.Translation, Film: l.Film, Note: l.Note,
			SavedAt: l.SavedAt, Box: l.Box, Due: l.Due,
			Stamps: storedStamps{
				Video: toStored(l.Stamps.Video), Original: toStored(l.Stamps.Original), Translation: toStored(l.Stamps.Translation),
				Note: toStored(l.Stamps.Note), Review: toStored(l.Stamps.Review),
			},
		}
	}
	for _, r := range b.Removed {
		out.Removed = append(out.Removed, storedGone{ID: r.ID, Video: r.Video, Start: r.Start.Seconds(), At: toStored(r.At)})
	}
	for _, w := range b.Words {
		out.Words = append(out.Words, storedWord{
			ID: w.ID, Device: w.Device, Text: w.Text, Spoken: w.Spoken, Reading: w.Reading, Meaning: w.Meaning, Language: w.Language,
			Context: w.Context, ContextTranslation: w.ContextTranslation, Video: w.Video, Start: w.Start.Seconds(), End: w.End.Seconds(), Film: w.Film,
			SavedAt: w.SavedAt, Box: w.Box, Due: w.Due,
			Stamps: storedWordStamps{Meaning: toStored(w.Stamps.Meaning), Review: toStored(w.Stamps.Review)},
		})
	}
	for _, r := range b.RemovedWords {
		out.RemovedWords = append(out.RemovedWords, storedGone{ID: r.ID, Video: r.Video, At: toStored(r.At)})
	}
	for _, w := range b.Watched {
		out.Watched = append(out.Watched, storedWatched{Day: w.Day, Device: w.Device, Seconds: w.Time.Seconds()})
	}
	for _, k := range slices.Sorted(maps.Keys(b.taken)) {
		t := b.taken[k]
		out.Taken = append(out.Taken, storedTook{Video: t.Video, Start: t.Start.Seconds(), SavedAt: t.SavedAt})
	}
	return out
}

// decode reads data, FileName, into the book.
func (b *Book) decode(data []byte) error {
	var stored storedBook
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Version > Version {
		return fmt.Errorf("%s is version %d, which a later Kando wrote", FileName, stored.Version)
	}
	b.imported = stored.Imported
	for _, s := range stored.Taken {
		b.took(took{Video: s.Video, Start: fromSeconds(s.Start), SavedAt: s.SavedAt})
	}
	for _, s := range stored.Lines {
		if s.ID == "" || s.Video == "" || s.Start < 0 {
			continue
		}
		b.Lines = append(b.Lines, Line{
			ID: s.ID, Device: s.Device,
			Video: s.Video, Start: fromSeconds(s.Start), End: fromSeconds(s.End),
			Original: s.Original, Translation: s.Translation, Film: s.Film, Note: s.Note,
			SavedAt: s.SavedAt, Box: max(s.Box, 0), Due: s.Due,
			Stamps: Stamps{
				Video: fromStored(s.Stamps.Video), Original: fromStored(s.Stamps.Original), Translation: fromStored(s.Stamps.Translation),
				Note: fromStored(s.Stamps.Note), Review: fromStored(s.Stamps.Review),
			},
		})
	}
	for _, s := range stored.Removed {
		if s.ID == "" {
			continue
		}
		b.Removed = append(b.Removed, Removed{ID: s.ID, Video: s.Video, Start: fromSeconds(s.Start), At: fromStored(s.At)})
	}
	for _, s := range stored.Words {
		if s.ID == "" || strings.TrimSpace(s.Text) == "" {
			continue
		}
		b.Words = append(b.Words, Word{
			ID: s.ID, Device: s.Device, Text: s.Text, Spoken: s.Spoken, Reading: s.Reading, Meaning: s.Meaning, Language: s.Language,
			Context: s.Context, ContextTranslation: s.ContextTranslation, Video: s.Video, Start: fromSeconds(s.Start), End: fromSeconds(s.End), Film: s.Film,
			SavedAt: s.SavedAt, Box: max(s.Box, 0), Due: s.Due,
			Stamps: WordStamps{Meaning: fromStored(s.Stamps.Meaning), Review: fromStored(s.Stamps.Review)},
		})
	}
	for _, s := range stored.RemovedWords {
		if s.ID != "" {
			b.RemovedWords = append(b.RemovedWords, Removed{ID: s.ID, Video: s.Video, At: fromStored(s.At)})
		}
	}
	for _, s := range stored.Watched {
		if s.Day != "" && s.Seconds > 0 {
			b.MergeWatched([]Watched{{Day: s.Day, Device: s.Device, Time: fromSeconds(s.Seconds)}})
		}
	}
	return nil
}

// fromSeconds reads a time to the millisecond, as seconds such as 2.3 are not exact in binary and would otherwise read back a nanosecond short.
func fromSeconds(s float64) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(math.Round(s*1000)) * time.Millisecond
}
