package study

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// legacyFile is a saved-lines.json as Kando wrote it before this package: the newest line first, times in seconds, a line reviewed twice in box 2, one saved while its film was being translated, so without a translation, and fields this version never wrote, which it passes over.
const legacyFile = `{
  "lines": [
    {
      "video": "Talk/video.mp4",
      "start": 10,
      "original": "begins",
      "savedAt": "2026-11-16T09:01:00Z",
      "box": 0,
      "due": "2026-11-16T09:01:00Z"
    },
    {
      "video": "Talk/video.mp4",
      "start": 2.3,
      "end": 4,
      "original": "Every river",
      "translation": "每条河",
      "film": "Talk",
      "savedAt": "2026-11-16T09:00:00Z",
      "box": 2,
      "due": "2026-11-20T09:00:00Z",
      "ease": 2.5
    }
  ]
}
`

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestLoadMigratesSavedLines reads a library that has only saved-lines.json: every line comes over with all it had, in the file's order, each with an id of its own and its fields stamped when it was saved; the book says it needs saving, Save writes study.json beside the old file and leaves the old file as it was, and the book read back has the same ids and needs no saving.
func TestLoadMigratesSavedLines(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, LegacyFileName)
	writeFile(t, legacy, legacyFile)

	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Migrated() {
		t.Fatal("a book read from saved-lines.json does not say it needs saving")
	}
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	if len(b.Lines) != 2 {
		t.Fatalf("lines %+v", b.Lines)
	}
	first, second := b.Lines[0], b.Lines[1]
	if first.Video != "Talk/video.mp4" || first.Start != 10*time.Second || first.Original != "begins" || first.Translation != "" || !first.SavedAt.Equal(at.Add(time.Minute)) || first.Box != 0 || !first.Due.Equal(at.Add(time.Minute)) {
		t.Fatalf("first line came over as %+v", first)
	}
	if second.Start != 2300*time.Millisecond || second.End != 4*time.Second || second.Original != "Every river" || second.Translation != "每条河" || second.Film != "Talk" || !second.SavedAt.Equal(at) || second.Box != 2 || !second.Due.Equal(at.Add(4*24*time.Hour)) {
		t.Fatalf("second line came over as %+v", second)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("ids %q and %q", first.ID, second.ID)
	}
	if s := second.Stamps; !s.Video.At.Equal(at) || !s.Original.At.Equal(at) || !s.Translation.At.Equal(at) || !s.Review.At.Equal(at) || !s.Note.At.Equal(at) {
		t.Fatalf("second line's stamps %+v, want all at %v", s, at)
	}

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, legacy); got != legacyFile {
		t.Fatalf("saved-lines.json became %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("no %s after the save: %v", FileName, err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Migrated() {
		t.Fatal("a book read after its migration was saved says it needs saving")
	}
	if !slices.EqualFunc(again.Lines, b.Lines, sameLine) {
		t.Fatalf("read back %+v, want %+v", again.Lines, b.Lines)
	}
}

// sameLine compares two lines field by field, times by Equal, as a time read back from JSON has no monotonic reading.
func sameLine(a, b Line) bool {
	stampsEqual := func(x, y Stamp) bool { return x.At.Equal(y.At) && x.Device == y.Device }
	return a.ID == b.ID && a.Device == b.Device && a.Video == b.Video && a.Start == b.Start && a.End == b.End &&
		a.Original == b.Original && a.Translation == b.Translation && a.Film == b.Film && a.Note == b.Note &&
		a.SavedAt.Equal(b.SavedAt) && a.Box == b.Box && a.Due.Equal(b.Due) &&
		stampsEqual(a.Stamps.Video, b.Stamps.Video) && stampsEqual(a.Stamps.Original, b.Stamps.Original) &&
		stampsEqual(a.Stamps.Translation, b.Stamps.Translation) && stampsEqual(a.Stamps.Note, b.Stamps.Note) &&
		stampsEqual(a.Stamps.Review, b.Stamps.Review)
}

// TestLoadTakesLinesAnOlderKandoSavedSince checks a library an older Kando used again after the migration: while saved-lines.json is as it was migrated, a line removed since stays removed; once that Kando writes the file again, the line it saved since comes over, the line removed here after it was saved stays removed, and the lines both have stay once, as they are here.
func TestLoadTakesLinesAnOlderKandoSavedSince(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, LegacyFileName)
	writeFile(t, legacy, legacyFile)
	b, _ := Load(dir)
	b.Device = "mac"
	b.Now = func() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC) }
	if !b.Remove("Talk/video.mp4", 10*time.Second) {
		t.Fatal("no line to remove")
	}
	b.SetNote(b.Lines[0].ID, "a note")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ = Load(dir)
	if len(b.Lines) != 1 || b.Lines[0].Original != "Every river" || b.Migrated() {
		t.Fatalf("with saved-lines.json as migrated: lines %+v, migrated %v", b.Lines, b.Migrated())
	}

	// An older Kando, which knows only saved-lines.json, saves a line on the 18th.
	writeFile(t, legacy, `{"lines": [
		{"video": "Talk/video.mp4", "start": 20, "original": "and ends", "savedAt": "2026-11-18T09:00:00Z", "box": 0, "due": "2026-11-18T09:00:00Z"},
		{"video": "Talk/video.mp4", "start": 10, "original": "begins", "savedAt": "2026-11-16T09:01:00Z", "box": 0, "due": "2026-11-16T09:01:00Z"},
		{"video": "Talk/video.mp4", "start": 2.3, "end": 4, "original": "Every river", "translation": "每条河", "film": "Talk", "savedAt": "2026-11-16T09:00:00Z", "box": 2, "due": "2026-11-20T09:00:00Z"}
	]}`)
	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range b.Lines {
		got = append(got, l.Original+"|"+l.Note)
	}
	if want := []string{"and ends|", "Every river|a note"}; !slices.Equal(got, want) || !b.Migrated() {
		t.Fatalf("lines %q, want %q; migrated %v", got, want, b.Migrated())
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if b, _ = Load(dir); len(b.Lines) != 2 || b.Migrated() {
		t.Fatalf("read again: lines %+v, migrated %v", b.Lines, b.Migrated())
	}
}

// TestLoadKeepsAnUnreadableOldFile checks that a saved-lines.json that cannot be read, in a library not migrated yet, is reported and left as it is: the book refuses to save, so no study.json takes the old file's place without its lines.
func TestLoadKeepsAnUnreadableOldFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, LegacyFileName)
	writeFile(t, legacy, "{")
	b, err := Load(dir)
	if err == nil || b.Unread() == nil {
		t.Fatalf("a cut-short saved-lines.json read as %v, unread %v", err, b.Unread())
	}
	b.Toggle(Line{Video: "a/video.mp4", Start: time.Second})
	if err := b.Save(); !errors.Is(err, ErrUnread) {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s written: %v", FileName, err)
	}
	if got := readFile(t, legacy); got != "{" {
		t.Fatalf("saved-lines.json became %q", got)
	}
}

// TestLoadPassesOverABrokenOldFileOnceMigrated checks that once a library has study.json, a saved-lines.json that cannot be read does not keep its saved lines from the learner: the book reads, and saves, and the old file stays as it is.
func TestLoadPassesOverABrokenOldFileOnceMigrated(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, LegacyFileName)
	writeFile(t, legacy, legacyFile)
	b, _ := Load(dir)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, legacy, "{")
	b, err := Load(dir)
	if err != nil || len(b.Lines) != 2 {
		t.Fatalf("lines %+v, %v", b.Lines, err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, legacy); got != "{" {
		t.Fatalf("saved-lines.json became %q", got)
	}
}

// TestLoadKeepsAnUnreadableFile checks that a study.json that cannot be read, or that a later Kando wrote in a version this one does not know, is left as it is: Save refuses to write over it, and saved-lines.json is not read in its place.
func TestLoadKeepsAnUnreadableFile(t *testing.T) {
	for name, data := range map[string]string{
		"cut short": "{",
		"newer":     `{"version": 99, "lines": [{"id": "x", "video": "a/video.mp4", "start": 1}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			writeFile(t, path, data)
			writeFile(t, filepath.Join(dir, LegacyFileName), legacyFile)
			b, err := Load(dir)
			if err == nil || b.Unread() == nil || len(b.Lines) != 0 {
				t.Fatalf("read as %+v, %v", b.Lines, err)
			}
			b.Toggle(Line{Video: "a/video.mp4", Start: time.Second})
			if err := b.Save(); !errors.Is(err, ErrUnread) {
				t.Fatalf("Save over an unread file: %v", err)
			}
			if got := readFile(t, path); got != data {
				t.Fatalf("the file became %q", got)
			}
		})
	}
}

// TestSaveKeepsWhatSyncNeeds checks that what this package adds to a line for sync is written and read back: its id, the device that saved it, its note, each field's stamp and the lines removed.
func TestSaveKeepsWhatSyncNeeds(t *testing.T) {
	dir := t.TempDir()
	b, _ := Load(dir)
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	b.Device = "mac-1"
	b.Now = func() time.Time { return at.Add(time.Hour) }
	b.Toggle(Line{Video: "Talk/video.mp4", Start: time.Second, Original: "One", SavedAt: at})
	b.Toggle(Line{Video: "Talk/video.mp4", Start: 2 * time.Second, Original: "Two", SavedAt: at})
	if !b.SetNote(b.Lines[1].ID, "  first  ") {
		t.Fatal("SetNote found no line")
	}
	b.Remove("Talk/video.mp4", 2*time.Second)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(got.Lines, b.Lines, sameLine) {
		t.Fatalf("read back %+v, want %+v", got.Lines, b.Lines)
	}
	l := got.Lines[0]
	if l.Note != "first" || l.Device != "mac-1" || l.Stamps.Note != (Stamp{At: at.Add(time.Hour), Device: "mac-1"}) || !l.Stamps.Original.At.Equal(at) {
		t.Fatalf("line %+v", l)
	}
	if len(got.Removed) != 1 || got.Removed[0].Video != "Talk/video.mp4" || got.Removed[0].Start != 2*time.Second || got.Removed[0].At != (Stamp{At: at.Add(time.Hour), Device: "mac-1"}) || got.Removed[0].ID == "" {
		t.Fatalf("removed %+v", got.Removed)
	}
}
