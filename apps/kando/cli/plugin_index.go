package cli

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
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/script"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// IndexKeyEnv is the environment variable `kando plugin index` reads the index key's secret from, the base64 of its seed as `go tool nsgo keygen` prints it: never a flag or a file argument, as nsgo's own NSGO_ROOT_KEY and NSGO_RELEASE_KEY, so the secret stays out of the shell's history and the process list.
const IndexKeyEnv = "KANDO_PLUGINS_INDEX_KEY"

// indexRoots are the roots `kando plugin index` verifies a script plugin's signature against: Kando's official root; tests trust a key of their own.
var indexRoots = func() script.Roots { return script.DefaultRoots(nil) }

// indexNow is when an index `kando plugin index` writes was made; tests fix it.
var indexNow = time.Now

// errNoIndexKey is `kando plugin index` run without the index key in IndexKeyEnv.
var errNoIndexKey = errors.New(IndexKeyEnv + " is not set: put the index key's secret (go tool nsgo keygen) in it")

// runPluginIndex is `kando plugin index`, args being what follows plugin without the word index: its flags, --base and --out, which no other subcommand of plugin takes, and --json, and one folder.
func runPluginIndex(e *env, args []string) error {
	fs := e.flags()
	fs.BoolVar(&e.json, "json", false, "print the result as JSON")
	base := fs.String("base", "", "the https address the pack files are published under")
	out := fs.String("out", "index.json", "the index file to write")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usagef("index takes one folder of pack files, got %d arguments", len(positional))
	}
	return pluginIndex(e, positional[0], *base, *out)
}

// pluginIndex is `kando plugin index <folder> --base <url> [--out <file>]`: it reads every pack file of folder (.zip, .kandopack, .kandoplugin; nothing else, and no folder), checks each as the app installs it (plugins.Stage, and for a script plugin its signature against Kando's official root, script.Verify, which only a plugin certified by that root passes, official or verified), and writes to out the community index listing them (index.Entry), each downloaded from base followed by its file's name, signed with the key in IndexKeyEnv. One pack that does not pass stops it, naming the file, so an index never leaves out a pack its publisher meant to list without saying so; so does an id listed twice. The pack files and the index are then published together, the files under base and the index at the address Kando reads (index.DefaultURL), docs/plugin-system.md 15.
func pluginIndex(e *env, folder, base, out string) error {
	secret := os.Getenv(IndexKeyEnv)
	if strings.TrimSpace(secret) == "" {
		return errNoIndexKey
	}
	key, err := pkg.DecodeSecretKey(secret)
	if err != nil {
		return fmt.Errorf("%s: %w", IndexKeyEnv, err)
	}
	if base == "" {
		return usagef("index needs --base, the https address the pack files are published under")
	}
	if err := plugins.CheckHTTPS(base); err != nil {
		return fmt.Errorf("--base %s: %w", base, err)
	}
	files, err := indexFiles(folder)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "kando-plugin-index-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	roots := indexRoots()
	trust, err := script.OpenTrust(filepath.Join(tmp, "plugins"), roots)
	if err != nil {
		return err
	}
	idx := index.Index{Format: index.Format, Version: index.Version, Generated: indexNow().UTC().Truncate(time.Second)}
	for _, name := range files {
		entry, err := indexEntry(filepath.Join(folder, name), strings.TrimSuffix(base, "/")+"/"+url.PathEscape(name), tmp, trust, roots)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, ok := idx.Lookup(entry.ID); ok {
			return fmt.Errorf("%s: %w: %s is in another file of the folder too", name, index.ErrBadEntry, entry.ID)
		}
		idx.Entries = append(idx.Entries, entry)
	}
	slices.SortFunc(idx.Entries, func(a, b index.Entry) int { return strings.Compare(a.ID, b.ID) })
	data, err := index.Sign(idx, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	if e.json {
		return pluginJSON(e, struct {
			Out     string        `json:"out"`
			Entries []index.Entry `json:"entries"`
		}{out, idx.Entries})
	}
	rows := [][]string{{"ID", "KIND", "VERSION", "SIZE", "SOURCE"}}
	for _, en := range idx.Entries {
		source := string(en.Source)
		if source == "" {
			source = "unsigned"
		}
		rows = append(rows, []string{en.ID, string(en.Kind), en.Version, strconv.FormatInt(en.Size, 10), source})
	}
	if err := table(e.stdout, rows); err != nil {
		return err
	}
	fmt.Fprintf(e.stderr, "Wrote %s: %d packs, signed by key %s.\n", printable(out), len(idx.Entries), pkg.KeyID(key.Public().(ed25519.PublicKey)))
	return nil
}

// indexExts are the names of the pack files `kando plugin index` lists.
var indexExts = []string{".zip", plugins.PackExt, ".kandoplugin"}

// indexFiles are the names of the pack files in folder, sorted: its regular files whose names end in one of indexExts. Any other entry, a folder, a link or another file, is refused, so nothing in the folder is left out without a word; files whose names start with a dot, as the file manager's, are skipped.
func indexFiles(folder string) ([]string, error) {
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
		if !entry.Type().IsRegular() || !slices.Contains(indexExts, strings.ToLower(filepath.Ext(name))) {
			return nil, fmt.Errorf("%s: %w: not a pack file (%s)", name, plugins.ErrBadShape, strings.Join(indexExts, ", "))
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no pack files", folder)
	}
	return out, nil
}

// indexEntry is the entry of the pack file at path, published at fileURL: the file is checked as the app installs it (plugins.Stage into a folder of tmp), a script plugin's signature verified against roots with trust, which only a certificate of the official root passes (a developer key's plugin is refused); its size and SHA-256 are those of the bytes read, the same the checks read.
func indexEntry(path, fileURL, tmp string, trust *pkg.Trust, roots script.Roots) (index.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return index.Entry{}, err
	}
	defer f.Close()
	// The file is copied once into tmp, and that copy is both hashed and checked, so a file changing while it is read cannot be listed with another's hash.
	copied, err := os.CreateTemp(tmp, "pack-*.zip")
	if err != nil {
		return index.Entry{}, err
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(copied, h), f)
	if cerr := copied.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return index.Entry{}, err
	}
	root, err := os.MkdirTemp(tmp, "stage-")
	if err != nil {
		return index.Entry{}, err
	}
	_, p, err := plugins.Stage(root, copied.Name())
	if err != nil {
		return index.Entry{}, err
	}
	var source plugins.Source
	if p.Kind == plugins.KindScript {
		v, err := script.Verify(trust, roots, p)
		if err != nil {
			return index.Entry{}, err
		}
		source = v.Source
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	entry := index.FromPack(p, fileURL, size, sum, source)
	if err := entry.Check(); err != nil {
		return index.Entry{}, err
	}
	return entry, nil
}
