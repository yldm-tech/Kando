package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PackExt is the extension of a pack file Kando writes; Stage reads it as the zip it is.
const PackExt = ".kandopack"

// Build writes the data pack of manifest m with files (by name) as a zip, as Kando's own 导出为插件包 makes one from the user's glossary or vocabulary: the files are first written to a folder of their own and checked by Validate, as an install checks them, so a pack Build returns installs as it is (Stage). It returns the zip and the pack as Validate read it; a pack Validate refuses is its error.
func Build(m Manifest, files map[string][]byte) ([]byte, Pack, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, Pack{}, err
	}
	dir, err := os.MkdirTemp("", "kando-pack-*")
	if err != nil {
		return nil, Pack{}, err
	}
	defer os.RemoveAll(dir)
	all := maps.Clone(files)
	if all == nil {
		all = map[string][]byte{}
	}
	all[ManifestName] = data
	for name, content := range all {
		if err := checkFileName(name); err != nil {
			return nil, Pack{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return nil, Pack{}, err
		}
	}
	p, err := Validate(dir)
	if err != nil {
		return nil, Pack{}, err
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range slices.Sorted(maps.Keys(all)) {
		f, err := w.Create(name)
		if err == nil {
			_, err = f.Write(all[name])
		}
		if err != nil {
			return nil, Pack{}, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, Pack{}, err
	}
	p.Dir = ""
	return b.Bytes(), p, nil
}

// Slug is name as the part of a pack id after its publisher: its ASCII letters and digits in lower case, every other run of characters one hyphen, at most 40 bytes; fallback when that leaves nothing, as for a name written in Chinese.
func Slug(name, fallback string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" || !validID("local."+s) {
		return fallback
	}
	return s
}
