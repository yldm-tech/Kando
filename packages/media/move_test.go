package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeFiles writes each file of files, by its path under dir, with its body.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// treeOf is the files under dir, hidden ones included, by their slash paths under it, with their bodies.
func treeOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	for name, body := range want {
		if got[name] != body {
			t.Fatalf("files %v, want %v", got, want)
		}
	}
}

var filmFolder = map[string]string{
	"video.mp4":               "picture",
	"video.srt":               "speech",
	"video.bilingual.srt":     "both",
	".video.translation.json": "{}",
	"cover.jpg":               "jpeg",
	"extras/notes.txt":        "notes",
}

// TestFilmMoveRenamesAFolder checks that a film's folder moves whole within a volume, the hidden files a job keeps included, and that the move names where each of its files went.
func TestFilmMoveRenamesAFolder(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	film := filepath.Join(from, "Talk [abc]")
	writeFiles(t, film, filmFolder)

	m := PlanFilmMove([]string{film}, true, to)
	if want := filepath.Join(to, "Talk [abc]"); m.Dest != want {
		t.Fatalf("Dest = %q, want %q", m.Dest, want)
	}
	if err := m.Rename(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(film); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder is still where it was: %v", err)
	}
	sameTree(t, treeOf(t, m.Dest), filmFolder)
	if got, ok := m.Path(filepath.Join(film, "video.mp4")); !ok || got != filepath.Join(m.Dest, "video.mp4") {
		t.Fatalf("Path(video) = %q, %v", got, ok)
	}
	if got, ok := m.Path(filepath.Join(film, "extras", "notes.txt")); !ok || got != filepath.Join(m.Dest, "extras", "notes.txt") {
		t.Fatalf("Path(notes) = %q, %v", got, ok)
	}
	if _, ok := m.Path(filepath.Join(from, "Other", "video.mp4")); ok {
		t.Fatal("Path moved a file of another folder")
	}
}

// TestFilmMoveGathersFilesIntoAFolderOfTheirOwn checks that a film sharing its folder moves its video and the files beside it that are its into a new folder, renamed when the name is taken, and leaves the folder's other files.
func TestFilmMoveGathersFilesIntoAFolderOfTheirOwn(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	writeFiles(t, from, map[string]string{
		"talk.mp4": "picture", "talk.srt": "speech", "talk.kando.srt": "kando", "talk.bilingual.srt": "both",
		".talk.translation.json": "{}", ".talk.pending.srt": "aside", ".talk.pending.json": "{}",
		"other.mp4": "other", "talk notes.txt": "mine",
	})
	writeFiles(t, to, map[string]string{"talk/video.mp4": "taken"})
	video := filepath.Join(from, "talk.mp4")
	companions := Companions(video)
	want := []string{"talk.srt", "talk.kando.srt", "talk.bilingual.srt", ".talk.translation.json", ".talk.pending.srt", ".talk.pending.json"}
	var names []string
	for _, c := range companions {
		names = append(names, filepath.Base(c))
	}
	if !slices.Equal(names, want) {
		t.Fatalf("Companions = %q, want %q", names, want)
	}

	m := PlanFilmMove(append([]string{video}, companions...), false, to)
	if want := filepath.Join(to, "talk 2"); m.Dest != want {
		t.Fatalf("Dest = %q, want %q beside the folder already named talk", m.Dest, want)
	}
	if err := m.Rename(); err != nil {
		t.Fatal(err)
	}
	sameTree(t, treeOf(t, m.Dest), map[string]string{
		"talk.mp4": "picture", "talk.srt": "speech", "talk.kando.srt": "kando", "talk.bilingual.srt": "both",
		".talk.translation.json": "{}", ".talk.pending.srt": "aside", ".talk.pending.json": "{}",
	})
	sameTree(t, treeOf(t, from), map[string]string{"other.mp4": "other", "talk notes.txt": "mine"})
	if got, ok := m.Path(video); !ok || got != filepath.Join(m.Dest, "talk.mp4") {
		t.Fatalf("Path(video) = %q, %v", got, ok)
	}
}

// TestFilmMoveCopies checks the move across volumes: the copy keeps each file's body, mode and time, says how far it has gone, leaves the sources until they are removed, and leaves nothing hidden behind in the other library.
func TestFilmMoveCopies(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	film := filepath.Join(from, "Talk")
	writeFiles(t, film, filmFolder)
	video := filepath.Join(film, "video.mp4")
	old := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(video, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(film, "video.srt"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := PlanFilmMove([]string{film}, true, to)
	var last, total int64
	if err := m.Copy(context.Background(), func(done, of int64) { last, total = done, of }); err != nil {
		t.Fatal(err)
	}
	var size int64
	for _, body := range filmFolder {
		size += int64(len(body))
	}
	if last != size || total != size {
		t.Fatalf("progress ended at %d of %d, want %d of %d", last, total, size, size)
	}
	sameTree(t, treeOf(t, m.Dest), filmFolder)
	sameTree(t, treeOf(t, film), filmFolder)
	if info, err := os.Stat(filepath.Join(m.Dest, "video.mp4")); err != nil || !info.ModTime().Equal(old) {
		t.Fatalf("the copy's time is %v (%v), want %v", info.ModTime(), err, old)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(m.Dest, "video.srt")); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("the copy's mode is %v (%v), want 0600", info.Mode().Perm(), err)
		}
	}
	entries, _ := os.ReadDir(to)
	if len(entries) != 1 {
		t.Fatalf("the other library holds %v, want the film's folder alone", entries)
	}
	if err := m.RemoveSources(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(film); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder is still there after RemoveSources: %v", err)
	}
}

// TestFilmMoveCopyStopped checks that a copy stopped partway, or one that cannot read a file, leaves the sources as they were and nothing in the other library.
func TestFilmMoveCopyStopped(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	film := filepath.Join(from, "Talk")
	writeFiles(t, film, filmFolder)
	m := PlanFilmMove([]string{film}, true, to)
	ctx, cancel := context.WithCancel(context.Background())
	err := m.Copy(ctx, func(int64, int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Copy = %v, want it stopped", err)
	}
	sameTree(t, treeOf(t, film), filmFolder)
	if entries, _ := os.ReadDir(to); len(entries) != 0 {
		t.Fatalf("a stopped copy left %v", entries)
	}

	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		return
	}
	locked := filepath.Join(film, "video.srt")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if err := m.Copy(context.Background(), nil); err == nil {
		t.Fatal("Copy of an unreadable file did not fail")
	}
	if entries, _ := os.ReadDir(to); len(entries) != 0 {
		t.Fatalf("a failed copy left %v", entries)
	}
	if _, err := os.Stat(filepath.Join(film, "video.mp4")); err != nil {
		t.Fatalf("a failed copy took the source: %v", err)
	}
}

// The sound a link's subtitles are made from goes with its film: named after the sound's extension, whatever the video's is, or without one as an older version named it; another film's beside it does not.
func TestCompanionsHoldTheSoundOfTheFilm(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Talk [x].mp4")
	for _, name := range []string{"Talk [x].mp4", ".Talk [x].sound.m4a", ".Talk [x].sound", ".Talk [x].sound.m4a.part.x", ".Talk [y].sound.m4a"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("s"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var sounds []string
	for _, path := range Companions(video) {
		if strings.Contains(filepath.Base(path), ".sound") {
			sounds = append(sounds, filepath.Base(path))
		}
	}
	slices.Sort(sounds)
	if want := []string{".Talk [x].sound", ".Talk [x].sound.m4a"}; !slices.Equal(sounds, want) {
		t.Fatalf("sounds %q, want %q", sounds, want)
	}
	if got := SoundVideo(filepath.Join(dir, ".Talk [x].sound.m4a")); got != filepath.Join(dir, "Talk [x].m4a") {
		t.Fatalf("the film goes by %q", got)
	}
	for _, name := range []string{".Talk [x].sound", "Talk [x].m4a", ".sound.m4a"} {
		if got := SoundVideo(filepath.Join(dir, name)); got != "" {
			t.Fatalf("%s goes by %q, want none", name, got)
		}
	}
}
