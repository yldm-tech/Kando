package media

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kando/packages/core"
)

// CandidatesMax is how many other translations of a line Candidates returns at most.
const CandidatesMax = 3

// CandidateRequest is a line the viewer says is translated wrongly (Line), with the lines before and after it, the language it is translated into and the film's glossary, of which the terms the lines hold go with the request.
type CandidateRequest struct {
	Language      string
	Line          core.Line
	Before, After []core.Line
	Terms         []core.Term
}

// candidateRule is what a request for other translations of a line asks of the model's reply.
const candidateRule = "The viewer marked one subtitle line as translated wrongly. The user sends it as JSON: \"line\" is the marked line, its original and its current translation, and \"before\" and \"after\" are the lines around it, for context only. " +
	"Read the context to work out what the marked line means here: who speaks, what it refers to, names and terms. Then give up to 3 better translations of the marked line alone, different from its current translation and from each other, best first. " +
	"Reply with only a JSON array of strings."

// candidateLine is a line as a request for other translations sends it.
type candidateLine struct {
	Original    string `json:"original"`
	Translation string `json:"translation,omitempty"`
}

// Candidates asks the model for other translations of req.Line, from the line and the lines around it, with the style instruction translations use (Prompt) and the glossary terms the lines hold: the viewer's "this line is wrong". It returns them best first, at most CandidatesMax, without the line's current translation, blanks or repeats; none when the model has nothing better.
func (t ChatTranslator) Candidates(ctx context.Context, req CandidateRequest) ([]string, error) {
	lines := slices.Concat(req.Before, []core.Line{req.Line}, req.After)
	texts := make([]string, 0, 2*len(lines))
	for _, l := range lines {
		texts = append(texts, l.Original, l.Translation)
	}
	system := stylePrompt(t.Prompt, req.Language)
	if glossary := glossaryPrompt(core.TermsIn(req.Terms, texts)); glossary != "" {
		system += "\n\n" + glossary
	}
	system += "\n\n" + candidateRule
	conv := func(ls []core.Line) []candidateLine {
		out := make([]candidateLine, len(ls))
		for i, l := range ls {
			out[i] = candidateLine{Original: l.Original, Translation: l.Translation}
		}
		return out
	}
	user, err := json.Marshal(struct {
		Before []candidateLine `json:"before"`
		Line   candidateLine   `json:"line"`
		After  []candidateLine `json:"after"`
	}{conv(req.Before), candidateLine{Original: req.Line.Original, Translation: req.Line.Translation}, conv(req.After)})
	if err != nil {
		return nil, err
	}
	reply, err := t.Chat(ctx, system, string(user))
	if err != nil {
		return nil, err
	}
	var got []string
	if err := json.Unmarshal([]byte(reply), &got); err != nil {
		return nil, fmt.Errorf("candidates: the reply is not a list of translations: %w", err)
	}
	return keepCandidates(got, req.Line.Translation), nil
}

// candidater is a translation source that offers other translations of a line the viewer says is wrong, as ChatTranslator does: a Tools.Engine that does is asked for them in place of Translator (Tools.Candidates).
type candidater interface {
	Candidates(ctx context.Context, req CandidateRequest) ([]string, error)
}

// Candidates asks for other translations of req.Line: Engine's, when it offers them, else Translator's (ChatTranslator.Candidates). Engine's are kept as ChatTranslator keeps the model's, best first, at most CandidatesMax, without the line's current translation, blanks or repeats; its failure names it (engineError), and offline mode refuses one not on this computer before anything is sent.
func (t Tools) Candidates(ctx context.Context, req CandidateRequest) ([]string, error) {
	source, ok := t.Engine.(candidater)
	if !ok {
		return t.withOffline().Candidates(ctx, req)
	}
	if err := t.engineOffline(); err != nil {
		return nil, err
	}
	got, err := source.Candidates(ctx, req)
	if err != nil {
		return nil, t.engineErr(err)
	}
	return keepCandidates(got, req.Line.Translation), nil
}

// keepCandidates are the translations got offers for a line translated as current, best first, with their spaces made single, at most CandidatesMax, without current, blanks or repeats.
func keepCandidates(got []string, current string) []string {
	current = strings.TrimSpace(current)
	var out []string
	for _, c := range got {
		c = strings.Join(strings.Fields(c), " ")
		if c == "" || c == current || slices.Contains(out, c) {
			continue
		}
		out = append(out, c)
		if len(out) == CandidatesMax {
			break
		}
	}
	return out
}
