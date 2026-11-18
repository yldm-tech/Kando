package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kando/packages/core"
)

// The glossary a job translates with (core.Term) reaches the translator in the context it runs under (WithGlossary): each batch is sent with the terms its lines hold (core.TermsIn), a locked term as one the translation must write as it is, any other as the translation the user prefers.

type glossaryKey struct{}

// WithGlossary returns ctx carrying terms, a film's glossary (core.MergeTerms), which ChatTranslator.Translate translates its lines with.
func WithGlossary(ctx context.Context, terms []core.Term) context.Context {
	return context.WithValue(ctx, glossaryKey{}, terms)
}

// glossaryOf is the glossary WithGlossary put in ctx, nil for none.
func glossaryOf(ctx context.Context) []core.Term {
	terms, _ := ctx.Value(glossaryKey{}).([]core.Term)
	return terms
}

// glossaryPrompt is the part of a translation's instruction that gives the model terms, "" for none: the locked ones as translations it must use as they are, the others as the ones it uses unless a line plainly means something else by the words.
func glossaryPrompt(terms []core.Term) string {
	type entry struct {
		Term        string `json:"term"`
		Translation string `json:"translation"`
	}
	var locked, preferred []entry
	for _, t := range terms {
		e := entry{Term: t.Source, Translation: t.Target}
		if t.Locked {
			locked = append(locked, e)
		} else {
			preferred = append(preferred, e)
		}
	}
	var parts []string
	if len(locked) > 0 {
		encoded, _ := json.Marshal(locked)
		parts = append(parts, "Glossary the user fixed. Wherever a line holds one of these terms, translate it exactly as given, in every line:\n"+string(encoded))
	}
	if len(preferred) > 0 {
		encoded, _ := json.Marshal(preferred)
		parts = append(parts, "Glossary of this video. Translate these terms as given, so they read the same in every line, unless a line plainly uses the words in another sense:\n"+string(encoded))
	}
	return strings.Join(parts, "\n\n")
}

// proofreadTermsMax bounds how many of a glossary's terms a proofreading request names.
const proofreadTermsMax = 100

// proofreadTerms is what a proofreading request is told of the glossary, "" for none: the terms as the film's lines write them, so a term the speech recognizer misheard is corrected to the spelling the user gave. Only the first proofreadTermsMax are named, the film's own first (core.MergeTerms).
func proofreadTerms(terms []core.Term) string {
	if len(terms) == 0 {
		return ""
	}
	sources := make([]string, 0, min(len(terms), proofreadTermsMax))
	for _, t := range terms[:min(len(terms), proofreadTermsMax)] {
		sources = append(sources, t.Source)
	}
	encoded, _ := json.Marshal(sources)
	return "\n\nNames and terms of this video, spelled as they are to be written. A word misheard for one of them is corrected to it:\n" + string(encoded)
}

// A film's own terms (core.ScopeFilm) are kept in a hidden file beside its video, "<video>.terms.json" with a dot before it (TermsPath), which no job writes, so processing the film again keeps them. It moves and goes to the Trash with the video (Companions). The terms of a series and the global ones are the app's.

// termsVersion is the version of the terms file this Kando writes. A file of a later version is read for the fields this one knows and is not written over (ErrTermsNewer).
const termsVersion = 1

// ErrTermsNewer is SaveTerms refusing to write over a terms file a newer Kando wrote, whose fields this one would drop.
var ErrTermsNewer = errors.New("the glossary was written by a newer Kando")

type termsFile struct {
	Version int         `json:"version"`
	Terms   []core.Term `json:"terms"`
}

// TermsPath is the terms file of the film whose video, or whose subtitles Kando reads or writes by the video's name, is at path: ".<video>.terms.json" beside it, as CorrectionsPath names the corrections.
func TermsPath(path string) string {
	edits := CorrectionsPath(path)
	return filepath.Join(filepath.Dir(edits), strings.TrimSuffix(filepath.Base(edits), ".edits.json")+".terms.json")
}

// LoadTerms reads the terms file at path (TermsPath), every term in it a film's own. A file that is not there holds none.
func LoadTerms(path string) ([]core.Term, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f termsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := make([]core.Term, 0, len(f.Terms))
	for _, t := range f.Terms {
		if t.Valid() {
			t.Scope = core.ScopeFilm
			out = append(out, t)
		}
	}
	return out, nil
}

// SaveTerms writes terms, a film's own, to the terms file at path (TermsPath), all of it or nothing (writeFileAtomic). With none it removes the file. A file a newer Kando wrote is left as it is (ErrTermsNewer).
func SaveTerms(path string, terms []core.Term) error {
	if data, err := os.ReadFile(path); err == nil {
		var f termsFile
		if json.Unmarshal(data, &f) == nil && f.Version > termsVersion {
			return ErrTermsNewer
		}
	}
	f := termsFile{Version: termsVersion, Terms: []core.Term{}}
	for _, t := range terms {
		if t.Valid() {
			t.Scope = core.ScopeFilm
			f.Terms = append(f.Terms, t)
		}
	}
	if len(f.Terms) == 0 {
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
