// Package lines reads the lines of a film's subtitles and finds those that hold a query and that the learner has watched, as the command palette and the control socket's lines.search and subtitles.get do. It imports no area of the window.
package lines

import (
	"strings"
	"time"

	"kando/packages/media"
)

// Slack is how far past the furthest point played a line may start and still count as seen, as the prototype's half second.
const Slack = 500 * time.Millisecond

// Line is a line of an exported subtitle file: where it starts, its translation and its original.
type Line struct {
	Start                 time.Duration
	Translation, Original string
	// folded is both in lower case for the search (Holds).
	folded string
}

// New is the line starting at start with translation and original.
func New(start time.Duration, translation, original string) Line {
	return Line{Start: start, Translation: translation, Original: original, folded: strings.ToLower(translation + "\n" + original)}
}

// Read is the lines of the bilingual .srt at path, with the user's corrections of the film laid over them (media.ReadSubtitles), each split into its translation and its original (Split).
func Read(path string) ([]Line, error) {
	segments, err := media.ReadSubtitles(path)
	if err != nil {
		return nil, err
	}
	out := make([]Line, len(segments))
	for i, seg := range segments {
		translation, original := Split(seg.Text)
		out[i] = New(seg.Start, translation, original)
	}
	return out, nil
}

// Split is a cue of an exported subtitle file as its translation and its original: the pipeline writes the translation on the first line and the original under it, and a cue of one line is an original without a translation.
func Split(text string) (translation, original string) {
	text = strings.TrimSpace(text)
	first, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return "", text
	}
	return strings.TrimSpace(first), strings.TrimSpace(rest)
}

// Fold is a query as Holds and Matches take it: without the spaces around it, in lower case.
func Fold(query string) string { return strings.ToLower(strings.TrimSpace(query)) }

// Holds reports whether the line's translation or original contains folded, a query Fold made.
func (l Line) Holds(folded string) bool { return strings.Contains(l.folded, folded) }

// Matches reports whether a line with translation and original contains folded, a query Fold made, ignoring case as Holds does.
func Matches(translation, original, folded string) bool {
	return strings.Contains(strings.ToLower(translation+"\n"+original), folded)
}

// Seen reports whether a line starting at start was watched in a film played as far as horizon: it starts no later than horizon and Slack.
func Seen(start, horizon time.Duration) bool { return start <= horizon+Slack }
