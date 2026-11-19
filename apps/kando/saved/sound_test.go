package saved

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/study"
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
	got, err := Export(context.Background(), "", b, func(string) time.Duration { return 0 }, out, Target{}, nil)
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

// TestMoveSoundFollowsTheLine moves a line's kept sound from one library to another under the id the line has there, leaving none behind; a line without a kept sound moves nothing.
func TestMoveSoundFollowsTheLine(t *testing.T) {
	from, _ := Load(t.TempDir())
	to, _ := Load(t.TempDir())
	src := StoredSound(from, "aaaa1111")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MoveSound(from, "aaaa1111", to, "bbbb2222"); err != nil {
		t.Fatal(err)
	}
	if HasSound(src) || !HasSound(StoredSound(to, "bbbb2222")) {
		t.Fatal("the sound did not move with its line")
	}
	if err := MoveSound(from, "cccc3333", to, "dddd4444"); err != nil || HasSound(StoredSound(to, "dddd4444")) {
		t.Fatalf("a line without a sound moved one: %v", err)
	}
}

// keepFile writes a stand-in for a sound kept at path.
func keepFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSoundsFollowTheirRecords checks that the sounds of the lines and words another device took out go as the books merge, the others staying; and that the sweep as a library is read takes away the sounds no line or word has and the halves of stopped cuts, leaving what is being cut now, and touches nothing of a study record it could not read.
func TestSoundsFollowTheirRecords(t *testing.T) {
	lib := t.TempDir()
	b, _ := Load(lib)
	b.Device = "desk"
	b.Toggle(Line{Video: "Talk/video.mp4", Start: time.Second, End: 2 * time.Second, Original: "Stays", Film: "Talk"})
	b.Toggle(Line{Video: "Talk/video.mp4", Start: 3 * time.Second, End: 4 * time.Second, Original: "Goes", Film: "Talk"})
	b.AddWord(Word{Text: "stays", Spoken: "en", Video: "Talk/video.mp4", Start: time.Second})
	b.AddWord(Word{Text: "goes", Spoken: "en", Video: "Talk/video.mp4", Start: 3 * time.Second})
	stays, goes := b.Lines[b.Find("Talk/video.mp4", time.Second)].ID, b.Lines[b.Find("Talk/video.mp4", 3*time.Second)].ID
	wordStays, wordGoes := b.Words[b.WordIndex("stays", "en")].ID, b.Words[b.WordIndex("goes", "en")].ID
	for _, id := range []string{stays, goes, wordStays, wordGoes} {
		keepFile(t, StoredSound(b, id))
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	after := b
	after.Lines, after.Words = slices.Clone(b.Lines), slices.Clone(b.Words)
	at := study.Stamp{At: time.Now(), Device: "phone"}
	after.Merge(nil, []study.Removed{{ID: goes, At: at}})
	after.MergeWords(nil, []study.Removed{{ID: wordGoes, Video: "goes", At: at}})
	DropGoneSounds(b, after)
	for id, want := range map[string]bool{stays: true, goes: false, wordStays: true, wordGoes: false} {
		if got := HasSound(StoredSound(after, id)); got != want {
			t.Errorf("after the merge the sound of %s is kept: %v, want %v", id, got, want)
		}
	}

	// Left behind: a sound no record has, a cut stopped halfway, and a cut going on now with its half.
	orphan, cutting := StoredSound(after, "0ddba11"), StoredSound(after, "c0ffee")
	half := strings.TrimSuffix(StoredSound(after, stays), ".m4a") + ".cutting.m4a"
	cuttingHalf := strings.TrimSuffix(cutting, ".m4a") + ".cutting.m4a"
	for _, p := range []string{orphan, half, cuttingHalf} {
		keepFile(t, p)
	}
	if err := after.Save(); err != nil {
		t.Fatal(err)
	}
	read, err := Load(lib)
	if err != nil {
		t.Fatal(err)
	}
	if n := SweepSounds(read, func(id string) bool { return id == "c0ffee" }); n != 2 {
		t.Errorf("the sweep took away %d files, want 2", n)
	}
	for p, want := range map[string]bool{StoredSound(read, stays): true, StoredSound(read, wordStays): true, orphan: false, half: false, cuttingHalf: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("after the sweep %s is there: %v, want %v", filepath.Base(p), err == nil, want)
		}
	}

	// A study record that cannot be read names no lines: nothing is swept.
	keepFile(t, orphan)
	if err := os.WriteFile(filepath.Join(lib, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	unread, _ := Load(lib)
	if unread.Unread() == nil {
		t.Fatal("a broken study record was read")
	}
	if n := SweepSounds(unread, nil); n != 0 || !HasSound(StoredSound(read, stays)) || !HasSound(orphan) {
		t.Errorf("the sweep of an unread record took away %d files", n)
	}
}
