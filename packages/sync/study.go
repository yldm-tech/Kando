package sync

import (
	"context"
	"errors"
	"strings"
)

// Lookups and explanations (yldm-tech/Kando#222), follow-up questions and recaps (#215). A phone looks up a word of a line, has a line explained, asks about it, or has the part of a film it watched summed up, through the desktop it is paired with: the desktop asks the AI service set in its own settings, so the service's key never leaves the desktop, and keeps the words looked up in its own cache, so a word asked once answers again at once, from any device, also while the AI service cannot be reached. All of them are reads (not Mutates): they change nothing a device syncs, a read-only device (Caller.ReadOnly) may call them, and a call sent again asks again; a desktop keeps no word a read-only device looked up in its cache, so such a device changes nothing on it. A word put in the vocabulary travels as a KindWord record of the learning state, not here.

// The operations of lookups, explanations, follow-up questions and recaps, which Hello lists among its Ops when the desktop answers them.
const (
	// OpLookUp takes WordLookup and answers WordEntry.
	OpLookUp = "study.lookup"
	// OpExplain takes LineExplain and answers Explanation.
	OpExplain = "study.explain"
	// OpExplainAsk takes ExplainAsk and answers ExplainAnswer: a follow-up question about a line explained (yldm-tech/Kando#215).
	OpExplainAsk = "study.ask"
	// OpRecap takes FilmRecap and answers Recap: what happened in the part of a film already watched, for the 前情提要 card a film opens with where it was left (yldm-tech/Kando#215).
	OpRecap = "study.recap"
)

// CodeNoAI is a lookup or an explanation the desktop cannot ask for: no AI service is set in its settings, or the one set cannot answer, as while it is offline or signed out, or refuses the desktop's account, as one out of credit; Message says which in a fixed phrase of the desktop's, in English, never with what the service answered or the desktop's words to its own user (status 409).
const CodeNoAI = "no_ai"

// studyFailed is the Message of a lookup, an explanation, a follow-up question or a recap that failed with an error the StudyService did not give a code (CallError): what the AI service answered stays on the desktop, as it may hold the service's prices or a link to buy credit, which a phone must never show.
const studyFailed = "the desktop's AI service did not answer"

// studyErr is err as a call of the study's answers it: a CallError as the StudyService gave it, anything else CodeFailed with studyFailed alone.
func studyErr(err error) error {
	var e *CallError
	if err == nil || errors.As(err, &e) {
		return err
	}
	return &CallError{Code: CodeFailed, Message: studyFailed}
}

// WordLookup is a word a phone looks up: the word as tapped, the original of the line it was tapped in, as what it means is what it means there, and the film and library the line is of, whose language and title the desktop asks with; a film the desktop does not serve is asked about without them.
type WordLookup struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Word    string `json:"word"`
	Line    string `json:"line"`
}

// WordEntry is what the desktop said of a word: the word as a dictionary lists it, how it is said ("" for none) and what it means in the line, in Language, the language the meaning is written in, as the desktop names it (简体中文), and where that came from (Source). A desktop older than Source leaves it out, and a phone then says nothing of where the meaning came from; Source is an addition to v2, which a phone that does not know it skips, so it needs no new version.
type WordEntry struct {
	Word     string     `json:"word"`
	Reading  string     `json:"reading"`
	Meaning  string     `json:"meaning"`
	Language string     `json:"language"`
	Source   WordSource `json:"source,omitzero"`
}

// WordSource is where a WordEntry came from: Kind is SourceDictionary, a dictionary pack installed on the desktop, with its name (Name) and the credit its license asks for (Credit, "" for none), both in the desktop's interface language, SourcePlugin, a lookup source of a script plugin on the desktop, with the source's name (Name) and its plugin's (Plugin), both in the desktop's interface language, or SourceAI, the desktop's AI service, now or when the word was looked up before; a kind a phone does not know it says nothing of, so later kinds can be added. Desktop is the name of the desktop that answered, as Hello names it, which a phone keeps with the entry, so a word it answers from its own lookups later still names the desktop that answered it; "" from a desktop that did not say. The zero WordSource is a desktop that did not say where the entry came from.
type WordSource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
	Credit  string `json:"credit,omitempty"`
	Plugin  string `json:"plugin,omitempty"`
	Desktop string `json:"desktop,omitempty"`
}

// The kinds of a WordSource.
const (
	SourceDictionary = "dictionary"
	SourceAI         = "ai"
	SourcePlugin     = "plugin"
)

// LineExplain is a line a phone has explained: Lines are the line with the lines around it, as its context, in the order they play, and Index the one explained among them; Film and Library are the film and library they are of, as for WordLookup.
type LineExplain struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Lines   []Cue  `json:"lines"`
	Index   int    `json:"index"`
}

// MaxExplainLines is how many lines a LineExplain carries at most, the line explained among them.
const MaxExplainLines = 8

// Explanation is the desktop's AI service's explanation of a line: its text, in paragraphs apart by blank lines, in Language, the same paragraphs as Sections, each with what it is about, for a phone that heads them as the desktop's side panel does, the model that wrote it, and Local when that model runs on the desktop itself rather than at a cloud service. A desktop older than Sections and Local leaves them out, and a phone shows Text.
type Explanation struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Model    string           `json:"model"`
	Sections []ExplainSection `json:"sections,omitempty"`
	Local    bool             `json:"local,omitempty"`
}

// ExplainSection is a paragraph of an explanation: Kind, one of the Explain kinds, says what it is about; a kind a phone does not know it shows under no heading, so later kinds can be added.
type ExplainSection struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// The kinds of an explanation's sections, in the order a desktop gives them: what the line means, its words and phrases one a line as "word · meaning", what is worth knowing of its translation, its tone, and related expressions one a line.
const (
	ExplainMeaning     = "meaning"
	ExplainBreakdown   = "breakdown"
	ExplainTranslation = "translation"
	ExplainTone        = "tone"
	ExplainRelated     = "related"
)

// ExplainAsk is a follow-up question about a line explained: the line with the lines around it, as LineExplain carries them, of which the desktop sends its AI service only the line and those before it; what the explanation said (Explained), the questions asked of the line before and their answers (Asked), and the question.
type ExplainAsk struct {
	LineExplain
	Explained []ExplainSection `json:"explained"`
	Asked     []ExplainQA      `json:"asked"`
	Question  string           `json:"question"`
}

// ExplainQA is a follow-up question asked before and its answer.
type ExplainQA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// MaxExplainAsked is how many questions asked before an ExplainAsk carries at most; a phone sends the latest.
const MaxExplainAsked = 10

// ExplainAnswer is the desktop's AI service's answer to a follow-up question, in Language, with the model that wrote it and whether it runs on the desktop, as for Explanation.
type ExplainAnswer struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Model    string `json:"model"`
	Local    bool   `json:"local,omitempty"`
}

// FilmRecap is the part of a film a phone has watched, to be summed up: the lines of it, in the order they play, from the latest back as far as fit in a call (RecapLines), and the film and library they are of, as for WordLookup. The desktop sums up these lines alone, so the recap gives nothing away of what comes after them.
type FilmRecap struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Lines   []Cue  `json:"lines"`
}

// Recap is what the desktop's AI service said of the part watched: at most RecapItems moments, in the order of the film, each at the start of one of the lines sent, in Language, with the model that wrote it and whether it runs on the desktop, as for Explanation.
type Recap struct {
	Items    []RecapItem `json:"items"`
	Language string      `json:"language"`
	Model    string      `json:"model"`
	Local    bool        `json:"local,omitempty"`
}

// RecapItem is a moment of a recap: the start of the line it is at, in milliseconds as Cue.StartMS, and what happened there.
type RecapItem struct {
	StartMS int64  `json:"start_ms"`
	Text    string `json:"text"`
}

// RecapItems is how many moments a Recap has at most, as the design's card shows three.
const RecapItems = 3

// recapBudget is how many bytes of lines a FilmRecap carries at most, below MaxCall by room for the call around them.
const recapBudget = MaxCall - 8<<10

// RecapLines are the latest of watched, the lines of a film watched in the order they play, that fit in a FilmRecap: the earliest are left out past what a call carries.
func RecapLines(watched []Cue) []Cue {
	size, from := 0, len(watched)
	for from > 0 {
		c := watched[from-1]
		// A line's JSON is its texts, escaped at worst to twice their bytes, and its fields' names and numbers.
		n := 2*(len(c.Original)+len(c.Translation)) + 80
		if size+n > recapBudget {
			break
		}
		size += n
		from--
	}
	return watched[from:]
}

// StudyService is what a desktop does for lookups, explanations, follow-up questions and recaps, whichever transport carries them. Desk has checked what it is given (Check) before it gets it. Its errors are as Service's.
type StudyService interface {
	LookUp(ctx context.Context, caller Caller, req WordLookup) (WordEntry, error)
	Explain(ctx context.Context, caller Caller, req LineExplain) (Explanation, error)
	Ask(ctx context.Context, caller Caller, req ExplainAsk) (ExplainAnswer, error)
	Recap(ctx context.Context, caller Caller, req FilmRecap) (Recap, error)
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

// Check reports why req cannot be asked, nil when it can: its line can be explained (LineExplain.Check), it asks something, and it carries at most MaxExplainAsked questions asked before.
func (req ExplainAsk) Check() error {
	if err := req.LineExplain.Check(); err != nil {
		return err
	}
	if strings.TrimSpace(req.Question) == "" {
		return Errorf(CodeInvalid, "a follow-up asks a question")
	}
	if len(req.Asked) > MaxExplainAsked {
		return Errorf(CodeInvalid, "a follow-up carries at most %d questions asked before", MaxExplainAsked)
	}
	return nil
}

// Check reports why req cannot be summed up, nil when it can: it carries a line with text in it.
func (req FilmRecap) Check() error {
	for _, l := range req.Lines {
		if strings.TrimSpace(l.Original) != "" || strings.TrimSpace(l.Translation) != "" {
			return nil
		}
	}
	return Errorf(CodeInvalid, "a recap carries a line watched")
}

// answerStudy runs a call of lookups, explanations, follow-up questions and recaps.
func (d *Desk) answerStudy(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Study == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not look words up")
	}
	switch call.Op {
	case OpLookUp:
		var req WordLookup
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if err := req.Check(); err != nil {
			return nil, err
		}
		out, err := d.Study.LookUp(ctx, caller, req)
		return out, studyErr(err)
	case OpExplainAsk:
		var req ExplainAsk
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if err := req.Check(); err != nil {
			return nil, err
		}
		out, err := d.Study.Ask(ctx, caller, req)
		return out, studyErr(err)
	case OpRecap:
		var req FilmRecap
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if err := req.Check(); err != nil {
			return nil, err
		}
		out, err := d.Study.Recap(ctx, caller, req)
		return out, studyErr(err)
	}
	var req LineExplain
	if err := args(call, &req); err != nil {
		return nil, err
	}
	if err := req.Check(); err != nil {
		return nil, err
	}
	out, err := d.Study.Explain(ctx, caller, req)
	return out, studyErr(err)
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

// Ask has the desktop answer req.Question about line req.Index of req.Lines.
func (c *Client) Ask(ctx context.Context, req ExplainAsk) (ExplainAnswer, error) {
	var out ExplainAnswer
	err := c.Call(ctx, OpExplainAsk, req, &out)
	return out, err
}

// Recap has the desktop sum up req.Lines, the part of a film watched.
func (c *Client) Recap(ctx context.Context, req FilmRecap) (Recap, error) {
	var out Recap
	err := c.Call(ctx, OpRecap, req, &out)
	return out, err
}
