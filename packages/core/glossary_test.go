package core

import (
	"reflect"
	"testing"
)

func TestMergeTermsKeepsTheNarrowestScopeOfEachSource(t *testing.T) {
	film := []Term{{Source: "meander", Target: "曲流", Scope: ScopeFilm}, {Source: " ", Target: "x", Scope: ScopeFilm}}
	series := []Term{{Source: "Meander", Target: "河曲", Scope: ScopeSeries}, {Source: "delta", Target: "三角洲", Scope: ScopeSeries}}
	global := []Term{{Source: "DELTA", Target: "德尔塔", Scope: ScopeGlobal}, {Source: " sediment ", Target: " 泥沙 ", Scope: ScopeGlobal, Locked: true}, {Source: "silt", Scope: ScopeGlobal}}
	got := MergeTerms(film, series, global)
	want := []Term{
		{Source: "meander", Target: "曲流", Scope: ScopeFilm},
		{Source: "delta", Target: "三角洲", Scope: ScopeSeries},
		{Source: "sediment", Target: "泥沙", Scope: ScopeGlobal, Locked: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeTerms = %+v, want %+v", got, want)
	}
}

func TestTermInFindsAWordAtItsStartOnly(t *testing.T) {
	for _, c := range []struct {
		text, source string
		want         bool
	}{
		{"The river forms a meander here.", "meander", true},
		{"Two Meanders meet.", "meander", true},
		{"Let's start the art class.", "art", true},
		{"Let's start now.", "art", false},
		{"An oxbow lake forms.", "Oxbow Lake", true},
		{"牛轭湖形成了", "牛轭", true},
		{"東京タワーへ行く", "タワー", true},
		{"", "delta", false},
		{"delta", "  ", false},
		{"v2.0 ships", "2.0", false},
		{"v 2.0 ships", "2.0", true},
	} {
		if got := TermIn(c.text, c.source); got != c.want {
			t.Errorf("TermIn(%q, %q) = %v, want %v", c.text, c.source, got, c.want)
		}
	}
}

func TestTermsInKeepsTheTermsTheTextsHold(t *testing.T) {
	glossary := []Term{{Source: "meander", Target: "河曲"}, {Source: "delta", Target: "三角洲"}, {Source: "sediment", Target: "泥沙"}}
	got := TermsIn(glossary, []string{"Sediment piles up.", "The meander moves."})
	want := []Term{glossary[0], glossary[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TermsIn = %+v, want %+v", got, want)
	}
	if got := TermsIn(glossary, []string{"Nothing here."}); got != nil {
		t.Fatalf("TermsIn without a term = %+v, want nil", got)
	}
}

func TestSetTermReplacesAddsAndRemoves(t *testing.T) {
	terms := []Term{{Source: "meander", Target: "曲流"}}
	replaced := SetTerm(terms, Term{Source: "Meander", Target: "河曲", Locked: true})
	if want := []Term{{Source: "Meander", Target: "河曲", Locked: true}}; !reflect.DeepEqual(replaced, want) {
		t.Fatalf("SetTerm replacing = %+v, want %+v", replaced, want)
	}
	if terms[0].Target != "曲流" {
		t.Fatal("SetTerm changed the terms it was given")
	}
	added := SetTerm(terms, Term{Source: "delta", Target: "三角洲"})
	if len(added) != 2 || added[1].Source != "delta" {
		t.Fatalf("SetTerm adding = %+v", added)
	}
	if removed := SetTerm(terms, Term{Source: "meander"}); len(removed) != 0 {
		t.Fatalf("SetTerm without a target = %+v, want none", removed)
	}
	if kept := SetTerm(terms, Term{Source: "delta"}); !reflect.DeepEqual(kept, terms) {
		t.Fatalf("SetTerm of an empty new term = %+v, want %+v", kept, terms)
	}
	if removed := RemoveTerm(added, " DELTA "); !reflect.DeepEqual(removed, terms) {
		t.Fatalf("RemoveTerm = %+v, want %+v", removed, terms)
	}
}
