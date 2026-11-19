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
