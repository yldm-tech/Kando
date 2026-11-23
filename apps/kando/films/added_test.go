package films

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/media"
)

// A video added to the library without a job is listed though it is outside the library's folder, by when it was added among the jobs' films, with the subtitles beside it, and named after its folder when it is a video.<ext>; one a job holds is the job's film alone, and a video found in the folder still comes last. Sorted by when it was added, it is not among those the order knows nothing of.
func TestAddedVideosAreListedWhereTheyWereAdded(t *testing.T) {
	root, outside := filepath.Join(t.TempDir(), "lib.kando"), t.TempDir()
	newer, older, held := filepath.Join(outside, "新.mp4"), filepath.Join(outside, "Show.S01", "video.mp4"), filepath.Join(outside, "held.mp4")
	if err := os.MkdirAll(filepath.Dir(older), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(older[:len(older)-len(".mp4")]+".bilingual.srt", []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noon := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	jobs := []core.Job{
		{ID: "running", Stage: core.StageTranscribing, LibraryDir: root, Source: "https://example.com/a"},
		{ID: "done-late", Stage: core.StageComplete, LibraryDir: root, CompletedAt: noon.Add(2 * time.Hour), VideoPath: filepath.Join(root, "x", "video.mp4")},
		{ID: "held", Stage: core.StageComplete, LibraryDir: root, CompletedAt: noon.Add(-time.Hour), VideoPath: held, Source: core.FileURL(held)},
		{ID: "done-early", Stage: core.StageComplete, LibraryDir: root, VideoPath: filepath.Join(root, "y", "video.mp4")},
	}
	videos := AddedVideos(root, jobs, nil, map[string]time.Time{newer: noon.Add(time.Hour), older: noon.Add(-90 * time.Minute), held: noon})
	videos = append(videos, media.Video{Path: filepath.Join(root, "found.mp4"), Name: "found.mp4"})
	name := func(core.Job) string { return "" }
	items := List(root, jobs, videos, nil, name, name)
	var keys []string
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	want := []string{"running", "done-late", newer, "held", older, "done-early", filepath.Join(root, "found.mp4")}
	if len(keys) != len(want) {
		t.Fatalf("listed %q, want %q", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("listed %q, want %q", keys, want)
		}
	}
	if items[4].Name != "Show.S01" || items[4].SubtitlePath == "" {
		t.Fatalf("the video.mp4 added: %+v", items[4])
	}
	reversed := Sorted(items, Order{By: SortAdded, Reversed: true}, "en", nil, nil)
	if reversed[len(reversed)-1].Key != filepath.Join(root, "found.mp4") || reversed[0].Key != "done-early" {
		t.Fatalf("reversed %v", reversed)
	}
}

// A video added to one library stays on its shelf when another library has a job for the same video: that job is listed on the other library's shelf, so hiding the entry here would leave the video on neither shelf of this library.
func TestAddedVideoIsNotHiddenByAnotherLibrarysJob(t *testing.T) {
	root, other, outside := filepath.Join(t.TempDir(), "a.kando"), filepath.Join(t.TempDir(), "b.kando"), t.TempDir()
	video := filepath.Join(outside, "talk.mp4")
	jobs := []core.Job{{ID: "other", Stage: core.StageComplete, LibraryDir: other, VideoPath: video, Source: core.FileURL(video)}}
	added := map[string]time.Time{video: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	name := func(core.Job) string { return "" }

	items := List(root, jobs, AddedVideos(root, jobs, nil, added), nil, name, name)
	if len(items) != 1 || items[0].Key != video || items[0].JobID != "" {
		t.Fatalf("library a lists %+v, want the added video alone", items)
	}
	// The library the job was started in lists the job, once.
	items = List(other, jobs, AddedVideos(other, jobs, nil, added), nil, name, name)
	if len(items) != 1 || items[0].JobID != "other" {
		t.Fatalf("library b lists %+v, want its job alone", items)
	}
}
