package saved

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The stand-in for ffmpeg is this test binary under the name ffmpeg (standInFFmpeg), so it runs on Windows too, which starts no shell script. Started under that name, it adds its arguments as a line to the file args beside it and writes "sound" to the file its last argument names, as ffmpeg writes the clip there.
func init() {
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") != "ffmpeg" {
		return
	}
	args, err := os.OpenFile(filepath.Join(filepath.Dir(os.Args[0]), "args"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(1)
	}
	_, err = args.WriteString(strings.Join(os.Args[1:], " ") + "\n")
	if err = errors.Join(err, args.Close()); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("sound\n"), 0o644); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// standInFFmpeg puts the stand-in for ffmpeg in dir and returns its path: a link to this test binary named ffmpeg, ffmpeg.exe on Windows, which runs a file as a program only by that name and makes symbolic links only in developer mode, so there it is a hard link, or a copy where dir is on another volume.
func standInFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ffmpeg")
	if runtime.GOOS != "windows" {
		if err := os.Symlink(exe, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path += ".exe"
	if os.Link(exe, path) == nil {
		return path
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestWriteNotesIsAnAnkiTextFile writes notes whose lines hold quotes, tabs, line breaks and a leading #, and reads them back as Anki's text import does (rslib's csv reader: a tab between fields, " quoting with "" for a quote, # starting a comment line, and no column names, so the fields go into Basic's Front and Back in order): every note is one record of two fields, front the original and back the translation, the sound and where the line is, each as it was.
func TestWriteNotesIsAnAnkiTextFile(t *testing.T) {
	var b strings.Builder
	rows := []Row{
		{Line: Line{Original: "\"Every river\"\nbegins", Translation: "每条河\t始于", Film: "#Talk", Start: 72400 * time.Millisecond}, Sound: "kando-1.m4a"},
		{Line: Line{Original: "#1 \"fan", Film: "Talk", Start: time.Second}},
		{Line: Line{Original: "Bye.", Translation: "再见。\r\n", Start: 2 * time.Second}},
	}
	if err := WriteNotes(&b, rows); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	header := "#separator:tab\n#html:false\n#notetype:Basic\n"
	if !strings.HasPrefix(text, header) || strings.Contains(text, "#columns:") {
		t.Fatalf("the header of\n%s", text)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.Comment, r.FieldsPerRecord = '\t', '#', -1
	got, err := r.ReadAll()
	if err != nil {
		t.Fatalf("Anki's reader would not read\n%s\n%v", text, err)
	}
	want := [][]string{
		{"\"Every river\"\nbegins", "每条河\t始于\n[sound:kando-1.m4a]\n#Talk · 1:12.4"},
		{"#1 \"fan", "Talk · 0:01.0"},
		{"Bye.", "再见。\n0:02.0"},
	}
	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Fatalf("notes read back as\n%q\nwant\n%q", got, want)
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

// TestExportWritesNotesAndSounds exports through the stand-in for ffmpeg, which writes the file it is asked for: each line whose video is there gets its sound, a line whose video is gone is a note without one, and progress counts every line.
func TestExportWritesNotesAndSounds(t *testing.T) {
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "Talk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Talk", "video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffmpeg := standInFFmpeg(t, t.TempDir())
	args := filepath.Join(filepath.Dir(ffmpeg), "args")
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
	if !strings.Contains(string(notes), "\"Hello.\"\t\"你好。\n[sound:"+sound+"]\nTalk · 0:02.0\"\n") || !strings.Contains(string(notes), "\"Gone\"\t\"0:01.0\"\n") {
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
