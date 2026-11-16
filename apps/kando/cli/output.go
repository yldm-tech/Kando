package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"kando/apps/kando/remote"
	"kando/packages/core"
)

// writeJSON writes an answer of the app indented, "null" for an empty one.
func writeJSON(w io.Writer, raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	var b bytes.Buffer
	if err := json.Indent(&b, raw, "", "  "); err != nil {
		return err
	}
	b.WriteByte('\n')
	_, err := w.Write(b.Bytes())
	return err
}

// table writes rows of tab-separated cells as aligned columns, each cell made printable, so a line break in a title cannot break a row either.
func table(w io.Writer, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = printable(cell)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

// printable is s made safe to write to a terminal. Film titles, sources, the app's errors and subtitles come from websites and files Kando does not control, and printed raw a control character in them, such as the ESC that starts a terminal escape sequence, could retitle the window, rewrite the screen or worse. Every C0 control character but the tab, DEL, every C1 control character and every byte that is not UTF-8 becomes U+FFFD. --json needs none of this, as JSON escapes them.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || !isControl(r) {
			return r
		}
		return utf8.RuneError
	}, s)
}

// printableLines is printable for text whose line breaks are kept, as a subtitle's lines in an .srt are: \n stays and \r, which would only move the cursor back over the line, is dropped.
func printableLines(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t' || !isControl(r):
			return r
		case r == '\r':
			return -1
		}
		return utf8.RuneError
	}, s)
}

// isControl reports whether r is a C0 or C1 control character or DEL.
func isControl(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

// orDash is s, or "-" for an empty one, so a column never looks missing.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeStatus(w io.Writer, s remote.Status) error {
	ready := "no"
	if s.Ready {
		ready = "yes"
	}
	library := s.LibraryDir
	if library == "" {
		library = "none"
	}
	rows := [][]string{
		{"version", s.Version},
		{"library", library},
		{"ready", ready},
		{"readiness", orDash(s.Readiness)},
		{"speech model", orDash(s.SpeechModel)},
		{"translation model", orDash(s.TranslationModel)},
		{"language", orDash(s.Language)},
		{"running", orDash(s.Running)},
		{"queued", fmt.Sprint(s.Queued)},
	}
	names := make([]string, 0, len(s.Tools))
	for name := range s.Tools {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := s.Tools[name]
		if path == "" {
			path = "missing"
		}
		rows = append(rows, []string{name, path})
	}
	// Each library the app knows is a row of its own, the open one marked, so it shows which films the commands see and where the others are.
	for i, lib := range s.Libraries {
		key := ""
		if i == 0 {
			key = "libraries"
		}
		mark := " "
		if lib.Open {
			mark = "*"
		}
		films := fmt.Sprintf("%d films", lib.Films)
		switch {
		case lib.Missing:
			films = "missing"
		case lib.Films == 1:
			films = "1 film"
		}
		rows = append(rows, []string{key, mark + " " + lib.Name + "  " + films + "  " + lib.Path})
	}
	return table(w, rows)
}

// stageText is a job's stage as the command line shows it: with its progress while it works, and "canceled" for a job that was stopped.
func stageText(stage string, progress float64, canceled bool) string {
	if canceled {
		return "canceled"
	}
	switch core.Stage(stage) {
	case "":
		return "-"
	case core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting:
		if progress >= 0 && progress <= 1 {
			return fmt.Sprintf("%s %d%%", stage, int(progress*100))
		}
	}
	return stage
}

func writeFilms(w io.Writer, films []remote.Film) error {
	rows := [][]string{{"ID", "SOURCE", "STAGE", "TITLE"}}
	for _, f := range films {
		rows = append(rows, []string{f.ID, orDash(f.Source), stageText(f.Stage, f.Progress, false), f.Title})
	}
	return table(w, rows)
}

func writeFilm(w io.Writer, f remote.Film) error {
	rows := [][]string{
		{"id", f.ID},
		{"title", f.Title},
		{"source", orDash(f.Source)},
	}
	optional := [][2]string{
		{"link", f.Link},
		{"stage", stageTextIf(f.Stage, f.Progress)},
		{"status", f.Status},
		{"job", f.JobID},
		{"language", f.Language},
		{"video", f.VideoPath},
		{"subtitles", f.Subtitles},
		{"original subtitles", f.OriginalSubtitles},
		{"resume at", clockIf(f.ResumeMS)},
		{"watched to", clockIf(f.WatchedMS)},
	}
	for _, kv := range optional {
		if kv[1] != "" {
			rows = append(rows, []string{kv[0], kv[1]})
		}
	}
	return table(w, rows)
}

// clockIf is ms as remote.Clock writes it, or "" for 0, so the line is left out.
func clockIf(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return remote.Clock(ms)
}

// stageTextIf is stageText, or "" for no stage, so the line is left out.
func stageTextIf(stage string, progress float64) string {
	if stage == "" {
		return ""
	}
	return stageText(stage, progress, false)
}

// jobFields are a job's cells as `kando jobs` and `kando add --wait` show it: its id, its stage, its title or source, and why it failed.
func jobFields(j remote.Job) []string {
	name := j.Title
	if name == "" {
		name = j.Source
	}
	fields := []string{j.ID, stageText(j.Stage, j.Progress, j.Canceled), name}
	if j.Error != "" && !j.Canceled {
		fields = append(fields, j.Error)
	}
	return fields
}

func writeJobs(w io.Writer, jobs []remote.Job) error {
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, jobFields(j))
	}
	return table(w, rows)
}

func writeSites(w io.Writer, sites []remote.Site) error {
	rows := make([][]string, 0, len(sites))
	for _, s := range sites {
		rows = append(rows, []string{s.Name, s.Domain})
	}
	return table(w, rows)
}

// writeLines writes the lines `kando search` found, one a row: the film's id, when the line shows, the film's title, and the line as the text format of `kando subtitles` has it.
func writeLines(w io.Writer, found []remote.Line) error {
	rows := [][]string{{"FILM", "AT", "TITLE", "LINE"}}
	for _, l := range found {
		line := remote.CueLine(remote.Cue{Text: l.Text, Translation: l.Translation})
		rows = append(rows, []string{l.FilmID, remote.Clock(l.StartMS), l.Title, line})
	}
	return table(w, rows)
}
