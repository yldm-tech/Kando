package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildRoundTrips checks that a pack Build writes is a zip Stage installs as it is, with the files given, and that a manifest Validate refuses is Build's error.
func TestBuildRoundTrips(t *testing.T) {
	m := Manifest{Schema: 1, ID: "local.my-words", Kind: KindWordlist, Version: "2026.11.19", License: "Private", Name: Text{"en": "My words"},
		Wordlist: &Wordlist{Language: "ja", File: "words.tsv", Tiers: []Tier{{ID: "learning", Label: Text{"en": "Learning"}, Tone: ToneAccent}}}}
	data, p, err := Build(m, map[string][]byte{"words.tsv": []byte("猫\tlearning\n")})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != m.ID || len(p.Hash) != 64 {
		t.Fatalf("pack %+v", p)
	}
	file := filepath.Join(t.TempDir(), "my-words"+PackExt)
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	staged, got, err := Stage(root, file)
	if err != nil || got.Hash != p.Hash {
		t.Fatalf("staged %+v, %v", got, err)
	}
	if err := Swap(root, staged, &got); err != nil {
		t.Fatal(err)
	}
	if packs, issues := Scan(root); len(packs) != 1 || len(issues) != 0 || packs[0].ID != m.ID {
		t.Fatalf("scan %+v, %+v", packs, issues)
	}
	m.ID = "kando.mine"
	if _, _, err := Build(m, map[string][]byte{"words.tsv": []byte("猫\tlearning\n")}); !errors.Is(err, ErrReservedID) {
		t.Fatalf("a reserved id built: %v", err)
	}
}

func TestSlug(t *testing.T) {
	for name, want := range map[string]string{"My Anime Terms!": "my-anime-terms", "  JLPT N3 ": "jlpt-n3", "我的术语": "mine", "con": "mine"} {
		if got := Slug(name, "mine"); got != want {
			t.Errorf("Slug(%q) = %q, want %q", name, got, want)
		}
	}
}
