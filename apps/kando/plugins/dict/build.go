package dict

import (
	"archive/zip"
	"bufio"
	"cmp"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/plugins"
)

// ErrEmpty is a dictionary none of whose files has an entry Kando reads, which Open refuses.
var ErrEmpty = errors.New("dict: the dictionary has no entry Kando can read")

// indexKey is a key of the index and where its entry is.
type indexKey struct {
	key string
	at  int64
}

// writer writes the entries of an index after its header, keeping their keys for the table.
type writer struct {
	w    *bufio.Writer
	at   int64
	keys []indexKey
}

// add writes e, its definition clipped (definitionMax), and keeps each of keys for it, once each, those over keyMax bytes left out. An entry without a headword or a definition is left out.
func (w *writer) add(e entry, keys ...string) error {
	e.headword, e.reading = clip(strings.TrimSpace(e.headword), keyMax), clip(strings.TrimSpace(e.reading), keyMax)
	e.definition = clip(strings.TrimSpace(e.definition), definitionMax)
	if e.headword == "" || e.definition == "" {
		return nil
	}
	var body []byte
	for _, s := range []string{e.headword, e.reading, e.definition} {
		body = binary.AppendUvarint(body, uint64(len(s)))
		body = append(body, s...)
	}
	rec := binary.AppendUvarint(nil, uint64(len(body)))
	rec = append(rec, body...)
	var added []string
	for _, k := range keys {
		k = key(k)
		if k == "" || len(k) > keyMax || slices.Contains(added, k) {
			continue
		}
		added = append(added, k)
		if len(w.keys) >= keysMax {
			return fmt.Errorf("dict: the dictionary has over %d words", keysMax)
		}
		w.keys = append(w.keys, indexKey{key: k, at: w.at})
	}
	if len(added) == 0 {
		return nil
	}
	n, err := w.w.Write(rec)
	w.at += int64(n)
	return err
}

// buildSuffix starts the name of the file an index is written to before it is renamed into place, after the index's own name (IndexName), so the file a build left behind when it was cut short is known by its pack and content (Forget, Open).
const buildSuffix = ".build-"

// build reads the files of the dictionary pack p and writes its index at path, through a file of its own beside it (buildSuffix) that is renamed into place once whole, so an index is never read half written.
func build(p plugins.Pack, path string) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+buildSuffix+"*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	w := &writer{w: bufio.NewWriterSize(f, 1<<20), at: headerSize}
	if _, err := w.w.Write(make([]byte, headerSize)); err != nil {
		return err
	}
	for _, name := range p.Dictionary.Files {
		file := filepath.Join(p.Dir, name)
		switch p.Dictionary.Format {
		case plugins.FormatYomitan:
			err = readYomitan(file, w)
		case plugins.FormatCCCEDICT:
			err = readLines(file, w, cedictLine)
		case plugins.FormatTSV:
			err = readLines(file, w, tsvLine)
		default:
			err = fmt.Errorf("dict: format %q", p.Dictionary.Format)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if len(w.keys) == 0 {
		return ErrEmpty
	}
	// Entries of one key stay in the order the dictionary has them.
	slices.SortStableFunc(w.keys, func(a, b indexKey) int { return strings.Compare(a.key, b.key) })
	keysAt := w.at
	table := make([]byte, 0, len(w.keys)*8)
	for _, k := range w.keys {
		table = binary.BigEndian.AppendUint64(table, uint64(w.at))
		rec := binary.AppendUvarint(nil, uint64(len(k.key)))
		rec = append(rec, k.key...)
		rec = binary.AppendUvarint(rec, uint64(k.at))
		n, err := w.w.Write(rec)
		if err != nil {
			return err
		}
		w.at += int64(n)
	}
	tableAt := w.at
	if _, err := w.w.Write(table); err != nil {
		return err
	}
	if err := w.w.Flush(); err != nil {
		return err
	}
	var h [headerSize]byte
	copy(h[:], magic)
	binary.BigEndian.PutUint64(h[8:], uint64(len(w.keys)))
	binary.BigEndian.PutUint64(h[16:], uint64(keysAt))
	binary.BigEndian.PutUint64(h[24:], uint64(tableAt))
	if _, err := f.WriteAt(h[:], 0); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// record is an entry of a line of text and the keys it is found by.
type record struct {
	entry
	keys []string
}

// readLines reads the text file at name a line at a time, a byte order mark at its start left out, each through line, which returns the entries of a line, none for a comment or a line it does not read.
func readLines(name string, w *writer, line func(s string) []record) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	first := true
	for sc.Scan() {
		s := strings.TrimSuffix(sc.Text(), "\r")
		if first {
			s, first = strings.TrimPrefix(s, "\uFEFF"), false
		}
		for _, r := range line(s) {
			if err := w.add(r.entry, r.keys...); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// cedictLine reads a line of CC-CEDICT, "傳統 传统 [chuan2 tong3] /tradition/convention/": an entry for each of its two forms, Traditional and Simplified, one when they are the same, each found by its own form and headed by it, so a word clicked in Traditional characters shows in them; its reading the pinyin with tone marks (pinyin) and its definitions joined by "; ". A comment (#) is none.
func cedictLine(s string) []record {
	if strings.HasPrefix(s, "#") {
		return nil
	}
	trad, rest, ok1 := strings.Cut(s, " ")
	simp, rest, ok2 := strings.Cut(rest, " ")
	if !ok1 || !ok2 || !strings.HasPrefix(rest, "[") {
		return nil
	}
	reading, rest, ok := strings.Cut(rest[1:], "]")
	if !ok {
		return nil
	}
	var defs []string
	for _, d := range strings.Split(strings.TrimSpace(rest), "/") {
		if d = strings.TrimSpace(d); d != "" {
			defs = append(defs, d)
		}
	}
	e := entry{headword: simp, reading: pinyin(reading), definition: strings.Join(defs, "; ")}
	out := []record{{entry: e, keys: []string{simp}}}
	if trad != simp {
		e.headword = trad
		out = append(out, record{entry: e, keys: []string{trad}})
	}
	return out
}

// tsvLine reads a line of a table of tab-separated values: "headword<TAB>reading<TAB>definition", or "headword<TAB>definition" without a reading; columns after the third are left out. A comment (#) or an empty line is none.
func tsvLine(s string) []record {
	if strings.HasPrefix(s, "#") || strings.TrimSpace(s) == "" {
		return nil
	}
	cols := strings.Split(s, "\t")
	var e entry
	switch {
	case len(cols) == 2:
		e = entry{headword: cols[0], definition: cols[1]}
	case len(cols) >= 3:
		e = entry{headword: cols[0], reading: cols[1], definition: cols[2]}
	default:
		return nil
	}
	return []record{{entry: e, keys: []string{e.headword, e.reading}}}
}

// readYomitan reads a Yomitan dictionary, the zip at name: its term banks (term_bank_<n>.json), each a JSON array of terms, [expression, reading, definition tags, rules, score, glossary, sequence, term tags] in the format of version 3 and [expression, reading, tags, rules, score, glossary…] in that of version 1. A term is found by its expression and by its reading. The pack passed plugins.Validate, which bounds the zip's entries and the bytes they unpack to.
func readYomitan(name string, w *writer) error {
	zr, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer zr.Close()
	var banks []*zip.File
	for _, zf := range zr.File {
		base := path.Base(zf.Name)
		if strings.HasPrefix(base, "term_bank_") && strings.HasSuffix(base, ".json") && zf.Mode().IsRegular() {
			banks = append(banks, zf)
		}
	}
	// term_bank_2 before term_bank_10, as Yomitan numbers them.
	slices.SortFunc(banks, func(a, b *zip.File) int {
		x, y := path.Base(a.Name), path.Base(b.Name)
		return cmp.Or(cmp.Compare(len(x), len(y)), strings.Compare(x, y), strings.Compare(a.Name, b.Name))
	})
	for _, zf := range banks {
		if err := readTermBank(zf, w); err != nil {
			return fmt.Errorf("%s: %w", zf.Name, err)
		}
	}
	return nil
}

func readTermBank(zf *zip.File, w *writer) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := json.NewDecoder(rc)
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return fmt.Errorf("not a term bank")
	}
	for dec.More() {
		var term []json.RawMessage
		if err := dec.Decode(&term); err != nil {
			return err
		}
		if len(term) < 6 {
			continue
		}
		var expression, reading string
		if json.Unmarshal(term[0], &expression) != nil || json.Unmarshal(term[1], &reading) != nil {
			continue
		}
		var defs []string
		if len(term[5]) > 0 && term[5][0] == '[' {
			var items []json.RawMessage
			if json.Unmarshal(term[5], &items) == nil {
				for _, item := range items {
					if s := glossText(item); s != "" {
						defs = append(defs, s)
					}
				}
			}
		} else {
			for _, raw := range term[5:] {
				var s string
				if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
					defs = append(defs, strings.TrimSpace(s))
				}
			}
		}
		if err := w.add(entry{headword: expression, reading: reading, definition: strings.Join(defs, "; ")}, expression, reading); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	if err == io.EOF {
		err = nil
	}
	return err
}

// glossText is the text of an item of a Yomitan glossary: a string, a text item, or the text of structured content with its markup left out; an image, or a form of another term (an array), is none.
func glossText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var item struct {
		Type    string          `json:"type"`
		Text    string          `json:"text"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return ""
	}
	switch item.Type {
	case "text":
		return strings.TrimSpace(item.Text)
	case "structured-content":
		var b strings.Builder
		contentText(&b, item.Content, 0)
		return strings.Join(strings.Fields(b.String()), " ")
	}
	return ""
}

// contentText writes the text of Yomitan's structured content raw to b: its strings, those of its arrays and of the content of its elements, a space after each block, at most 32 elements deep. An image's text, as its alt, is left out.
func contentText(b *strings.Builder, raw json.RawMessage, depth int) {
	if len(raw) == 0 || depth > 32 {
		return
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			b.WriteString(s)
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			for _, item := range items {
				contentText(b, item, depth+1)
			}
		}
	case '{':
		var el struct {
			Tag     string          `json:"tag"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &el) != nil || el.Tag == "img" {
			return
		}
		contentText(b, el.Content, depth+1)
		switch el.Tag {
		case "div", "li", "br", "p", "tr", "td", "th", "ul", "ol", "summary", "details":
			b.WriteString(" ")
		}
	}
}
