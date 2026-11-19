package sync

import (
	"context"
	"hash/fnv"
	"strconv"
)

// The word marks of a film's lines (docs/plugin-system.md, E2): readings (furigana, pinyin) over the words of each line's original, a thick line in a level's tone under the words of the word list the desktop's user chose, and a dashed one under the words in the vocabulary. The desktop computes them from its plugins, as its own player draws them, with the switches of its 逐词标注 card; a phone runs no plugin and reads no dictionary for them, it only draws what OpFilmMarks answers. It is an addition to v2: a desktop that answers it lists it in Hello.Ops, one before it answers it CodeUnsupported, as a v1 desktop answers PathCall not_found, and the phone then draws its lines unmarked; a phone before it never asks. It only reads (not Mutates), so a read-only device may ask, and it goes over any transport, though a phone asks on the local network, as the marks of a film may take the desktop a while to compute.

// OpFilmMarks takes FilmRef and answers FilmMarks.
const OpFilmMarks = "films.marks"

// Mark is a run of a line's original marked, by byte offsets into it, Start before End: one with Ruby has its reading drawn over its characters; one without has a line drawn under them, a thick solid one in the color of Tone (ToneAccent…, "" for none), and a dashed one with Underline, a word in the vocabulary. Its fields are the desktop player's (kando/apps/kando/film/subtitle Mark), field for field, so the desktop converts one into the other.
type Mark struct {
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Ruby      string `json:"ruby,omitempty"`
	Tone      string `json:"tone,omitempty"`
	Underline bool   `json:"underline,omitempty"`
}

// The tones of a Mark, as a word list's tiers name them, which a phone draws in its theme's roles. A tone a phone does not know draws no line of a level.
const (
	ToneAccent  = "accent"
	ToneWarn    = "warn"
	ToneSuccess = "success"
	ToneMuted   = "muted"
)

// FilmMarks are the marks of a film's lines, those of the lines with marks only, in the order of the lines. Pending is a film whose marks the desktop is still computing, with no Lines: the phone asks again later. A film the desktop marks nothing on, with its marks turned off, answers no Lines and is not Pending, so a phone drops the marks it kept of it.
type FilmMarks struct {
	Pending bool        `json:"pending,omitempty"`
	Lines   []LineMarks `json:"lines"`
}

// LineMarks are the marks of line Line of a film, its index among the cues PathSubtitles answers, and Hash, the LineHash of the original they were computed for: a phone whose line Line has another original, as one corrected since or of subtitles read before the desktop's changed, draws none of them.
type LineMarks struct {
	Line  int    `json:"line"`
	Hash  string `json:"hash"`
	Marks []Mark `json:"marks"`
}

// LineHash is the hash of a line's original that LineMarks are computed for: FNV-1a's 64 bits, in base 36.
func LineHash(original string) string {
	h := fnv.New64a()
	h.Write([]byte(original))
	return strconv.FormatUint(h.Sum64(), 36)
}

// Of are the marks of each of originals, a film's lines' originals in their order, index for index: those of m's lines computed for the same original, nil for the others.
func (m FilmMarks) Of(originals []string) [][]Mark {
	out := make([][]Mark, len(originals))
	for _, l := range m.Lines {
		if l.Line >= 0 && l.Line < len(originals) && len(l.Marks) > 0 && LineHash(originals[l.Line]) == l.Hash {
			out[l.Line] = l.Marks
		}
	}
	return out
}

// MarkService is what a desktop does for OpFilmMarks, whichever transport carries it. Its errors are as Service's.
type MarkService interface {
	FilmMarks(ctx context.Context, caller Caller, ref FilmRef) (FilmMarks, error)
}

// answerMarks runs OpFilmMarks.
func (d *Desk) answerMarks(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Marks == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not mark its films' words")
	}
	var ref FilmRef
	if err := args(call, &ref); err != nil {
		return nil, err
	}
	if ref.ID == "" {
		return nil, Errorf(CodeInvalid, "%s names no film", call.Op)
	}
	m, err := d.Marks.FilmMarks(ctx, caller, ref)
	if err == nil && m.Lines == nil {
		m.Lines = []LineMarks{}
	}
	return m, err
}

// FilmMarks are the marks of film id's lines.
func (c *Client) FilmMarks(ctx context.Context, id string) (FilmMarks, error) {
	var out FilmMarks
	err := c.Call(ctx, OpFilmMarks, FilmRef{ID: id}, &out)
	return out, err
}
