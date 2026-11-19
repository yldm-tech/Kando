package plugins

import (
	"fmt"
	"testing"

	"kando/packages/core"
)

func TestSameLanguage(t *testing.T) {
	for _, c := range []struct {
		pack, tag string
		want      bool
	}{
		{"zh-Hans", "zh-hans", true},
		{"zh", "zh-Hans", true},
		{"zh", "zh-Hant", true},
		{"zh-Hans", "zh", true},
		{"zh-Hans", "zh-Hant", false},
		{"ja", "zh-Hans", false},
		{"en", "en", true},
	} {
		if got := SameLanguage(c.pack, c.tag); got != c.want {
			t.Errorf("SameLanguage(%q, %q) = %v, want %v", c.pack, c.tag, got, c.want)
		}
	}
}

// TestAddTerms checks that AddTerms keeps the terms it is given first and in their order, adds the others' new words after them each once, whatever their case and spaces, and leaves out entries without a source or a target, as core.MergeTerms does.
func TestAddTerms(t *testing.T) {
	user := core.MergeTerms([]core.Term{{Source: "Senpai", Target: "学长", Scope: core.ScopeFilm, Origin: core.OriginUser}}, nil, nil)
	pack := []core.Term{
		{Source: " senpai ", Target: "前辈", Origin: core.OriginPack},
		{Source: "お兄ちゃん ", Target: " 哥哥", Origin: core.OriginPack},
		{Source: "お兄ちゃん", Target: "兄长", Origin: core.OriginPack},
		{Source: "", Target: "空"},
		{Source: "空", Target: ""},
	}
	got := AddTerms(user, pack)
	want := []core.Term{user[0], {Source: "お兄ちゃん", Target: "哥哥", Origin: core.OriginPack}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("AddTerms = %+v, want %+v", got, want)
	}
	if fmt.Sprint(core.MergeTerms(user, nil, pack)) != fmt.Sprint(got) {
		t.Errorf("AddTerms differs from core.MergeTerms: %+v", core.MergeTerms(user, nil, pack))
	}
	if got := AddTerms(user, nil); len(got) != 1 {
		t.Errorf("AddTerms without more = %+v", got)
	}
}

// TestTermKeyFoldsAsSameTerm checks that AddTerms tells words apart as core.SameTerm does (strings.EqualFold), letters strings.ToLower keeps apart included: the long s and s, the final sigma and sigma, the Kelvin sign and k, the title-case Dž and its other cases.
func TestTermKeyFoldsAsSameTerm(t *testing.T) {
	for _, c := range [][2]string{{"ſensei", "Sensei"}, {"ΛΌΓΟΣ", "λόγος"}, {"Kelvin", "kelvin"}, {"ǅungla", "ǆungla"}, {"ǄUNGLA", "ǆungla"}, {" Straße ", "STRASSE"}} {
		same := core.SameTerm(c[0], c[1])
		if got := termKey(c[0]) == termKey(c[1]); got != same {
			t.Errorf("termKey(%q) == termKey(%q) is %v, core.SameTerm %v", c[0], c[1], got, same)
		}
		terms := AddTerms([]core.Term{{Source: c[0], Target: "a"}}, []core.Term{{Source: c[1], Target: "b"}})
		if (len(terms) == 1) != same {
			t.Errorf("AddTerms of %q and %q = %+v, core.SameTerm %v", c[0], c[1], terms, same)
		}
	}
}

// TestTermsIn checks that TermsIn finds the terms core.TermsIn finds, in the glossary's order: a word's start must start a word of the line, case does not matter, a Japanese term is found anywhere, and a term no line holds, or one without words, is left out.
func TestTermsIn(t *testing.T) {
	glossary := []core.Term{
		{Source: "art", Target: "艺术"},
		{Source: "Delta", Target: "三角洲"},
		{Source: "お兄ちゃん", Target: "哥哥"},
		{Source: "先輩", Target: "前辈"},
		{Source: " ", Target: "空"},
	}
	texts := []string{"The start of it", "Two DELTAS", "お兄ちゃん、待って"}
	got := TermsIn(glossary, texts)
	if want := []core.Term{glossary[1], glossary[2]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("TermsIn = %+v, want %+v", got, want)
	}
	if want := core.TermsIn(glossary, texts); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("TermsIn differs from core.TermsIn: %+v", want)
	}
	if got := TermsIn(glossary, nil); len(got) != 0 {
		t.Errorf("TermsIn without lines = %+v", got)
	}
}

// BenchmarkAddTerms is a full glossary pack (termsMax terms) joining a glossary of a hundred terms, as the player's 术语 tab does at each change while it shows.
func BenchmarkAddTerms(b *testing.B) {
	var user, pack []core.Term
	for i := range 100 {
		user = append(user, core.Term{Source: fmt.Sprintf("user %d", i), Target: "t"})
	}
	for i := range termsMax {
		pack = append(pack, core.Term{Source: fmt.Sprintf("pack %d", i), Target: "t"})
	}
	for b.Loop() {
		AddTerms(user, pack)
	}
}
