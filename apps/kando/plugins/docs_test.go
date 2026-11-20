package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docExamples are the JSON blocks of section 4 (清单格式) of docs/plugin-system.md, in order: a manifest is a block that is an object, a fragment one that is the keys of an object alone, which the doc shows as what a plugin adds to the manifest before it.
func docExamples(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "plugin-system.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(data), "\n## 4. ")
	if !ok {
		t.Fatal("no section 4 in docs/plugin-system.md")
	}
	section, _, _ = strings.Cut(section, "\n## 5. ")
	var blocks []string
	for {
		_, rest, ok := strings.Cut(section, "\n```json\n")
		if !ok {
			return blocks
		}
		block, after, ok := strings.Cut(rest, "\n```")
		if !ok {
			t.Fatal("a JSON block of section 4 is not closed")
		}
		blocks = append(blocks, block)
		section = after
	}
}

// TestDocManifestExamples checks that every manifest docs/plugin-system.md shows in section 4 is one Parse reads and Validate's checks of its fields take, so the doc's examples cannot drift from the rules they illustrate: a fragment is checked over the manifest before it, its keys in place of that one's, and a script plugin's package hash, which the doc elides, is filled in.
func TestDocManifestExamples(t *testing.T) {
	blocks := docExamples(t)
	if len(blocks) < 6 {
		t.Fatalf("found %d JSON blocks in section 4", len(blocks))
	}
	var last map[string]any
	for i, block := range blocks {
		var m map[string]any
		trimmed := strings.TrimSpace(block)
		fragment := !strings.HasPrefix(trimmed, "{")
		if fragment {
			trimmed = "{" + trimmed + "}"
		}
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			t.Fatalf("block %d is not JSON: %v\n%s", i+1, err, block)
		}
		if fragment {
			if last == nil {
				t.Fatalf("block %d is a fragment with no manifest before it", i+1)
			}
			merged := map[string]any{}
			for k, v := range last {
				merged[k] = v
			}
			for k, v := range m {
				merged[k] = v
			}
			m = merged
		} else {
			last = m
		}
		if s, ok := m["script"].(map[string]any); ok {
			s["sha256"] = strings.Repeat("0", 64)
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := Parse(data)
		if err == nil {
			err = checkManifest(&manifest)
		}
		if err != nil {
			t.Errorf("block %d (%v) is refused: %v", i+1, m["id"], err)
		}
	}
}
