package saved

import (
	"bytes"
	"encoding/csv"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	vocabWords = []Word{
		{Text: "trickle", Reading: "/ˈtrɪk(ə)l/", Meaning: "n. 细流", Context: "Every river begins as a trickle.", ContextTranslation: "每条河都始于一股细流。", Film: "Rivers"},
		{Text: "say \"hi\"", Meaning: "打招呼\t问好", Context: "Say \"hi\"\nto them.", Film: "Talk"},
	}
	vocabLines = []Line{{Original: "Picks up speed.", Translation: "加速。", Note: "picks up = 裹挟", Film: "Rivers", Start: time.Second}}
)

// TestVocabRowsFollowTheOptions checks the records of the export: a word's fields, the sentence and its translation only when asked for, and the saved lines after the words only when asked for, their translation and note as the meaning.
func TestVocabRowsFollowTheOptions(t *testing.T) {
	rows := VocabRows(vocabWords, vocabLines, VocabOptions{})
	if len(rows) != 2 || strings.Join(rows[0], "|") != "trickle|/ˈtrɪk(ə)l/|n. 细流|Rivers" {
		t.Fatalf("rows %q", rows)
	}
	rows = VocabRows(vocabWords, vocabLines, VocabOptions{Context: true, Translation: true, Lines: true})
	if len(rows) != 3 || strings.Join(rows[0], "|") != "trickle|/ˈtrɪk(ə)l/|n. 细流|Every river begins as a trickle.|每条河都始于一股细流。|Rivers" {
		t.Fatalf("rows %q", rows)
	}
	if strings.Join(rows[2], "|") != "Picks up speed.||加速。\npicks up = 裹挟|||Rivers" {
		t.Fatalf("the line's row %q", rows[2])
	}
}

// TestWriteVocabInEachFormat checks the files: CSV with the byte order mark and a header that a CSV reader reads back field for field, quotes and line breaks kept; TSV with one record a line, tabs and line breaks in a field made spaces; and Anki's notes with its header, the word on the front and the rest of the record on the back.
func TestWriteVocabInEachFormat(t *testing.T) {
	o := VocabOptions{Context: true}
	rows := VocabRows(vocabWords, nil, o)

	var buf bytes.Buffer
	o.Format = VocabCSV
	if err := WriteVocab(&buf, rows, o); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "\ufeff") {
		t.Fatal("no byte order mark")
	}
	read, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(buf.String(), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 3 || strings.Join(read[0], ",") != "Word,Reading,Meaning,Sentence,Film" || read[2][0] != `say "hi"` || read[2][3] != "Say \"hi\"\nto them." {
		t.Fatalf("CSV read back as %q", read)
	}

	buf.Reset()
	o.Format = VocabTSV
	if err := WriteVocab(&buf, rows, o); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 || lines[0] != "Word\tReading\tMeaning\tSentence\tFilm" || lines[2] != "say \"hi\"\t\t打招呼 问好\tSay \"hi\" to them.\tTalk" {
		t.Fatalf("TSV %q", lines)
	}

	buf.Reset()
	o.Format = VocabAnki
	if err := WriteVocab(&buf, rows, o); err != nil {
		t.Fatal(err)
	}
	want := "#separator:tab\n#html:false\n#notetype:Basic\n" +
		"\"trickle\"\t\"/ˈtrɪk(ə)l/\nn. 细流\nEvery river begins as a trickle.\nRivers\"\n" +
		"\"say \"\"hi\"\"\"\t\"打招呼\t问好\nSay \"\"hi\"\"\nto them.\nTalk\"\n"
	if buf.String() != want {
		t.Fatalf("Anki notes\n%s\nwant\n%s", buf.String(), want)
	}
}

// TestVocabPreviewAndExport checks the preview, the first records without the file's header, and that the export writes its file in the folder, named by its format.
func TestVocabPreviewAndExport(t *testing.T) {
	o := VocabOptions{Format: VocabTSV}
	rows := VocabRows(vocabWords, vocabLines, VocabOptions{Lines: true})
	if p := VocabPreview(rows, o, 1); p != "trickle\t/ˈtrɪk(ə)l/\tn. 细流\tRivers" {
		t.Fatalf("TSV preview %q", p)
	}
	if p := VocabPreview(rows, VocabOptions{Format: VocabAnki}, 1); p != "\"trickle\"\t\"/ˈtrɪk(ə)l/\nn. 细流\nRivers\"" {
		t.Fatalf("Anki preview %q", p)
	}
	if p := VocabPreview(nil, o, 3); p != "" {
		t.Fatalf("preview of nothing %q", p)
	}
	dir := t.TempDir()
	path, err := ExportVocab(dir, rows, o)
	if err != nil || !strings.HasSuffix(path, "Kando Vocabulary.tsv") {
		t.Fatalf("ExportVocab = %q, %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "\n") != 4 {
		t.Fatalf("wrote %q, %v", data, err)
	}
	if VocabFile(VocabCSV) != "Kando Vocabulary.csv" || VocabFile(VocabAnki) != "Kando Vocabulary.txt" {
		t.Fatal("VocabFile")
	}
}
