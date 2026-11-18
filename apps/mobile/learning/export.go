package learning

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Columns are the headings of the export's two tables, in the language the interface shows, so a table opened in a spreadsheet reads in it.
type Columns struct {
	// Word, Reading, Meaning, Line, Translation, Film and Note head the columns of the same names.
	Word, Reading, Meaning, Line, Translation, Film, Note string
}

// The files the export writes, in the folder it is given.
const (
	WordsFile = "Kando vocabulary.csv"
	LinesFile = "Kando saved lines.csv"
)

// Export writes items to dir as two CSV tables, the vocabulary and the saved lines, each with its headings, in UTF-8 with a byte-order mark so spreadsheets read the text as it is, and returns the files it wrote: none for a kind with no item.
func Export(dir string, items []Item, cols Columns) ([]string, error) {
	var words, lines [][]string
	for _, it := range items {
		if it.Kind == Word {
			words = append(words, []string{it.Text, it.Reading, it.Meaning, it.Context, it.ContextTranslation, it.Title})
		} else {
			lines = append(lines, []string{it.Text, it.Meaning, it.Note, it.Title})
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	var out []string
	write := func(name string, head []string, rows [][]string) error {
		if len(rows) == 0 {
			return nil
		}
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		err = writeCSV(f, head, rows)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		out = append(out, path)
		return nil
	}
	if err := write(WordsFile, []string{cols.Word, cols.Reading, cols.Meaning, cols.Line, cols.Translation, cols.Film}, words); err != nil {
		return nil, err
	}
	if err := write(LinesFile, []string{cols.Line, cols.Translation, cols.Note, cols.Film}, lines); err != nil {
		return nil, err
	}
	return out, nil
}

// writeCSV writes head and rows to w as CSV, after a byte-order mark; a line break in a cell stays in it, quoted.
func writeCSV(w io.Writer, head []string, rows [][]string) error {
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}
	c := csv.NewWriter(w)
	if err := c.Write(head); err != nil {
		return err
	}
	for _, r := range rows {
		for i := range r {
			r[i] = strings.TrimSpace(r[i])
		}
		if err := c.Write(r); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}
