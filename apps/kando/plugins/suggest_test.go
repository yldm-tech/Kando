package plugins

import (
	"reflect"
	"strings"
	"testing"

	kcore "kando/packages/core"
)

// TestSuggested checks what kando.glossary.suggest takes: at most SuggestMax terms, each a source and a target of 1 to SuggestLongest characters once cleaned, the film's own, by a machine and not locked, each source once; anything else is refused whole.
func TestSuggested(t *testing.T) {
	term := func(source, target string) any { return map[string]any{"source": source, "target": target} }
	film, terms, ok := Suggested(map[string]any{"film": "f_1", "terms": []any{term(" Nile\u200b ", "尼罗 \t 河"), term("nile", "Nile again"), term("delta", "三角洲")}})
	want := []kcore.Term{{Source: "Nile", Target: "尼罗 河", Scope: kcore.ScopeFilm, Origin: kcore.OriginAuto}, {Source: "delta", Target: "三角洲", Scope: kcore.ScopeFilm, Origin: kcore.OriginAuto}}
	if !ok || film != "f_1" || !reflect.DeepEqual(terms, want) {
		t.Fatalf("Suggested = %q, %+v, %v", film, terms, ok)
	}
	if film, terms, ok := Suggested(map[string]any{"terms": []any{}}); !ok || film != "" || len(terms) != 0 {
		t.Errorf("no terms and no film: %q, %+v, %v", film, terms, ok)
	}
	many := make([]any, SuggestMax)
	for i := range many {
		many[i] = term(strings.Repeat("a", i+1), "x")
	}
	if _, terms, ok := Suggested(map[string]any{"terms": many}); !ok || len(terms) != SuggestMax {
		t.Errorf("SuggestMax terms: %d, %v", len(terms), ok)
	}
	long := strings.Repeat("字", SuggestLongest)
	if _, _, ok := Suggested(map[string]any{"terms": []any{term(long, long)}}); !ok {
		t.Error("terms of SuggestLongest characters refused")
	}
	for name, v := range map[string]any{
		"not an object":        []any{},
		"no terms":             map[string]any{"film": "f_1"},
		"terms not a list":     map[string]any{"terms": "Nile"},
		"too many":             map[string]any{"terms": append(many, term("b", "y"))},
		"film not a string":    map[string]any{"film": 3.0, "terms": []any{}},
		"a term not an object": map[string]any{"terms": []any{"Nile"}},
		"no target":            map[string]any{"terms": []any{map[string]any{"source": "Nile"}}},
		"empty source":         map[string]any{"terms": []any{term(" \u200b ", "尼罗河")}},
		"target not a string":  map[string]any{"terms": []any{map[string]any{"source": "Nile", "target": 1.0}}},
		"source too long":      map[string]any{"terms": []any{term(long+"字", "x")}},
		"target too long":      map[string]any{"terms": []any{term("Nile", long+"字")}},
	} {
		if _, _, ok := Suggested(v); ok {
			t.Errorf("%s: taken", name)
		}
	}
}
