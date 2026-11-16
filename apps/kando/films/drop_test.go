package films

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A dropped folder gives its own videos and those one folder down, each folder's in the order of their names with numbers by value, and counts the files that are not videos; hidden files and anything deeper are left out. Dropped files keep the order they came in.
func TestDroppedVideosOpenFolders(t *testing.T) {
	root := t.TempDir()
	course := filepath.Join(root, "Course")
	files := []string{
		"Course/Episode 10.mp4", "Course/Episode 2.mkv", "Course/Episode 1.mov", "Course/notes.pdf", "Course/.DS_Store", "Course/Episode 1.srt",
		"Course/Section 10/b.mp4", "Course/Section 2/a.mp4", "Course/Section 2/slides.key", "Course/Section 2/Extras/deep.mp4", "Course/.hidden/secret.mp4",
		"loose.webm", "readme.txt",
	}
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	videos, skipped := DroppedVideos([]string{filepath.Join(root, "loose.webm"), course, filepath.Join(root, "readme.txt"), filepath.Join(root, "gone.mp4")}, "en")
	want := []string{
		filepath.Join(root, "loose.webm"),
		filepath.Join(course, "Episode 1.mov"), filepath.Join(course, "Episode 2.mkv"), filepath.Join(course, "Episode 10.mp4"),
		filepath.Join(course, "Section 2", "a.mp4"), filepath.Join(course, "Section 10", "b.mp4"),
		filepath.Join(root, "gone.mp4"),
	}
	if !slices.Equal(videos, want) {
		t.Fatalf("videos\n%q\nwant\n%q", videos, want)
	}
	// notes.pdf, Episode 1.srt, slides.key and readme.txt.
	if skipped != 4 {
		t.Fatalf("skipped %d, want 4", skipped)
	}
}

// A folder without videos gives none, and counts what it holds.
func TestDroppedVideosOfAFolderWithoutVideos(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if videos, skipped := DroppedVideos([]string{dir}, "zh-Hans"); len(videos) != 0 || skipped != 1 {
		t.Fatalf("videos %q skipped %d", videos, skipped)
	}
}
