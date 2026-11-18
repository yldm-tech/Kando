package saved

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kando/packages/study"
)

// Word is a word of the vocabulary, Stats what the study record says of the learner's study, and DayTime a day of its stats (study.Word, study.Stats, study.DayTime).
type (
	Word    = study.Word
	Stats   = study.Stats
	DayTime = study.DayTime
)

// The formats the vocabulary exports in: notes for Anki's text import, a CSV table, which spreadsheets open, and plain text with a tab between fields.
const (
	VocabAnki = "anki"
	VocabCSV  = "csv"
	VocabTSV  = "tsv"
)

// VocabOptions is what the export of the vocabulary writes: its Format, and with each word the line it was looked up in (Context) and that line's translation (Translation); Lines adds the saved lines after the words.
type VocabOptions struct {
	Format                      string
	Context, Translation, Lines bool
}

// VocabFile is the name of the file the export writes in format, in the folder the user picks.
func VocabFile(format string) string {
	switch format {
	case VocabCSV:
		return "Kando Vocabulary.csv"
	case VocabTSV:
		return "Kando Vocabulary.tsv"
	}
	return "Kando Vocabulary.txt"
}

// vocabColumns names the columns of a CSV or TSV export with o, in its first row. They are data, as a spreadsheet's other files' headers, not words of the interface.
func vocabColumns(o VocabOptions) []string {
	cols := []string{"Word", "Reading", "Meaning"}
	if o.Context {
		cols = append(cols, "Sentence")
	}
	if o.Translation {
		cols = append(cols, "Translation")
	}
	return append(cols, "Film")
}

// VocabRows is the records the export writes with o, in the columns vocabColumns names: each word, newest first, then with o.Lines each saved line, its original as the word and its translation, and its note, as the meaning.
func VocabRows(words []Word, lines []Line, o VocabOptions) [][]string {
	var rows [][]string
	add := func(front, reading, meaning, context, translation, film string) {
		row := []string{front, reading, meaning}
		if o.Context {
			row = append(row, context)
		}
		if o.Translation {
			row = append(row, translation)
		}
		rows = append(rows, append(row, film))
	}
	for _, w := range words {
		add(w.Text, w.Reading, w.Meaning, w.Context, w.ContextTranslation, w.Film)
	}
	if o.Lines {
		for _, l := range lines {
			meaning := strings.Join(nonEmpty(l.Translation, l.Note), "\n")
			add(l.Original, "", meaning, "", "", l.Film)
		}
	}
	return rows
}

// nonEmpty is those of parts that are not blank, trimmed.
func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// WriteVocab writes rows (VocabRows) in o.Format: for Anki, notes its text import reads into the Basic note type, the word on the front and the rest on the back, a line each, as WriteNotes writes the saved lines; as CSV, a table with a row of column names, UTF-8 with the byte order mark spreadsheets need to read it so; as TSV, the same with a tab between fields and none in them, the tabs and line breaks of a field written as spaces.
func WriteVocab(w io.Writer, rows [][]string, o VocabOptions) error {
	return writeVocab(w, rows, o, true)
}

// writeVocab is WriteVocab, without the lines that head the file unless head.
func writeVocab(w io.Writer, rows [][]string, o VocabOptions, head bool) error {
	switch o.Format {
	case VocabCSV:
		cw := csv.NewWriter(w)
		if head {
			if _, err := io.WriteString(w, "\ufeff"); err != nil {
				return err
			}
			if err := cw.Write(vocabColumns(o)); err != nil {
				return err
			}
		}
		if err := cw.WriteAll(rows); err != nil {
			return err
		}
		return cw.Error()
	case VocabTSV:
		bw := bufio.NewWriter(w)
		writeRow := func(row []string) {
			for i, f := range row {
				if i > 0 {
					bw.WriteByte('\t')
				}
				bw.WriteString(strings.Join(strings.Fields(f), " "))
			}
			bw.WriteByte('\n')
		}
		if head {
			writeRow(vocabColumns(o))
		}
		for _, row := range rows {
			writeRow(row)
		}
		return bw.Flush()
	}
	bw := bufio.NewWriter(w)
	if head {
		fmt.Fprint(bw, "#separator:tab\n#html:false\n#notetype:Basic\n")
	}
	for _, row := range rows {
		fmt.Fprint(bw, field(row[0]), "\t", field(strings.Join(nonEmpty(row[1:]...), "\n")), "\n")
	}
	return bw.Flush()
}

// VocabPreview is the first n records of rows as the export writes them in o.Format, without the lines that head the file.
func VocabPreview(rows [][]string, o VocabOptions, n int) string {
	var buf bytes.Buffer
	_ = writeVocab(&buf, rows[:min(n, len(rows))], o, false)
	return strings.TrimRight(buf.String(), "\n")
}

// ExportVocab writes rows (VocabRows) in o.Format into the folder dir, as VocabFile names the file, atomically, and returns the file's path.
func ExportVocab(dir string, rows [][]string, o VocabOptions) (string, error) {
	path := filepath.Join(dir, VocabFile(o.Format))
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	err = WriteVocab(f, rows, o)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}
