// Package cues is the player's model of a film's lines, in time: which line plays at a moment, the line before and after it, and where a looped line starts again. Which of a line's texts show, and in what order, is Kando's shared subtitle model (kando/packages/core CueLines and PictureLines).
package cues

import (
	"time"

	"kando/packages/sync"
)

// Cue is a line of the subtitles: when it shows and ends, a zero End for one that stays until the next, its original and its translation, "" where it was not translated.
type Cue struct {
	Start, End            time.Duration
	Original, Translation string
}

// FromSync are the cues the desktop sent, in the player's time.
func FromSync(in []sync.Cue) []Cue {
	out := make([]Cue, len(in))
	for i, c := range in {
		out[i] = Cue{Start: time.Duration(c.StartMS) * time.Millisecond, End: time.Duration(c.EndMS) * time.Millisecond, Original: c.Original, Translation: c.Translation}
	}
	return out
}

// Index is the index of the cue showing at at, the last of those that cover it, or -1 for none: before the first line, between two, or after the last has ended.
func Index(cues []Cue, at time.Duration) int {
	found := -1
	for i, c := range cues {
		if c.Start > at {
			break
		}
		if c.End > at || c.End == 0 {
			found = i
		}
	}
	return found
}

// Span is when cue i plays, from its start to its end; a cue without an end ends where the next starts, and the last one at the film's end, 0 when the film's length is not known.
func Span(cues []Cue, i int, length time.Duration) (start, end time.Duration) {
	c := cues[i]
	end = c.End
	if end == 0 {
		end = length
		if i+1 < len(cues) {
			end = cues[i+1].Start
		}
	}
	return c.Start, end
}

// Prev is the index of the line before the one playing at at, or before the moment when none plays, -1 at the first.
func Prev(cues []Cue, at time.Duration) int {
	if i := Index(cues, at); i >= 0 {
		return i - 1
	}
	prev := -1
	for i, c := range cues {
		if c.Start > at {
			break
		}
		prev = i
	}
	return prev
}

// Next is the index of the first line that starts after at, -1 after the last.
func Next(cues []Cue, at time.Duration) int {
	for i, c := range cues {
		if c.Start > at {
			return i
		}
	}
	return -1
}

// LoopSlack is how far before a looped line's start the playhead may be without the loop taking it back: a seek lands on a frame a little before the time asked.
const LoopSlack = 300 * time.Millisecond

// LoopBack reports whether the player looping cue i, at at in a film of length length (0 when not known), goes back to the line's start: once the line has ended, or when the playhead has left it for before it.
func LoopBack(cues []Cue, i int, at, length time.Duration) bool {
	start, end := Span(cues, i, length)
	if at < start-LoopSlack {
		return true
	}
	return end > start && at >= end
}
