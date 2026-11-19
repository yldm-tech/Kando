// Package publish is what checks and lists the packs of Kando's community plugin directory (docs/plugin-system.md 15): checking a pack as Kando installs it (Check), and building the signed index from a folder of pack files (IndexKey, Index), for `kando plugin validate` and `kando plugin index` and for the directory repository's CI tool (cmd/kando-plugin), which so never check a pack differently. It imports no interface package and builds without cgo.
package publish

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// IndexKeyEnv is the environment variable the index key's secret is read from, the base64 of its seed as `go tool nsgo keygen` prints it: never a flag or a file argument, as nsgo's own NSGO_ROOT_KEY and NSGO_RELEASE_KEY, so the secret stays out of the shell's history and the process list.
const IndexKeyEnv = "KANDO_PLUGINS_INDEX_KEY"

var (
	// ErrNoIndexKey is IndexKeyEnv empty or not set.
	ErrNoIndexKey = errors.New(IndexKeyEnv + " is not set: put the index key's secret (go tool nsgo keygen) in it")
	// ErrWrongIndexKey is a secret in IndexKeyEnv that is not the index key Kando checks with: the index it signed would be refused by every Kando.
	ErrWrongIndexKey = errors.New(IndexKeyEnv + " holds another key than the one Kando checks the index with (signature.IndexKey)")
	// ErrNoPacks is a folder with no pack file in it.
	ErrNoPacks = errors.New("no pack files")
)

// Checked is a pack checked as Kando installs it: the pack, where a script plugin's signature comes from (Source, official or verified; developer for a developer key's, which Index refuses) and the release key that signed it (Signer); for a pack file, its size and SHA-256, those of the bytes the checks read.
type Checked struct {
	Pack   plugins.Pack
	Source plugins.Source
	Signer string
	File   bool
	Size   int64
	SHA256 [sha256.Size]byte
}

// Trust opens the trust a script plugin's signature is verified with the first time it is called (signature.OpenTrust on the plugins folder root, trusting roots) and gives the same one after, so checking data packs alone opens none. One goroutine at a time.
func Trust(root string, roots signature.Roots) func() (*pkg.Trust, error) {
	var t *pkg.Trust
	var err error
	return func() (*pkg.Trust, error) {
		if t == nil && err == nil {
			t, err = signature.OpenTrust(root, roots)
		}
		return t, err
	}
}

// Check checks the pack at src, a pack file (a zip, whatever its extension) or a folder, as Kando installs it: a file is copied once into work, hashed as it is copied, and the copy is staged (plugins.Stage, which runs plugins.Validate) in a folder of work, so a file changing while it is read cannot be listed with another's hash; a script plugin's signature is then verified against roots with the trust trust opens (Trust), which only a script plugin opens. work is a folder the caller removes; Check leaves its copies there.
func Check(src, work string, trust func() (*pkg.Trust, error), roots signature.Roots) (Checked, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Checked{}, err
	}
	var c Checked
	staged := src
	if info.Mode().IsRegular() {
		c.File = true
		staged, c.Size, c.SHA256, err = copyHashed(src, work)
		if err != nil {
			return Checked{}, err
		}
	}
	root, err := os.MkdirTemp(work, "stage-")
	if err != nil {
		return Checked{}, err
	}
	_, p, err := plugins.Stage(root, staged)
	if err != nil {
		return Checked{}, err
	}
	c.Pack = p
	if p.Kind == plugins.KindScript {
		t, err := trust()
		if err != nil {
			return Checked{}, err
		}
		v, err := signature.Verify(t, roots, p)
		if err != nil {
			return Checked{}, err
		}
		c.Source, c.Signer = v.Source, v.Signer
	}
	return c, nil
}

// copyHashed copies the file at src into a new file of work and returns the copy, its size and its SHA-256.
func copyHashed(src, work string) (string, int64, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	f, err := os.Open(src)
	if err != nil {
		return "", 0, sum, err
	}
	defer f.Close()
	copied, err := os.CreateTemp(work, "pack-*.zip")
	if err != nil {
		return "", 0, sum, err
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(copied, h), f)
	if cerr := copied.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, sum, err
	}
	copy(sum[:], h.Sum(nil))
	return copied.Name(), size, sum, nil
}

// Entry is the index entry of the checked pack file c, published at fileURL (index.FromPack), checked as the index is signed (index.Entry.Check).
func (c Checked) Entry(fileURL string) (index.Entry, error) {
	if !c.File {
		return index.Entry{}, fmt.Errorf("%w: a folder is not a pack file", plugins.ErrBadShape)
	}
	entry := index.FromPack(c.Pack, fileURL, c.Size, c.SHA256, c.Source)
	if err := entry.Check(); err != nil {
		return index.Entry{}, err
	}
	return entry, nil
}

// IndexKey is the index key whose secret is secret, as IndexKeyEnv holds it, once it is the key whose public half is public (signature.IndexKey; tests check against their own): ErrNoIndexKey for none, ErrWrongIndexKey for another key.
func IndexKey(secret, public string) (ed25519.PrivateKey, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrNoIndexKey
	}
	key, err := pkg.DecodeSecretKey(secret)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", IndexKeyEnv, err)
	}
	if want, err := pkg.DecodePublicKey(public); err != nil || !want.Equal(key.Public()) {
		return nil, ErrWrongIndexKey
	}
	return key, nil
}

// IndexExts are the extensions of the pack files Index lists.
var IndexExts = []string{".zip", plugins.PackExt, ".kandoplugin"}

// IndexFiles are the names of the pack files in folder, sorted: its regular files whose names end in one of IndexExts. Any other entry, a folder, a link or another file, is refused, so nothing in the folder is left out without a word; files whose names start with a dot, as the file manager's, are skipped.
func IndexFiles(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.Type().IsRegular() || !slices.Contains(IndexExts, strings.ToLower(filepath.Ext(name))) {
			return nil, fmt.Errorf("%s: %w: not a pack file (%s)", name, plugins.ErrBadShape, strings.Join(IndexExts, ", "))
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %w", folder, ErrNoPacks)
	}
	return out, nil
}

// Index is the community index of the pack files in folder (IndexFiles), made at generated: each file checked as Kando installs it (Check), a script plugin against roots, which only a plugin certified by the official root passes (official or verified; a developer key's is refused), and listed (index.Entry) as downloaded from base, an https address, followed by its file's name. One pack that does not pass stops it, naming the file, so an index never leaves out a pack its publisher meant to list without saying so; so does an id listed twice. The entries are sorted by id; the caller signs the index (index.Sign).
func Index(folder, base string, generated time.Time, roots signature.Roots) (index.Index, error) {
	if base == "" {
		return index.Index{}, errors.New("no base address: give the https address the pack files are published under")
	}
	if err := plugins.CheckHTTPS(base); err != nil {
		return index.Index{}, fmt.Errorf("--base %s: %w", base, err)
	}
	files, err := IndexFiles(folder)
	if err != nil {
		return index.Index{}, err
	}
	work, err := os.MkdirTemp("", "kando-plugin-index-")
	if err != nil {
		return index.Index{}, err
	}
	defer os.RemoveAll(work)
	trust := Trust(filepath.Join(work, "plugins"), roots)
	idx := index.Index{Format: index.Format, Version: index.Version, Generated: generated.UTC().Truncate(time.Second)}
	for _, name := range files {
		c, err := Check(filepath.Join(folder, name), work, trust, roots)
		var entry index.Entry
		if err == nil {
			entry, err = c.Entry(strings.TrimSuffix(base, "/") + "/" + url.PathEscape(name))
		}
		if err != nil {
			return index.Index{}, fmt.Errorf("%s: %w", name, err)
		}
		if _, ok := idx.Lookup(entry.ID); ok {
			return index.Index{}, fmt.Errorf("%s: %w: %s is in another file of the folder too", name, index.ErrBadEntry, entry.ID)
		}
		idx.Entries = append(idx.Entries, entry)
	}
	slices.SortFunc(idx.Entries, func(a, b index.Entry) int { return strings.Compare(a.ID, b.ID) })
	return idx, nil
}
