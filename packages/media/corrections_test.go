package media

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/packages/core"
)

func TestCorrectionsPathIsTheFilmsWhicheverFileNamesIt(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, ".video.edits.json")
	for _, name := range []string{"video.mp4", "video.srt", "video.kando.srt", "video.bilingual.srt"} {
		if got := CorrectionsPath(filepath.Join(dir, name)); got != want {
			t.Fatalf("%s: %s, want %s", name, got, want)
		}
	}
}

func TestCorrectionsRoundTrip(t *testing.T) {
	path := CorrectionsPath(filepath.Join(t.TempDir(), "video.mp4"))
	if edits, err := LoadCorrections(path); err != nil || edits != nil {
		t.Fatalf("no file: %v %v", edits, err)
	}
	text := "再见。"
	edits := []core.Correction{
		{Span: core.Span{Start: time.Second, End: 2500 * time.Millisecond}, Translation: &text, StartShift: -100 * time.Millisecond},
		{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, NoSpeech: true},
		{Span: core.Span{Start: 6 * time.Second, End: 7 * time.Second}},
	}
	if err := SaveCorrections(path, edits); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCorrections(path)
	if err != nil || len(got) != 2 || *got[0].Translation != text || got[0].StartShift != -100*time.Millisecond || got[0].Span != edits[0].Span || !got[1].NoSpeech {
		t.Fatalf("read back %+v, %v", got, err)
	}
	// Saving none keeps a file holding none, whose time says when the corrections last changed; with no file before, it writes none.
	if err := SaveCorrections(path, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCorrections(path); err != nil || len(got) != 0 {
		t.Fatalf("after saving none, read back %+v, %v", got, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saving none removed the file: %v", err)
	}
	other := CorrectionsPath(filepath.Join(filepath.Dir(path), "other.mp4"))
	if err := SaveCorrections(other, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("saving none for a film never corrected wrote a file: %v", err)
	}
}

func TestSaveCorrectionsKeepsANewerKandosFile(t *testing.T) {
	path := CorrectionsPath(filepath.Join(t.TempDir(), "video.mp4"))
	newer := []byte(`{"version":2,"lines":[{"startMs":1000,"endMs":2000,"noSpeech":true,"speaker":"A"}]}`)
	if err := os.WriteFile(path, newer, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCorrections(path); err != nil || len(got) != 1 || !got[0].NoSpeech {
		t.Fatalf("read %+v, %v", got, err)
	}
	if err := SaveCorrections(path, nil); !errors.Is(err, ErrCorrectionsNewer) {
		t.Fatalf("saved over a newer file: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != string(newer) {
		t.Fatalf("the file changed: %s", data)
	}
}

// TestReadSubtitlesLaysTheCorrectionsOver checks that a reader of the subtitles gets the user's corrections, and that writing the subtitles again, as a redo does, keeps them.
func TestReadSubtitlesLaysTheCorrectionsOver(t *testing.T) {
	dir := t.TempDir()
	srt := filepath.Join(dir, "video.bilingual.srt")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(srt, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n(music)\n\n")
	original := "Hello!"
	if err := SaveCorrections(CorrectionsPath(srt), []core.Correction{
		{Span: core.Span{Start: time.Second, End: 2 * time.Second}, Original: &original},
		{Span: core.Span{Start: 4 * time.Second, End: 5 * time.Second}, NoSpeech: true},
	}); err != nil {
		t.Fatal(err)
	}
	read := func() []core.Segment {
		t.Helper()
		cues, err := ReadSubtitles(srt)
		if err != nil {
			t.Fatal(err)
		}
		return cues
	}
	if got := read(); len(got) != 1 || got[0].Text != "你好。\nHello!" {
		t.Fatalf("corrected %+v", got)
	}
	// A translation again writes new lines, the first a little later: the correction follows it.
	write("1\n00:00:01,100 --> 00:00:02,000\n嗨。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n（音乐）\n(music)\n\n")
	if got := read(); len(got) != 1 || got[0].Text != "嗨。\nHello!" || got[0].Start != 1100*time.Millisecond {
		t.Fatalf("after a redo %+v", got)
	}
}

func TestCompanionsHoldTheCorrections(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	path := CorrectionsPath(video)
	for _, p := range []string{video, path} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := Companions(video); !slices.Contains(got, path) {
		t.Fatalf("companions %v", got)
	}
}

func TestGatherVideosTakesTheCorrectionsAlong(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Talk.mp4", "Talk.bilingual.srt", ".Talk.edits.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := GatherVideos(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Talk", ".video.edits.json")); err != nil {
		t.Fatalf("the corrections did not move with the video: %v", err)
	}
}
