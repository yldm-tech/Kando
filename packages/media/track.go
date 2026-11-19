package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// ReadTrack is the document of the film whose video is at videoPath (track.Read), with subtitles, the file the film shows, taken in when it holds what the document did not last write out or take in of it (track.KindFile), as a file the pipeline, an older Kando or the user's editor wrote. Its cues are split into each line's original and translation by transcript, the film's .srt of the speech alone, when there is one (PairCues). When they show at the times the document's lines do, the lines keep their ids and take each text that changed; otherwise the cues become a new generation. What is taken in counts as the final layer, as subtitles made before the document were done films.
//
// Reading writes nothing: what a file adds is taken in for this read only, so a film read by a search over the library, on a folder that cannot be written to or one another computer shares, is left as it is. A file that is not there leaves the document as it is; a film without a video keeps no document and reads as its file alone.
func ReadTrack(videoPath, subtitles, transcript string) (track.Doc, error) {
	var doc track.Doc
	if videoPath != "" {
		var err error
		if doc, err = track.Read(videoPath); err != nil {
			return doc, err
		}
	}
	events, err := trackFiles(doc, subtitles, transcript)
	if err != nil || len(events) == 0 {
		return doc, err
	}
	return doc.Plus(trackFromFile.Source, events...), nil
}

// trackFiles is the records that take into doc what subtitles, the file a film shows, holds that doc did not last write out or take in of it, its cues split by transcript as ReadTrack splits them; none when the file is not there or holds nothing new.
func trackFiles(doc track.Doc, subtitles, transcript string) ([]track.Event, error) {
	paired, file, err := trackFileLines(doc, subtitles, transcript)
	if err != nil || file.Kind == "" {
		return nil, err
	}
	return append(trackTakeIn(doc.View(), paired), file), nil
}

// trackFileLines is the lines subtitles, the file a film shows, holds, its cues split by transcript as ReadTrack splits them, with the record (track.KindFile) of what it holds, when that is not what doc last wrote out or took in of it; no lines and the zero record when the file is not there or holds nothing new.
func trackFileLines(doc track.Doc, subtitles, transcript string) ([]core.Line, track.Event, error) {
	if subtitles == "" {
		subtitles = transcript
	}
	if subtitles == "" {
		return nil, track.Event{}, nil
	}
	// The film shows its transcript, the speech alone, while it is translated: each cue is an original line, whatever rows it holds.
	speechOnly := transcript != "" && filepath.Clean(transcript) == filepath.Clean(subtitles)
	data, err := os.ReadFile(subtitles)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, track.Event{}, nil
	}
	if err != nil {
		return nil, track.Event{}, err
	}
	var speech []byte
	if transcript != "" && !speechOnly {
		if speech, err = os.ReadFile(transcript); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, track.Event{}, err
		}
	}
	hash, name := trackFileHash(data, speech), filepath.Base(subtitles)
	if last, ok := doc.File(name); ok && last.Key == hash {
		return nil, track.Event{}, nil
	}
	cues, err := ParseSRT(data)
	if err != nil {
		return nil, track.Event{}, err
	}
	var original []core.Segment
	switch {
	case speechOnly:
		original = cues
	case speech != nil:
		if original, err = ParseSRT(speech); err != nil {
			return nil, track.Event{}, err
		}
	}
	return PairCues(original, cues), track.Event{Kind: track.KindFile, Source: name, Key: hash}, nil
}

// trackFileHash is what a subtitle file and the transcript it was paired with held, as a KindFile record keeps it.
func trackFileHash(subtitles, transcript []byte) string {
	h := sha256.New()
	h.Write(subtitles)
	if transcript != nil {
		h.Write([]byte{0})
		h.Write(transcript)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// trackTakeIn is the records that make lines, the document's, show paired, a film's lines as its files hold them: a revision of each text that differs when they show at the lines' times, one for one, and otherwise a new generation of them.
func trackTakeIn(lines []track.Line, paired []core.Line) []track.Event {
	originals := make([]core.Segment, len(paired))
	translations := make([]core.Segment, len(paired))
	for i, l := range paired {
		originals[i] = core.Segment{Start: l.Start, End: l.End, Text: l.Original}
		translations[i] = core.Segment{Start: l.Start, End: l.End, Text: l.Translation}
	}
	same := len(lines) > 0 && len(lines) == len(paired)
	for i := 0; same && i < len(lines); i++ {
		same = lines[i].Start == paired[i].Start && lines[i].End == paired[i].End
	}
	if !same {
		return track.Generation(trackFromFile, originals, translations)
	}
	var events []track.Event
	for i, l := range lines {
		if l.Original.Text != originals[i].Text {
			events = append(events, track.Event{Kind: track.KindRev, Line: l.ID, Side: track.Original, Layer: track.LayerFinal, Text: originals[i].Text, Source: trackFromFile.Source})
		}
		if l.Translation.Text != translations[i].Text {
			events = append(events, track.Event{Kind: track.KindRev, Line: l.ID, Side: track.Translation, Layer: track.LayerFinal, Text: translations[i].Text, Source: trackFromFile.Source})
		}
	}
	return events
}

// trackFromFile is how lines taken in from a subtitle file were made.
var trackFromFile = track.Made{Source: "file", Original: track.LayerFinal, Translation: track.LayerFinal}

// PairCues is a film's lines as its two subtitle files hold them: original, the .srt of the speech alone, and bilingual, whose cues are each translation over its original line (track.Bilingual), or the original alone where a line has no translation. Each line of original takes its translation from the bilingual cue at its index when both files have as many cues shown at the same times, as the pipeline writes them, and otherwise, as after a file was edited by hand, from the first bilingual cue shown at the same time that no earlier line took, so two lines shown at once keep their own. Without original, each bilingual cue is split as the film page splits it (core.SplitCue), which reads a cue of two lines as a translation over its original.
func PairCues(original, bilingual []core.Segment) []core.Line {
	if original == nil {
		out := make([]core.Line, len(bilingual))
		for i, s := range bilingual {
			translation, text := core.SplitCue(s.Text)
			out[i] = core.Line{Start: s.Start, End: s.End, Original: text, Translation: translation}
		}
		return out
	}
	aligned := len(original) == len(bilingual)
	for i := 0; aligned && i < len(original); i++ {
		aligned = original[i].Start == bilingual[i].Start && original[i].End == bilingual[i].End
	}
	type span struct{ start, end time.Duration }
	var waiting map[span][]string
	if !aligned {
		waiting = make(map[span][]string, len(bilingual))
		for _, s := range bilingual {
			key := span{s.Start, s.End}
			waiting[key] = append(waiting[key], s.Text)
		}
	}
	out := make([]core.Line, 0, len(original))
	for i, s := range original {
		l := core.Line{Start: s.Start, End: s.End, Original: strings.TrimSpace(s.Text)}
		var text string
		var ok bool
		if aligned {
			text, ok = bilingual[i].Text, true
		} else if queue := waiting[span{s.Start, s.End}]; len(queue) > 0 {
			text, ok = queue[0], true
			waiting[span{s.Start, s.End}] = queue[1:]
		}
		if ok && strings.TrimSpace(text) != l.Original {
			if translation, cut := strings.CutSuffix(strings.TrimSpace(text), "\n"+l.Original); cut {
				l.Translation = strings.TrimSpace(translation)
			} else {
				l.Translation, _ = core.SplitCue(text)
			}
		}
		out = append(out, l)
	}
	return out
}
