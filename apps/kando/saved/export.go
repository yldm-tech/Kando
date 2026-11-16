package saved

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kando/packages/media"
)

// The export's files, in the folder the user picks: the notes, which Anki's File › Import reads, and the folder of the lines' sound beside them, whose files go into Anki's collection.media folder.
const (
	ExportFile  = "Kando Saved Lines.txt"
	ExportAudio = "Kando Saved Lines audio"
)

// clipPad is how much of the film before and after a line its sound keeps, so the first and the last word are not cut off where the subtitles' times are a little early or late.
const clipPad = 250 * time.Millisecond

// clipNoEnd is how long the sound of a line that has no end runs.
const clipNoEnd = 4 * time.Second

// Exported is what an export wrote: how many lines, how many of them with their sound, and the notes' file.
type Exported struct {
	Lines, Sounds int
	File          string
}

// Export writes the lines of b into dir for Anki: ExportFile, a note per line, and in ExportAudio the sound of each line, cut out of its video with ffmpeg, which the line's note plays. delay is how far the subtitles of the video at a path were moved against its picture, so the sound is where the line is heard. A line whose video is gone, or every line when ffmpeg is "", is written without its sound. progress, when set, is told after each line. A stopped ctx stops it, leaving what it wrote.
func Export(ctx context.Context, ffmpeg string, b Book, delay func(video string) time.Duration, dir string, progress func(done, total int)) (Exported, error) {
	out := Exported{File: filepath.Join(dir, ExportFile)}
	audio := filepath.Join(dir, ExportAudio)
	if ffmpeg != "" && len(b.Lines) > 0 {
		if err := os.MkdirAll(audio, 0o755); err != nil {
			return out, err
		}
	}
	rows := make([]Row, len(b.Lines))
	for i, l := range b.Lines {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		rows[i] = Row{Line: l}
		video := b.Abs(l)
		if _, err := os.Stat(video); ffmpeg != "" && err == nil {
			from, to := ClipSpan(l, delay(video))
			name := SoundName(l)
			if err := CutClip(ctx, ffmpeg, video, from, to, filepath.Join(audio, name)); err == nil {
				rows[i].Sound = name
				out.Sounds++
			} else if ctx.Err() != nil {
				return out, ctx.Err()
			}
		}
		if progress != nil {
			progress(i+1, len(b.Lines))
		}
	}
	tmp := out.File + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return out, err
	}
	err = WriteNotes(f, rows)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, out.File)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return out, err
	}
	out.Lines = len(rows)
	return out, nil
}

// Row is a line as a note of the export: the line, and the file of its sound in ExportAudio, "" without one.
type Row struct {
	Line  Line
	Sound string
}

// WriteNotes writes rows as a file Anki's text import reads: UTF-8, a tab between fields, a note per line, and the header lines Anki 2.1.55 and later read, which say so and name the columns, so no option of the import needs changing. The fields are the original, the translation, the film, where in it the line starts and the line's sound, [sound:file] for Anki to play. Text is written as it is, not as HTML, and tabs or line breaks in it become spaces.
func WriteNotes(w io.Writer, rows []Row) error {
	bw := bufio.NewWriter(w)
	fmt.Fprint(bw, "#separator:tab\n#html:false\n#columns:Original\tTranslation\tFilm\tTime\tAudio\n")
	for _, r := range rows {
		sound := ""
		if r.Sound != "" {
			sound = "[sound:" + r.Sound + "]"
		}
		fields := []string{field(r.Line.Original), field(r.Line.Translation), field(r.Line.Film), Clock(r.Line.Start), sound}
		fmt.Fprint(bw, strings.Join(fields, "\t"), "\n")
	}
	return bw.Flush()
}

// field is s as one field of a note: on one line, without tabs, and not starting with the # Anki takes for a header line.
func field(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\t' || r == '\n' || r == '\r' }), " ")
	return strings.TrimPrefix(strings.TrimSpace(s), "#")
}

// SoundName is the file the sound of l is written to: kando- and a hash of its video and its start, so the lines of every export share Anki's one media folder without two of them taking one name, and exporting again writes each line's sound over its own.
func SoundName(l Line) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%d", l.Video, l.Start.Milliseconds())
	return fmt.Sprintf("kando-%016x.m4a", h.Sum64())
}

// ClipSpan is the stretch of the video l's sound is cut from, with the subtitles moved by delay: the line and clipPad either side of it, from no earlier than the video's start.
func ClipSpan(l Line, delay time.Duration) (from, to time.Duration) {
	end := l.End
	if end <= l.Start {
		end = l.Start + clipNoEnd
	}
	return max(l.Start+delay-clipPad, 0), max(end+delay+clipPad, 0)
}

// CutClip writes the sound of video from from to to into out, AAC in an .m4a, which Anki and the system's player play.
func CutClip(ctx context.Context, ffmpeg, video string, from, to time.Duration, out string) error {
	if to <= from {
		return errors.New("the clip is empty")
	}
	seconds := func(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }
	cmd := media.Command(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-ss", seconds(from), "-i", video, "-t", seconds(to-from), "-vn", "-ac", "1", "-c:a", "aac", "-b:a", "96k", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(out)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(msg)))
	}
	return nil
}
