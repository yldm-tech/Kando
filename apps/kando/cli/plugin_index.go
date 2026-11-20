package cli

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strconv"
	"time"

	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/publish"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// IndexKeyEnv is the environment variable `kando plugin index` reads the index key's secret from (publish.IndexKeyEnv).
const IndexKeyEnv = publish.IndexKeyEnv

// indexRoots are the roots `kando plugin index` verifies a script plugin's signature against: Kando's official root; tests trust a key of their own.
var indexRoots = func() signature.Roots { return signature.DefaultRoots(nil) }

// indexPublicKey is the public key Kando checks the index with, signature.IndexKey; tests check against a key of their own.
var indexPublicKey = func() string { return signature.IndexKey }

// indexNow is when an index `kando plugin index` writes was made; tests fix it.
var indexNow = time.Now

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

// pluginIndex is `kando plugin index <folder> --base <url> [--out <file>]`: it lists every pack file of folder in the community index, each checked as the app installs it (publish.Index), and writes the index to out signed with the key in IndexKeyEnv, which must be the one Kando checks with (publish.IndexKey). The pack files and the index are then published together, the files under base and the index at the address Kando reads (index.DefaultURL), docs/plugin-system.md 15.
func pluginIndex(e *env, folder, base, out string) error {
	key, err := publish.IndexKey(os.Getenv(IndexKeyEnv), indexPublicKey())
	if err != nil {
		return err
	}
	if base == "" {
		return usagef("index needs --base, the https address the pack files are published under")
	}
	idx, err := publish.Index(folder, base, indexNow(), indexRoots(), nil)
	if err != nil {
		return err
	}
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
