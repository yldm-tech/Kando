package plugins

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateKinds(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"glossary":   {ManifestName: glossaryManifest, "README.md": "notes"},
		"wordlist":   {ManifestName: wordlistManifest, "jlpt.tsv": "猫\tn5\n", "LICENSE.txt": "CC-BY"},
		"ai-service": {ManifestName: serviceManifest},
		"site":       {ManifestName: siteManifest},
		"dictionary": {ManifestName: dictionaryManifest, "jmdict.zip": string(zipBytes(t, []entry{{name: "index.json", content: "{}"}, {name: "term_bank_1.json", content: "[]"}}))},
	} {
		p, err := Validate(writePack(t, files))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(p.Kind) != name || len(p.Hash) != 64 {
			t.Errorf("%s: pack = %+v", name, p)
		}
	}
}

// TestValidateManifest checks each rule of the manifest's fields, changing one field of a valid manifest at a time.
func TestValidateManifest(t *testing.T) {
	long := strings.Repeat("a", 41)
	for _, tt := range []struct {
		name     string
		manifest string
		old, new string
		want     error
	}{
		{"id dot", glossaryManifest, `"example.anime-ja-zh"`, `"."`, ErrBadID},
		{"id dotdot", glossaryManifest, `"example.anime-ja-zh"`, `".."`, ErrBadID},
		{"id double dot", glossaryManifest, `"example.anime-ja-zh"`, `"a..b"`, ErrBadID},
		{"id leading hyphen", glossaryManifest, `"example.anime-ja-zh"`, `"-a.b"`, ErrBadID},
		{"id trailing dot", glossaryManifest, `"example.anime-ja-zh"`, `"a.b."`, ErrBadID},
		{"id no dot", glossaryManifest, `"example.anime-ja-zh"`, `"example"`, ErrBadID},
		{"id uppercase", glossaryManifest, `"example.anime-ja-zh"`, `"Example.terms"`, ErrBadID},
		{"id slash", glossaryManifest, `"example.anime-ja-zh"`, `"a/../b.c"`, ErrBadID},
		{"id too long", glossaryManifest, `"example.anime-ja-zh"`, `"example.` + strings.Repeat("a", 57) + `"`, ErrBadID},
		{"id device publisher", glossaryManifest, `"example.anime-ja-zh"`, `"con.terms"`, ErrBadID},
		{"id device name", glossaryManifest, `"example.anime-ja-zh"`, `"example.nul"`, ErrBadID},
		{"id device inner", glossaryManifest, `"example.anime-ja-zh"`, `"example.lpt1.terms"`, ErrBadID},
		{"id kando", glossaryManifest, `"example.anime-ja-zh"`, `"kando.terms"`, ErrReservedID},
		{"schema new", glossaryManifest, `"schema": 1`, `"schema": 2`, ErrSchema},
		{"schema none", glossaryManifest, `"schema": 1,`, ``, ErrSchema},
		{"version", glossaryManifest, `"1.2.0"`, `"1 2"`, ErrBadField},
		{"license", glossaryManifest, `"CC-BY-4.0"`, `""`, ErrBadField},
		{"homepage http", glossaryManifest, `"license": "CC-BY-4.0",`, `"license": "CC-BY-4.0", "homepage": "http://example.com",`, ErrInsecureURL},
		{"no english name", glossaryManifest, `"en": "Anime terms", `, ``, ErrBadName},
		{"name too long", glossaryManifest, `"Anime terms"`, `"` + long + `"`, ErrBadName},
		{"name invisible", glossaryManifest, `"Anime terms"`, `"\u200b\u202e"`, ErrBadName},
		{"name Kando", glossaryManifest, `"Anime terms"`, `"Kando"`, ErrReservedName},
		{"name 看懂", glossaryManifest, `"动画常用词"`, `"看懂"`, ErrReservedName},
		{"name full width", glossaryManifest, `"Anime terms"`, `"ＯｐｅｎＡＩ"`, ErrReservedName},
		{"name spaced", glossaryManifest, `"Anime terms"`, `"lm  studio"`, ErrReservedName},
		{"name hidden in bidi", glossaryManifest, `"Anime terms"`, `"Open\u200bAI"`, ErrReservedName},
		{"name official in Japanese", glossaryManifest, `"Anime terms"`, `"公式"`, ErrReservedName},
		{"name official in Korean", glossaryManifest, `"Anime terms"`, `"공식"`, ErrReservedName},
		{"localized provider", glossaryManifest, `"动画常用词"`, `"通义千问"`, ErrReservedName},
		{"language not canonical", glossaryManifest, `"to": "zh-Hans"`, `"to": "zh-hans"`, ErrBadField},
		{"language not a tag", glossaryManifest, `"from": "ja"`, `"from": "japanese!"`, ErrBadField},
		{"text language", glossaryManifest, `"zh-Hans": "动画常用词"`, `"chinese!": "动画常用词"`, ErrBadField},
		{"empty term", glossaryManifest, `"target": "前辈"`, `"target": " "`, ErrBadField},
		{"no terms", glossaryManifest, `[{"source": "先輩", "target": "前辈"}]`, `[]`, ErrBadField},
		{"two kinds", glossaryManifest, `"license": "CC-BY-4.0",`, `"license": "CC-BY-4.0", "service": {"baseURL": "https://a.example", "group": "cloud"},`, ErrBadField},
		{"service http", serviceManifest, `"https://llm.example.com/v1"`, `"http://llm.example.com/v1"`, ErrInsecureURL},
		{"key page http", serviceManifest, `"https://llm.example.com/keys"`, `"http://llm.example.com/keys"`, ErrInsecureURL},
		{"service scheme", serviceManifest, `"https://llm.example.com/v1"`, `"ftp://llm.example.com/v1"`, ErrInsecureURL},
		{"service user", serviceManifest, `"https://llm.example.com/v1"`, `"https://me:pw@llm.example.com/v1"`, ErrBadField},
		{"service relative", serviceManifest, `"https://llm.example.com/v1"`, `"/v1"`, ErrBadField},
		{"service no address", serviceManifest, `"https://llm.example.com/v1"`, `""`, ErrBadField},
		{"service group", serviceManifest, `"group": "cloud"`, `"group": "other"`, ErrBadField},
		{"tone", wordlistManifest, `"tone": "success"`, `"tone": "red"`, ErrBadField},
		{"tier twice", wordlistManifest, `"id": "n1"`, `"id": "n5"`, ErrBadField},
		{"tier id", wordlistManifest, `"id": "n1"`, `"id": "N 1"`, ErrBadField},
		{"tier label", wordlistManifest, `"label": {"en": "N1"}`, `"label": {"ja": "N1"}`, ErrBadName},
		{"dictionary format", dictionaryManifest, `"yomitan-zip"`, `"stardict"`, ErrBadField},
		{"dictionary zip format", dictionaryManifest, `"yomitan-zip"`, `"tsv"`, ErrBadField},
		{"dictionary no language", dictionaryManifest, `"to": ["en"]`, `"to": []`, ErrBadField},
		{"dictionary file path", dictionaryManifest, `["jmdict.zip"]`, `["../jmdict.zip"]`, ErrUnsafePath},
	} {
		if !strings.Contains(tt.manifest, tt.old) {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.old)
		}
		files := map[string]string{ManifestName: strings.Replace(tt.manifest, tt.old, tt.new, 1)}
		switch tt.manifest {
		case wordlistManifest:
			files["jlpt.tsv"] = "猫\tn5\n"
		case dictionaryManifest:
			files["jmdict.zip"] = string(zipBytes(t, []entry{{name: "index.json", content: "{}"}}))
		}
		_, err := Validate(writePack(t, files))
		wantErr(t, tt.name, err, tt.want)
	}
}

// TestCheckURL checks that an address a pack names has a host name and nothing a reader cannot see or that rearranges it.
func TestCheckURL(t *testing.T) {
	for _, raw := range []string{"https://llm.example.com/v1", "https://例子.example/keys", "http://127.0.0.1:11434/v1"} {
		if err := checkURL(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
	for name, raw := range map[string]string{
		"port alone":  "https://:443/v1",
		"no host":     "https:///v1",
		"bidi":        "https://example.com/\u202egnp.exe",
		"bidi host":   "https://moc.\u202eexample.com/v1",
		"zero width":  "https://open\u200bai.com/v1",
		"word joiner": "https://openai\u2060.com/v1",
	} {
		wantErr(t, name, checkURL(raw), ErrBadField)
	}
}

// TestValidateLoopback checks that an AI service on this computer may be reached over http.
func TestValidateLoopback(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:11434/v1", "http://localhost:1234/v1", "http://[::1]:8080/v1"} {
		manifest := strings.Replace(serviceManifest, `"https://llm.example.com/v1"`, fmt.Sprintf("%q", base), 1)
		if _, err := Validate(writePack(t, map[string]string{ManifestName: manifest})); err != nil {
			t.Errorf("%s: %v", base, err)
		}
	}
}

// TestValidateCleansNames checks that the characters that could disguise a name are taken out of it, rather than refusing a pack that has them.
func TestValidateCleansNames(t *testing.T) {
	manifest := strings.Replace(glossaryManifest, `"Anime terms"`, `"\u202eAnime\u200b terms\u2066 "`, 1)
	p, err := Validate(writePack(t, map[string]string{ManifestName: manifest}))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Name["en"]; got != "Anime terms" {
		t.Errorf("name = %q", got)
	}
}

func TestValidateGlossaryLimit(t *testing.T) {
	terms := make([]string, termsMax+1)
	for i := range terms {
		terms[i] = fmt.Sprintf(`{"source": "s%d", "target": "t%d"}`, i, i)
	}
	manifest := strings.Replace(glossaryManifest, `[{"source": "先輩", "target": "前辈"}]`, "["+strings.Join(terms[:termsMax], ",")+"]", 1)
	if _, err := Validate(writePack(t, map[string]string{ManifestName: manifest})); err != nil {
		t.Errorf("%d terms: %v", termsMax, err)
	}
	manifest = strings.Replace(glossaryManifest, `[{"source": "先輩", "target": "前辈"}]`, "["+strings.Join(terms, ",")+"]", 1)
	_, err := Validate(writePack(t, map[string]string{ManifestName: manifest}))
	wantErr(t, "too many terms", err, ErrBadField)
}

// TestValidateSite checks the rules of a site pack: its sites are kcore.ValidateVideoSites's, so a pattern that could take a page's script exponential time, or that JavaScript reads otherwise, is refused, as are a public suffix, a site Kando knows itself, overlapping domains and too many sites; and it is data alone, so a key for scripts, cookies, sign-in, yt-dlp's arguments or buttons in the pages is a key the format does not know.
func TestValidateSite(t *testing.T) {
	for _, tt := range []struct {
		name     string
		old, new string
		want     error
	}{
		{"nested quantifier", `"^/v/[0-9]+/?$"`, `"^/v/([0-9]+)+$"`, ErrBadSite},
		{"alternatives repeated", `"^/v/[0-9]+/?$"`, `"^/v/(a|aa)*$"`, ErrBadSite},
		{"backreference", `"^/v/[0-9]+/?$"`, `"^/v/([0-9])\\1$"`, ErrBadSite},
		{"lookahead", `"^/v/[0-9]+/?$"`, `"^/v/(?=[0-9])[0-9]+$"`, ErrBadSite},
		{"flags", `"^/v/[0-9]+/?$"`, `"^/v/(?i)x$"`, ErrBadSite},
		{"long pattern", `"^/v/[0-9]+/?$"`, `"^/` + strings.Repeat("v", 260) + `$"`, ErrBadSite},
		{"three unbounded", `"^/v/[0-9]+/?$"`, `"^/[a-z]+/[a-z]+/[0-9]+$"`, ErrBadSite},
		{"unanchored", `"^/v/[0-9]+/?$"`, `"/v/[0-9]+"`, ErrBadSite},
		{"value unanchored", `"^[0-9]+$"`, `"[0-9]+"`, ErrBadSite},
		{"not a host", `"videos.example.org", "not"`, `"https://videos.example.org", "not"`, ErrBadSite},
		{"not outside", `"live.videos.example.org"`, `"live.example.com"`, ErrBadSite},
		{"public suffix", `"clips.example.net"`, `"co.uk"`, ErrBadSite},
		{"private public suffix", `"clips.example.net"`, `"github.io"`, ErrBadSite},
		{"tld", `"clips.example.net"`, `"net"`, ErrBadSite},
		{"same domain", `"clips.example.net"`, `"videos.example.org"`, ErrBadSite},
		{"domain under another", `"clips.example.net"`, `"a.videos.example.org"`, ErrBadSite},
		{"kando's own", `"clips.example.net"`, `"youtube.com"`, ErrSiteKnown},
		{"under kando's own", `"clips.example.net"`, `"music.youtube.com"`, ErrSiteKnown},
		{"over kando's own", `"clips.example.net"`, `"bilibili.com"`, ErrSiteKnown},
		{"catalog", `"clips.example.net"`, `"twitch.tv"`, ErrSiteKnown},
		{"under the catalog", `"clips.example.net"`, `"clips.twitch.tv"`, ErrSiteKnown},
		{"no shape", `"videos": [{"path": "^/c/[a-z0-9]{6,12}$"}]`, `"videos": []`, ErrBadSite},
		{"script", `"site": {"sites": [`, `"site": {"script": "alert(1)", "sites": [`, ErrBadManifest},
		{"cookies", `"site": {"sites": [`, `"site": {"cookies": "chrome", "sites": [`, ErrBadManifest},
		{"sign-in", `{"domain": "clips.example.net",`, `{"domain": "clips.example.net", "signin": {"cookies": ["sid"]},`, ErrBadManifest},
		{"yt-dlp arguments", `{"domain": "clips.example.net",`, `{"domain": "clips.example.net", "args": ["--exec", "x"],`, ErrBadManifest},
		{"buttons", `{"domain": "clips.example.net",`, `{"domain": "clips.example.net", "rows": [{"selector": "#x"}],`, ErrBadManifest},
		{"menu", `{"path": "^/c/[a-z0-9]{6,12}$"}`, `{"path": "^/c/[a-z0-9]{6,12}$", "menu": ["/c/*"]}`, ErrBadManifest},
		{"two kinds", `"license": "MIT",`, `"license": "MIT", "glossary": {"from": "ja", "to": "en", "terms": [{"source": "a", "target": "b"}]},`, ErrBadField},
	} {
		manifest := strings.Replace(siteManifest, tt.old, tt.new, 1)
		if manifest == siteManifest {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.old)
		}
		_, err := Validate(writePack(t, map[string]string{ManifestName: manifest}))
		wantErr(t, tt.name, err, tt.want)
	}
	empty := strings.Replace(siteManifest, siteManifest[strings.Index(siteManifest, `"sites": [`):strings.LastIndex(siteManifest, "]}")+1], `"sites": []`, 1)
	_, err := Validate(writePack(t, map[string]string{ManifestName: empty}))
	wantErr(t, "no sites", err, ErrBadField)
	var sites []string
	for i := range sitesMax + 1 {
		sites = append(sites, fmt.Sprintf(`{"domain": "s%d.example.com", "videos": [{"path": "^/v$"}]}`, i))
	}
	manifest := strings.Replace(siteManifest, siteManifest[strings.Index(siteManifest, `"sites": [`):strings.LastIndex(siteManifest, "]}")+1], `"sites": [`+strings.Join(sites, ",")+`]`, 1)
	_, err = Validate(writePack(t, map[string]string{ManifestName: manifest}))
	wantErr(t, "too many sites", err, ErrBadField)
	p, err := Validate(writePack(t, map[string]string{ManifestName: siteManifest}))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Site.Domains(); len(got) != 2 || got[0] != "videos.example.org" || got[1] != "clips.example.net" {
		t.Fatalf("domains %q", got)
	}
}

// TestValidateUnsupportedKinds checks that the kinds of later phases are refused as not supported yet, not for keys this version does not know.
func TestValidateUnsupportedKinds(t *testing.T) {
	for _, kind := range []Kind{KindScript, KindTask, KindStyle, KindExportTemplate} {
		manifest := fmt.Sprintf(`{"schema": 1, "id": "example.x", "kind": %q, "future": {"anything": true}}`, kind)
		_, err := Validate(writePack(t, map[string]string{ManifestName: manifest}))
		wantErr(t, string(kind), err, ErrUnsupportedKind)
	}
}

func TestValidateShape(t *testing.T) {
	tsv := "猫\tn5\n"
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  error
	}{
		{"no manifest", map[string]string{"jlpt.tsv": tsv}, ErrBadManifest},
		{"named file missing", map[string]string{ManifestName: wordlistManifest}, ErrBadShape},
		{"file not named", map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": tsv, "extra.json": "{}"}, ErrBadShape},
		{"hidden file", map[string]string{ManifestName: wordlistManifest, "jlpt.tsv": tsv, ".hidden": ""}, ErrBadShape},
		{"not ASCII", map[string]string{ManifestName: strings.Replace(wordlistManifest, "jlpt.tsv", "単語.tsv", 1), "単語.tsv": tsv}, ErrBadShape},
		{"too many files", map[string]string{ManifestName: glossaryManifest, "1.md": "", "2.md": "", "3.md": "", "4.md": "", "5.md": "", "6.md": "", "7.md": "", "8.md": "", "9.md": "", "10.md": "", "11.md": "", "12.md": "", "13.md": "", "14.md": "", "15.md": "", "16.md": ""}, ErrTooLarge},
	} {
		_, err := Validate(writePack(t, tt.files))
		wantErr(t, tt.name, err, tt.want)
	}
}

func TestValidateRefusesFoldersAndLinks(t *testing.T) {
	dir := writePack(t, map[string]string{ManifestName: glossaryManifest})
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Validate(dir)
	wantErr(t, "folder", err, ErrBadShape)

	dir = writePack(t, map[string]string{ManifestName: wordlistManifest})
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "jlpt.tsv")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("making a link needs developer mode on Windows")
		}
		t.Fatal(err)
	}
	_, err = Validate(dir)
	wantErr(t, "link", err, ErrUnsafePath)
}

// TestValidateNestedZip checks a dictionary's Yomitan zip: it may not hold a link, a path out of it or another zip, and its files count against the pack's limits along with the pack's own.
func TestValidateNestedZip(t *testing.T) {
	for _, tt := range []struct {
		name  string
		inner []entry
		want  error
	}{
		{"zip in a zip", []entry{{name: "index.json", content: "{}"}, {name: "more.zip", content: "PK"}}, ErrBadShape},
		{"path out", []entry{{name: "../index.json", content: "{}"}}, ErrUnsafePath},
		{"link", []entry{{name: "index.json", content: "/etc/passwd", mode: os.ModeSymlink | 0o777}}, ErrUnsafePath},
		{"not a zip", nil, ErrBadArchive},
	} {
		inner := "not a zip"
		if tt.inner != nil {
			inner = string(zipBytes(t, tt.inner))
		}
		_, err := Validate(writePack(t, map[string]string{ManifestName: dictionaryManifest, "jmdict.zip": inner}))
		wantErr(t, tt.name, err, tt.want)
	}

	// The pack's two files and 63 in the zip are one more than a dictionary may have.
	limit := limitsOf(KindDictionary).entries
	many := make([]entry, limit-1)
	for i := range many {
		many[i] = entry{name: fmt.Sprintf("term_bank_%d.json", i), content: "[]"}
	}
	files := map[string]string{ManifestName: dictionaryManifest, "jmdict.zip": string(zipBytes(t, many[:limit-2]))}
	if _, err := Validate(writePack(t, files)); err != nil {
		t.Errorf("%d files: %v", limit, err)
	}
	files["jmdict.zip"] = string(zipBytes(t, many))
	_, err := Validate(writePack(t, files))
	wantErr(t, "too many files", err, ErrTooLarge)
}

// TestNestedBytesShareTheBudget checks that the bytes a nested zip unpacks to come out of what the pack has left, counted as they are read.
func TestNestedBytesShareTheBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.zip")
	if err := os.WriteFile(path, zipBytes(t, []entry{{name: "a.json", content: strings.Repeat("x", 600)}, {name: "b.json", content: strings.Repeat("y", 600)}}), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &budget{bytes: 1000, entries: 10}
	wantErr(t, "over the bytes left", checkNested(path, b), ErrTooLarge)
	b = &budget{bytes: 1200, entries: 10}
	if err := checkNested(path, b); err != nil || b.bytes != 0 || b.entries != 8 {
		t.Errorf("checkNested = %v, budget left %+v", err, *b)
	}
}

// TestNestedZipCountedOnce checks that a dictionary's Yomitan zip takes from the pack's bytes what its entries unpack to, and not also the zip's own bytes read to hash it.
func TestNestedZipCountedOnce(t *testing.T) {
	inner := zipBytes(t, []entry{{name: "term_bank_1.json", content: strings.Repeat("x", 600)}})
	dir := writePack(t, map[string]string{ManifestName: dictionaryManifest, "jmdict.zip": string(inner)})
	m, err := Parse([]byte(dictionaryManifest))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"jmdict.zip", ManifestName}
	unpacked := int64(len(dictionaryManifest) + 600)
	b := &budget{bytes: unpacked, entries: 10}
	if _, err := readFiles(dir, names, m, b); err != nil || b.bytes != 0 || b.entries != 9 {
		t.Errorf("readFiles = %v, budget left %+v", err, *b)
	}
	b = &budget{bytes: unpacked - 1, entries: 10}
	_, err = readFiles(dir, names, m, b)
	wantErr(t, "a byte short", err, ErrTooLarge)
}

func TestCheckFileName(t *testing.T) {
	for _, name := range []string{"jlpt.tsv", "term_bank-1.json", "README.md"} {
		if err := checkFileName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for name, want := range map[string]error{
		"../a.tsv": ErrUnsafePath, "/etc/passwd": ErrUnsafePath, `a\b.tsv`: ErrUnsafePath, "C:a.tsv": ErrUnsafePath, "..": ErrUnsafePath,
		"": ErrBadShape, ".hidden": ErrBadShape, "a.": ErrBadShape, "a b.tsv": ErrBadShape, "猫.tsv": ErrBadShape, "NUL.txt": ErrBadShape, "lpt1": ErrBadShape, strings.Repeat("a", 101): ErrBadShape,
	} {
		wantErr(t, name, checkFileName(name), want)
	}
}

func TestCleanText(t *testing.T) {
	for in, want := range map[string]string{
		" Anime\u200d terms ":   "Anime terms",
		"\u202eevil\u202c":      "evil",
		"\ufeffBOM":             "BOM",
		"a\u061cb\u2067c\u2069": "abc",
		"tab\there":             "tab here",
		"line\nbreak\x1b[2J":    "linebreak[2J",
	} {
		if got := cleanText(in, false); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cleanText("two\nlines", true); got != "two\nlines" {
		t.Errorf("cleanText with lines = %q", got)
	}
}

// TestNestedFoldersShareTheBudget checks that a nested zip's folder entries count against the files a pack may have, as its files do.
func TestNestedFoldersShareTheBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.zip")
	if err := os.WriteFile(path, zipBytes(t, []entry{{name: "a/", mode: os.ModeDir | 0o755}, {name: "a/b.json", content: "[]"}}), 0o644); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "one entry left", checkNested(path, &budget{bytes: 100, entries: 1}), ErrTooLarge)
	b := &budget{bytes: 100, entries: 2}
	if err := checkNested(path, b); err != nil || b.entries != 0 {
		t.Errorf("checkNested = %v, budget left %+v", err, *b)
	}
}

// TestNestedDamaged checks that a nested zip's file whose bytes do not match its checksum is ErrBadArchive.
func TestNestedDamaged(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateHeader(&zip.FileHeader{Name: "term_bank_1.json", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(`["hello world"]`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	damaged := bytes.Replace(buf.Bytes(), []byte("hello world"), []byte("hellO world"), 1)
	_, err = Validate(writePack(t, map[string]string{ManifestName: dictionaryManifest, "jmdict.zip": string(damaged)}))
	wantErr(t, "damaged", err, ErrBadArchive)
}

// TestOpenZipBoundsTheDirectory checks that a zip whose central directory is larger than a pack's zip can need is refused before archive/zip has read it all, and that the files of a zip it opens can still be read through it.
func TestOpenZipBoundsTheDirectory(t *testing.T) {
	many := make([]entry, 30000)
	for i := range many {
		many[i] = entry{name: fmt.Sprintf("entry_with_a_long_name_%06d.json", i)}
	}
	data := zipBytes(t, many)
	_, err := openZip(bytes.NewReader(data), int64(len(data)))
	wantErr(t, "huge directory", err, ErrTooLarge)

	data = zipBytes(t, []entry{{name: "a.json", content: strings.Repeat("x", 2<<20)}})
	zr, err := openZip(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	n, err := (&budget{bytes: 4 << 20}).copy(io.Discard, rc)
	if err != nil || n != 2<<20 {
		t.Errorf("read %d bytes, %v", n, err)
	}
}
