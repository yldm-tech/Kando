package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installAged installs the pack of files into root as Stage and Swap do, and sets the times of its files an hour back, past the window in which Scan does not keep a hash (racyWindow). It returns the pack.
func installAged(t *testing.T, root string, files map[string]string) Pack {
	t.Helper()
	staged, p, err := Stage(root, writePack(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		age(t, filepath.Join(p.Dir, name), time.Hour)
	}
	return p
}

// age sets the time of change of the file at path ago before now, to the second.
func age(t *testing.T, path string, ago time.Duration) time.Time {
	t.Helper()
	when := time.Now().Add(-ago).Truncate(time.Second)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return when
}

// rewrite writes content over the file at path and sets its time of change back to when, as a change that keeps the file's size and time would.
func rewrite(t *testing.T, path, content string, when time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// scanOne scans root, which must hold one pack and no issue, and returns the pack.
func scanOne(t *testing.T, root string) Pack {
	t.Helper()
	packs, issues := Scan(root)
	if len(packs) != 1 || len(issues) != 0 {
		t.Fatalf("Scan = %+v, %+v", packs, issues)
	}
	return packs[0]
}

// TestScanKeepsTheHashOfAnUnchangedPack checks the cache of hashes: a pack whose files keep their sizes and times keeps the hash the scan before took, without its files being read again, so a change that keeps them goes unseen; a file whose time moves on is read again, and the pack takes the hash Validate takes; and the manifest is checked at every scan, so a manifest no longer valid is an issue even when its size and time are as they were.
func TestScanKeepsTheHashOfAnUnchangedPack(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	p := installAged(t, root, map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n"})
	if got := scanOne(t, root); got.Hash != p.Hash {
		t.Fatalf("first scan hash %s, want %s", got.Hash, p.Hash)
	}
	cache := loadHashCache(root, time.Now())
	if cached, ok := cache.old["wordlist/example.jlpt"]; !ok || cached.Hash != p.Hash || len(cached.Files) != 2 {
		t.Fatalf("the cache: %+v", cache.old)
	}

	list := filepath.Join(p.Dir, "jlpt.tsv")
	info, err := os.Stat(list)
	if err != nil {
		t.Fatal(err)
	}
	rewrite(t, list, "犬\tn5\n", info.ModTime())
	if got := scanOne(t, root); got.Hash != p.Hash {
		t.Fatalf("a file as large and as old as it was was read again: hash %s", got.Hash)
	}

	age(t, list, 2*time.Hour)
	want, err := Validate(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := scanOne(t, root); got.Hash != want.Hash || got.Hash == p.Hash {
		t.Fatalf("a file changed later: hash %s, want %s", got.Hash, want.Hash)
	}

	manifest := filepath.Join(p.Dir, ManifestName)
	info, err = os.Stat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rewrite(t, manifest, strings.Replace(wordlistManifest, `"success"`, `"sxccess"`, 1), info.ModTime())
	packs, issues := Scan(root)
	if len(packs) != 0 || len(issues) != 1 {
		t.Fatalf("a manifest no longer valid: %+v, %+v", packs, issues)
	}
	wantErr(t, "the manifest", issues[0].Err, ErrBadField)
	if _, ok := loadHashCache(root, time.Now()).old["wordlist/example.jlpt"]; ok {
		t.Error("the cache keeps a pack no longer valid")
	}
}

// TestScanReadsADictionaryOnce checks that the zips of a dictionary are checked when the pack is first scanned and not at the scans after, while they are as they were, and again once one of them changed.
func TestScanReadsADictionaryOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	zip := string(zipBytes(t, []entry{{name: "index.json", content: "{}"}, {name: "term_bank_1.json", content: "[]"}}))
	p := installAged(t, root, map[string]string{ManifestName: dictionaryManifest, "jmdict.zip": zip})
	scanOne(t, root)

	// The zip's bytes are no zip now, but as many, and as old.
	path := filepath.Join(p.Dir, "jmdict.zip")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rewrite(t, path, strings.Repeat("\x00", len(zip)), info.ModTime())
	if got := scanOne(t, root); got.Hash != p.Hash {
		t.Fatalf("hash %s, want %s", got.Hash, p.Hash)
	}

	age(t, path, 2*time.Hour)
	packs, issues := Scan(root)
	if len(packs) != 0 || len(issues) != 1 {
		t.Fatalf("a zip changed: %+v, %+v", packs, issues)
	}
	wantErr(t, "the changed zip", issues[0].Err, ErrBadArchive)
}

// TestScanReadsAFreshPackAgain checks that a pack whose files changed within racyWindow of the scan has its hash taken but not kept, as a change in the same tick of the clock would not show in its files' times; and that a cache of another format is not trusted.
func TestScanReadsAFreshPackAgain(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	staged, p, err := Stage(root, writePack(t, map[string]string{ManifestName: glossaryManifest}))
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	if got := scanOne(t, root); got.Hash != p.Hash {
		t.Fatalf("hash %s, want %s", got.Hash, p.Hash)
	}
	if old := loadHashCache(root, time.Now()).old; len(old) != 0 {
		t.Fatalf("the hash of a pack just written was kept: %+v", old)
	}

	if err := os.WriteFile(filepath.Join(root, hashCacheName), []byte(`{"format": 99, "packs": {"glossary/example.anime-ja-zh": {"hash": "x", "files": []}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if old := loadHashCache(root, time.Now()).old; len(old) != 0 {
		t.Fatalf("a cache of another format was read: %+v", old)
	}
}
