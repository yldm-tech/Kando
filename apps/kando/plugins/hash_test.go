package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func packHash(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := writePack(t, files)
	names, err := listFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := hashFiles(dir, names, one(&budget{bytes: 1 << 20, entries: 16}))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestHash checks that the hash depends on the files' names and bytes alone, and that moving bytes from a name to its content, or from one file to the next, changes it.
func TestHash(t *testing.T) {
	a := packHash(t, map[string]string{"a.md": "one", "b.md": "two"})
	if b := packHash(t, map[string]string{"b.md": "two", "a.md": "one"}); a != b {
		t.Errorf("the same files hash differently: %s, %s", a, b)
	}
	// The hash of the same pack is fixed, so a grant tied to it keeps across versions of Kando.
	if want := "65a916c5e2f85096ae5cfc3aa6298fe3a9cca3fcc1732fba3bc4c6fa68f2ccf5"; a != want {
		t.Errorf("hash = %q, want %q", a, want)
	}
	for name, files := range map[string]map[string]string{
		"content":       {"a.md": "one", "b.md": "twO"},
		"name":          {"a.md": "one", "c.md": "two"},
		"bytes moved":   {"a.md": "onet", "b.md": "wo"},
		"one file more": {"a.md": "one", "b.md": "two", "c.md": ""},
	} {
		if packHash(t, files) == a {
			t.Errorf("%s: the hash did not change", name)
		}
	}
}

func TestHashChecksTheBudget(t *testing.T) {
	dir := writePack(t, map[string]string{"a.md": "12345"})
	_, err := hashFiles(dir, []string{"a.md"}, one(&budget{bytes: 4, entries: 1}))
	wantErr(t, "over the budget", err, ErrTooLarge)
	if err := os.Remove(filepath.Join(dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := hashFiles(dir, []string{"a.md"}, one(&budget{bytes: 10, entries: 1})); err == nil {
		t.Error("a missing file hashed")
	}
}

// one is a budgetOf for hashFiles that gives b for every file.
func one(b *budget) func(string) *budget {
	return func(string) *budget { return b }
}
