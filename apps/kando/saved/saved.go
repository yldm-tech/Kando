// Package saved keeps the subtitle lines a learner saved while watching, with when each is next due for review, in one file in the library folder (FileName). It knows nothing of the interface: the app reads and writes a Book, and the export (export.go) writes the lines out for Anki.
package saved

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// FileName is the file the saved lines are kept in, at the top of the library folder, beside the films' folders. The library lists only videos, so it is not taken for one.
const FileName = "saved-lines.json"

// Line is a subtitle line the learner saved: where it is in which video, its two lines as they were when it was saved, so it stays as it was when the film is translated again, and its review schedule.
type Line struct {
	// Video is the path of the line's video under the library folder, with forward slashes, so the file stays right when the library folder is renamed or moved; a video kept outside the library has its full path (Key).
	Video string
	// Start and End are the line's times in its subtitles, before the subtitle delay the user set for the video moves them against the picture.
	Start, End time.Duration
	// Original is what is said, and Translation its translation, "" for a line that has none.
	Original, Translation string
	// Film is the film's title when the line was saved, for the list and the export.
	Film    string
	SavedAt time.Time
	// Box and Due are the line's review schedule (Grade): a line starts in box 0, due at once.
	Box int
	Due time.Time
}

// Book is the saved lines of one library, newest first.
type Book struct {
	dir   string
	Lines []Line
	// unread is why Load could not read the file. While it is set Save does not write, as the file may hold lines this book lacks.
	unread error
}

// ErrUnread is what Save returns for a book whose file Load could not read, so it is not written over.
var ErrUnread = errors.New(FileName + " could not be read, so it is not written over")

// Load reads the saved lines of the library folder dir. A missing file is a book without lines. A file that is there but is not one Load can read gives a book without lines and the error, and Save then leaves the file as it is (ErrUnread). Fields a later version adds are passed over, and a line without a time is dropped.
func Load(dir string) (Book, error) {
	b := Book{dir: dir}
	if dir == "" {
		return b, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		b.unread = err
		return b, err
	}
	var stored storedBook
	if err := json.Unmarshal(data, &stored); err != nil {
		b.unread = err
		return b, err
	}
	for _, s := range stored.Lines {
		if s.Video == "" || s.Start < 0 {
			continue
		}
		b.Lines = append(b.Lines, Line{
			Video: s.Video, Start: fromSeconds(s.Start), End: fromSeconds(s.End),
			Original: s.Original, Translation: s.Translation, Film: s.Film,
			SavedAt: s.SavedAt, Box: max(s.Box, 0), Due: s.Due,
		})
	}
	return b, nil
}

// Dir is the library folder the book belongs to.
func (b Book) Dir() string { return b.dir }

// Save writes the book atomically. A book without a folder is kept in memory only, as tests use it. A book with no lines and no file leaves no file behind.
func (b Book) Save() error {
	if b.dir == "" {
		return nil
	}
	if b.unread != nil {
		return fmt.Errorf("%w: %v", ErrUnread, b.unread)
	}
	path := filepath.Join(b.dir, FileName)
	if len(b.Lines) == 0 {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	stored := storedBook{Lines: make([]storedLine, len(b.Lines))}
	for i, l := range b.Lines {
		stored.Lines[i] = storedLine{
			Video: l.Video, Start: l.Start.Seconds(), End: l.End.Seconds(),
			Original: l.Original, Translation: l.Translation, Film: l.Film,
			SavedAt: l.SavedAt, Box: l.Box, Due: l.Due,
		}
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Key is how a Line names the video at path: its path under the book's library folder, with forward slashes, or for a video kept outside the library its full path as it is.
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
	return filepath.Clean(path)
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

// Toggle saves l, first and due at once, or removes the line of its video and start when it was saved, and reports whether it is saved now.
func (b *Book) Toggle(l Line) bool {
	if i := b.Find(l.Video, l.Start); i >= 0 {
		b.Lines = slices.Delete(b.Lines, i, i+1)
		return false
	}
	l.Box = 0
	l.Due = l.SavedAt
	b.Lines = slices.Insert(b.Lines, 0, l)
	return true
}

// FillTranslations gives each line of video, a path as Key gives it, that was saved without a translation, as one saved while its film was still being translated, the translation at gives for its start now: the translation and the original of the subtitle line starting there, ok false with none. It takes it only when that line still says the line's original, so a film made again with other lines leaves it alone. A line that has a translation keeps it as it was saved. It reports whether any line changed.
func (b *Book) FillTranslations(video string, at func(start time.Duration) (translation, original string, ok bool)) bool {
	changed := false
	for i, l := range b.Lines {
		if l.Video != video || l.Translation != "" {
			continue
		}
		translation, original, ok := at(l.Start)
		if !ok || strings.TrimSpace(translation) == "" || strings.TrimSpace(original) != strings.TrimSpace(l.Original) {
			continue
		}
		b.Lines[i].Translation = strings.TrimSpace(translation)
		changed = true
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
	b.Lines = slices.Delete(b.Lines, i, i+1)
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
			moved = true
		}
	}
	return moved
}

// Counts is how many lines each video has saved, by its path as Key gives it.
func (b Book) Counts() map[string]int {
	out := map[string]int{}
	for _, l := range b.Lines {
		out[l.Video]++
	}
	return out
}

// Clock is where a line starts, as the player's subtitle list writes it: "1:02.4", or "1:02:03.4" past an hour.
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h, m, s, tenth := ms/3_600_000, ms/60_000%60, ms/1000%60, ms/100%10
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%d", h, m, s, tenth)
	}
	return fmt.Sprintf("%d:%02d.%d", m, s, tenth)
}

// storedBook is FileName. Times are in seconds.
type storedBook struct {
	Lines []storedLine `json:"lines"`
}

type storedLine struct {
	Video       string    `json:"video"`
	Start       float64   `json:"start"`
	End         float64   `json:"end,omitempty"`
	Original    string    `json:"original"`
	Translation string    `json:"translation,omitempty"`
	Film        string    `json:"film,omitempty"`
	SavedAt     time.Time `json:"savedAt,omitzero"`
	Box         int       `json:"box"`
	Due         time.Time `json:"due,omitzero"`
}

// fromSeconds reads a time to the millisecond, as seconds such as 2.3 are not exact in binary and would otherwise read back a nanosecond short.
func fromSeconds(s float64) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(math.Round(s*1000)) * time.Millisecond
}
