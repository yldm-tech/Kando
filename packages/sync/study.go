package sync

import (
	"context"
	"strings"
)

// Lookups and explanations (yldm-tech/Kando#222). A phone looks up a word of a line, or has a line explained, through the desktop it is paired with: the desktop asks the AI service set in its own settings, so the service's key never leaves the desktop, and keeps the words looked up in its own cache, so a word asked once answers again at once, from any device, also while the AI service cannot be reached. Both are reads (not Mutates): they change nothing a device syncs, a read-only device (Caller.ReadOnly) may call them, and a call sent again asks again; a desktop keeps no word a read-only device looked up in its cache, so such a device changes nothing on it. A word put in the vocabulary travels as a KindWord record of the learning state, not here.

// The operations of lookups and explanations, which Hello lists among its Ops when the desktop answers them.
const (
	// OpLookUp takes WordLookup and answers WordEntry.
	OpLookUp = "study.lookup"
	// OpExplain takes LineExplain and answers Explanation.
	OpExplain = "study.explain"
)

// CodeNoAI is a lookup or an explanation the desktop cannot ask for: no AI service is set in its settings, or the one set cannot answer, as while it is offline or signed out; Message says why, in English (status 409).
const CodeNoAI = "no_ai"

// WordLookup is a word a phone looks up: the word as tapped, the original of the line it was tapped in, as what it means is what it means there, and the film and library the line is of, whose language and title the desktop asks with; a film the desktop does not serve is asked about without them.
type WordLookup struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Word    string `json:"word"`
	Line    string `json:"line"`
}

// WordEntry is what the desktop's AI service said of a word: the word as a dictionary lists it, how it is said ("" for none) and what it means in the line, in Language, the language the meaning is written in, as the desktop names it (简体中文).
type WordEntry struct {
	Word     string `json:"word"`
	Reading  string `json:"reading"`
	Meaning  string `json:"meaning"`
	Language string `json:"language"`
}

// LineExplain is a line a phone has explained: Lines are the line with the lines around it, as its context, in the order they play, and Index the one explained among them; Film and Library are the film and library they are of, as for WordLookup.
type LineExplain struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Lines   []Cue  `json:"lines"`
	Index   int    `json:"index"`
}

// MaxExplainLines is how many lines a LineExplain carries at most, the line explained among them.
const MaxExplainLines = 8

// Explanation is the desktop's AI service's explanation of a line: its text, in paragraphs apart by blank lines, in Language, and the model that wrote it.
type Explanation struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Model    string `json:"model"`
}

// StudyService is what a desktop does for lookups and explanations, whichever transport carries them. Desk has checked what it is given (Check) before it gets it. Its errors are as Service's.
type StudyService interface {
	LookUp(ctx context.Context, caller Caller, req WordLookup) (WordEntry, error)
	Explain(ctx context.Context, caller Caller, req LineExplain) (Explanation, error)
}

// Check reports why req cannot be looked up, nil when it can: it names a word.
func (req WordLookup) Check() error {
	if strings.TrimSpace(req.Word) == "" {
		return Errorf(CodeInvalid, "a lookup names a word")
	}
	return nil
}

// Check reports why req cannot be explained, nil when it can: it carries at least one line and at most MaxExplainLines, and Index is one of them with text in it.
func (req LineExplain) Check() error {
	if len(req.Lines) == 0 || len(req.Lines) > MaxExplainLines {
		return Errorf(CodeInvalid, "an explanation carries from 1 to %d lines", MaxExplainLines)
	}
	if req.Index < 0 || req.Index >= len(req.Lines) {
		return Errorf(CodeInvalid, "index %d is none of the %d lines", req.Index, len(req.Lines))
	}
	if l := req.Lines[req.Index]; strings.TrimSpace(l.Original) == "" && strings.TrimSpace(l.Translation) == "" {
		return Errorf(CodeInvalid, "the line explained has no text")
	}
	return nil
}

// answerStudy runs a call of lookups and explanations.
func (d *Desk) answerStudy(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Study == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not look words up")
	}
	if call.Op == OpLookUp {
		var req WordLookup
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if err := req.Check(); err != nil {
			return nil, err
		}
		return d.Study.LookUp(ctx, caller, req)
	}
	var req LineExplain
	if err := args(call, &req); err != nil {
		return nil, err
	}
	if err := req.Check(); err != nil {
		return nil, err
	}
	return d.Study.Explain(ctx, caller, req)
}

// LookUp has the desktop look word up as it reads in req.Line.
func (c *Client) LookUp(ctx context.Context, req WordLookup) (WordEntry, error) {
	var out WordEntry
	err := c.Call(ctx, OpLookUp, req, &out)
	return out, err
}

// Explain has the desktop explain line req.Index of req.Lines.
func (c *Client) Explain(ctx context.Context, req LineExplain) (Explanation, error) {
	var out Explanation
	err := c.Call(ctx, OpExplain, req, &out)
	return out, err
}
