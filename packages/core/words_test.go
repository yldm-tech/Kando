package core

import (
	"strings"
	"testing"
)

func TestWordTokens(t *testing.T) {
	cases := []struct {
		line string
		want []WordToken
	}{
		{"Every river begins as a trickle.", []WordToken{{"Every", true}, {" ", false}, {"river", true}, {" ", false}, {"begins", true}, {" ", false}, {"as", true}, {" ", false}, {"a", true}, {" ", false}, {"trickle", true}, {".", false}}},
		{"Don't—it's well-known, 'quoted' 42", []WordToken{{"Don't", true}, {"—", false}, {"it's", true}, {" ", false}, {"well-known", true}, {", '", false}, {"quoted", true}, {"' 42", false}}},
		{"Ça va? Très bien!", []WordToken{{"Ça", true}, {" ", false}, {"va", true}, {"? ", false}, {"Très", true}, {" ", false}, {"bien", true}, {"!", false}}},
		{"今日はいい天気", []WordToken{{"今日はいい天気", false}}},
		{"コーヒー, please", []WordToken{{"コーヒー, ", false}, {"please", true}}},
		{"안녕 세상", []WordToken{{"안녕", true}, {" ", false}, {"세상", true}}},
		{"", nil},
	}
	for _, c := range cases {
		got := WordTokens(c.line)
		if len(got) != len(c.want) {
			t.Fatalf("WordTokens(%q) = %+v, want %+v", c.line, got, c.want)
		}
		var joined strings.Builder
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("WordTokens(%q)[%d] = %+v, want %+v", c.line, i, got[i], c.want[i])
			}
			joined.WriteString(got[i].Text)
		}
		if joined.String() != c.line {
			t.Fatalf("WordTokens(%q) joins to %q", c.line, joined.String())
		}
	}
}
