package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lingocut/packages/core"
)

type whisperOutput struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// parseWhisperJSON reads the file whisper-cli writes with -oj, whose offsets are milliseconds.
func parseWhisperJSON(data []byte) ([]core.Segment, error) {
	var out whisperOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode whisper output: %w", err)
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
	return segments, nil
}

// exportSubtitles writes "<video>.srt" with the original speech and "<video>.bilingual.srt" with each translation above its original line, returning the bilingual path.
func exportSubtitles(videoPath string, original, translated []core.Segment) (string, error) {
	if len(original) != len(translated) {
		return "", fmt.Errorf("%d translated lines for %d original lines", len(translated), len(original))
	}
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	if err := os.WriteFile(base+".srt", []byte(formatSRT(original)), 0o644); err != nil {
		return "", err
	}
	bilingual := make([]core.Segment, len(original))
	for i, s := range original {
		s.Text = translated[i].Text + "\n" + s.Text
		bilingual[i] = s
	}
	path := base + ".bilingual.srt"
	if err := os.WriteFile(path, []byte(formatSRT(bilingual)), 0o644); err != nil {
		return "", err
	}
	return path, nil
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
