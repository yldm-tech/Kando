package index

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// testKey is a throwaway index key.
func testKey(t testing.TB) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return private, public
}

// testEntry is a valid glossary entry of id, whose file is data.
func testEntry(id string, data []byte) Entry {
	sum := sha256.Sum256(data)
	publisher, _, _ := strings.Cut(id, ".")
	return Entry{ID: id, Kind: plugins.KindGlossary, Name: plugins.Text{"en": "Terms " + id, "zh-Hans": "术语"}, Summary: plugins.Text{"en": "Some terms."},
		Version: "1.0.0", Publisher: publisher, License: "CC-BY-4.0", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
		URL: "https://plugins.example.org/packs/" + id + ".zip", Languages: []string{"ja", "zh-Hans"}}
}

var testTime = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

// testIndex is an index of entries made at when.
func testIndex(when time.Time, entries ...Entry) Index {
	return Index{Format: Format, Version: Version, Generated: when, Entries: entries}
}

// signRaw signs body as an index file with key, whatever body holds, as a test needs to sign an index Sign would refuse.
func signRaw(key ed25519.PrivateKey, body []byte) []byte {
	f := signedFile{Format: Format, Version: Version, Signer: pkg.KeyID(key.Public().(ed25519.PublicKey)),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, framed(body))), Body: base64.StdEncoding.EncodeToString(body)}
	data, _ := json.Marshal(f)
	return data
}

func mustSign(t testing.TB, key ed25519.PrivateKey, idx Index) []byte {
	t.Helper()
	data, err := Sign(idx, key)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestIndexSignature checks that an index is read only when the index key signed exactly it: the same file under another key, without a signature, with its body changed, or naming another signer is refused, and a Kando without a key reads none.
func TestIndexSignature(t *testing.T) {
	key, public := testKey(t)
	other, otherPublic := testKey(t)
	data := mustSign(t, key, testIndex(testTime, testEntry("example.terms", []byte("x"))))
	idx, err := Parse(data, public)
	if err != nil || len(idx.Entries) != 1 || idx.Entries[0].ID != "example.terms" {
		t.Fatalf("Parse = %+v, %v", idx, err)
	}
	if _, err := Parse(data, otherPublic); !errors.Is(err, ErrSignature) {
		t.Errorf("another key: %v", err)
	}
	if _, err := Parse(data, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: %v", err)
	}
	if _, err := DecodeKey(""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("DecodeKey(\"\"): %v", err)
	}
	var f signedFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	change := func(edit func(f *signedFile)) []byte {
		g := f
		edit(&g)
		out, _ := json.Marshal(g)
		return out
	}
	body, _ := base64.StdEncoding.DecodeString(f.Body)
	tampered := bytes.Replace(body, []byte("1.0.0"), []byte("9.0.0"), 1)
	otherSigned := signRaw(other, body)
	var o signedFile
	json.Unmarshal(otherSigned, &o)
	for name, data := range map[string][]byte{
		"unsigned":      change(func(f *signedFile) { f.Signature = "" }),
		"tampered body": change(func(f *signedFile) { f.Body = base64.StdEncoding.EncodeToString(tampered) }),
		"bad signature": change(func(f *signedFile) {
			f.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
		}),
		"another signer": otherSigned,
		"signer renamed": change(func(f *signedFile) { f.Signature = o.Signature }),
		"not base64":     change(func(f *signedFile) { f.Signature = "!!" }),
	} {
		if _, err := Parse(data, public); !errors.Is(err, ErrSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestIndexFormat checks the file's shape: a key this version does not know, another format, a newer version, the format inside and outside the signature differing, and data after the index are refused.
func TestIndexFormat(t *testing.T) {
	key, public := testKey(t)
	good := mustSign(t, key, testIndex(testTime, testEntry("example.terms", []byte("x"))))
	var f map[string]any
	json.Unmarshal(good, &f)
	edit := func(k string, v any) []byte {
		g := map[string]any{}
		for kk, vv := range f {
			g[kk] = vv
		}
		g[k] = v
		out, _ := json.Marshal(g)
		return out
	}
	for name, c := range map[string]struct {
		data []byte
		want error
	}{
		"unknown key":   {edit("extra", 1), ErrFormat},
		"other format":  {edit("format", "something-else"), ErrFormat},
		"newer version": {edit("version", 2), ErrVersion},
		"not JSON":      {[]byte("<html>"), ErrFormat},
		"trailing data": {append(append([]byte{}, good...), []byte(" {}")...), ErrFormat},
		"inner version": {signRaw(key, []byte(`{"format":"kando-plugins-index","version":2,"generated":"2026-11-19T10:00:00Z","entries":[]}`)), ErrFormat},
		"inner unknown": {signRaw(key, []byte(`{"format":"kando-plugins-index","version":1,"generated":"2026-11-19T10:00:00Z","entries":[],"x":1}`)), ErrFormat},
		"no time":       {signRaw(key, []byte(`{"format":"kando-plugins-index","version":1,"generated":"0001-01-01T00:00:00Z","entries":[]}`)), ErrFormat},
		"entry key":     {signRaw(key, []byte(`{"format":"kando-plugins-index","version":1,"generated":"2026-11-19T10:00:00Z","entries":[{"id":"a.b","extra":1}]}`)), ErrFormat},
	} {
		if _, err := Parse(c.data, public); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}

// TestIndexEntryFields checks every field of an entry against the rules its pack's manifest follows and its own: one bad entry refuses the whole index, as does an id listed twice.
func TestIndexEntryFields(t *testing.T) {
	key, public := testKey(t)
	script := func(e *Entry) { e.Kind, e.Source, e.Languages = plugins.KindScript, plugins.SourceVerified, nil }
	for name, edit := range map[string]func(e *Entry){
		"no dot in id":          func(e *Entry) { e.ID, e.Publisher = "terms", "terms" },
		"id with ..":            func(e *Entry) { e.ID = "example..terms" },
		"kando data pack":       func(e *Entry) { e.ID, e.Publisher = "kando.terms", "kando" },
		"publisher not the id":  func(e *Entry) { e.Publisher = "acme" },
		"unknown kind":          func(e *Entry) { e.Kind = "theme" },
		"no English name":       func(e *Entry) { e.Name = plugins.Text{"ja": "用語"} },
		"reserved name":         func(e *Entry) { e.Name = plugins.Text{"en": "Kando"} },
		"long name":             func(e *Entry) { e.Name = plugins.Text{"en": strings.Repeat("a", 41)} },
		"bad name language":     func(e *Entry) { e.Name["zh-hans"] = "术语" },
		"long summary":          func(e *Entry) { e.Summary = plugins.Text{"en": strings.Repeat("a", 501)} },
		"bad version":           func(e *Entry) { e.Version = "1.0 beta" },
		"no license":            func(e *Entry) { e.License = " " },
		"http homepage":         func(e *Entry) { e.Homepage = "http://example.org" },
		"empty file":            func(e *Entry) { e.Size = 0 },
		"file over its kind":    func(e *Entry) { e.Size = plugins.ZipMax(plugins.KindGlossary) + 1 },
		"uppercase sha256":      func(e *Entry) { e.SHA256 = strings.ToUpper(e.SHA256) },
		"short sha256":          func(e *Entry) { e.SHA256 = e.SHA256[:63] },
		"http url":              func(e *Entry) { e.URL = "http://plugins.example.org/x.zip" },
		"http loopback url":     func(e *Entry) { e.URL = "http://127.0.0.1/x.zip" },
		"url with a password":   func(e *Entry) { e.URL = "https://user:pw@plugins.example.org/x.zip" },
		"url with a bidi":       func(e *Entry) { e.URL = "https://plugins.example.org/\u202ex.zip" },
		"url without a host":    func(e *Entry) { e.URL = "https:///x.zip" },
		"bad language":          func(e *Entry) { e.Languages = []string{"Japanese"} },
		"language twice":        func(e *Entry) { e.Languages = []string{"ja", "ja"} },
		"too many languages":    func(e *Entry) { e.Languages = strings.Fields(strings.Repeat("ja ", MaxLanguages+1)) },
		"minKando of data":      func(e *Entry) { e.MinKando = "0.30.0" },
		"bad minKando":          func(e *Entry) { script(e); e.MinKando = ">=0.30.0" },
		"script without source": func(e *Entry) { script(e); e.Source = "" },
		"developer script":      func(e *Entry) { script(e); e.Source = plugins.SourceDeveloper },
		"data pack with source": func(e *Entry) { e.Source = plugins.SourceOfficial },
	} {
		e := testEntry("example.terms", []byte("x"))
		edit(&e)
		body, _ := json.Marshal(testIndex(testTime, e))
		if _, err := Parse(signRaw(key, body), public); !errors.Is(err, ErrBadEntry) {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := Sign(testIndex(testTime, e), key); !errors.Is(err, ErrBadEntry) {
			t.Errorf("%s: Sign took it: %v", name, err)
		}
	}
	twice := testIndex(testTime, testEntry("example.terms", []byte("x")), testEntry("example.terms", []byte("y")))
	if _, err := Sign(twice, key); !errors.Is(err, ErrBadEntry) {
		t.Errorf("an id twice: %v", err)
	}
	// What a reader cannot see is cleaned from the texts, as from a manifest's.
	e := testEntry("example.terms", []byte("x"))
	e.Name["en"] = "Terms\u200b"
	script(&e)
	e.Source, e.MinKando = plugins.SourceOfficial, "0.30.0"
	idx, err := Parse(mustSign(t, key, testIndex(testTime, e)), public)
	if err != nil || idx.Entries[0].Name["en"] != "Terms" {
		t.Fatalf("a valid script entry: %+v, %v", idx, err)
	}
	if idx.Entries[0].RunsOn("0.29.9") || !idx.Entries[0].RunsOn("0.30.0") || !idx.Entries[0].RunsOn("dev") {
		t.Error("RunsOn does not follow minKando")
	}
}

// TestIndexSizeCaps checks that an index file over MaxBytes and one with more than MaxEntries entries are refused.
func TestIndexSizeCaps(t *testing.T) {
	key, public := testKey(t)
	if _, err := Parse(make([]byte, MaxBytes+1), public); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a file over MaxBytes: %v", err)
	}
	entries := make([]Entry, MaxEntries+1)
	for i := range entries {
		entries[i] = testEntry(fmt.Sprintf("example.t%d", i), []byte("x"))
	}
	if _, err := Sign(testIndex(testTime, entries...), key); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Sign over MaxEntries: %v", err)
	}
	body, _ := json.Marshal(testIndex(testTime, entries...))
	if data := signRaw(key, body); len(data) <= MaxBytes {
		if _, err := Parse(data, public); !errors.Is(err, ErrTooLarge) {
			t.Errorf("Parse over MaxEntries: %v", err)
		}
	}
}

// indexServer serves the index file *served at /index.json, with its SHA-256 as its ETag, answering 304 to that ETag; hits counts the requests and full those answered with the file.
type indexServer struct {
	*httptest.Server
	served     atomic.Pointer[[]byte]
	hits, full atomic.Int32
	mux        *http.ServeMux
}

func newIndexServer(t *testing.T, data []byte) *indexServer {
	t.Helper()
	s := &indexServer{mux: http.NewServeMux()}
	s.served.Store(&data)
	s.mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		data := *s.served.Load()
		sum := sha256.Sum256(data)
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		s.full.Add(1)
		w.Header().Set("ETag", etag)
		w.Write(data)
	})
	s.Server = httptest.NewTLSServer(s.mux)
	t.Cleanup(s.Close)
	return s
}

func (s *indexServer) source(t *testing.T, key ed25519.PublicKey) Source {
	return Source{URL: s.URL + "/index.json", Key: key, Dir: CacheDir(t.TempDir()), Client: s.Client()}
}

// TestIndexRefresh checks the cache: the first read keeps the index and its ETag, the next asks with the ETag and keeps the copy on 304; it is due again a day later; a newer index replaces it, an older one (rollback), a tampered one and one over the limit are refused and leave the copy kept as it was.
func TestIndexRefresh(t *testing.T) {
	key, public := testKey(t)
	first := mustSign(t, key, testIndex(testTime, testEntry("example.terms", []byte("x"))))
	srv := newIndexServer(t, first)
	src := srv.source(t, public)
	ctx := context.Background()
	if !src.Due(testTime) {
		t.Error("an empty cache is not due")
	}
	if _, _, err := src.Cached(); err == nil {
		t.Error("an empty cache read")
	}
	idx, meta, err := src.Refresh(ctx, testTime)
	if err != nil || len(idx.Entries) != 1 || meta.ETag == "" || !meta.Fetched.Equal(testTime) || meta.URL != src.URL {
		t.Fatalf("Refresh = %+v, %+v, %v", idx, meta, err)
	}
	if src.Due(testTime.Add(time.Hour)) || !src.Due(testTime.Add(RefreshEvery)) {
		t.Error("Due does not follow RefreshEvery")
	}
	later := testTime.Add(2 * time.Hour)
	if _, meta, err := src.Refresh(ctx, later); err != nil || srv.full.Load() != 1 || srv.hits.Load() != 2 || !meta.Fetched.Equal(later) {
		t.Fatalf("a second Refresh: %+v, %v; %d hits, %d full", meta, err, srv.hits.Load(), srv.full.Load())
	}
	if _, meta, err := src.Cached(); err != nil || !meta.Fetched.Equal(later) {
		t.Errorf("Cached after a 304: %+v, %v", meta, err)
	}

	newer := mustSign(t, key, testIndex(testTime.Add(time.Hour), testEntry("example.terms", []byte("x")), testEntry("example.more", []byte("y"))))
	srv.served.Store(&newer)
	if idx, _, err := src.Refresh(ctx, later); err != nil || len(idx.Entries) != 2 {
		t.Fatalf("a newer index: %+v, %v", idx, err)
	}

	keep := func(name string, data []byte, want error) {
		t.Helper()
		srv.served.Store(&data)
		if _, _, err := src.Refresh(ctx, later); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
		if idx, _, err := src.Cached(); err != nil || len(idx.Entries) != 2 {
			t.Errorf("%s: the copy kept is now %+v, %v", name, idx, err)
		}
	}
	keep("an older index", first, ErrRollback)
	other, _ := testKey(t)
	keep("another key's", mustSign(t, other, testIndex(testTime.Add(3*time.Hour))), ErrSignature)
	keep("too large", bytes.Repeat([]byte(" "), MaxBytes+1), ErrTooLarge)

	// A server error keeps the copy too.
	srv.mux.HandleFunc("/gone.json", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "gone", http.StatusGone) })
	gone := src
	gone.URL = srv.URL + "/gone.json"
	if _, _, err := gone.Refresh(ctx, later); !errors.Is(err, ErrHTTP) {
		t.Errorf("a 410: %v", err)
	}
	if !gone.Due(later) {
		t.Error("an index from another address is not due")
	}
	// Only https.
	plain := src
	plain.URL = strings.Replace(src.URL, "https:", "http:", 1)
	if _, _, err := plain.Refresh(ctx, later); !errors.Is(err, plugins.ErrInsecureURL) {
		t.Errorf("an http index: %v", err)
	}
	if _, _, err := (Source{URL: src.URL, Dir: src.Dir}).Refresh(ctx, later); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: %v", err)
	}
}

// TestIndexRedirect checks that a request follows a redirect within its host, and refuses one to another host.
func TestIndexRedirect(t *testing.T) {
	key, public := testKey(t)
	data := mustSign(t, key, testIndex(testTime))
	srv := newIndexServer(t, data)
	elsewhere := newIndexServer(t, data)
	srv.mux.HandleFunc("/moved.json", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/index.json", http.StatusFound) })
	srv.mux.HandleFunc("/away.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/index.json", http.StatusFound)
	})
	src := srv.source(t, public)
	src.URL = srv.URL + "/moved.json"
	if _, _, err := src.Refresh(context.Background(), testTime); err != nil {
		t.Errorf("a redirect within the host: %v", err)
	}
	src.URL = srv.URL + "/away.json"
	if _, _, err := src.Refresh(context.Background(), testTime); !errors.Is(err, ErrRedirect) {
		t.Errorf("a redirect to another host: %v", err)
	}
	if elsewhere.hits.Load() != 0 {
		t.Error("the other host was asked")
	}
}

// TestIndexDownload checks a download: kept when its size and SHA-256 are the entry's, refused and removed when either differs, cut off past the entry's size, and only from https.
func TestIndexDownload(t *testing.T) {
	pack := []byte("PK pack bytes")
	srv := newIndexServer(t, nil)
	files := map[string][]byte{"/ok.zip": pack, "/other.zip": []byte("PK pack bytez"), "/short.zip": pack[:5], "/long.zip": append(append([]byte{}, pack...), "and more"...)}
	for path, data := range files {
		srv.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	}
	src := srv.source(t, nil)
	dir := t.TempDir()
	entry := func(path string) Entry {
		e := testEntry("example.terms", pack)
		e.URL = srv.URL + path
		return e
	}
	path, err := src.Download(context.Background(), entry("/ok.zip"), dir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, pack) {
		t.Errorf("downloaded %q", got)
	}
	os.Remove(path)
	for p, want := range map[string]error{"/other.zip": ErrChecksum, "/short.zip": ErrChecksum, "/long.zip": ErrTooLarge, "/missing.zip": ErrHTTP} {
		if _, err := src.Download(context.Background(), entry(p), dir); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", p, err, want)
		}
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("refused downloads left %d files", len(left))
	}
	plain := entry("/ok.zip")
	plain.URL = strings.Replace(plain.URL, "https:", "http:", 1)
	if _, err := src.Download(context.Background(), plain, dir); !errors.Is(err, plugins.ErrInsecureURL) {
		t.Errorf("an http download: %v", err)
	}
}

// TestIndexNewer checks how versions compare.
func TestIndexNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true}, {"1.10.0", "1.9.0", true}, {"1.0", "1.0.0", false}, {"1.0.0", "1.0", false}, {"2", "1.9.9", true},
		{"1.0.0", "1.0.0-beta", true}, {"1.0.0-beta", "1.0.0", false}, {"1.0.0-beta.2", "1.0.0-beta.1", true}, {"1.0.0+b2", "1.0.0+b1", false},
		{"2026.11.0", "2026.2.5", true}, {"1.0.a", "1.0.1", true}, {"0.9.0", "1.0.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// TestIndexFromPack checks the entry made of a pack: the fields its manifest gives, its languages by kind, and its file's size, hash and address; and that it matches the pack it was made of only.
func TestIndexFromPack(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	manifest := `{"schema": 1, "id": "example.anime", "kind": "glossary", "version": "1.2.0", "license": "CC-BY-4.0", "homepage": "https://example.org", "name": {"en": "Anime terms"}, "description": {"en": "Terms."}, "glossary": {"from": "ja", "to": "zh-Hans", "terms": [{"source": "先輩", "target": "前辈"}]}}`
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(manifest), 0o644)
	_, p, err := plugins.Stage(root, src)
	if err != nil {
		t.Fatal(err)
	}
	e := FromPack(p, "https://plugins.example.org/packs/anime.zip", 123, sha256.Sum256([]byte("zip")), plugins.SourceLocal)
	if err := e.Check(); err != nil {
		t.Fatal(err)
	}
	if e.Publisher != "example" || e.Summary["en"] != "Terms." || strings.Join(e.Languages, ",") != "ja,zh-Hans" || e.Homepage != "https://example.org" || e.Size != 123 {
		t.Errorf("FromPack = %+v", e)
	}
	if err := e.Matches(p); err != nil {
		t.Error(err)
	}
	p.Version = "1.3.0"
	if err := e.Matches(p); !errors.Is(err, ErrNotListed) {
		t.Errorf("another version matched: %v", err)
	}
}
