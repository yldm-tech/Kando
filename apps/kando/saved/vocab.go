package saved

import (
	"context"
	"io"
	"slices"
	"time"

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

// VocabAudio is the folder beside the vocabulary's notes for Anki that holds the sound of the line each word was looked up in, and of each saved line, whose files go into Anki's collection.media folder.
const VocabAudio = "Kando Vocabulary audio"

// VocabSounds writes into the folder dir the sound of the line each of b's words was looked up in, and with o.Lines of each saved line, as the export writes them (soundFor): cut out of its video, or copied from the sound kept when it was saved. It returns the files' names by the word's or the line's id; one without a sound is not there. progress, when set, is told after each. A stopped ctx stops it.
func VocabSounds(ctx context.Context, ffmpeg string, b Book, o VocabOptions, delay func(video string) time.Duration, dir string, progress func(done, total int)) (map[string]string, error) {
	lines := make([]Line, 0, len(b.Words)+len(b.Lines))
	for _, w := range b.Words {
		if w.Video != "" {
			lines = append(lines, WordLine(w))
		}
	}
	if o.Lines {
		lines = append(lines, b.Lines...)
	}
	out := map[string]string{}
	for i, l := range lines {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		name, err := soundFor(ctx, ffmpeg, b, l, delay, dir)
		if err != nil {
			return out, err
		}
		if name != "" {
			out[l.ID] = name
		}
		if progress != nil {
			progress(i+1, len(lines))
		}
	}
	return out, nil
}

// WithSounds is rows, as VocabRows makes them of words and lines with o, with the sound of each word's line and each saved line on the back of its note for Anki, [sound:file] before the film, sounds naming the file by the word's or the line's id (VocabSounds). Rows of another format, and records without a sound, are as they were.
func WithSounds(rows [][]string, words []Word, lines []Line, o VocabOptions, sounds map[string]string) [][]string {
	if o.Format != VocabAnki || len(sounds) == 0 {
		return rows
	}
	ids := make([]string, 0, len(rows))
	for _, w := range words {
		ids = append(ids, w.ID)
	}
	if o.Lines {
		for _, l := range lines {
			ids = append(ids, l.ID)
		}
	}
	out := make([][]string, len(rows))
	for k, row := range rows {
		out[k] = row
		if k >= len(ids) || len(row) == 0 {
			continue
		}
		if name := sounds[ids[k]]; name != "" {
			out[k] = slices.Insert(slices.Clone(row), len(row)-1, "[sound:"+name+"]")
		}
	}
	return out
}
