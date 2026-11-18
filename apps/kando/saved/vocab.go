package saved

import (
	"io"

	"kando/packages/study"
)

// Word is a word of the vocabulary, Stats what the study record says of the learner's study, and DayTime a day of its stats (study.Word, study.Stats, study.DayTime).
type (
	Word    = study.Word
	Stats   = study.Stats
	DayTime = study.DayTime
)

// VocabOptions is what the export of the vocabulary writes (study.VocabOptions), which the phone's export writes too.
type VocabOptions = study.VocabOptions

// The formats the vocabulary exports in (study.VocabAnki, study.VocabCSV, study.VocabTSV).
const (
	VocabAnki = study.VocabAnki
	VocabCSV  = study.VocabCSV
	VocabTSV  = study.VocabTSV
)

// VocabFile is the name of the file the export writes in format (study.VocabFile).
func VocabFile(format string) string { return study.VocabFile(format) }

// VocabRows is the records the export writes with o (study.VocabRows).
func VocabRows(words []Word, lines []Line, o VocabOptions) [][]string {
	return study.VocabRows(words, lines, o)
}

// WriteVocab writes rows in o.Format (study.WriteVocab).
func WriteVocab(w io.Writer, rows [][]string, o VocabOptions) error {
	return study.WriteVocab(w, rows, o)
}

// VocabPreview is the first n records of rows as the export writes them (study.VocabPreview).
func VocabPreview(rows [][]string, o VocabOptions, n int) string {
	return study.VocabPreview(rows, o, n)
}

// ExportVocab writes rows in o.Format into the folder dir and returns the file's path (study.ExportVocab).
func ExportVocab(dir string, rows [][]string, o VocabOptions) (string, error) {
	return study.ExportVocab(dir, rows, o)
}
