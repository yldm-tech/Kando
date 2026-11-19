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
