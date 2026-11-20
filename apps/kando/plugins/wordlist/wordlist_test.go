package wordlist

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/plugins"
)

var tiers = []plugins.Tier{{ID: "n5", Tone: plugins.ToneSuccess}, {ID: "n1", Tone: plugins.ToneWarn}}

// TestParse checks a list's file: a word and its tier on each line, the columns after left out; comments, empty lines, lines without a tab and lines of a tier the pack does not declare left out; a word listed again keeps its first tier; and words found in any case.
func TestParse(t *testing.T) {
	file := "\ufeff猫\tn5\n# a comment\n\n犬 \t n1 \tnoun\r\nnotab\n鳥\tn9\n猫\tn1\nRiver\tn5\n"
	l, err := Parse(strings.NewReader(file), tiers)
	if err != nil {
		t.Fatal(err)
	}
	for word, want := range map[string]string{"猫": "n5", "犬": "n1", "river": "n5", " RIVER ": "n5"} {
		if got, ok := l.Tier(word); !ok || got.ID != want {
			t.Errorf("Tier(%q) = %+v, %v; want %s", word, got, ok, want)
		}
	}
	for _, word := range []string{"鳥", "notab", "# a comment", ""} {
		if got, ok := l.Tier(word); ok {
			t.Errorf("Tier(%q) = %+v; want none", word, got)
		}
	}
	if got, _ := l.Tier("犬"); got.Tone != plugins.ToneWarn {
		t.Errorf("the tier's tone: %+v", got)
	}
}

// TestParseRank checks the optional third column, a word's rank by frequency: a whole number from 1 to RankMax is kept, anything else leaves the word without a rank but in its tier, the columns after it are left out, and a word listed again keeps its first rank.
func TestParseRank(t *testing.T) {
	file := "猫\tn5\t12\n犬\tn1\t 3 \tnoun\n鳥\tn5\t0\n魚\tn5\t-4\n馬\tn5\t1000001\n牛\tn5\tmany\n羊\tn5\n虎\tn1\t1000000\n猫\tn1\t1\n"
	l, err := Parse(strings.NewReader(file), tiers)
	if err != nil {
		t.Fatal(err)
	}
	for word, want := range map[string]int{"猫": 12, "犬": 3, "鳥": 0, "魚": 0, "馬": 0, "牛": 0, "羊": 0, "虎": RankMax, "missing": 0} {
		if got := l.Rank(word); got != want {
			t.Errorf("Rank(%q) = %d; want %d", word, got, want)
		}
	}
	for _, word := range []string{"鳥", "魚", "馬", "牛", "羊"} {
		if got, ok := l.Tier(word); !ok || got.ID != "n5" {
			t.Errorf("Tier(%q) = %+v, %v; want n5", word, got, ok)
		}
	}
	if got, _ := l.Tier("猫"); got.ID != "n5" {
		t.Errorf("a word listed again changed its tier: %+v", got)
	}
}

// TestParseEmpty checks that a list with no word of a declared tier is refused.
func TestParseEmpty(t *testing.T) {
	for _, file := range []string{"", "# only a comment\n", "猫\tn9\n", "猫\n"} {
		if _, err := Parse(strings.NewReader(file), tiers); !errors.Is(err, ErrEmpty) {
			t.Errorf("Parse(%q): %v; want ErrEmpty", file, err)
		}
	}
}

// TestOpen checks that Open reads the file the pack names in its folder.
func TestOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jlpt.tsv"), []byte("猫\tn5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := plugins.Pack{Manifest: plugins.Manifest{ID: "example.jlpt", Kind: plugins.KindWordlist, Wordlist: &plugins.Wordlist{Language: "ja", File: "jlpt.tsv", Tiers: tiers}}, Dir: dir}
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := l.Tier("猫"); !ok || got.ID != "n5" {
		t.Fatalf("Tier: %+v, %v", got, ok)
	}
	p.Wordlist.File = "missing.tsv"
	if _, err := Open(p); err == nil {
		t.Fatal("a missing file opened")
	}
}
