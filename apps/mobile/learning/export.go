package learning

import (
	"os"

	"kando/packages/study"
)

// Export writes items into the folder dir, made if need be, as the computer's Export Vocabulary writes a library's (study.VocabRows, study.ExportVocab): in o.Format, the words, the latest saved first, each with the line it was looked up in and that line's translation as o asks, then with o.Lines the saved lines, their translation and note as the meaning. It returns the file's path, which study.VocabFile names.
func Export(dir string, items []Item, o study.VocabOptions) (string, error) {
	words, lines := ExportCounts(items, o)
	rows := study.VocabRows(words, lines, o)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return study.ExportVocab(dir, rows, o)
}

// ExportCounts are the words and the saved lines of items as the export writes them with o: no lines without o.Lines.
func ExportCounts(items []Item, o study.VocabOptions) (words []study.Word, lines []study.Line) {
	for _, it := range items {
		switch {
		case it.Kind == Word:
			words = append(words, study.Word{Text: it.Text, Reading: it.Reading, Meaning: it.Meaning, Context: it.Context, ContextTranslation: it.ContextTranslation, Film: it.Title})
		case o.Lines:
			lines = append(lines, study.Line{Original: it.Text, Translation: it.Meaning, Note: it.Note, Film: it.Title, Start: it.Start})
		}
	}
	return words, lines
}
