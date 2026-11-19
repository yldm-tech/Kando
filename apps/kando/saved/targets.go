package saved

import (
	"bufio"
	"cmp"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"

	"kando/packages/study"
)

// The formats an export target writes: Anki's text import, comma-separated values, or tab-separated values.
const (
	FormatAnki = "anki"
	FormatCSV  = "csv"
	FormatTSV  = "tsv"
)

// Placeholders are what a column of an export template may hold, each written {{name}}: the line's original and translation, the learner's note on it, the film's name, where in it the line starts (m:ss), and its sound, [sound:file] for Anki to play or the file's name in ExportAudio for CSV and TSV. Nothing else of a line reaches a template, no path of the computer among it. The plugins package keeps the same list to check a pack's templates against (plugins.TemplatePlaceholders; a test of the app's checks the two agree).
var Placeholders = []string{"original", "translation", "note", "film", "time", "sound"}

// Target is where Export writes the saved lines: Anki's notes as Kando writes them (the zero Target, WriteNotes), or the template of an export-template pack. ID names the pack, "" for Kando's own. Format is one of the Format constants; Columns are the template of each field of a note or row, in order, with Headers, the CSV or TSV file's first row when any is set; NoteType is the Anki note type the notes go into, Basic when "".
type Target struct {
	ID       string
	Format   string
	Headers  []string
	Columns  []string
	NoteType string
}

// File is the name of the file the target writes in the folder picked: ExportFile for Kando's own notes, and for a pack's, the same name with the pack's id, which is plain ASCII, and the format's extension.
func (t Target) File() string {
	if t.ID == "" {
		return ExportFile
	}
	ext := ".txt"
	switch t.Format {
	case FormatCSV:
		ext = ".csv"
	case FormatTSV:
		ext = ".tsv"
	}
	return strings.TrimSuffix(ExportFile, ".txt") + " (" + t.ID + ")" + ext
}

// Sounds reports whether the target writes the lines' sound: Kando's own notes always do, a template only when a column has {{sound}}, so an export that does not use it cuts no clip.
func (t Target) Sounds() bool {
	return t.ID == "" || slices.ContainsFunc(t.Columns, func(c string) bool { return strings.Contains(c, "{{sound}}") })
}

// Write writes rows as the target says: Kando's own notes (WriteNotes); a template's Anki notes, a field a column, in a file Anki's text import reads into the template's note type in order; or a CSV or TSV file, its header row first when the template has one, quoted as encoding/csv quotes, so a comma, a tab, a quote or a line break stays in its field.
func (t Target) Write(w io.Writer, rows []Row) error {
	if t.ID == "" {
		return WriteNotes(w, rows)
	}
	bw := bufio.NewWriter(w)
	if t.Format == FormatAnki {
		fmt.Fprintf(bw, "#separator:tab\n#html:false\n#notetype:%s\n", cmp.Or(strings.TrimSpace(t.NoteType), "Basic"))
		for _, r := range rows {
			fields := make([]string, len(t.Columns))
			for i, c := range t.Columns {
				fields[i] = study.AnkiField(Expand(c, r, FormatAnki))
			}
			fmt.Fprint(bw, strings.Join(fields, "\t"), "\n")
		}
		return bw.Flush()
	}
	cw := csv.NewWriter(bw)
	if t.Format == FormatTSV {
		cw.Comma = '\t'
	}
	if slices.ContainsFunc(t.Headers, func(h string) bool { return h != "" }) {
		header := make([]string, len(t.Columns))
		copy(header, t.Headers)
		if err := cw.Write(header); err != nil {
			return err
		}
	}
	for _, r := range rows {
		record := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			record[i] = Expand(c, r, t.Format)
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	return bw.Flush()
}

// Expand is the template column with each of Placeholders filled in from r for format: {{sound}} is [sound:file] for Anki and the file's name otherwise, "" for a line without one. A placeholder not in Placeholders stays as it is; the plugins package refuses a template that has one.
func Expand(column string, r Row, format string) string {
	sound := r.Sound
	if sound != "" && format == FormatAnki {
		sound = "[sound:" + sound + "]"
	}
	return strings.NewReplacer(
		"{{original}}", strings.TrimSpace(r.Line.Original),
		"{{translation}}", strings.TrimSpace(r.Line.Translation),
		"{{note}}", strings.TrimSpace(r.Line.Note),
		"{{film}}", strings.TrimSpace(r.Line.Film),
		"{{time}}", Clock(r.Line.Start),
		"{{sound}}", sound,
	).Replace(column)
}
