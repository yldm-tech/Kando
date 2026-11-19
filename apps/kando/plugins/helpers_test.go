package plugins

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// glossaryManifest is a valid glossary pack's manifest, which the tests change one field of at a time.
const glossaryManifest = `{
  "schema": 1,
  "id": "example.anime-ja-zh",
  "kind": "glossary",
  "version": "1.2.0",
  "license": "CC-BY-4.0",
  "name": {"en": "Anime terms", "zh-Hans": "动画常用词"},
  "glossary": {"from": "ja", "to": "zh-Hans", "terms": [{"source": "先輩", "target": "前辈"}]}
}`

const wordlistManifest = `{
  "schema": 1,
  "id": "example.jlpt",
  "kind": "wordlist",
  "version": "1.0.0",
  "license": "CC-BY-4.0",
  "name": {"en": "JLPT levels"},
  "wordlist": {"language": "ja", "file": "jlpt.tsv", "tiers": [{"id": "n5", "label": {"en": "N5"}, "tone": "success"}, {"id": "n1", "label": {"en": "N1"}, "tone": "warn"}]}
}`

const serviceManifest = `{
  "schema": 1,
  "id": "acme.corp-gateway",
  "kind": "ai-service",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Company LLM gateway"},
  "service": {"baseURL": "https://llm.example.com/v1", "model": "qwen3-32b", "keyURL": "https://llm.example.com/keys", "group": "cloud"}
}`

const dictionaryManifest = `{
  "schema": 1,
  "id": "example.jmdict-en",
  "kind": "dictionary",
  "version": "2026.11.0",
  "license": "CC-BY-SA-4.0",
  "name": {"en": "JMdict (English)"},
  "dictionary": {"format": "yomitan-zip", "files": ["jmdict.zip"], "from": ["ja"], "to": ["en"], "attribution": {"en": "JMdict © EDRDG"}}
}`

// siteManifest is a valid site pack's manifest of two sites.
const siteManifest = `{
  "schema": 1,
  "id": "example.videos",
  "kind": "site",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Example videos"},
  "site": {"sites": [
    {"domain": "videos.example.org", "not": ["live.videos.example.org"], "videos": [{"path": "^/v/[0-9]+/?$"}, {"path": "^/watch$", "param": "id", "value": "^[0-9]+$"}]},
    {"domain": "clips.example.net", "videos": [{"path": "^/c/[a-z0-9]{6,12}$"}]}
  ]}
}`

const taskManifest = `{
  "schema": 1,
  "id": "example.quiz",
  "kind": "task",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Quiz me"},
  "task": {"scope": "line", "prompt": "Ask the learner one question about the line.", "render": "cards", "maxTokens": 600,
    "reply": {"fields": [{"key": "question", "label": {"en": "Question"}}, {"key": "answer", "label": {"en": "Answer"}}], "items": 3}}
}`

const styleManifest = `{
  "schema": 1,
  "id": "example.casual",
  "kind": "translate-style",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Casual"},
  "style": {"prompt": "Translate every line into {{language}} as friends talk."}
}`

const templateManifest = `{
  "schema": 1,
  "id": "example.cards",
  "kind": "export-template",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Flash cards (CSV)"},
  "template": {"format": "csv", "columns": [{"header": "Front", "value": "{{original}}"}, {"header": "Back", "value": "{{translation}}\n{{film}} {{time}}"}]}
}`

// writePack writes a pack folder of files, by name, into a new folder of dir, and returns it.
func writePack(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// entry is one entry of a zip makeZip writes: a file, unless mode says otherwise.
type entry struct {
	name    string
	content string
	mode    fs.FileMode
}

// zipBytes is a zip of entries.
func zipBytes(t testing.TB, entries []entry) []byte {
	t.Helper()
	var b strings.Builder
	w := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

// makeZip writes a zip of entries to a new file and returns its path.
func makeZip(t *testing.T, entries []entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(path, zipBytes(t, entries), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// wantErr fails t unless err is target.
func wantErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("%s: err = %v, want %v", what, err, target)
	}
}

// leftovers is the names in root of the folders Stage and Swap work in.
func leftovers(t testing.TB, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagingPrefix) || strings.HasPrefix(e.Name(), oldPrefix) {
			names = append(names, e.Name())
		}
	}
	return names
}
