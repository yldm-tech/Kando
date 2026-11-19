package plugins

import (
	"slices"
	"strings"
	"unicode"

	"kando/packages/core"
)

// SameLanguage reports whether a glossary pack's language tag and a job's are the same language: the same tag, whatever its case, or the same language where one of them names no script or region, so a pack into zh is for zh-Hans and zh-Hant alike, but one into zh-Hans is not for zh-Hant.
func SameLanguage(pack, tag string) bool {
	if strings.EqualFold(pack, tag) {
		return true
	}
	if strings.Contains(pack, "-") && strings.Contains(tag, "-") {
		return false
	}
	return core.BaseLanguage(pack) == core.BaseLanguage(tag)
}

// AddTerms is terms, a film's glossary as core.MergeTerms made it, with the terms of more after it whose words it does not have yet, each source once and the first kept, as MergeTerms keeps them; entries without a source or a target are left out. It finds the words in a map, as a glossary pack brings up to thousands of terms and MergeTerms compares each with every one before it.
func AddTerms(terms, more []core.Term) []core.Term {
	if len(more) == 0 {
		return terms
	}
	out := make([]core.Term, len(terms), len(terms)+len(more))
	copy(out, terms)
	seen := make(map[string]bool, len(terms)+len(more))
	for _, t := range terms {
		seen[termKey(t.Source)] = true
	}
	for _, t := range more {
		key := termKey(t.Source)
		if !t.Valid() || seen[key] {
			continue
		}
		seen[key] = true
		t.Source, t.Target = strings.TrimSpace(t.Source), strings.TrimSpace(t.Target)
		out = append(out, t)
	}
	return out
}

// TermsIn is the terms of glossary that one of texts holds, as core.TermsIn finds them (core.TermIn), in the glossary's order. A glossary pack brings up to thousands of terms, so each is first looked for in all the texts at once, lowered once, and only one found there is checked line by line.
func TermsIn(glossary []core.Term, texts []string) []core.Term {
	if len(glossary) == 0 || len(texts) == 0 {
		return nil
	}
	all := strings.ToLower(strings.Join(texts, "\n"))
	var out []core.Term
	for _, t := range glossary {
		source := strings.ToLower(strings.TrimSpace(t.Source))
		if source == "" || !strings.Contains(all, source) {
			continue
		}
		if slices.ContainsFunc(texts, func(text string) bool { return core.TermIn(text, t.Source) }) {
			out = append(out, t)
		}
	}
	return out
}

// termKey is what AddTerms tells the words of a term by: core.SameTerm's spaces and case. SameTerm compares with strings.EqualFold, which takes two letters as the same when one is in the other's simple case folding (unicode.SimpleFold), as ſ and s or ς and σ, which strings.ToLower keeps apart; so each letter is made the least of its folding, which every letter of that folding shares.
func termKey(source string) string {
	return strings.Map(foldRune, strings.TrimSpace(source))
}

// foldRune is the least rune of r's simple case folding (unicode.SimpleFold), the same for every rune strings.EqualFold takes as r.
func foldRune(r rune) rune {
	least := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		least = min(least, f)
	}
	return least
}
