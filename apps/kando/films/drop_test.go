package films

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A dropped folder gives its own videos, then those of its folders however deep, each folder's in the order of their names with numbers by value, and counts the files that are not videos; hidden files and folders are left out. Dropped files keep the order they came in.
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
	d := Dropped([]string{filepath.Join(root, "loose.webm"), course, filepath.Join(root, "readme.txt"), filepath.Join(root, "gone.mp4")}, "en")
	videos, skipped := d.Videos, d.Skipped
	want := []string{
		filepath.Join(root, "loose.webm"),
		filepath.Join(course, "Episode 1.mov"), filepath.Join(course, "Episode 2.mkv"), filepath.Join(course, "Episode 10.mp4"),
		filepath.Join(course, "Section 2", "a.mp4"), filepath.Join(course, "Section 2", "Extras", "deep.mp4"), filepath.Join(course, "Section 10", "b.mp4"),
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
	if d := Dropped([]string{dir}, "zh-Hans"); len(d.Videos) != 0 || d.Skipped != 1 {
		videos, skipped := d.Videos, d.Skipped
		t.Fatalf("videos %q skipped %d", videos, skipped)
	}
}

// Sounds are dropped as videos are, in their names' order; a sound beside a video of its name is the video's, which is added once in its place, whichever of the two is dropped; a clip of Kando's own export is neither added nor counted.
func TestDroppedVideosTakeSound(t *testing.T) {
	root := t.TempDir()
	cd := filepath.Join(root, "N3 Listening")
	for _, name := range []string{"N3 Listening/Track 10.mp3", "N3 Listening/Track 2.m4a", "N3 Listening/booklet.pdf", "lesson.mp4", "lesson.mp3", "kando-0123456789abcdef.m4a"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d := Dropped([]string{cd, filepath.Join(root, "lesson.mp3"), filepath.Join(root, "lesson.mp4"), filepath.Join(root, "kando-0123456789abcdef.m4a")}, "en")
	videos, skipped := d.Videos, d.Skipped
	want := []string{filepath.Join(cd, "Track 2.m4a"), filepath.Join(cd, "Track 10.mp3"), filepath.Join(root, "lesson.mp4")}
	if !slices.Equal(videos, want) || skipped != 1 {
		t.Fatalf("videos %q skipped %d, want %q and the booklet", videos, skipped, want)
	}
}

// A drop leaves out programs named as videos are, though not a shortcut, naming them apart from the files that are not videos, and the folders a system, a NAS or another library keeps, though not a folder of the user's whose name starts as a NAS's do; it adds what lies past a link to a file, never follows one to a folder, and says how much the films take.
func TestDroppedLeavesOutProgramsAndSystemFolders(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"Show/第01话.mp4": "12345", "Show/第02话.mp4.exe": "x", "Show/第03话\u202e4pm.exe": "x", "Show/Bonus.mkv.app/inside.mp4": "x",
		"Show/$RECYCLE.BIN/old.mp4": "x", "Show/System Volume Information/v.mp4": "x", "Show/@eaDir/thumb.mp4": "x", "Show/Other.kando/film/video.mp4": "x",
		"elsewhere/target.mp4": "123", "elsewhere/deep/hidden.mp4": "x", "Show/#01 入門/第1課.mp4": "x", "Show/第06话.mp4.lnk": "x",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	show := filepath.Join(root, "Show")
	if err := os.Symlink(filepath.Join(root, "elsewhere", "target.mp4"), filepath.Join(show, "link.mp4")); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere", "deep"), filepath.Join(show, "linked folder")); err != nil {
		t.Fatal(err)
	}
	d := Dropped([]string{show}, "zh-Hans")
	if want := []string{filepath.Join(show, "link.mp4"), filepath.Join(show, "第01话.mp4"), filepath.Join(show, "#01 入門", "第1課.mp4")}; !slices.Equal(d.Videos, want) {
		t.Fatalf("videos %q, want %q", d.Videos, want)
	}
	// A shortcut is no program, so 第06话.mp4.lnk is only a file that is not a video.
	if len(d.Disguised) != 3 || d.Skipped != 1 {
		t.Fatalf("disguised %q, skipped %d", d.Disguised, d.Skipped)
	}
	if d.Size != 9 || d.Cut {
		t.Fatalf("size %d, cut %v", d.Size, d.Cut)
	}
}
