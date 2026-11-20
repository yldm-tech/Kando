package plugins

import (
	"slices"
	"strings"
	"unicode/utf8"

	kcore "kando/packages/core"
)

// The limits of kando.glossary.suggest: how many terms a script plugin proposes in one call, how many characters a term's words and its translation may have, as a glossary holds names and terms, not phrases (the limit of the AI service's own suggestions, packages/media's suggestLongest), and how many terms the plugins may have proposed for a film while it stays open, which a plugin that proposes again and again does not grow past.
const (
	SuggestMax     = 20
	SuggestLongest = 60
	SuggestHeld    = 60
)

// Suggested reads what a script plugin sent kando.glossary.suggest as v: {film, terms: [{source, target}]}, film the id the plugin knows the open film by (FilmID), which may be left out, and at most SuggestMax terms, each its words and their translation, on one line without invisible or control characters, the spaces in them collapsed, at most SuggestLongest characters. The terms are the film's own, suggested by a machine (kcore.OriginAuto) and not locked, each source once, the first kept; ok is false for anything else, which the call is refused for as a whole.
func Suggested(v any) (film string, terms []kcore.Term, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return "", nil, false
	}
	film, isString := m["film"].(string)
	if m["film"] != nil && !isString {
		return "", nil, false
	}
	list, isList := m["terms"].([]any)
	if !isList || len(list) > SuggestMax {
		return "", nil, false
	}
	terms = []kcore.Term{}
	for _, e := range list {
		t, isMap := e.(map[string]any)
		if !isMap {
			return "", nil, false
		}
		source, ok1 := suggestedText(t["source"])
		target, ok2 := suggestedText(t["target"])
		if !ok1 || !ok2 {
			return "", nil, false
		}
		if !slices.ContainsFunc(terms, func(h kcore.Term) bool { return kcore.SameTerm(h.Source, source) }) {
			terms = append(terms, kcore.Term{Source: source, Target: target, Scope: kcore.ScopeFilm, Origin: kcore.OriginAuto})
		}
	}
	return film, terms, true
}

// suggestedText is v, a term's words or their translation as a plugin sent them, cleaned (cleanText) with its spaces collapsed, and whether it is a string that holds between 1 and SuggestLongest characters then.
func suggestedText(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.Join(strings.Fields(cleanText(s, false)), " ")
	n := utf8.RuneCountInString(s)
	return s, n >= 1 && n <= SuggestLongest
}
