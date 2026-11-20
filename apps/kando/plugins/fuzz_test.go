package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzParse checks that no manifest makes Parse panic, and that one it accepts of a kind this version implements has that kind.
func FuzzParse(f *testing.F) {
	for _, m := range []string{glossaryManifest, wordlistManifest, serviceManifest, dictionaryManifest, `{"schema": 2, "id": "a.b", "kind": "script"}`, `{"kind": 1}`, `[]`, ``} {
		f.Add([]byte(m))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Parse(data)
		if err != nil {
			return
		}
		if !m.Kind.known() {
			t.Errorf("Parse accepted kind %q", m.Kind)
		}
		// Validate's checks of the fields run on whatever Parse accepts.
		_ = checkManifest(&m, false)
	})
}

// FuzzUnzip checks that no zip makes Stage panic, that a refused one leaves nothing in the plugins folder, and that one it stages is a folder of that plugins folder.
func FuzzUnzip(f *testing.F) {
	f.Add(zipBytes(f, []entry{{name: ManifestName, content: wordlistManifest}, {name: "jlpt.tsv", content: "猫\tn5\n"}}))
	f.Add(zipBytes(f, []entry{{name: ManifestName, content: glossaryManifest}}))
	f.Add(zipBytes(f, []entry{{name: ManifestName, content: wordlistManifest}, {name: "../jlpt.tsv", content: "x"}}))
	f.Add(zipBytes(f, []entry{{name: ManifestName, content: dictionaryManifest}, {name: "jmdict.zip", content: string(zipBytes(f, []entry{{name: "index.json", content: "{}"}}))}}))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pack.zip")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(dir, "plugins")
		staged, p, err := Stage(root, path)
		if err != nil {
			if staged != "" {
				t.Errorf("Stage failed with %v and returned %q", err, staged)
			}
			if left := leftovers(t, root); len(left) != 0 {
				t.Errorf("Stage failed with %v and left %q", err, left)
			}
			if errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stage: %v", err)
			}
			return
		}
		if filepath.Dir(staged) != root || !strings.HasPrefix(filepath.Base(staged), stagingPrefix) || p.Dir != staged {
			t.Errorf("Stage = %q, %+v", staged, p)
		}
	})
}
