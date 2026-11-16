package saved

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBookRoundTrip(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(dir)
	if err != nil || len(b.Lines) != 0 {
		t.Fatalf("a library without the file: %d lines, %v", len(b.Lines), err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty book wrote its file: %v", err)
	}
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	video := b.Key(filepath.Join(dir, "Talk", "video.mp4"))
	if video != "Talk/video.mp4" {
		t.Fatalf("Key = %q", video)
	}
	if !b.Toggle(Line{Video: video, Start: 2300 * time.Millisecond, End: 4 * time.Second, Original: "Every river", Translation: "每条河", Film: "Talk", SavedAt: at}) {
		t.Fatal("Toggle did not save the line")
	}
	b.Toggle(Line{Video: video, Start: 10 * time.Second, Original: "begins", SavedAt: at.Add(time.Minute)})
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 2 || got.Lines[0].Original != "begins" {
		t.Fatalf("lines = %+v, want the newest first", got.Lines)
	}
	l := got.Lines[1]
	if l.Start != 2300*time.Millisecond || l.End != 4*time.Second || l.Translation != "每条河" || l.Film != "Talk" || !l.SavedAt.Equal(at) || !l.Due.Equal(at) || l.Box != 0 {
		t.Fatalf("line read back as %+v", l)
	}
	if got.Abs(l) != filepath.Join(dir, "Talk", "video.mp4") {
		t.Fatalf("Abs = %q", got.Abs(l))
	}
	if got.Toggle(Line{Video: video, Start: 2300*time.Millisecond + 400*time.Microsecond}) {
		t.Fatal("Toggle on a saved line's start saved it again")
	}
	if len(got.Lines) != 1 {
		t.Fatalf("lines after removing one: %+v", got.Lines)
	}
}

// TestLoadKeepsAnUnreadableFile checks that a file Load cannot read is left as it is: Save refuses to write over it.
func TestLoadKeepsAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dir)
	if err == nil {
		t.Fatal("a cut-short file read without an error")
	}
	b.Toggle(Line{Video: "a/video.mp4", Start: time.Second})
	if err := b.Save(); !errors.Is(err, ErrUnread) {
		t.Fatalf("Save over an unread file: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{" {
		t.Fatalf("the file became %q", data)
	}
}

// TestLoadReadsLaterFiles checks that fields a later version writes are passed over and a line without a video is dropped.
func TestLoadReadsLaterFiles(t *testing.T) {
	dir := t.TempDir()
	data := `{"version": 3, "lines": [{"video": "a/video.mp4", "start": 1.5, "original": "Hi", "ease": 2.5}, {"start": 2}]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dir)
	if err != nil || len(b.Lines) != 1 || b.Lines[0].Start != 1500*time.Millisecond {
		t.Fatalf("lines %+v, %v", b.Lines, err)
	}
}

// TestKeyKeepsOutsideVideosWhole checks that a video kept outside the library is named by its full path, which Abs gives back.
func TestKeyKeepsOutsideVideosWhole(t *testing.T) {
	root := filepath.Join(t.TempDir(), "a.kando")
	b := Book{dir: root}
	outside := filepath.Join(filepath.Dir(root), "b.mp4")
	if key := b.Key(outside); key != outside || b.Abs(Line{Video: key}) != outside {
		t.Fatalf("Key(%q) = %q, Abs %q", outside, key, b.Abs(Line{Video: key}))
	}
	if key := b.Key(""); key != "" {
		t.Fatalf("Key of no video = %q", key)
	}
}

func TestMoveVideosFollowsRenamedFolders(t *testing.T) {
	dir := t.TempDir()
	b, _ := Load(dir)
	b.Toggle(Line{Video: "Talk/video.mp4", Start: time.Second})
	b.Toggle(Line{Video: "Other/video.mp4", Start: time.Second})
	old, next := filepath.Join(dir, "Talk", "video.mp4"), filepath.Join(dir, "A Talk", "video.mp4")
	moved := b.MoveVideos(func(p string) (string, bool) {
		if p == old {
			return next, true
		}
		if strings.HasPrefix(p, filepath.Join(dir, "Other")) {
			return filepath.Join(string(filepath.Separator), "elsewhere", "video.mp4"), true
		}
		return "", false
	})
	if !moved || b.Lines[1].Video != "A Talk/video.mp4" || b.Lines[0].Video != filepath.Join(string(filepath.Separator), "elsewhere", "video.mp4") {
		t.Fatalf("moved %v, lines %+v", moved, b.Lines)
	}
}
