package core

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A glossary is how the user wants a film's names and terms translated, so they read the same in every line of a film, in every film of a series and across the library: a term in the film's language (Source) and its translation (Target). The translation is told the terms its lines hold (media.WithGlossary).

// TermScope is which films a term is for: the film it was added in, the films of that film's series, or every film.
type TermScope string

const (
	ScopeFilm   TermScope = "film"
	ScopeSeries TermScope = "series"
	ScopeGlobal TermScope = "global"
)

// Known reports whether s is one of the scopes Kando knows.
func (s TermScope) Known() bool {
	return s == ScopeFilm || s == ScopeSeries || s == ScopeGlobal
}

// TermOrigin is where a term came from: the user, or a machine pass that suggested it, which may change it again unless it is locked.
type TermOrigin string

const (
	OriginUser TermOrigin = "user"
	OriginAuto TermOrigin = "auto"
)

// Term is one entry of a glossary: Source as the film's lines write it and Target as the translation is to write it. A Locked term is the user's to change alone: the translation must use it as it is, and no machine pass changes or removes it.
type Term struct {
	Source string     `json:"source"`
	Target string     `json:"target"`
	Scope  TermScope  `json:"scope"`
	Origin TermOrigin `json:"origin,omitempty"`
	Locked bool       `json:"locked,omitempty"`
}

// Valid reports whether t has a source and a target.
func (t Term) Valid() bool {
	return strings.TrimSpace(t.Source) != "" && strings.TrimSpace(t.Target) != ""
}

// SameTerm reports whether a and b are entries for the same words, whatever their case and the spaces around them.
func SameTerm(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// MergeTerms is the glossary of a film: its own terms, then its series', then the global ones, each source once, a narrower scope's entry taking the place of a wider one's for the same words. Entries without a source or a target are left out.
func MergeTerms(film, series, global []Term) []Term {
	var out []Term
	for _, list := range [][]Term{film, series, global} {
		for _, t := range list {
			if !t.Valid() || slices.ContainsFunc(out, func(o Term) bool { return SameTerm(o.Source, t.Source) }) {
				continue
			}
			t.Source, t.Target = strings.TrimSpace(t.Source), strings.TrimSpace(t.Target)
			out = append(out, t)
		}
	}
	return out
}

// TermsIn is the terms of glossary that one of texts holds (TermIn), in the glossary's order, which is what a batch of lines is translated with: a long film's whole glossary would crowd the request.
func TermsIn(glossary []Term, texts []string) []Term {
	var out []Term
	for _, t := range glossary {
		if slices.ContainsFunc(texts, func(text string) bool { return TermIn(text, t.Source) }) {
			out = append(out, t)
		}
	}
	return out
}

// TermIn reports whether text holds source, whatever its case. A source that starts with a letter or a digit of a script written with spaces must start a word in text, so "art" is not found in "start"; its end is not checked, so "delta" is found in "deltas". A source in Chinese or Japanese is found anywhere.
func TermIn(text, source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return false
	}
	text = strings.ToLower(text)
	first, _ := utf8.DecodeRuneInString(source)
	wordy := (unicode.IsLetter(first) || unicode.IsDigit(first)) && !unspacedScript(first)
	for from := 0; ; {
		i := strings.Index(text[from:], source)
		if i < 0 {
			return false
		}
		at := from + i
		if !wordy || at == 0 {
			return true
		}
		before, _ := utf8.DecodeLastRuneInString(text[:at])
		if !unicode.IsLetter(before) && !unicode.IsDigit(before) {
			return true
		}
		from = at + len(source)
	}
}

// unspacedScript reports whether r is of a script written without spaces between words.
func unspacedScript(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai)
}

// SetTerm is terms with t in place of the entry for the same source, or added at the end when there is none, and without it when t has no target. terms is not changed.
func SetTerm(terms []Term, t Term) []Term {
	out := slices.Clone(terms)
	i := slices.IndexFunc(out, func(o Term) bool { return SameTerm(o.Source, t.Source) })
	switch {
	case !t.Valid() && i >= 0:
		return slices.Delete(out, i, i+1)
	case !t.Valid():
		return out
	case i >= 0:
		out[i] = t
		return out
	}
	return append(out, t)
}

// RemoveTerm is terms without the entry for source. terms is not changed.
func RemoveTerm(terms []Term, source string) []Term {
	return slices.DeleteFunc(slices.Clone(terms), func(o Term) bool { return SameTerm(o.Source, source) })
}
