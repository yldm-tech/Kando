package core

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// The user's corrections of a film's lines: what the user changed of a line the machine wrote (its original, its translation, when it starts and ends) or that it is no speech at all. They are kept apart from the subtitle files the pipeline writes, so processing or translating a film again rewrites those files and never the user's corrections, and every reader of the lines lays them over the machine's (Correct): the player, the control socket and MCP, and the phone.

// Span is when a line shows, from Start to End, as the machine wrote it: what a correction is keyed by, rather than by the line's place in the file, which a redo that splits or merges lines changes.
type Span struct {
	Start, End time.Duration
}

// overlap is how long a and b show at the same time.
func (s Span) overlap(o Span) time.Duration {
	return max(min(s.End, o.End)-max(s.Start, o.Start), 0)
}

// Correction is what the user changed of the machine's line at Span. A nil Original or Translation keeps the machine's; StartShift and EndShift move its start and end; NoSpeech leaves the line out, as a line the machine heard in music or noise.
type Correction struct {
	Span        Span
	Original    *string
	Translation *string
	StartShift  time.Duration
	EndShift    time.Duration
	NoSpeech    bool
}

// Empty reports whether c changes nothing of its line.
func (c Correction) Empty() bool {
	return c.Original == nil && c.Translation == nil && c.StartShift == 0 && c.EndShift == 0 && !c.NoSpeech
}

// Line is a line of a film's subtitles as a reader takes it: when it shows, what was said and its translation, "" for a line not translated.
type Line struct {
	Start, End            time.Duration
	Original, Translation string
}

// Corrected is a line after the user's corrections: the line as it shows, the machine's line it comes from (Machine, the span a new correction of it is keyed by), and Edit, the index of the correction laid over it among those Correct took, or -1 for a line the user did not correct.
type Corrected struct {
	Line
	Machine  Span
	Edit     int
	NoSpeech bool
}

// Correct lays edits over lines, the machine's, in the order they start, and returns them in the order they start once corrected, the lines marked NoSpeech among them for the caller to leave out or offer back. A correction takes the line whose span is its own, or, after a redo moved the lines, the one that shows longest with it, as long as that is at least half of the shorter of the two; a correction that takes no line is kept for when one fits again, and shows nowhere. A line takes one correction at most, the one that fits it best.
func Correct(lines []Line, edits []Correction) []Corrected {
	out := make([]Corrected, len(lines))
	for i, l := range lines {
		out[i] = Corrected{Line: l, Machine: Span{l.Start, l.End}, Edit: -1}
	}
	if len(edits) == 0 {
		return out
	}
	// fit is how well correction e fits line i: its overlap, the whole of the span for an exact match so that one always wins.
	fit := make([]time.Duration, len(lines))
	for e, c := range edits {
		best, bestFit := -1, time.Duration(0)
		for i, l := range out {
			if l.Machine.Start > c.Span.End {
				break
			}
			f := c.Span.overlap(l.Machine)
			if l.Machine == c.Span {
				f = time.Duration(1 << 62)
			} else if shorter := min(c.Span.End-c.Span.Start, l.Machine.End-l.Machine.Start); f <= 0 || 2*f < shorter {
				continue
			}
			if f > bestFit {
				best, bestFit = i, f
			}
		}
		if best < 0 || out[best].Edit >= 0 && fit[best] >= bestFit {
			continue
		}
		out[best].Edit, fit[best] = e, bestFit
	}
	for i := range out {
		if out[i].Edit >= 0 {
			out[i].apply(edits[out[i].Edit])
		}
	}
	slices.SortStableFunc(out, func(a, b Corrected) int { return cmp.Compare(a.Start, b.Start) })
	return out
}

// apply lays c over the line.
func (l *Corrected) apply(c Correction) {
	if c.Original != nil {
		l.Original = *c.Original
	}
	if c.Translation != nil {
		l.Translation = *c.Translation
	}
	l.Start = max(l.Start+c.StartShift, 0)
	l.End = max(l.End+c.EndShift, l.Start)
	l.NoSpeech = c.NoSpeech
}

// Spoken is lines without those the user marked as no speech.
func Spoken(lines []Corrected) []Corrected {
	return slices.DeleteFunc(slices.Clone(lines), func(l Corrected) bool { return l.NoSpeech })
}

// CorrectSegments lays edits over cues, the cues of a subtitle file as the pipeline exports them: a bilingual cue holds its translation on the first line and its original under it, and a cue of one line is an original alone (SplitCue). It returns the cues once corrected, without those marked as no speech, in the order they start, and the corrected lines they come from, index for index.
func CorrectSegments(cues []Segment, edits []Correction) ([]Segment, []Corrected) {
	lines := make([]Line, len(cues))
	for i, c := range cues {
		translation, original := SplitCue(c.Text)
		lines[i] = Line{Start: c.Start, End: c.End, Original: original, Translation: translation}
	}
	corrected := Spoken(Correct(lines, edits))
	out := make([]Segment, len(corrected))
	for i, l := range corrected {
		out[i] = Segment{Start: l.Start, End: l.End, Text: JoinCue(l.Translation, l.Original)}
	}
	return out, corrected
}

// SplitCue is a cue of an exported subtitle file as its translation and its original: the translation on the first line and the original under it, and a cue of one line an original without a translation.
func SplitCue(text string) (translation, original string) {
	first, rest, ok := strings.Cut(strings.TrimSpace(text), "\n")
	if !ok {
		return "", strings.TrimSpace(first)
	}
	return strings.TrimSpace(first), strings.TrimSpace(rest)
}

// JoinCue is the text of a cue with translation over original, as SplitCue reads it back; a line without a translation is its original alone, and one without an original its translation alone.
func JoinCue(translation, original string) string {
	switch {
	case translation == "":
		return original
	case original == "":
		return translation
	}
	return translation + "\n" + original
}

// SetCorrection is edits with c in place of the correction at index at, or added when at is -1, and without it when it changes nothing (Correction.Empty). edits is not changed.
func SetCorrection(edits []Correction, at int, c Correction) []Correction {
	out := slices.Clone(edits)
	switch {
	case at < 0 && c.Empty():
	case at < 0:
		out = append(out, c)
	case c.Empty():
		out = slices.Delete(out, at, at+1)
	default:
		out[at] = c
	}
	return out
}
