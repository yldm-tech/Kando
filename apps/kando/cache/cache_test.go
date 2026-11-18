package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClearRemovesFilesButKeepsWhatIsInUse(t *testing.T) {
	root := t.TempDir()
	posters, playback := filepath.Join(root, "posters"), filepath.Join(root, "playback")
	write(t, filepath.Join(posters, "a.jpg"), 100)
	write(t, filepath.Join(posters, "sites", "b.jpg"), 50)
	playing := filepath.Join(playback, "x", "film.mp4")
	write(t, playing, 1000)
	write(t, filepath.Join(playback, "y", "old.mp4"), 300)
	freed, err := Clear([]string{posters, playback, filepath.Join(root, "missing"), ""}, playing)
	if err != nil {
		t.Fatal(err)
	}
	if freed != 450 {
		t.Fatalf("freed %d bytes, want 450", freed)
	}
	if _, err := os.Stat(playing); err != nil {
		t.Fatalf("the copy in use is gone: %v", err)
	}
	for _, gone := range []string{filepath.Join(posters, "a.jpg"), filepath.Join(posters, "sites"), filepath.Join(playback, "y")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s is still there", gone)
		}
	}
	// The folders themselves stay, so what is cached next goes where it went.
	if _, err := os.Stat(posters); err != nil {
		t.Errorf("posters folder: %v", err)
	}
	if freed, err := Clear([]string{posters}); err != nil || freed != 0 {
		t.Fatalf("second clear freed %d, %v", freed, err)
	}
}

// TestClearKeepsTheCopyBeingMade keeps the part ffmpeg writes beside the copy it is making, named after it, while the copy itself is not there yet.
func TestClearKeepsTheCopyBeingMade(t *testing.T) {
	playback := t.TempDir()
	dest := filepath.Join(playback, "abc-1.mp4")
	part := dest + "-123.part"
	write(t, part, 10)
	write(t, filepath.Join(playback, "abd-1.mp4"), 20)
	freed, err := Clear([]string{playback}, dest)
	if err != nil || freed != 20 {
		t.Fatalf("freed %d, %v", freed, err)
	}
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("the part being written is gone: %v", err)
	}
}
