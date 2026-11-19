// Package dict reads the dictionary packs of Kando's plugins (plugins.KindDictionary) and looks words up in them offline. A pack's files, a Yomitan dictionary's zip, CC-CEDICT text or a table of tab-separated values, are read once, the first time the pack is opened after it was installed or changed, into a sorted index file kept beside the plugins (Open); a lookup then searches that file on disk, a few small reads, without reading the dictionary into memory. It imports no interface package. docs/plugin-system.md is the design.
package dict

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
)

// The index file: a header (magic, the number of keys, where the keys and the table start), the entries one after another, the keys, each with where its entry is, and the table of where each key is, in the keys' order, so a key is found by binary search over the table.
const (
	magic      = "KANDODX1"
	headerSize = 32
	// keyMax is the most bytes of a key; longer headwords are left out of the index, as no click looks them up.
	keyMax = 200
	// definitionMax is the most bytes of an entry's definition kept, cut at a character.
	definitionMax = 4000
	// keysMax is the most keys an index holds, more than the largest dictionaries have.
	keysMax = 8 << 20
	// matchesMax is the most entries a lookup reads of one key.
	matchesMax = 8
)

// ErrBadIndex is an index file that is not one this version writes, or is damaged; Open builds it again.
var ErrBadIndex = errors.New("the dictionary's index is damaged")

// entry is a dictionary's entry: its headword, its reading ("" when none) and its definition.
type entry struct {
	headword, reading, definition string
}

// Dict is a dictionary pack opened for lookups: its index file stays open until Close. It is safe for use from several goroutines at once; a lookup after Close finds nothing.
type Dict struct {
	id string

	mu    sync.RWMutex
	f     *os.File
	count int64
	table int64
}

// ID is the id of the pack the dictionary is.
func (d *Dict) ID() string { return d.id }

// IndexName is the name of the index file of the pack id whose content is hash, in the cache folder: the id, then the start of the hash after an underscore, which no id has, so the indexes of one pack are told from another's.
func IndexName(id, hash string) string {
	if len(hash) > 16 {
		hash = hash[:16]
	}
	return id + "_" + hash + ".idx"
}

// Open opens the dictionary pack p, building its index in the folder cache the first time it is opened with this content (IndexName), which reads the whole dictionary and takes a while for a large one, so it is called off the UI's goroutine. Indexes of the pack's other contents are left for the caller to remove (Forget) once it knows which content is wanted then, as an Open that finishes after its pack was replaced or removed must not remove the index of the content that replaced it. A dictionary none of whose files has an entry is an error. Before it builds an index it removes what a build of the same content cut short left behind (buildSuffix), so it is not called for one content while another call for it runs, as the app opens a content once at a time.
func Open(p plugins.Pack, cache string) (*Dict, error) {
	if p.Dictionary == nil {
		return nil, fmt.Errorf("dict: %s is not a dictionary", p.ID)
	}
	path := filepath.Join(cache, IndexName(p.ID, p.Hash))
	d, err := openIndex(p.ID, path)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrBadIndex) {
		return nil, err
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}
	removeBuilds(path)
	if err := build(p, path); err != nil {
		return nil, err
	}
	return openIndex(p.ID, path)
}

// Forget removes the index files of the pack id from the folder cache, but the one named keep, if any: those of contents it no longer has, or all of them once it is uninstalled; and the files builds of those contents were writing (buildSuffix), as one cut short leaves, while those of keep are left to its Open. A build of a content Forget removes, as one still running after its pack was replaced, then fails, which is all an index of a content no longer wanted is good for. An index still open elsewhere, as on Windows, stays until a later Forget.
func Forget(cache, id, keep string) {
	names, _ := filepath.Glob(filepath.Join(cache, id+"_*.idx"))
	builds, _ := filepath.Glob(filepath.Join(cache, id+"_*.idx"+buildSuffix+"*"))
	for _, name := range names {
		if filepath.Base(name) != keep {
			os.Remove(name)
		}
	}
	for _, name := range builds {
		if keep == "" || !strings.HasPrefix(filepath.Base(name), keep+buildSuffix) {
			os.Remove(name)
		}
	}
}

// removeBuilds removes the files builds of the index at path were writing (buildSuffix), as one cut short leaves.
func removeBuilds(path string) {
	names, _ := filepath.Glob(path + buildSuffix + "*")
	for _, name := range names {
		os.Remove(name)
	}
}

// Retry reports whether err, of Open or of a lookup, may not come again, so opening the dictionary again is worth it: an error reading or writing a file, as a full disk gives, or a damaged index, which Open builds again. One of a dictionary Kando cannot read, as one without an entry (ErrEmpty) or with a file it cannot parse, comes again until the dictionary changes.
func Retry(err error) bool {
	var path *fs.PathError
	var link *os.LinkError
	return err != nil && !errors.Is(err, ErrEmpty) && (errors.Is(err, ErrBadIndex) || errors.As(err, &path) || errors.As(err, &link))
}

// openIndex opens the index file at path, checking its header against its size.
func openIndex(id, path string) (*Dict, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var h [headerSize]byte
	info, err := f.Stat()
	if err == nil {
		_, err = f.ReadAt(h[:], 0)
	}
	if err != nil {
		f.Close()
		if errors.Is(err, io.EOF) {
			return nil, ErrBadIndex
		}
		return nil, err
	}
	count := int64(binary.BigEndian.Uint64(h[8:]))
	keys, table := int64(binary.BigEndian.Uint64(h[16:])), int64(binary.BigEndian.Uint64(h[24:]))
	if string(h[:8]) != magic || count < 0 || count > keysMax || keys < headerSize || table < keys || table+count*8 != info.Size() {
		f.Close()
		return nil, ErrBadIndex
	}
	return &Dict{id: id, f: f, count: count, table: table}, nil
}

// Close closes the index file; the dictionary finds nothing after.
func (d *Dict) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return nil
	}
	err := d.f.Close()
	d.f = nil
	return err
}

// key is how a word is found in the index: trimmed and in lower case.
func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// LookUp looks the word up: by base, its dictionary form when the caller knows it (segment.Annotated), first, then as clicked. reading, the word's reading when known (katakana or hiragana alike), picks the entries read so when the headword has several; it is the reading of the word as clicked, so it picks only when the word was found as clicked, not by a base it inflects from (行った is read イッタ, its base 行く いく or ゆく). The entry's Word is the headword, its Reading the reading of the first entry shown and its Meaning their definitions, one a line, each after its reading when they are read differently.
func (d *Dict) LookUp(word, base, reading string) (lookup.Entry, bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.f == nil {
		return lookup.Entry{}, false, nil
	}
	for _, k := range []string{key(base), key(word)} {
		if k == "" {
			continue
		}
		found, err := d.find(k)
		if err != nil {
			return lookup.Entry{}, false, err
		}
		if len(found) > 0 {
			if k != key(word) {
				reading = ""
			}
			return compose(found, reading), true, nil
		}
	}
	return lookup.Entry{}, false, nil
}

// find is the entries of key k, at most matchesMax, in the order the dictionary has them.
func (d *Dict) find(k string) ([]entry, error) {
	var ferr error
	keyAt := func(i int64) (string, int64) {
		var off [8]byte
		if _, err := d.f.ReadAt(off[:], d.table+i*8); err != nil {
			ferr = err
			return "", 0
		}
		buf := make([]byte, keyMax+2*binary.MaxVarintLen64)
		n, err := d.f.ReadAt(buf, int64(binary.BigEndian.Uint64(off[:])))
		if err != nil && !errors.Is(err, io.EOF) {
			ferr = err
			return "", 0
		}
		r := bytes.NewReader(buf[:n])
		s, err := readString(r, keyMax)
		if err != nil {
			ferr = ErrBadIndex
			return "", 0
		}
		rec, err := binary.ReadUvarint(r)
		if err != nil {
			ferr = ErrBadIndex
			return "", 0
		}
		return s, int64(rec)
	}
	lo, hi := int64(0), d.count
	for lo < hi && ferr == nil {
		mid := lo + (hi-lo)/2
		if s, _ := keyAt(mid); s < k {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	var out []entry
	for i := lo; i < d.count && len(out) < matchesMax && ferr == nil; i++ {
		s, rec := keyAt(i)
		if ferr != nil || s != k {
			break
		}
		e, err := d.entryAt(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, ferr
}

// entryAt reads the entry at offset off of the index.
func (d *Dict) entryAt(off int64) (entry, error) {
	var head [binary.MaxVarintLen64]byte
	n, err := d.f.ReadAt(head[:], off)
	if err != nil && !errors.Is(err, io.EOF) {
		return entry{}, err
	}
	size, w := binary.Uvarint(head[:n])
	if w <= 0 || size > 3*(keyMax+definitionMax) {
		return entry{}, ErrBadIndex
	}
	buf := make([]byte, size)
	if _, err := d.f.ReadAt(buf, off+int64(w)); err != nil {
		return entry{}, ErrBadIndex
	}
	r := bytes.NewReader(buf)
	var e entry
	for _, s := range []*string{&e.headword, &e.reading, &e.definition} {
		if *s, err = readString(r, definitionMax); err != nil {
			return entry{}, ErrBadIndex
		}
	}
	return e, nil
}

// readString reads a string of at most max bytes, written after its length.
func readString(r *bytes.Reader, max int) (string, error) {
	n, err := binary.ReadUvarint(r)
	if err != nil || n > uint64(max) || n > uint64(r.Len()) {
		return "", ErrBadIndex
	}
	b := make([]byte, n)
	_, err = io.ReadFull(r, b)
	return string(b), err
}

// compose is the lookup entry of found, the entries of one key: those read as reading, when some are, else all of them; their definitions one a line, each after its reading when not all are read alike.
func compose(found []entry, reading string) lookup.Entry {
	if want := hiragana(reading); want != "" {
		var same []entry
		for _, e := range found {
			if hiragana(e.reading) == want || (e.reading == "" && hiragana(e.headword) == want) {
				same = append(same, e)
			}
		}
		if len(same) > 0 {
			found = same
		}
	}
	readings := map[string]bool{}
	for _, e := range found {
		readings[e.reading] = true
	}
	var lines []string
	for _, e := range found {
		line := e.definition
		if len(readings) > 1 && e.reading != "" {
			line = e.reading + ": " + line
		}
		if line != "" && !slices.Contains(lines, line) {
			lines = append(lines, line)
		}
	}
	return lookup.Entry{Word: found[0].headword, Reading: found[0].reading, Meaning: strings.Join(lines, "\n")}
}

// hiragana is s with its katakana written in hiragana, so a reading from the IPA dictionary (katakana) matches one of a dictionary (hiragana).
func hiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, strings.TrimSpace(s))
}

// clip is s cut to at most max bytes, at a character.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	i := max
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i]
}
