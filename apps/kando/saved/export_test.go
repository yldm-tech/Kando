package saved

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriteNotesIsAnAnkiTextFile(t *testing.T) {
	var b strings.Builder
	rows := []Row{
		{Line: Line{Original: "Every river\nbegins", Translation: "每条河\t始于", Film: "#Talk", Start: 72400 * time.Millisecond}, Sound: "kando-1.m4a"},
		{Line: Line{Original: "Bye.", Film: "Talk", Start: time.Second}},
	}
	if err := WriteNotes(&b, rows); err != nil {
		t.Fatal(err)
	}
	want := "#separator:tab\n#html:false\n#columns:Original\tTranslation\tFilm\tTime\tAudio\n" +
		"Every river begins\t每条河 始于\tTalk\t1:12.4\t[sound:kando-1.m4a]\n" +
		"Bye.\t\tTalk\t0:01.0\t\n"
	if b.String() != want {
		t.Fatalf("notes\n%q\nwant\n%q", b.String(), want)
	}
}

func TestSoundNamesTellLinesApart(t *testing.T) {
	a := SoundName(Line{Video: "Talk/video.mp4", Start: time.Second})
	if a != SoundName(Line{Video: "Talk/video.mp4", Start: time.Second, Original: "changed"}) {
		t.Fatal("a line's sound is named by more than its video and start")
	}
	if a == SoundName(Line{Video: "Talk/video.mp4", Start: 2 * time.Second}) || a == SoundName(Line{Video: "Other/video.mp4", Start: time.Second}) {
		t.Fatal("two lines share a sound's name")
	}
	if !strings.HasPrefix(a, "kando-") || filepath.Ext(a) != ".m4a" {
		t.Fatalf("sound named %q", a)
	}
}

func TestClipSpanPadsTheLine(t *testing.T) {
	from, to := ClipSpan(Line{Start: 2 * time.Second, End: 3 * time.Second}, 500*time.Millisecond)
	if from != 2250*time.Millisecond || to != 3750*time.Millisecond {
		t.Fatalf("span %v–%v", from, to)
	}
	if from, to := ClipSpan(Line{Start: 100 * time.Millisecond}, 0); from != 0 || to != 100*time.Millisecond+clipNoEnd+clipPad {
		t.Fatalf("a line at the start without an end: %v–%v", from, to)
	}
}

// TestExportWritesNotesAndSounds exports through a stand-in for ffmpeg that writes the file it is asked for: each line whose video is there gets its sound, a line whose video is gone is a note without one, and progress counts every line.
func TestExportWritesNotesAndSounds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for ffmpeg is a shell script")
	}
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "Talk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Talk", "video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	args := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\necho \"$@\" >> " + args + "\nfor last; do :; done\necho sound > \"$last\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := Load(lib)
	b.Toggle(Line{Video: "Gone/video.mp4", Start: time.Second, Original: "Gone"})
	b.Toggle(Line{Video: "Talk/video.mp4", Start: 2 * time.Second, End: 3 * time.Second, Original: "Hello.", Translation: "你好。", Film: "Talk"})
	out := t.TempDir()
	var counted []int
	got, err := Export(context.Background(), ffmpeg, b, func(video string) time.Duration {
		if video == filepath.Join(lib, "Talk", "video.mp4") {
			return time.Second
		}
		return 0
	}, out, func(done, total int) { counted = append(counted, done, total) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines != 2 || got.Sounds != 1 || got.File != filepath.Join(out, ExportFile) || len(counted) != 4 || counted[2] != 2 || counted[3] != 2 {
		t.Fatalf("exported %+v, progress %v", got, counted)
	}
	notes, _ := os.ReadFile(got.File)
	sound := SoundName(b.Lines[0])
	if !strings.Contains(string(notes), "Hello.\t你好。\tTalk\t0:02.0\t[sound:"+sound+"]\n") || !strings.Contains(string(notes), "Gone\t\t\t0:01.0\t\n") {
		t.Fatalf("notes %q", notes)
	}
	if data, err := os.ReadFile(filepath.Join(out, ExportAudio, sound)); err != nil || string(data) != "sound\n" {
		t.Fatalf("the sound: %q, %v", data, err)
	}
	called, _ := os.ReadFile(args)
	if !strings.Contains(string(called), "-ss 2.750 -i "+filepath.Join(lib, "Talk", "video.mp4")+" -t 1.500 -vn") {
		t.Fatalf("ffmpeg was asked %q", called)
	}
}
