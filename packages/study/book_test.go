package study

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// TestLoadReadsLaterFiles checks that fields a later Kando writes in the same version of the file are passed over, and a line without a video or an id is dropped.
func TestLoadReadsLaterFiles(t *testing.T) {
	dir := t.TempDir()
	data := `{"version": 1, "lines": [{"id": "x", "video": "a/video.mp4", "start": 1.5, "original": "Hi", "ease": 2.5}, {"id": "y", "start": 2}, {"video": "a/video.mp4", "start": 3}]}`
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

// TestKeyWritesRelativePathsWithSlashes checks that a relative path, as a book with no library folder is given, is keyed with forward slashes whatever the system's separator, so it names the same line as the key saved on another device and as Abs reads it.
func TestKeyWritesRelativePathsWithSlashes(t *testing.T) {
	var b Book
	native := filepath.Join("Talk", "video.mp4")
	if key := b.Key(native); key != "Talk/video.mp4" || b.Abs(Line{Video: key}) != native {
		t.Fatalf("Key(%q) = %q, Abs %q", native, key, b.Abs(Line{Video: key}))
	}
	if key := b.Key("Talk/video.mp4"); key != "Talk/video.mp4" {
		t.Fatalf("Key of a slash path = %q", key)
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

// TestFollowSubtitlesCompletesAndCorrectsLines keeps the lines of one video as its subtitles say them now: a line saved without a translation takes the one its subtitle line has, to the millisecond the book keeps; a line whose original was proofread since takes the corrected original and keeps its translation; a line with a translation keeps its own; one whose subtitle line now says something else, or has no translation yet, stays as it was; and the lines of another video are left alone.
func TestFollowSubtitlesCompletesAndCorrectsLines(t *testing.T) {
	b := Book{Lines: []Line{
		{Video: "Talk/video.mp4", Start: time.Second, Original: "Hello."},
		{Video: "Talk/video.mp4", Start: 2 * time.Second, Original: "Kept.", Translation: "保留。"},
		{Video: "Talk/video.mp4", Start: 3 * time.Second, Original: "Changed."},
		{Video: "Talk/video.mp4", Start: 4 * time.Second, Original: "Later."},
		{Video: "Talk/video.mp4", Start: 5 * time.Second, Original: "今天讲神经网罗", Translation: "Today: neural networks"},
		{Video: "Other/video.mp4", Start: time.Second, Original: "Hello."},
	}}
	subs := map[time.Duration][2]string{
		time.Second + 400*time.Microsecond: {"你好。", "Hello."},
		2 * time.Second:                    {"新的。", "Kept."},
		3 * time.Second:                    {"别的。", "Something else."},
		4 * time.Second:                    {"", "Later."},
		5 * time.Second:                    {"Today: neural networks", "今天讲神经网络"},
	}
	at := func(start time.Duration) (string, string, bool) {
		for s, l := range subs {
			if SameStart(s, start) {
				return l[0], l[1], true
			}
		}
		return "", "", false
	}
	proofreads := func(heard, now string) bool {
		return heard == "今天讲神经网罗" && now == "今天讲神经网络"
	}
	if !b.FollowSubtitles("Talk/video.mp4", at, proofreads) {
		t.Fatal("no line changed")
	}
	var got []string
	for _, l := range b.Lines {
		got = append(got, l.Original+"|"+l.Translation)
	}
	want := []string{"Hello.|你好。", "Kept.|保留。", "Changed.|", "Later.|", "今天讲神经网络|Today: neural networks", "Hello.|"}
	if !slices.Equal(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	if b.FollowSubtitles("Talk/video.mp4", at, proofreads) {
		t.Fatal("following again changed a line")
	}
}

// TestTakeAndAddVideoMoveLinesBetweenLibraries checks that a video's lines leave one library's book and join another's, keyed by the video's path there, among its lines by when each was saved, with their schedule, while a line the other book has already stays as it was; and that a book whose file could not be read says so.
func TestTakeAndAddVideoMoveLinesBetweenLibraries(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	src, _ := Load(from)
	dst, _ := Load(to)
	day := func(d int) time.Time { return time.Date(2026, 11, d, 9, 0, 0, 0, time.UTC) }
	old := filepath.Join(from, "Talk", "video.mp4")
	next := filepath.Join(to, "Talk 2", "video.mp4")
	src.Lines = []Line{
		{Video: "Talk/video.mp4", Start: 3 * time.Second, Original: "three", SavedAt: day(5), Box: 2, Due: day(9)},
		{Video: "Other/video.mp4", Start: time.Second, Original: "other", SavedAt: day(4)},
		{Video: "Talk/video.mp4", Start: time.Second, Original: "one", SavedAt: day(2)},
	}
	dst.Lines = []Line{
		{Video: "Mine/video.mp4", Start: time.Second, Original: "mine", SavedAt: day(6)},
		{Video: "Talk 2/video.mp4", Start: time.Second, Original: "kept", SavedAt: day(3)},
		{Video: "Mine/video.mp4", Start: 2 * time.Second, Original: "older", SavedAt: day(1)},
	}

	taken := src.TakeVideo(old)
	if len(taken) != 2 || len(src.Lines) != 1 || src.Lines[0].Original != "other" {
		t.Fatalf("took %+v, left %+v", taken, src.Lines)
	}
	dst.AddVideo(next, taken)
	var got []string
	for _, l := range dst.Lines {
		got = append(got, l.Video+" "+l.Original)
	}
	want := []string{"Mine/video.mp4 mine", "Talk 2/video.mp4 three", "Talk 2/video.mp4 kept", "Mine/video.mp4 older"}
	if !slices.Equal(got, want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	if l := dst.Lines[1]; l.Box != 2 || !l.Due.Equal(day(9)) {
		t.Fatalf("the moved line lost its schedule: %+v", l)
	}

	if err := os.WriteFile(filepath.Join(to, FileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken, err := Load(to)
	if err == nil || broken.Unread() == nil {
		t.Fatalf("Load of a broken file = %v, Unread %v", err, broken.Unread())
	}
	if src.Unread() != nil {
		t.Fatalf("a book read whole is unread: %v", src.Unread())
	}
}
