package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
