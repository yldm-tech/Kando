package saved

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKeepSoundWritesTheLinesSoundInTheLibrary keeps a line's sound through the stand-in for ffmpeg: it goes into the library's hidden folder under the line's id, cut from the line with its delay, and nothing is left beside it; a book without a folder or a record with an id that is not one keeps none.
func TestKeepSoundWritesTheLinesSoundInTheLibrary(t *testing.T) {
	lib := t.TempDir()
	ffmpeg := standInFFmpeg(t, t.TempDir())
	b, _ := Load(lib)
	l := Line{ID: "0123abcd", Video: "Talk/video.mp4", Start: 2 * time.Second, End: 3 * time.Second}
	out := StoredSound(b, l.ID)
	if out != filepath.Join(lib, SoundDir, "0123abcd.m4a") {
		t.Fatalf("StoredSound is %q", out)
	}
	if HasSound(out) {
		t.Fatal("a sound is kept before it was cut")
	}
	if err := KeepSound(context.Background(), ffmpeg, filepath.Join(lib, "Talk", "video.mp4"), l, time.Second, out); err != nil {
		t.Fatal(err)
	}
	if !HasSound(out) {
		t.Fatal("no sound kept")
	}
	entries, _ := os.ReadDir(filepath.Dir(out))
	if len(entries) != 1 {
		t.Fatalf("the folder holds %v", entries)
	}
	called, _ := os.ReadFile(filepath.Join(filepath.Dir(ffmpeg), "args"))
	if !strings.Contains(string(called), "-ss 2.750 -i "+filepath.Join(lib, "Talk", "video.mp4")+" -t 1.500 -vn") {
		t.Fatalf("ffmpeg was asked %q", called)
	}
	for _, id := range []string{"", "../x", "a.b"} {
		if p := StoredSound(b, id); p != "" {
			t.Fatalf("StoredSound(%q) is %q", id, p)
		}
	}
	if p := StoredSound(Book{}, "abc"); p != "" {
		t.Fatalf("a book without a folder keeps its sounds at %q", p)
	}
}

// TestExportTakesTheKeptSoundOfAGoneVideo exports a line whose video is gone but whose sound was kept when it was saved, without ffmpeg: its note plays the kept sound, copied into the export's folder.
func TestExportTakesTheKeptSoundOfAGoneVideo(t *testing.T) {
	lib := t.TempDir()
	b, _ := Load(lib)
	b.Toggle(Line{Video: "Gone/video.mp4", Start: time.Second, End: 2 * time.Second, Original: "Gone", Film: "Gone"})
	kept := StoredSound(b, b.Lines[0].ID)
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	got, err := Export(context.Background(), "", b, func(string) time.Duration { return 0 }, out, nil)
	if err != nil || got.Sounds != 1 {
		t.Fatalf("exported %+v, %v", got, err)
	}
	sound := SoundName(b.Lines[0])
	if data, err := os.ReadFile(filepath.Join(out, ExportAudio, sound)); err != nil || string(data) != "kept\n" {
		t.Fatalf("the sound: %q, %v", data, err)
	}
	notes, _ := os.ReadFile(got.File)
	if !strings.Contains(string(notes), "[sound:"+sound+"]") {
		t.Fatalf("notes %q", notes)
	}
}

// TestVocabSoundsGoOnTheAnkiNotes writes the sound of each word's line and saved line for the vocabulary's Anki export: a word whose video is there has its line cut, from its start to its end, a word looked up in no film has none, the saved lines' only with Lines, and the notes play each before the film; the CSV export is as it was.
func TestVocabSoundsGoOnTheAnkiNotes(t *testing.T) {
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "Talk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "Talk", "video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffmpeg := standInFFmpeg(t, t.TempDir())
	b, _ := Load(lib)
	b.AddWord(Word{Text: "nowhere", Meaning: "无处"})
	b.AddWord(Word{Text: "hello", Meaning: "你好", Context: "Hello.", Video: "Talk/video.mp4", Start: 2 * time.Second, End: 4 * time.Second, Film: "Talk"})
	b.Toggle(Line{Video: "Talk/video.mp4", Start: 6 * time.Second, End: 7 * time.Second, Original: "Bye.", Film: "Talk"})
	o := VocabOptions{Format: VocabAnki}
	dir := filepath.Join(t.TempDir(), VocabAudio)
	sounds, err := VocabSounds(context.Background(), ffmpeg, b, o, func(string) time.Duration { return 0 }, dir, nil)
	if err != nil || len(sounds) != 1 || sounds[b.Words[0].ID] != SoundName(WordLine(b.Words[0])) {
		t.Fatalf("sounds %v, %v", sounds, err)
	}
	called, _ := os.ReadFile(filepath.Join(filepath.Dir(ffmpeg), "args"))
	if !strings.Contains(string(called), "-ss 1.750 -i "+filepath.Join(lib, "Talk", "video.mp4")+" -t 2.500 -vn") {
		t.Fatalf("ffmpeg was asked %q", called)
	}
	rows := WithSounds(VocabRows(b.Words, b.Lines, o), b.Words, b.Lines, o, sounds)
	if strings.Join(rows[0], "|") != "hello||你好|[sound:"+sounds[b.Words[0].ID]+"]|Talk" || strings.Join(rows[1], "|") != "nowhere||无处|" {
		t.Fatalf("rows %q", rows)
	}

	o.Lines = true
	sounds, _ = VocabSounds(context.Background(), ffmpeg, b, o, func(string) time.Duration { return 0 }, dir, nil)
	rows = WithSounds(VocabRows(b.Words, b.Lines, o), b.Words, b.Lines, o, sounds)
	if len(sounds) != 2 || len(rows) != 3 || !strings.Contains(strings.Join(rows[2], "|"), "[sound:"+SoundName(b.Lines[0])+"]") {
		t.Fatalf("with the lines: sounds %v, rows %q", sounds, rows)
	}
	o.Format = VocabCSV
	if csv := WithSounds(VocabRows(b.Words, b.Lines, o), b.Words, b.Lines, o, sounds); strings.Contains(strings.Join(csv[0], "|"), "[sound:") {
		t.Fatalf("the CSV rows play sounds: %q", csv)
	}
}
