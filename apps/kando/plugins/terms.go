package plugins

import (
	"strings"

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

// termKey is what AddTerms tells the words of a term by: core.SameTerm's spaces and case, folded with strings.ToLower.
func termKey(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}
