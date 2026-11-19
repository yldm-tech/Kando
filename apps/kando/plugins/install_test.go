package plugins

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStageAndSwap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	src := makeZip(t, []entry{{name: ManifestName, content: wordlistManifest}, {name: "jlpt.tsv", content: "猫\tn5\n"}, {name: "README.md", content: "notes"}})
	staged, p, err := Stage(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "example.jlpt" || p.Dir != staged || filepath.Dir(staged) != root {
		t.Errorf("Stage = %q, %+v", staged, p)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "wordlist", "example.jlpt")
	if p.Dir != dir {
		t.Errorf("after Swap p.Dir = %q, want %q", p.Dir, dir)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "jlpt.tsv")); err != nil || string(data) != "猫\tn5\n" {
		t.Errorf("installed jlpt.tsv = %q, %v", data, err)
	}

	// Installing the same id again replaces the pack, and leaves nothing of the old one.
	src = makeZip(t, []entry{{name: ManifestName, content: strings.Replace(wordlistManifest, "1.0.0", "1.1.0", 1)}, {name: "jlpt.tsv", content: "犬\tn5\n"}})
	staged2, p2, err := Stage(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged2, &p2); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "jlpt.tsv")); string(data) != "犬\tn5\n" {
		t.Errorf("replaced jlpt.tsv = %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); !os.IsNotExist(err) {
		t.Errorf("the old pack's README.md is still there: %v", err)
	}
	if left := leftovers(t, root); len(left) != 0 {
		t.Errorf("left in root: %q", left)
	}
	packs, issues := Scan(root)
	if len(issues) != 0 || len(packs) != 1 || packs[0].Version != "1.1.0" || packs[0].Hash != p2.Hash || packs[0].Hash == p.Hash {
		t.Errorf("Scan = %+v, %+v", packs, issues)
	}
}

// TestSwapIDTaken checks that an id is one pack whatever its kind: a pack of another kind with an installed pack's id is refused and changes nothing, and is installed once that pack is removed.
func TestSwapIDTaken(t *testing.T) {
	root := t.TempDir()
	install := func(manifest string) error {
		staged, p, err := Stage(root, writePack(t, map[string]string{ManifestName: manifest}))
		if err != nil {
			t.Fatal(err)
		}
		return Swap(root, staged, &p)
	}
	if err := install(glossaryManifest); err != nil {
		t.Fatal(err)
	}
	service := strings.Replace(serviceManifest, "acme.corp-gateway", "example.anime-ja-zh", 1)
	wantErr(t, "same id, other kind", install(service), ErrIDTaken)
	if _, err := os.Stat(filepath.Join(root, string(KindAIService), "example.anime-ja-zh")); !os.IsNotExist(err) {
		t.Errorf("the refused pack was installed: %v", err)
	}
	packs, issues := Scan(root)
	if len(issues) != 0 || len(packs) != 1 || packs[0].Kind != KindGlossary {
		t.Errorf("Scan = %+v, %+v", packs, issues)
	}
	if err := Remove(root, "example.anime-ja-zh"); err != nil {
		t.Fatal(err)
	}
	if err := install(service); err != nil {
		t.Errorf("after removing the glossary: %v", err)
	}
}

// TestScanIgnoresFileManagerFiles checks that the files Finder and Explorer write into a folder they show leave an installed pack valid and its hash as it was.
func TestScanIgnoresFileManagerFiles(t *testing.T) {
	root := t.TempDir()
	staged, p, err := Stage(root, writePack(t, map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{finderMetadata, "Thumbs.db", "desktop.ini"} {
		if err := os.WriteFile(filepath.Join(p.Dir, name), []byte("file manager"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	packs, issues := Scan(root)
	if len(issues) != 0 || len(packs) != 1 || packs[0].Hash != p.Hash {
		t.Errorf("Scan = %+v, %+v, want the pack with hash %s", packs, issues, p.Hash)
	}
}

func TestSwapRefusesAFolderStageDidNotMake(t *testing.T) {
	root := t.TempDir()
	other := writePack(t, map[string]string{ManifestName: glossaryManifest})
	p, err := Validate(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, other, &p); err == nil {
		t.Error("Swap took a folder outside root")
	}
	p.ID = "../evil"
	staged, _, err := Stage(root, other)
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, "bad id", Swap(root, staged, &p), ErrBadID)
}

// TestStageZipRefusesUnsafeEntries checks the entries of a zip that would reach outside the pack, or be anything but a file in it, and that a refused zip leaves nothing behind.
func TestStageZipRefusesUnsafeEntries(t *testing.T) {
	manifest := entry{name: ManifestName, content: wordlistManifest}
	tsv := entry{name: "jlpt.tsv", content: "猫\tn5\n"}
	for _, tt := range []struct {
		name    string
		entries []entry
		want    error
	}{
		{"zip slip", []entry{manifest, tsv, {name: "../evil.md", content: "x"}}, ErrUnsafePath},
		{"absolute", []entry{manifest, tsv, {name: "/tmp/evil.md", content: "x"}}, ErrUnsafePath},
		{"backslash", []entry{manifest, tsv, {name: `..\evil.md`, content: "x"}}, ErrUnsafePath},
		{"in a folder", []entry{manifest, tsv, {name: "docs/evil.md", content: "x"}}, ErrUnsafePath},
		{"folder", []entry{manifest, tsv, {name: "docs/", mode: os.ModeDir | 0o755}}, ErrBadShape},
		{"link", []entry{manifest, {name: "jlpt.tsv", content: "/etc/passwd", mode: os.ModeSymlink | 0o777}}, ErrUnsafePath},
		{"twice", []entry{manifest, tsv, {name: "JLPT.tsv", content: "x"}}, ErrBadShape},
		{"no manifest", []entry{tsv}, ErrBadManifest},
		{"manifest in a folder", []entry{{name: "pack/" + ManifestName, content: wordlistManifest}}, ErrBadManifest},
		{"unsupported", []entry{{name: ManifestName, content: `{"schema": 1, "id": "example.x", "kind": "script"}`}}, ErrUnsupportedKind},
		{"newer schema", []entry{{name: ManifestName, content: `{"schema": 2, "id": "example.x", "kind": "script"}`}}, ErrSchema},
		{"invalid", []entry{{name: ManifestName, content: strings.Replace(wordlistManifest, "example.jlpt", "kando.jlpt", 1)}, tsv}, ErrReservedID},
	} {
		root := t.TempDir()
		_, _, err := Stage(root, makeZip(t, tt.entries))
		wantErr(t, tt.name, err, tt.want)
		if left := leftovers(t, root); len(left) != 0 {
			t.Errorf("%s: left in root: %q", tt.name, left)
		}
	}
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Stage(root, path)
	wantErr(t, "not a zip", err, ErrBadArchive)
}

// TestStageZipCountsBytes checks that the bytes a zip unpacks to are counted as they are read: an entry over the limit is refused, whether its header says how large it is or lies about it.
func TestStageZipCountsBytes(t *testing.T) {
	big := int(limitsOf(KindWordlist).bytes) + 1
	var compressed bytes.Buffer
	fw, err := flate.NewWriter(&compressed, flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	zeros := make([]byte, 1<<20)
	for n := big; n > 0; n -= len(zeros) {
		if _, err := fw.Write(zeros[:min(n, len(zeros))]); err != nil {
			t.Fatal(err)
		}
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, declared := range []uint64{uint64(big), 10} {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		f, err := w.Create(ManifestName)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(f, wordlistManifest)
		raw, err := w.CreateRaw(&zip.FileHeader{Name: "jlpt.tsv", Method: zip.Deflate, CompressedSize64: uint64(compressed.Len()), UncompressedSize64: declared})
		if err != nil {
			t.Fatal(err)
		}
		raw.Write(compressed.Bytes())
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "bomb.zip")
		if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		_, _, err = Stage(root, path)
		if !errorsIsAny(err, ErrTooLarge, ErrBadArchive) {
			t.Errorf("declared %d bytes: err = %v", declared, err)
		}
		if left := leftovers(t, root); len(left) != 0 {
			t.Errorf("declared %d bytes: left in root: %q", declared, left)
		}
	}
}

func TestStageZipSize(t *testing.T) {
	data := make([]byte, limitsOf(KindWordlist).zip)
	rand.Read(data)
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create(ManifestName)
	io.WriteString(f, wordlistManifest)
	f, _ = w.CreateHeader(&zip.FileHeader{Name: "jlpt.tsv", Method: zip.Store})
	f.Write(data)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "big.zip")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Stage(t.TempDir(), path)
	wantErr(t, "zip over the limit", err, ErrTooLarge)
}

// TestStageFolder checks that a folder is copied file by file, without Finder's .DS_Store, and that the pack is the copy: changing the folder afterwards changes nothing that was staged.
func TestStageFolder(t *testing.T) {
	root := t.TempDir()
	src := writePack(t, map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n", finderMetadata: "finder"})
	staged, p, err := Stage(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staged, finderMetadata)); !os.IsNotExist(err) {
		t.Errorf(".DS_Store was copied: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "jlpt.tsv"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := Validate(staged)
	if err != nil || again.Hash != p.Hash {
		t.Errorf("the staged copy changed with its source: %v, %s != %s", err, again.Hash, p.Hash)
	}
	if data, _ := os.ReadFile(filepath.Join(staged, "jlpt.tsv")); string(data) != "猫\tn5\n" {
		t.Errorf("staged jlpt.tsv = %q", data)
	}

	src = writePack(t, map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n"})
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	_, _, err = Stage(root, src)
	wantErr(t, "folder in the folder", err, ErrBadShape)
	if left := leftovers(t, root); len(left) != 0 {
		t.Errorf("left in root: %q", left)
	}
}

func TestStageRemovesStaleFolders(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, stagingPrefix+"1")
	staleOld := filepath.Join(root, oldPrefix+"2")
	fresh := filepath.Join(root, stagingPrefix+"3")
	for _, dir := range []string{stale, staleOld, fresh} {
		if err := os.MkdirAll(filepath.Join(dir, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().Add(-staleAfter - time.Minute)
	for _, dir := range []string{stale, staleOld} {
		if err := os.Chtimes(dir, day, day); err != nil {
			t.Fatal(err)
		}
	}
	staged, _, err := Stage(root, writePack(t, map[string]string{ManifestName: glossaryManifest}))
	if err != nil {
		t.Fatal(err)
	}
	left := leftovers(t, root)
	slices.Sort(left)
	want := []string{filepath.Base(fresh), filepath.Base(staged)}
	slices.Sort(want)
	if !slices.Equal(left, want) {
		t.Errorf("left = %q, want %q", left, want)
	}
}

// TestStaleAgeFromTheMove checks that a pack moved out of the way just now is not taken for a stale one, though its folder, renamed, keeps the time it was last changed, a day ago or more for a pack installed long ago: a Stage of another Kando must not remove the old pack a Swap may still put back.
func TestStaleAgeFromTheMove(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	moved := filepath.Join(root, asideName(now))
	if err := os.MkdirAll(filepath.Join(moved, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed := now.Add(-3 * staleAfter)
	if err := os.Chtimes(moved, installed, installed); err != nil {
		t.Fatal(err)
	}
	cleanStale(root, now.Add(time.Hour))
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("removed an hour after the move: %v", err)
	}
	cleanStale(root, now.Add(staleAfter))
	if _, err := os.Stat(moved); !os.IsNotExist(err) {
		t.Errorf("still there a day after the move: %v", err)
	}
}

func TestRemove(t *testing.T) {
	root := t.TempDir()
	staged, p, err := Stage(root, writePack(t, map[string]string{ManifestName: glossaryManifest}))
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"..", "../glossary", "a/../../b", "glossary/../../x", "", "nodot"} {
		wantErr(t, id, Remove(root, id), ErrBadID)
	}
	wantErr(t, "other id", Remove(root, "example.other"), ErrNotInstalled)
	if err := Remove(root, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "glossary", p.ID)); !os.IsNotExist(err) {
		t.Errorf("the pack is still there: %v", err)
	}
	if left := leftovers(t, root); len(left) != 0 {
		t.Errorf("left in root: %q", left)
	}
}

func TestPackDir(t *testing.T) {
	root := t.TempDir()
	dir, err := packDir(root, KindGlossary, "example.terms")
	if err != nil || dir != filepath.Join(root, "glossary", "example.terms") {
		t.Errorf("packDir = %q, %v", dir, err)
	}
	for _, id := range []string{".", "..", "a..b", "-a.b", "a.b.", "../x.y", "x.y/../../z"} {
		_, err := packDir(root, KindGlossary, id)
		wantErr(t, id, err, ErrBadID)
	}
	_, err = packDir(root, Kind("../x"), "example.terms")
	wantErr(t, "kind", err, ErrBadID)
}

// TestScanIssues checks that a folder that is not a valid pack, or is not named after its pack, is reported rather than listed, and that the folders Stage and Swap work in are skipped.
func TestScanIssues(t *testing.T) {
	root := t.TempDir()
	write := func(dir string, files map[string]string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(filepath.Join(root, "glossary", "example.anime-ja-zh"), map[string]string{ManifestName: glossaryManifest})
	write(filepath.Join(root, "glossary", "example.renamed"), map[string]string{ManifestName: glossaryManifest})
	write(filepath.Join(root, "ai-service", "example.anime-ja-zh"), map[string]string{ManifestName: glossaryManifest})
	write(filepath.Join(root, "wordlist", "example.broken"), map[string]string{ManifestName: "{"})
	// A pack copied in by hand with the id of a pack of a kind Scan reads before it.
	write(filepath.Join(root, "ai-service", "example.jlpt"), map[string]string{ManifestName: strings.Replace(serviceManifest, "acme.corp-gateway", "example.jlpt", 1)})
	write(filepath.Join(root, "wordlist", "example.jlpt"), map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n"})
	write(filepath.Join(root, "glossary", stagingPrefix+"x"), map[string]string{ManifestName: "{"})
	write(filepath.Join(root, stagingPrefix+"y"), map[string]string{ManifestName: "{"})
	packs, issues := Scan(root)
	if len(packs) != 2 || packs[0].Dir != filepath.Join(root, "glossary", "example.anime-ja-zh") || packs[1].Dir != filepath.Join(root, "wordlist", "example.jlpt") {
		t.Errorf("packs = %+v", packs)
	}
	want := map[string]error{
		filepath.Join(root, "glossary", "example.renamed"):       ErrMismatch,
		filepath.Join(root, "ai-service", "example.anime-ja-zh"): ErrMismatch,
		filepath.Join(root, "wordlist", "example.broken"):        ErrBadManifest,
		filepath.Join(root, "ai-service", "example.jlpt"):        ErrIDTaken,
	}
	if len(issues) != len(want) {
		t.Errorf("issues = %+v", issues)
	}
	for _, issue := range issues {
		wantErr(t, issue.Dir, issue.Err, want[issue.Dir])
	}
	if packs, issues := Scan(filepath.Join(root, "missing")); len(packs) != 0 || len(issues) != 0 {
		t.Errorf("Scan of a missing folder = %+v, %+v", packs, issues)
	}
}

func errorsIsAny(err error, targets ...error) bool {
	return slices.ContainsFunc(targets, func(target error) bool { return errors.Is(err, target) })
}
