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

func TestListVideosIncludesAFolderPerVideo(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "talk")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(folder, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "talk.bilingual.srt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	videos, err := ListVideos(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].Path != video {
		t.Fatalf("videos = %#v", videos)
	}
}

func TestGatherVideosMovesLooseFilesIntoAFolder(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	srt := filepath.Join(dir, "talk.srt")
	bilingual := filepath.Join(dir, "talk.bilingual.srt")
	note := filepath.Join(dir, "notes.txt")
	for _, path := range []string{video, srt, bilingual, note} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := GatherVideos(dir)
	if err != nil {
		t.Fatal(err)
	}
	if moved[video] != filepath.Join(dir, "talk", "video.mp4") || moved[bilingual] != filepath.Join(dir, "talk", "video.bilingual.srt") {
		t.Fatalf("moved = %#v", moved)
	}
	if _, err := os.Stat(filepath.Join(dir, "talk", "video.srt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(video); !os.IsNotExist(err) {
		t.Fatal("video stayed in the library root")
	}
}

func TestListVideosUsesTheFolderNameForAnItemVideo(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "Learn Rust [abc]")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "video.mp4"), []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "video.en.srt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	videos, err := ListVideos(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].Name != "Learn Rust [abc].mp4" {
		t.Fatalf("videos = %#v", videos)
	}
}

func TestAdoptLibraryMovesAFolderIntoTheBundle(t *testing.T) {
	from := filepath.Join(t.TempDir(), "Videos")
	to := filepath.Join(t.TempDir(), "影视库.kando")
	folder := filepath.Join(from, "talk [abc]")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(folder, "talk [abc].mp4")
	srt := filepath.Join(folder, "talk [abc].bilingual.srt")
	kept := filepath.Join(to, "already")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "video.mp4"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "library.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{video, srt} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := AdoptLibrary(from, to)
	if err != nil {
		t.Fatal(err)
	}
	wantVideo := filepath.Join(to, "talk [abc]", "video.mp4")
	wantSRT := filepath.Join(to, "talk [abc]", "video.bilingual.srt")
	if moved[video] != wantVideo || moved[srt] != wantSRT {
		t.Fatalf("moved = %#v", moved)
	}
	if _, err := os.Stat(filepath.Join(kept, "video.mp4")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(to, "library.json")); err != nil || string(data) != "{}" {
		t.Fatalf("library.json = %q %v", data, err)
	}
}

func TestDetachOwnVideosLeavesTheOtherAppsItems(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "影视库.vimpo")
	to := filepath.Join(root, "影视库.kando")
	own := filepath.Join(from, "talk")
	theirs := filepath.Join(from, "course")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(theirs, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(own, "video.mp4")
	if err := os.WriteFile(video, []byte("own"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(theirs, "item.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(theirs, "video.mp4"), []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "library.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := DetachOwnVideos(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if moved[video] != filepath.Join(to, "talk", "video.mp4") {
		t.Fatalf("moved = %#v", moved)
	}
	if _, err := os.Stat(filepath.Join(theirs, "video.mp4")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(from, "library.json")); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptLibraryRemembersAnItemThatIsAlreadyNamedVideo(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "Videos")
	to := filepath.Join(root, "影视库.kando")
	folder := filepath.Join(from, "talk")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(folder, "video.mp4")
	srt := filepath.Join(folder, "video.bilingual.srt")
	for _, path := range []string{video, srt} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := AdoptLibrary(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if moved[video] != filepath.Join(to, "talk", "video.mp4") || moved[srt] != filepath.Join(to, "talk", "video.bilingual.srt") {
		t.Fatalf("moved = %#v", moved)
	}
}

func TestRelocateFindsTheSameItemUnderTheBundle(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "talk")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "video.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := Relocate(root, filepath.Join(root, "gone", "talk", "video.mp4"))
	if !ok || got != filepath.Join(folder, "video.mp4") {
		t.Fatalf("relocate = %q %v", got, ok)
	}
	if _, ok := Relocate(root, filepath.Join(folder, "video.mp4")); ok {
		t.Fatal("an existing file was relocated")
	}
}

func TestWatchLibraryReportsANewFile(t *testing.T) {
	dir := t.TempDir()
	stop := make(chan struct{})
	defer close(stop)
	got := make(chan struct{}, 1)
	go WatchLibrary(dir, 20*time.Millisecond, stop, func(string) {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	time.Sleep(40 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "talk.mp4"), []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("watch did not notice the new video")
	}
}

func TestWatchLibraryFollowsARename(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "影视库.kando")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	got := make(chan string, 1)
	go WatchLibrary(dir, 20*time.Millisecond, stop, func(next string) {
		select {
		case got <- next:
		default:
		}
	})
	time.Sleep(40 * time.Millisecond)
	renamed := filepath.Join(parent, "missav.kando")
	if err := os.Rename(dir, renamed); err != nil {
		t.Fatal(err)
	}
	select {
	case next := <-got:
		if next != renamed {
			t.Fatalf("watch reported %q", next)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not notice the renamed library")
	}
}

func TestRetargetRenamedFolder(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "Talk [abc]")
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(old, "video.mp4")
	srt := filepath.Join(old, "video.bilingual.srt")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srt, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(root, "Renamed [abc]")
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	moved := RetargetRenamed(root, []string{video, srt}, nil)
	if moved[video] != filepath.Join(next, "video.mp4") || moved[srt] != filepath.Join(next, "video.bilingual.srt") {
		t.Fatalf("moved = %#v", moved)
	}
}

func TestRetargetRenamedSingleFolderWithoutAnID(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "talk")
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(old, "video.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(root, "renamed")
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	moved := RetargetRenamed(root, []string{video}, nil)
	if moved[video] != filepath.Join(next, "video.mp4") {
		t.Fatalf("moved = %#v", moved)
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
