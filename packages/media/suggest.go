package media

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"unicode/utf8"

	"kando/packages/core"
)

// Terms suggested for a film's glossary (yldm-tech/Kando#213): once a film is translated, a pipeline that is a TermSuggester reads its lines with their translation and names the names and terms a glossary should fix, as the people, places and recurring proper nouns of the film, each with how the translation wrote it. Run asks for it when the job's context says so (WithTermSuggestions) and keeps what it found beside the video (SuggestionsPath), for the user to add to the glossary or dismiss in the player's 术语 tab. A suggestion is kept only when the film's lines hold it at least twice and its glossary does not hold it yet, and a dismissed one is not suggested again.

// TermSuggester is a Pipeline that can suggest terms for a film's glossary, as Tools does through its translator.
type TermSuggester interface {
	// SuggestTerms reads translated, the translation of original line for line into language, title naming the video for context, and returns the terms it suggests, at most SuggestMax, the ones the most lines hold first (rankSuggestions). A batch it could not read suggests nothing.
	SuggestTerms(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Term, error)
}

type suggestKey struct{}

// WithTermSuggestions returns ctx asking Run to suggest terms for the glossary of the film it translates.
func WithTermSuggestions(ctx context.Context) context.Context {
	return context.WithValue(ctx, suggestKey{}, true)
}

// suggestsTerms reports whether WithTermSuggestions asked for suggestions in ctx.
func suggestsTerms(ctx context.Context) bool {
	on, _ := ctx.Value(suggestKey{}).(bool)
	return on
}

// SuggestMax is how many terms a film is suggested at most.
const SuggestMax = 20

// suggestBatch bounds how many lines go into one request for suggestions.
const suggestBatch = 200

// suggestLongest is how many characters a suggested term or its translation may have: a glossary holds names and terms, not phrases.
const suggestLongest = 60

// suggestLeast is how many of a film's lines must hold a term for it to be suggested: one said once reads the same in every line already.
const suggestLeast = 2

// suggestPrompt is the instruction a request for suggestions is sent with; {{title}} becomes the video's title and {{language}} the language it was translated into.
const suggestPrompt = `You read the subtitles of a video titled "{{title}}", translated line by line into {{language}} by a machine. Find the names and terms a glossary should fix so that every line translates them the same way: names of people, places, organizations, works and products, and the technical terms and proper nouns the video comes back to. Leave out common words, and words that need no fixed translation.

The user sends a JSON array of the lines, each {"original": the line as spoken, "translation": its translation}. Reply with only a JSON array of at most 20 elements, the most important first: {"term": the words exactly as the original lines write them, "translation": how they are best written in {{language}}, as most lines translate them}. Reply [] when there are none.`

// suggestion is an element of a reply for suggestions.
type suggestion struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
}

// SuggestTerms asks for suggestions for the film's glossary in batches of suggestBatch translated lines, through the same endpoint and model translations use, and ranks what the batches found (rankSuggestions) against the glossary the context carries (WithGlossary). A term is kept from a batch only when one of the batch's original lines holds it. A batch whose reply cannot be used after it was sent again suggests nothing, and the rest of the film is still read; a key, model or address the API rejects, or an account out of credit, fails it.
func (t ChatTranslator) SuggestTerms(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Term, error) {
	if len(original) != len(translated) {
		return nil, fmt.Errorf("%d translated lines for %d original lines", len(translated), len(original))
	}
	if err := t.offline(); err != nil {
		return nil, err
	}
	endpoint := Endpoint{BaseURL: t.BaseURL, APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return nil, err
		}
	}
	system := strings.NewReplacer("{{title}}", strings.TrimSpace(title), "{{language}}", strings.TrimSpace(language)).Replace(suggestPrompt)
	var at []int
	for i, s := range translated {
		if strings.TrimSpace(s.Text) != "" {
			at = append(at, i)
		}
	}
	renewed := false
	var found []core.Term
	for start := 0; start < len(at); start += suggestBatch {
		batch := at[start:min(start+suggestBatch, len(at))]
		terms, err := t.suggestBatch(ctx, endpoint, system, original, translated, batch)
		var status *statusError
		if t.Credential != nil && !renewed && errors.As(err, &status) && status.code == http.StatusUnauthorized {
			renewed = true
			if endpoint, err = t.Credential(ctx, true); err != nil {
				return nil, err
			}
			terms, err = t.suggestBatch(ctx, endpoint, system, original, translated, batch)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if !skippableBatch(err) {
				return nil, err
			}
			continue
		}
		found = append(found, terms...)
	}
	return rankSuggestions(found, original, glossaryOf(ctx)), nil
}

// suggestBatch sends the lines of the film at the indexes batch, again while the reply is not a JSON array of terms, up to maxBatchSends times, and returns the terms one of the batch's original lines holds.
func (t ChatTranslator) suggestBatch(ctx context.Context, endpoint Endpoint, system string, original, translated []core.Segment, batch []int) ([]core.Term, error) {
	type pair struct {
		Original    string `json:"original"`
		Translation string `json:"translation"`
	}
	lines := make([]pair, len(batch))
	texts := make([]string, len(batch))
	for k, i := range batch {
		lines[k] = pair{Original: original[i].Text, Translation: strings.TrimSpace(translated[i].Text)}
		texts[k] = original[i].Text
	}
	input, err := json.Marshal(lines)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(chatRequest{
		Model:    t.Model,
		Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: string(input)}},
	})
	if err != nil {
		return nil, err
	}
	for send := 1; ; send++ {
		reply, err := t.complete(ctx, endpoint, body, core.Fraction(1))
		if err != nil {
			return nil, err
		}
		var got []suggestion
		if len(reply.Choices) == 0 {
			err = errors.New("the suggestions API returned no choices")
		} else if err = json.Unmarshal([]byte(stripCodeFence(reply.Choices[0].Message.Content)), &got); err != nil {
			err = fmt.Errorf("the suggested terms are not a JSON array: %w", err)
		}
		if err == nil {
			var out []core.Term
			for _, s := range got {
				term := core.Term{Source: strings.TrimSpace(s.Term), Target: strings.TrimSpace(s.Translation), Scope: core.ScopeFilm, Origin: core.OriginAuto}
				if term.Valid() && slices.ContainsFunc(texts, func(text string) bool { return core.TermIn(text, term.Source) }) {
					out = append(out, term)
				}
			}
			return out, nil
		}
		if send == maxBatchSends || ctx.Err() != nil {
			return nil, err
		}
	}
}

// rankSuggestions is found as the film is suggested it: each source once, the first batch's translation of it kept; none longer than suggestLongest, none glossary holds already and none fewer than suggestLeast of original's lines hold; the ones the most lines hold first, at most SuggestMax.
func rankSuggestions(found []core.Term, original []core.Segment, glossary []core.Term) []core.Term {
	type ranked struct {
		term  core.Term
		lines int
	}
	var out []ranked
	for _, t := range found {
		if !t.Valid() || utf8.RuneCountInString(t.Source) > suggestLongest || utf8.RuneCountInString(t.Target) > suggestLongest ||
			slices.ContainsFunc(glossary, func(g core.Term) bool { return core.SameTerm(g.Source, t.Source) }) ||
			slices.ContainsFunc(out, func(r ranked) bool { return core.SameTerm(r.term.Source, t.Source) }) {
			continue
		}
		n := 0
		for _, s := range original {
			if core.TermIn(s.Text, t.Source) {
				n++
			}
		}
		if n >= suggestLeast {
			t.Scope, t.Origin, t.Locked = core.ScopeFilm, core.OriginAuto, false
			out = append(out, ranked{t, n})
		}
	}
	slices.SortStableFunc(out, func(a, b ranked) int { return cmp.Compare(b.lines, a.lines) })
	terms := make([]core.Term, 0, min(len(out), SuggestMax))
	for _, r := range out[:min(len(out), SuggestMax)] {
		terms = append(terms, r.term)
	}
	return terms
}

// SuggestTerms suggests terms for the film's glossary through the translator (ChatTranslator.SuggestTerms).
func (t Tools) SuggestTerms(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Term, error) {
	return t.withOffline().SuggestTerms(ctx, original, translated, title, language)
}

// suggest asks p for suggestions for the film's glossary when the job's context asks for them (WithTermSuggestions) and p can (TermSuggester). ok is false when none were asked for, or the request failed, which keeps the suggestions the film had.
func suggest(ctx context.Context, p Pipeline, original, translated []core.Segment, title, language string) (terms []core.Term, ok bool) {
	s, can := p.(TermSuggester)
	if !suggestsTerms(ctx) || !can {
		return nil, false
	}
	terms, err := s.SuggestTerms(ctx, original, translated, title, language)
	return terms, err == nil
}

// The terms suggested for a film's glossary are kept in a hidden file beside its video, "<video name>.suggestions.json" with a dot before it (SuggestionsPath), with the sources the user dismissed, which are not suggested again. The job that translates the film writes it (KeepSuggestions); the 术语 tab dismisses a suggestion in it, and one added to the glossary is no longer shown, as the glossary holds it. It moves and goes to the Trash with the video (Companions).

// suggestionsVersion is the version of the suggestions file this Kando writes. A file of a later version is read for the fields this one knows and is not written over (ErrSuggestionsNewer).
const suggestionsVersion = 1

// ErrSuggestionsNewer is SaveSuggestions refusing to write over a suggestions file a newer Kando wrote, whose fields this one would drop.
var ErrSuggestionsNewer = errors.New("the suggested terms were written by a newer Kando")

// Suggestions are the terms suggested for a film's glossary, the ones the most lines hold first, and the sources the user dismissed.
type Suggestions struct {
	Terms     []core.Term `json:"terms"`
	Dismissed []string    `json:"dismissed,omitempty"`
}

// Dismiss is s with the suggestion for source dismissed: out of Terms, and among Dismissed, so it is not suggested again. s is not changed.
func (s Suggestions) Dismiss(source string) Suggestions {
	out := Suggestions{Terms: core.RemoveTerm(s.Terms, source), Dismissed: slices.Clone(s.Dismissed)}
	if source = strings.TrimSpace(source); source != "" && !slices.ContainsFunc(out.Dismissed, func(d string) bool { return core.SameTerm(d, source) }) {
		out.Dismissed = append(out.Dismissed, source)
	}
	return out
}

type suggestionsFile struct {
	Version int `json:"version"`
	Suggestions
}

// SuggestionsPath is the suggestions file of the film whose video, or whose subtitles Kando reads or writes by the video's name, is at path: ".<video name>.suggestions.json" beside it, as CorrectionsPath names the corrections.
func SuggestionsPath(path string) string {
	edits := CorrectionsPath(path)
	return filepath.Join(filepath.Dir(edits), strings.TrimSuffix(filepath.Base(edits), ".edits.json")+".suggestions.json")
}

// LoadSuggestions reads the suggestions file at path (SuggestionsPath). A file that is not there holds none.
func LoadSuggestions(path string) (Suggestions, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Suggestions{}, nil
	}
	if err != nil {
		return Suggestions{}, err
	}
	var f suggestionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Suggestions{}, fmt.Errorf("read %s: %w", path, err)
	}
	out := Suggestions{Dismissed: slices.DeleteFunc(f.Dismissed, func(d string) bool { return strings.TrimSpace(d) == "" })}
	for _, t := range f.Terms {
		if t.Valid() {
			t.Scope, t.Origin, t.Locked = core.ScopeFilm, core.OriginAuto, false
			out.Terms = append(out.Terms, t)
		}
	}
	return out, nil
}

// SaveSuggestions writes s to the suggestions file at path (SuggestionsPath), all of it or nothing (writeFileAtomic); with neither terms nor dismissed sources it removes the file. A file a newer Kando wrote is left as it is (ErrSuggestionsNewer).
func SaveSuggestions(path string, s Suggestions) error {
	if data, err := os.ReadFile(path); err == nil {
		var f suggestionsFile
		if json.Unmarshal(data, &f) == nil && f.Version > suggestionsVersion {
			return ErrSuggestionsNewer
		}
	}
	if len(s.Terms) == 0 && len(s.Dismissed) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	f := suggestionsFile{Version: suggestionsVersion, Suggestions: Suggestions{Terms: []core.Term{}, Dismissed: s.Dismissed}}
	for _, t := range s.Terms {
		if t.Valid() {
			f.Terms = append(f.Terms, t)
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// KeepSuggestions keeps terms as the suggestions of the film whose video is at video, in place of those it had, but for the sources the user dismissed, which stay dismissed and are left out.
func KeepSuggestions(video string, terms []core.Term) error {
	suggestionsMu.Lock()
	defer suggestionsMu.Unlock()
	path := SuggestionsPath(video)
	had, err := LoadSuggestions(path)
	if err != nil {
		return err
	}
	kept := Suggestions{Dismissed: had.Dismissed}
	for _, t := range terms {
		if !slices.ContainsFunc(had.Dismissed, func(d string) bool { return core.SameTerm(d, t.Source) }) {
			kept.Terms = append(kept.Terms, t)
		}
	}
	return SaveSuggestions(path, kept)
}

// suggestionsMu orders the changes of the suggestions files that read what is there and write it back (KeepSuggestions, DismissSuggestion), as a job keeps the suggestions of a film the window shows while the user dismisses one of them: neither writes over what the other wrote.
var suggestionsMu gosync.Mutex

// DismissSuggestion takes the term suggested for source off the suggestions kept beside video for good (Suggestions.Dismiss), as the file holds them now, and returns what it holds then.
func DismissSuggestion(video, source string) (Suggestions, error) {
	suggestionsMu.Lock()
	defer suggestionsMu.Unlock()
	path := SuggestionsPath(video)
	had, err := LoadSuggestions(path)
	if err != nil {
		return Suggestions{}, err
	}
	next := had.Dismiss(source)
	if err := SaveSuggestions(path, next); err != nil {
		return Suggestions{}, err
	}
	return next, nil
}
