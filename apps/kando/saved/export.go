package saved

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kando/packages/media"
	"kando/packages/study"
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

// Export writes the lines of b into dir for Anki: ExportFile, a note per line, and in ExportAudio the sound of each line, cut out of its video with ffmpeg, which the line's note plays. delay is how far the subtitles of the video at a path were moved against its picture, so the sound is where the line is heard. A line whose video is gone, or every line when ffmpeg is "", takes the sound kept when it was saved (StoredSound), and one that has none either is written without its sound, which the log says when ffmpeg was at hand; Exported.Sounds counts the lines with one. progress, when set, is told after each line. A stopped ctx stops it, leaving what it wrote.
func Export(ctx context.Context, ffmpeg string, b Book, delay func(video string) time.Duration, dir string, progress func(done, total int)) (Exported, error) {
	out := Exported{File: filepath.Join(dir, ExportFile)}
	audio := filepath.Join(dir, ExportAudio)
	rows := make([]Row, len(b.Lines))
	for i, l := range b.Lines {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		rows[i] = Row{Line: l}
		name, err := soundFor(ctx, ffmpeg, b, l, delay, audio)
		if err != nil {
			return out, err
		}
		if name != "" {
			rows[i].Sound = name
			out.Sounds++
		} else if ffmpeg != "" {
			log.Printf("saved: no sound for the line at %s of %s", Clock(l.Start), filepath.Base(l.Video))
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

// WriteNotes writes rows as a file Anki's text import (File › Import) reads into its stock Basic note type as it is: UTF-8, a tab between fields, a note per line, and the header lines Anki 2.1.55 and later read. Every field is in double quotes, any quote in it doubled, as Anki's reader takes them, so tabs, line breaks, quotes and a leading # stay in the field rather than split it, run it into the next notes or make it a comment. The header names no columns, so Anki puts the fields into the note type's fields in order, whatever Anki's language calls them: the first, Front, is the original; the second, Back, is the translation, the learner's note on the line, the line's sound, [sound:file] for Anki to play, and the film and where in it the line starts, each on a line of its own. #notetype picks the note type named Basic where the collection has one; Anki keeps the one it adds with where it has none, as in a collection made in another language. The text is not HTML: Anki escapes it and turns its line breaks into line breaks of the card.
func WriteNotes(w io.Writer, rows []Row) error {
	bw := bufio.NewWriter(w)
	fmt.Fprint(bw, "#separator:tab\n#html:false\n#notetype:Basic\n")
	for _, r := range rows {
		var back []string
		if t := strings.TrimSpace(r.Line.Translation); t != "" {
			back = append(back, t)
		}
		if n := strings.TrimSpace(r.Line.Note); n != "" {
			back = append(back, n)
		}
		if r.Sound != "" {
			back = append(back, "[sound:"+r.Sound+"]")
		}
		where := Clock(r.Line.Start)
		if f := strings.TrimSpace(r.Line.Film); f != "" {
			where = f + " · " + where
		}
		back = append(back, where)
		fmt.Fprint(bw, study.AnkiField(r.Line.Original), "\t", study.AnkiField(strings.Join(back, "\n")), "\n")
	}
	return bw.Flush()
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
