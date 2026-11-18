package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kando/packages/core"
)

type whisperOutput struct {
	// Result.Language is the language whisper heard, or the one -l told it to hear.
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// parseWhisperJSON reads the file whisper-cli writes with -oj, whose offsets are milliseconds: the lines it heard and the language it heard them in, as its code, "" when the file does not say.
func parseWhisperJSON(data []byte) ([]core.Segment, string, error) {
	var out whisperOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, "", fmt.Errorf("decode whisper output: %w", err)
	}
	segments := make([]core.Segment, 0, len(out.Transcription))
	for _, t := range out.Transcription {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		segments = append(segments, core.Segment{
			Start: time.Duration(t.Offsets.From) * time.Millisecond,
			End:   time.Duration(t.Offsets.To) * time.Millisecond,
			Text:  text,
		})
	}
	return segments, strings.ToLower(strings.TrimSpace(out.Result.Language)), nil
}

// exportSubtitles writes the original speech to the video's transcript, the file ctx names (withTranscript) or else TranscriptOf, and "<video>.bilingual.srt" with each translation above its original line, or the original alone for a line left untranslated, returning the bilingual path.
func exportSubtitles(ctx context.Context, videoPath string, original, translated []core.Segment) (string, error) {
	if len(original) != len(translated) {
		return "", fmt.Errorf("%d translated lines for %d original lines", len(translated), len(original))
	}
	transcript, _ := ctx.Value(transcriptKey{}).(string)
	if transcript == "" {
		transcript = TranscriptOf(videoPath)
	}
	if err := saveTranscript(transcript, original); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	// The original is one of the two files written; the context's Trace hears that half the export is done.
	reportProgress(ctx, core.Fraction(0.5))
	bilingual := make([]core.Segment, len(original))
	for i, s := range original {
		// A line left untranslated keeps its original alone: an empty line above it would end the cue before its text.
		if translation := strings.TrimSpace(translated[i].Text); translation != "" {
			s.Text = translation + "\n" + s.Text
		}
		bilingual[i] = s
	}
	path := base + ".bilingual.srt"
	if err := os.WriteFile(path, []byte(formatSRT(bilingual)), 0o644); err != nil {
		return "", err
	}
	reportProgress(ctx, core.Fraction(1))
	return path, nil
}

// transcriptPath is the file that holds the original speech of the video at videoPath unless a file of the user's took that name first: "<video>.srt", beside it.
func transcriptPath(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".srt"
}

// asideTranscriptPath is where Kando keeps the original speech of the video at videoPath when a "<video>.srt" it did not write lies beside the video, as subtitles the user brought with it: "<video>.kando.srt". GatherVideos moves it with the video.
func asideTranscriptPath(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".kando.srt"
}

// TranscriptOf is the file that holds the original speech Kando transcribed from the video at videoPath: "<video>.kando.srt" when Kando kept it there, beside a "<video>.srt" of the user's (Run), else "<video>.srt". It need not exist.
func TranscriptOf(videoPath string) string {
	if aside := asideTranscriptPath(videoPath); fileExists(aside) {
		return aside
	}
	return transcriptPath(videoPath)
}

// foreignTranscript reports whether "<video>.srt" beside the video at videoPath is a file Kando did not write, which a job making the video's subtitles must leave as it is: it is there, Kando keeps no transcript of the video beside it under its own name, and nothing Kando writes beside a transcript it made is there, neither the bilingual subtitles it exports with it nor the translation it keeps while it translates (translationCheckpointPath).
func foreignTranscript(videoPath string) bool {
	if !fileExists(transcriptPath(videoPath)) || fileExists(asideTranscriptPath(videoPath)) {
		return false
	}
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	return !fileExists(base+".bilingual.srt") && !fileExists(translationCheckpointPath(videoPath))
}

type transcriptKey struct{}

// withTranscript returns ctx naming path as the file exportSubtitles writes the original speech to, the transcript Run chose for the job (TranscriptOf, or asideTranscriptPath beside a file of the user's).
func withTranscript(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, transcriptKey{}, path)
}

// pendingTranscriptPath is where a job processing a done film again entirely (core.RedoAll) keeps the speech it transcribed until it exports: a hidden file beside the video, so the film's own transcript, which its bilingual subtitles and a later translation of them again read, stays as it is until the new subtitles replace both.
func pendingTranscriptPath(videoPath string) string {
	return hiddenBeside(videoPath, ".pending.srt")
}

// pendingHeard is how the speech a RedoAll job keeps aside was heard (pendingTranscriptPath), which Run keeps in a hidden file beside it (pendingHeardPath): by Model in Language, as Tools hears it (hearer), and Spoken, the language the transcription said it heard, "" when it said none; Proofread is true once that speech was proofread and written back. A redo that failed or was stopped after it transcribed reads that speech back when run again the same way, rather than transcribing the film again, and does not proofread it again.
type pendingHeard struct {
	Model     string `json:"model"`
	Language  string `json:"language"`
	Spoken    string `json:"spoken,omitempty"`
	Proofread bool   `json:"proofread,omitempty"`
	// Platform is whether the speech kept aside is the site's subtitles checked against it, and Local how many of their lines the speech replaced (core.Job.PlatformSubtitles).
	Platform bool `json:"platform,omitempty"`
	Local    int  `json:"local,omitempty"`
}

// proofreadTranscriptPath is where a job translating a done film again (core.Redo.TranslatesOnly) keeps the film's transcript once it has proofread it, until it exports: a hidden file beside the video, so the film's own transcript, which its bilingual subtitles match line for line, stays as it is should the job fail or be stopped, and a job run again reads the proofread lines back rather than paying for them again.
func proofreadTranscriptPath(videoPath string) string {
	return hiddenBeside(videoPath, ".proofread.srt")
}

// readProofread is the proofread transcript a translating redo kept beside the video at videoPath (proofreadTranscriptPath), if it is the proofreading of original, line for line at the same times.
func readProofread(videoPath string, original []core.Segment) ([]core.Segment, bool) {
	lines, err := loadTranscript(proofreadTranscriptPath(videoPath))
	if err != nil || len(lines) != len(original) {
		return nil, false
	}
	for i, s := range lines {
		if s.Start != original[i].Start || s.End != original[i].End {
			return nil, false
		}
	}
	return lines, true
}

// pendingHeardPath is the hidden file beside the video at videoPath that says how the speech kept aside was heard (pendingHeard).
func pendingHeardPath(videoPath string) string {
	return hiddenBeside(videoPath, ".pending.json")
}

// readPendingHeard is how the speech kept aside beside the video at videoPath was heard, ok false when nothing says.
func readPendingHeard(videoPath string) (heard pendingHeard, ok bool) {
	data, err := os.ReadFile(pendingHeardPath(videoPath))
	if err != nil || json.Unmarshal(data, &heard) != nil {
		return pendingHeard{}, false
	}
	return heard, true
}

// savePendingHeard writes how the speech kept aside beside the video at videoPath was heard. One that cannot be written only means a retry transcribes again.
func savePendingHeard(videoPath string, heard pendingHeard) {
	if data, err := json.Marshal(heard); err == nil {
		_ = writeFileAtomic(pendingHeardPath(videoPath), data)
	}
}

// hiddenBeside is a hidden file beside the video at videoPath, named after the video with suffix in place of its extension, such as ".video.pending.srt". The library lists no hidden file and its watcher does not look at them.
func hiddenBeside(videoPath, suffix string) string {
	name := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	return filepath.Join(filepath.Dir(videoPath), "."+name+suffix)
}

// saveTranscript writes segments, the original speech of a video, to path as SubRip (writeFileAtomic), so a job that reads the transcript back to resume never finds one cut off by a crash in the middle of writing it.
func saveTranscript(path string, segments []core.Segment) error {
	return writeFileAtomic(path, []byte(formatSRT(segments)))
}

// writeFileAtomic writes data to path through a hidden file beside it, renamed over it once written, so path holds either what it held before or all of data.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// loadTranscript reads back the original speech saveTranscript wrote to path.
func loadTranscript(path string) ([]core.Segment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSRT(data)
}

// readExported reads back what exportSubtitles wrote beside the video at videoPath: the original speech from its transcript (TranscriptOf) and, when withTranslation, each line's translation from "<video>.bilingual.srt", empty for a line it left untranslated. A transcript that is gone, unreadable or empty is ErrNoTranscript. A bilingual file that is gone leaves every translation empty, so every line is translated; one that does not match the transcript cue for cue, as one edited by hand, is ErrSubtitlesChanged, since which lines are missing cannot be told and translating them all would overwrite the edits.
func readExported(videoPath string, withTranslation bool) (original, translated []core.Segment, err error) {
	if videoPath == "" {
		return nil, nil, ErrNoTranscript
	}
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	data, err := os.ReadFile(TranscriptOf(videoPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, ErrNoTranscript
	}
	if err != nil {
		return nil, nil, err
	}
	if original, err = ParseSRT(data); err != nil || len(original) == 0 {
		return nil, nil, ErrNoTranscript
	}
	translated = make([]core.Segment, len(original))
	for i, s := range original {
		translated[i] = core.Segment{Start: s.Start, End: s.End}
	}
	if !withTranslation {
		return original, translated, nil
	}
	data, err = os.ReadFile(base + ".bilingual.srt")
	if errors.Is(err, fs.ErrNotExist) {
		return original, translated, nil
	}
	if err != nil {
		return nil, nil, err
	}
	bilingual, err := ParseSRT(data)
	if err != nil || len(bilingual) != len(original) {
		return nil, nil, ErrSubtitlesChanged
	}
	lines := make([]string, len(original))
	for i, cue := range bilingual {
		s := original[i]
		if cue.Start != s.Start || cue.End != s.End {
			return nil, nil, ErrSubtitlesChanged
		}
		// A translated cue is its translation above the original line; an untranslated one is the original alone.
		if cue.Text == s.Text {
			continue
		}
		line, ok := strings.CutSuffix(cue.Text, "\n"+s.Text)
		if !ok {
			return nil, nil, ErrSubtitlesChanged
		}
		lines[i] = strings.TrimSpace(line)
	}
	for i := range translated {
		translated[i].Text = lines[i]
	}
	return original, translated, nil
}

func formatSRT(segments []core.Segment) string {
	var b strings.Builder
	for i, s := range segments {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(s.Start), srtTime(s.End), s.Text)
	}
	return b.String()
}

func srtTime(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

// ParseSRT reads a SubRip file. A cue's text keeps its line breaks, so a bilingual cue is the translation, a newline, then the original line.
func ParseSRT(data []byte) ([]core.Segment, error) {
	text := strings.TrimPrefix(string(data), "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out []core.Segment
	bad := 0
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		i := 0
		if !strings.Contains(lines[0], "-->") {
			i = 1
		}
		if i >= len(lines) {
			bad++
			continue
		}
		start, end, ok := parseSRTSpan(lines[i])
		if !ok {
			bad++
			continue
		}
		body := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		if body == "" {
			continue
		}
		out = append(out, core.Segment{Start: start, End: end, Text: body})
	}
	if bad > 0 && len(out) == 0 {
		return nil, fmt.Errorf("could not read subtitles")
	}
	return out, nil
}

func parseSRTSpan(line string) (time.Duration, time.Duration, bool) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := parseSRTClock(parts[0])
	end, ok2 := parseSRTClock(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return start, end, true
}

func parseSRTClock(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	rest := strings.ReplaceAll(parts[2], ",", ".")
	secParts := strings.SplitN(rest, ".", 2)
	sec, err3 := strconv.Atoi(secParts[0])
	if err1 != nil || err2 != nil || err3 != nil || h < 0 || m < 0 || sec < 0 {
		return 0, false
	}
	ms := 0
	if len(secParts) == 2 {
		frac := secParts[1]
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		var err error
		ms, err = strconv.Atoi(frac)
		if err != nil {
			return 0, false
		}
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second + time.Duration(ms)*time.Millisecond, true
}
