package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"
)

// The user's corrections of a film's lines (core.Correction) are kept in a hidden file beside its video, "<video>.edits.json" with a dot before it (CorrectionsPath), which no job writes: processing or translating the film again rewrites its subtitle files and leaves the corrections as they are, and readers lay them over the lines again (ReadSubtitles). It moves and goes to the Trash with the video (Companions).

// correctionsVersion is the version of the corrections file this Kando writes. A file of a later version is read for the fields this one knows and is not written over, so a newer Kando's corrections are not lost (ErrCorrectionsNewer).
const correctionsVersion = 1

// ErrCorrectionsNewer is SaveCorrections refusing to write over a corrections file a newer Kando wrote, whose fields this one would drop.
var ErrCorrectionsNewer = errors.New("the corrections were written by a newer Kando")

// correctionsFile is the corrections file as JSON, its times in milliseconds.
type correctionsFile struct {
	Version int               `json:"version"`
	Lines   []correctionEntry `json:"lines"`
}

type correctionEntry struct {
	StartMS      int64   `json:"startMs"`
	EndMS        int64   `json:"endMs"`
	Original     *string `json:"original,omitempty"`
	Translation  *string `json:"translation,omitempty"`
	StartShiftMS int64   `json:"startShiftMs,omitempty"`
	EndShiftMS   int64   `json:"endShiftMs,omitempty"`
	NoSpeech     bool    `json:"noSpeech,omitempty"`
}

// CorrectionsPath is the corrections file of the film whose video, or whose subtitles Kando reads or writes by the video's name (<video>.srt, <video>.kando.srt, <video>.bilingual.srt), is at path: ".<video>.edits.json" beside it.
func CorrectionsPath(path string) string {
	base := filepath.Base(path)
	stem := ""
	for _, suffix := range []string{".bilingual.srt", ".kando.srt", ".srt"} {
		if s, ok := strings.CutSuffix(base, suffix); ok {
			stem = s
			break
		}
	}
	if stem == "" {
		stem = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return filepath.Join(filepath.Dir(path), "."+stem+".edits.json")
}

// LoadCorrections reads the corrections file at path (CorrectionsPath). A file that is not there holds none.
func LoadCorrections(path string) ([]core.Correction, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f correctionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := make([]core.Correction, 0, len(f.Lines))
	for _, e := range f.Lines {
		c := core.Correction{
			Span:        core.Span{Start: correctionMS(e.StartMS), End: correctionMS(e.EndMS)},
			Original:    e.Original,
			Translation: e.Translation,
			StartShift:  correctionMS(e.StartShiftMS),
			EndShift:    correctionMS(e.EndShiftMS),
			NoSpeech:    e.NoSpeech,
		}
		if !c.Empty() {
			out = append(out, c)
		}
	}
	return out, nil
}

// SaveCorrections writes edits to the corrections file at path (CorrectionsPath), all of it or nothing (writeFileAtomic), and removes the file when there are none. A file a newer Kando wrote is left as it is (ErrCorrectionsNewer).
func SaveCorrections(path string, edits []core.Correction) error {
	if data, err := os.ReadFile(path); err == nil {
		var f correctionsFile
		if json.Unmarshal(data, &f) == nil && f.Version > correctionsVersion {
			return ErrCorrectionsNewer
		}
	}
	f := correctionsFile{Version: correctionsVersion, Lines: []correctionEntry{}}
	for _, c := range edits {
		if c.Empty() {
			continue
		}
		f.Lines = append(f.Lines, correctionEntry{
			StartMS: c.Span.Start.Milliseconds(), EndMS: c.Span.End.Milliseconds(),
			Original: c.Original, Translation: c.Translation,
			StartShiftMS: c.StartShift.Milliseconds(), EndShiftMS: c.EndShift.Milliseconds(),
			NoSpeech: c.NoSpeech,
		})
	}
	if len(f.Lines) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// ReadSubtitles reads the subtitle file at path, as ParseSRT does, with the user's corrections of the film laid over it (core.CorrectSegments), the lines marked as no speech left out. A corrections file that cannot be read leaves the lines as the machine wrote them.
func ReadSubtitles(path string) ([]core.Segment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cues, err := ParseSRT(data)
	if err != nil {
		return nil, err
	}
	edits, err := LoadCorrections(CorrectionsPath(path))
	if err != nil || len(edits) == 0 {
		return cues, nil
	}
	corrected, _ := core.CorrectSegments(cues, edits)
	return corrected, nil
}

func correctionMS(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
