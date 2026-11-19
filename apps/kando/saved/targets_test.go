package saved

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTargetWritesATemplatesColumns checks a pack's templates: a CSV file with its header row and each placeholder filled in, quoted where a field holds a comma, a quote or a line break; a TSV file without a header when the template names none; and Anki notes into the template's note type, the sound as Anki plays it.
func TestTargetWritesATemplatesColumns(t *testing.T) {
	rows := []Row{
		{Line: Line{Original: "Hello, \"you\".", Translation: "你好。", Note: "a\nb", Film: "Talk", Start: 2 * time.Second}, Sound: "kando-1.m4a"},
		{Line: Line{Original: "Bye.", Start: 3 * time.Second}},
	}
	csvTarget := Target{ID: "example.cards", Format: FormatCSV, Headers: []string{"Front", "Back"}, Columns: []string{"{{original}}", "{{translation}} ({{film}} {{time}}) {{sound}} {{unknown}}"}}
	var b strings.Builder
	if err := csvTarget.Write(&b, rows); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"Front", "Back"}, {`Hello, "you".`, "你好。 (Talk 0:02.0) kando-1.m4a {{unknown}}"}, {"Bye.", " ( 0:03.0)  {{unknown}}"}}
	if len(records) != len(want) || strings.Join(records[1], "|") != strings.Join(want[1], "|") || strings.Join(records[2], "|") != strings.Join(want[2], "|") || strings.Join(records[0], "|") != "Front|Back" {
		t.Fatalf("records %q", records)
	}
	if csvTarget.File() != "Kando Saved Lines (example.cards).csv" || !csvTarget.Sounds() {
		t.Fatalf("file %q, sounds %v", csvTarget.File(), csvTarget.Sounds())
	}

	b.Reset()
	tsv := Target{ID: "example.tsv", Format: FormatTSV, Columns: []string{"{{original}}", "{{note}}"}}
	if err := tsv.Write(&b, rows[:1]); err != nil {
		t.Fatal(err)
	}
	if b.String() != "\"Hello, \"\"you\"\".\"\t\"a\nb\"\n" || tsv.Sounds() {
		t.Fatalf("tsv %q, sounds %v", b.String(), tsv.Sounds())
	}

	b.Reset()
	anki := Target{ID: "example.anki", Format: FormatAnki, NoteType: "Cloze", Columns: []string{"{{original}}", "{{sound}}"}}
	if err := anki.Write(&b, rows[:1]); err != nil {
		t.Fatal(err)
	}
	if b.String() != "#separator:tab\n#html:false\n#notetype:Cloze\n\"Hello, \"\"you\"\".\"\t\"[sound:kando-1.m4a]\"\n" || anki.File() != "Kando Saved Lines (example.anki).txt" {
		t.Fatalf("anki %q, file %q", b.String(), anki.File())
	}
}

// TestExportWithATemplateCutsNoSoundItDoesNotUse checks that an export to a template without {{sound}} asks ffmpeg for nothing and writes the template's file.
func TestExportWithATemplateCutsNoSoundItDoesNotUse(t *testing.T) {
	lib := t.TempDir()
	ffmpeg := standInFFmpeg(t, t.TempDir())
	b, _ := Load(lib)
	b.Toggle(Line{Video: "Talk/video.mp4", Start: 2 * time.Second, Original: "Hello."})
	out := t.TempDir()
	target := Target{ID: "example.cards", Format: FormatCSV, Columns: []string{"{{original}}"}}
	got, err := Export(context.Background(), ffmpeg, b, func(string) time.Duration { return 0 }, out, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(out, target.File()))
	if got.Lines != 1 || got.Sounds != 0 || got.File != filepath.Join(out, target.File()) || string(data) != "Hello.\n" {
		t.Fatalf("exported %+v: %q", got, data)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(ffmpeg), "args")); err == nil {
		t.Fatal("ffmpeg was asked for a sound the template does not use")
	}
}
