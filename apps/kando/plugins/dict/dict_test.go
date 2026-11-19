package dict

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
)

// pack is a dictionary pack of format with files (name to content) in a folder of its own, its hash hash.
func pack(t *testing.T, format, hash string, files map[string][]byte) plugins.Pack {
	t.Helper()
	dir := t.TempDir()
	var names []string
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return plugins.Pack{Manifest: plugins.Manifest{ID: "example.dict", Kind: plugins.KindDictionary, Dictionary: &plugins.Dictionary{Format: format, Files: names}}, Dir: dir, Hash: hash}
}

// zipped is a zip of files, name to content.
func zipped(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b strings.Builder
	zw := zip.NewWriter(&b)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(data))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

func open(t *testing.T, p plugins.Pack, cache string) *Dict {
	t.Helper()
	d, err := Open(p, cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func lookUp(t *testing.T, d *Dict, word, base, reading string) (lookup.Entry, bool) {
	t.Helper()
	e, ok, err := d.LookUp(word, base, reading)
	if err != nil {
		t.Fatal(err)
	}
	return e, ok
}

// TestYomitanTermsAreFoundByFormAndReading checks that a Yomitan dictionary's terms, of both versions of its term banks, with plain and structured glossaries, are found by their dictionary form (the base a word clicked as it inflects gives), by their reading, and not by another word; that a word read two ways shows the reading asked for when there is one, and both otherwise, as when it was found by its base, whose reading the reading of the word as clicked is not; and that the dictionary's other banks are left out.
func TestYomitanTermsAreFoundByFormAndReading(t *testing.T) {
	bank1 := `[["行く","いく","v5k-s","v5",10,["to go","to move"],1,""],["行く","ゆく","v5k-s","v5",5,["to go (literary)"],2,""],` +
		`["公園","こうえん","n","",0,[{"type":"structured-content","content":{"tag":"ul","content":[{"tag":"li","content":"park"},{"tag":"li","content":["public ",{"tag":"b","content":"garden"}]},{"tag":"img","alt":"x"}]}}],3,""]]`
	bank10 := `[["猫","ねこ","n","",0,"cat","feline"]]`
	p := pack(t, plugins.FormatYomitan, "0123456789abcdef0123", map[string][]byte{"jmdict.zip": zipped(t, map[string]string{
		"index.json": `{"title":"JMdict","revision":"1","format":3}`, "term_bank_1.json": bank1, "dir/term_bank_10.json": bank10,
		"kanji_bank_1.json": `[["日","ニチ","ひ","","",["day"],{}]]`,
	})})
	d := open(t, p, t.TempDir())
	if e, ok := lookUp(t, d, "行った", "行く", "イッタ"); !ok || e.Word != "行く" || e.Meaning != "いく: to go; to move\nゆく: to go (literary)" {
		t.Fatalf("行った by its base: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "行った", "行く", "ゆく"); !ok || e.Meaning != "いく: to go; to move\nゆく: to go (literary)" {
		t.Fatalf("the reading of a word found by its base picked a reading: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "行く", "", "ゆく"); !ok || e.Reading != "ゆく" || e.Meaning != "to go (literary)" {
		t.Fatalf("行く read ゆく: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "こうえん", "", ""); !ok || e.Word != "公園" || e.Reading != "こうえん" || e.Meaning != "park public garden" {
		t.Fatalf("公園 by its reading: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "猫", "", ""); !ok || e.Meaning != "cat; feline" {
		t.Fatalf("a term of version 1: %+v, %v", e, ok)
	}
	for _, word := range []string{"日", "犬", ""} {
		if e, ok := lookUp(t, d, word, "", ""); ok {
			t.Fatalf("%q found: %+v", word, e)
		}
	}
}

// TestCCCEDICTGivesPinyinForBothScripts checks that a CC-CEDICT word is found by its Traditional and its Simplified form, each the headword of what it finds, with its pinyin in tone marks, comments left out, a word in any case found, and a word with several readings showing each.
func TestCCCEDICTGivesPinyinForBothScripts(t *testing.T) {
	text := "# CC-CEDICT\r\n#! version=1\r\n傳統 传统 [chuan2 tong3] /tradition/traditional/convention/\r\n綠 绿 [lu:4] /green/\r\n行 行 [xing2] /to walk/OK/\r\n行 行 [hang2] /row/line/\r\nＡ Ａ [A] /(slang) (Tw) to steal/\r\n學 学 [xue2] /to learn/\r\n"
	d := open(t, pack(t, plugins.FormatCCCEDICT, "fedcba9876543210ffff", map[string][]byte{"cedict_ts.u8": []byte(text)}), t.TempDir())
	for _, word := range []string{"傳統", "传统"} {
		if e, ok := lookUp(t, d, word, "", ""); !ok || e.Word != word || e.Reading != "chuán tǒng" || e.Meaning != "tradition; traditional; convention" {
			t.Fatalf("%s: %+v, %v", word, e, ok)
		}
	}
	if e, ok := lookUp(t, d, "綠", "", ""); !ok || e.Reading != "lǜ" {
		t.Fatalf("綠: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "行", "", ""); !ok || e.Meaning != "xíng: to walk; OK\nháng: row; line" {
		t.Fatalf("行: %+v, %v", e, ok)
	}
	for _, word := range []string{"學", "学"} {
		if e, ok := lookUp(t, d, word, "", ""); !ok || e.Word != word || e.Meaning != "to learn" {
			t.Fatalf("%s: %+v, %v", word, e, ok)
		}
	}
}

// TestTSVAndPinyin checks a table of tab-separated values with and without a reading, found in any case, its first word read though a byte order mark comes before it, and the tone marks of pinyin.
func TestTSVAndPinyin(t *testing.T) {
	text := "\uFEFFcat\tneko\n# word\treading\tdefinition\nHello\t/həˈləʊ/\ta greeting\textra\nbye\tfarewell\n\nbroken\n"
	d := open(t, pack(t, plugins.FormatTSV, "aaaaaaaaaaaaaaaaaaaa", map[string][]byte{"words.tsv": []byte(text)}), t.TempDir())
	if e, ok := lookUp(t, d, "hello", "", ""); !ok || e.Word != "Hello" || e.Reading != "/həˈləʊ/" || e.Meaning != "a greeting" {
		t.Fatalf("hello: %+v, %v", e, ok)
	}
	if e, ok := lookUp(t, d, "BYE", "", ""); !ok || e.Reading != "" || e.Meaning != "farewell" {
		t.Fatalf("bye: %+v, %v", e, ok)
	}
	if _, ok := lookUp(t, d, "broken", "", ""); ok {
		t.Fatal("a line of one column was read")
	}
	if e, ok := lookUp(t, d, "cat", "", ""); !ok || e.Word != "cat" || e.Meaning != "neko" {
		t.Fatalf("the word after the byte order mark: %+v, %v", e, ok)
	}
	for in, want := range map[string]string{"zhong1 guo2": "zhōng guó", "nu:3 er2": "nǚ ér", "gou3": "gǒu", "liu2": "liú", "gui4": "guì", "de5": "de", "Bei3 jing1": "Běi jīng", "A": "A", "r5": "r"} {
		if got := pinyin(in); got != want {
			t.Errorf("pinyin(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTheIndexIsBuiltOnceAndClosed checks that the index is built the first time a pack is opened and read again after, not built again; that a damaged index is built again; that opening another content of the pack leaves the index of the earlier one, which Forget then removes but the one it keeps, and the rest after; and that a dictionary closed finds nothing, and its pack's folder can then be replaced (plugins.Swap renames it).
func TestTheIndexIsBuiltOnceAndClosed(t *testing.T) {
	cache := t.TempDir()
	p := pack(t, plugins.FormatTSV, "1111111111111111aaaa", map[string][]byte{"words.tsv": []byte("cat\tneko\n")})
	d, err := Open(p, cache)
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(cache, IndexName(p.ID, p.Hash))
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	// The pack's file changes without its hash: the index already built is what is read.
	os.WriteFile(filepath.Join(p.Dir, "words.tsv"), []byte("dog\tinu\n"), 0o644)
	d = open(t, p, cache)
	if _, ok := lookUp(t, d, "cat", "", ""); !ok {
		t.Fatal("the index was built again")
	}
	d.Close()
	if _, ok := lookUp(t, d, "cat", "", ""); ok {
		t.Fatal("a closed dictionary found a word")
	}
	moved := p.Dir + "-moved"
	if err := os.Rename(p.Dir, moved); err != nil {
		t.Fatalf("the pack's folder could not be moved once its dictionary was closed: %v", err)
	}
	p.Dir = moved
	os.WriteFile(index, []byte("KANDODX1 broken"), 0o644)
	d = open(t, p, cache)
	if _, ok := lookUp(t, d, "dog", "", ""); !ok {
		t.Fatal("a damaged index was not built again")
	}
	d.Close()
	p.Hash = "2222222222222222bbbb"
	d = open(t, p, cache)
	d.Close()
	names, _ := filepath.Glob(filepath.Join(cache, "*.idx"))
	if len(names) != 2 {
		t.Fatalf("indexes after another content: %v", names)
	}
	Forget(cache, p.ID, IndexName(p.ID, p.Hash))
	names, _ = filepath.Glob(filepath.Join(cache, "*.idx"))
	if len(names) != 1 || filepath.Base(names[0]) != IndexName(p.ID, p.Hash) {
		t.Fatalf("indexes after Forget kept another content: %v", names)
	}
	other := filepath.Join(cache, IndexName("example.dict-two", "3333333333333333"))
	os.WriteFile(other, nil, 0o644)
	Forget(cache, p.ID, "")
	names, _ = filepath.Glob(filepath.Join(cache, "*.idx"))
	if len(names) != 1 || names[0] != other {
		t.Fatalf("Forget left %v", names)
	}
	if _, err := Open(pack(t, plugins.FormatTSV, "4444444444444444", map[string][]byte{"words.tsv": []byte("# nothing\n")}), cache); err == nil {
		t.Fatal("a dictionary without an entry opened")
	}
}

// TestBuildsCutShortAreRemoved checks the files builds cut short leave beside the indexes: Open removes those of the content it builds, Forget those of the pack's other contents but not those of the content it keeps, whose build may be running, nor another pack's, and all of the pack's once nothing is kept; a build that finishes leaves none.
func TestBuildsCutShortAreRemoved(t *testing.T) {
	cache := t.TempDir()
	p := pack(t, plugins.FormatTSV, "6666666666666666aaaa", map[string][]byte{"words.tsv": []byte("cat\tneko\n")})
	left := func(name string) string {
		path := filepath.Join(cache, name+buildSuffix+"123")
		if err := os.WriteFile(path, []byte("half"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	builds := func() []string {
		names, _ := filepath.Glob(filepath.Join(cache, "*"+buildSuffix+"*"))
		return names
	}
	ours, older, others := left(IndexName(p.ID, p.Hash)), left(IndexName(p.ID, "7777777777777777")), left(IndexName("example.other", "7777777777777777"))
	d := open(t, p, cache)
	d.Close()
	if got := builds(); !slices.Equal(got, []string{others, older}) && !slices.Equal(got, []string{older, others}) {
		t.Fatalf("after Open: %v", got)
	}
	ours = left(IndexName(p.ID, p.Hash))
	Forget(cache, p.ID, IndexName(p.ID, p.Hash))
	if got := builds(); !slices.Equal(got, []string{others, ours}) && !slices.Equal(got, []string{ours, others}) {
		t.Fatalf("after Forget: %v", got)
	}
	Forget(cache, p.ID, "")
	if got := builds(); !slices.Equal(got, []string{others}) {
		t.Fatalf("after Forget of every content: %v", got)
	}
}

// TestManyWordsAreFoundByBinarySearch checks a dictionary of many words: each is found, one between them is not.
func TestManyWordsAreFoundByBinarySearch(t *testing.T) {
	var b strings.Builder
	for i := range 5000 {
		b.WriteString("w" + strings.Repeat("x", i%7) + strconv.Itoa(i) + "\tdef " + strconv.Itoa(i) + "\n")
	}
	d := open(t, pack(t, plugins.FormatTSV, "5555555555555555", map[string][]byte{"words.tsv": []byte(b.String())}), t.TempDir())
	for _, i := range []int{0, 1, 2499, 4998, 4999} {
		w := "w" + strings.Repeat("x", i%7) + strconv.Itoa(i)
		if e, ok := lookUp(t, d, w, "", ""); !ok || e.Meaning != "def "+strconv.Itoa(i) {
			t.Fatalf("%s: %+v, %v", w, e, ok)
		}
	}
	if _, ok := lookUp(t, d, "w", "", ""); ok {
		t.Fatal("found a word not there")
	}
}

// TestRetry checks which errors are worth opening a dictionary again for: a file that could not be read or written, or a damaged index, and not a dictionary without an entry, one Kando cannot parse, or no error.
func TestRetry(t *testing.T) {
	for err, want := range map[error]bool{
		nil: false,
		fmt.Errorf("words.tsv: %w", &fs.PathError{Op: "read", Path: "words.tsv", Err: fs.ErrPermission}): true,
		&os.LinkError{Op: "rename", Old: "a", New: "b", Err: fs.ErrPermission}:                           true,
		ErrBadIndex: true,
		ErrEmpty:    false,
		fmt.Errorf("jmdict.zip: %w", zip.ErrFormat): false,
	} {
		if got := Retry(err); got != want {
			t.Errorf("Retry(%v) = %v, want %v", err, got, want)
		}
	}
}
