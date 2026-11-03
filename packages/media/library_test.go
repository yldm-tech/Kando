package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListVideosFiltersAndSortsByNewest(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "older.mp4")
	newer := filepath.Join(dir, "newer.mkv")
	for _, path := range []string{old, newer, filepath.Join(dir, "captions.srt"), filepath.Join(dir, ".hidden.mp4")} {
		if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(old, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}

	videos, err := ListVideos(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 || videos[0].Path != newer || videos[1].Path != old {
		t.Fatalf("videos = %#v", videos)
	}
}

func TestListVideosReturnsEmptyForMissingDirectory(t *testing.T) {
	videos, err := ListVideos(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 0 {
		t.Fatalf("videos = %#v", videos)
	}
}
