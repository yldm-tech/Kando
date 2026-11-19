// Command kando-plugin checks and lists the packs of Kando's community plugin directory, for the CI of the directory's repository (yldm-tech/kando-plugins), which checks out this repository and runs it with go run (docs/plugin-system.md 15):
//
//	go run ./apps/kando/cmd/kando-plugin validate [--json] <pack>...
//	go run ./apps/kando/cmd/kando-plugin preview [--markdown] <pack>...
//	KANDO_PLUGINS_INDEX_KEY=<secret> go run ./apps/kando/cmd/kando-plugin index <folder> --base <url> [--out <file>] [--json]
//
// validate checks each pack, a pack file or a folder, as Kando installs it, a script plugin's signature against Kando's official root (publish.Check, the same code `kando plugin validate` runs); preview writes what each pack is and asks for as Markdown, for a comment on the pull request; index builds and signs the community index (publish.Index, the same code as `kando plugin index`). It builds without cgo and imports no interface package, so it runs on any CI machine without a desktop's libraries.
//
// Exit codes: 0 when every pack passed, 1 when one did not or the command failed, 2 for a usage error.
package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/plugins/publish"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Exit codes.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

// tool is what a run reads besides its arguments: the environment, the roots a script plugin's signature is checked against, the public key the index must be signed with, the clock and the outputs. Tests set their own roots and key.
type tool struct {
	getenv         func(string) string
	roots          signature.Roots
	indexPublicKey string
	now            func() time.Time
	stdout, stderr io.Writer
}

func main() {
	t := tool{getenv: os.Getenv, roots: signature.DefaultRoots(nil), indexPublicKey: signature.IndexKey, now: time.Now, stdout: os.Stdout, stderr: os.Stderr}
	os.Exit(t.run(os.Args[1:]))
}

// errUsage is a usage error, which exits with exitUsage.
type errUsage struct{ msg string }

func (e errUsage) Error() string { return e.msg }

func usagef(format string, args ...any) error { return errUsage{fmt.Sprintf(format, args...)} }

const usage = `usage:
  kando-plugin validate [--json] <pack>...
  kando-plugin preview [--markdown] <pack>...
  kando-plugin index <folder> --base <https url> [--out <file>] [--json]   (the key's secret in ` + publish.IndexKeyEnv + `)`

// run runs the command of args and returns its exit code.
func (t tool) run(args []string) int {
	err := t.dispatch(args)
	var u errUsage
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &u):
		fmt.Fprintf(t.stderr, "kando-plugin: %s\n%s\n", u.msg, usage)
		return exitUsage
	case errors.Is(err, errReported):
		return exitFailed
	}
	fmt.Fprintf(t.stderr, "kando-plugin: %s\n", oneLine(err.Error()))
	return exitFailed
}

// errReported is a failure the command already reported in its output, as a pack that did not pass.
var errReported = errors.New("reported")

func (t tool) dispatch(args []string) error {
	if len(args) == 0 {
		return usagef("needs a command: validate, preview or index")
	}
	verb, args := args[0], args[1:]
	fs := flag.NewFlagSet("kando-plugin "+verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch verb {
	case "validate":
		asJSON := fs.Bool("json", false, "print the result as JSON")
		paths, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return usagef("validate takes one or more packs, pack files or folders")
		}
		return t.validate(paths, *asJSON)
	case "preview":
		markdown := fs.Bool("markdown", true, "write Markdown (the only format)")
		paths, err := parse(fs, args)
		if err != nil {
			return err
		}
		if !*markdown {
			return usagef("preview writes Markdown only")
		}
		if len(paths) == 0 {
			return usagef("preview takes one or more packs, pack files or folders")
		}
		return t.preview(paths)
	case "index":
		asJSON := fs.Bool("json", false, "print the result as JSON")
		base := fs.String("base", "", "the https address the pack files are published under")
		out := fs.String("out", "index.json", "the index file to write")
		folders, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(folders) != 1 {
			return usagef("index takes one folder of pack files, got %d arguments", len(folders))
		}
		return t.index(folders[0], *base, *out, *asJSON)
	}
	return usagef("unknown command %q", verb)
}

// parse parses the flags of fs anywhere among args, before or after the paths, as `kando plugin` does. The first "--" ends them: everything after it is a path, even one that starts with a dash.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usagef("%s", err.Error())
		}
		args = fs.Args()
		if len(args) == 0 {
			return append(positional, rest...), nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// checked is a pack as validate and preview check it: its path as given, and what publish.Check found or why it did not pass.
type checked struct {
	path string
	publish.Checked
	err error
}

// check checks each of paths as Kando installs it (publish.Check), a script plugin's signature against the tool's roots, with no revocation list: one trust for all of them, in a folder of its own removed afterwards.
func (t tool) check(paths []string) ([]checked, error) {
	work, err := os.MkdirTemp("", "kando-plugin-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	trust := publish.Trust(work+string(os.PathSeparator)+"plugins", t.roots)
	out := make([]checked, len(paths))
	for i, path := range paths {
		dir, err := os.MkdirTemp(work, "pack-")
		if err != nil {
			return nil, err
		}
		out[i].path = path
		out[i].Checked, out[i].err = publish.Check(path, dir, trust, t.roots)
	}
	return out, nil
}

// validated is a pack as validate --json prints it: whether it passed, what it is, and why it did not pass, with a stable code for the error (errorCode).
type validated struct {
	Path    string `json:"path"`
	OK      bool   `json:"ok"`
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Version string `json:"version,omitempty"`
	Name    string `json:"name,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Source  string `json:"source,omitempty"`
	Signer  string `json:"signer,omitempty"`
	Size    int64  `json:"size,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
}

func newValidated(c checked) validated {
	v := validated{Path: c.path, OK: c.err == nil}
	if c.err != nil {
		v.Code, v.Error = errorCode(c.err), oneLine(c.err.Error())
		return v
	}
	p := c.Pack
	v.ID, v.Kind, v.Version, v.Name, v.Hash = p.ID, string(p.Kind), p.Version, p.Name.In("en"), p.Hash
	v.Source, v.Signer = string(c.Source), c.Signer
	if c.File {
		v.Size, v.SHA256 = c.Size, fmt.Sprintf("%x", c.SHA256)
	}
	return v
}

// validate checks every pack of paths and prints a line for each, or with asJSON {"ok": …, "packs": [...]}; it fails when one did not pass, after checking them all.
func (t tool) validate(paths []string, asJSON bool) error {
	results, err := t.check(paths)
	if err != nil {
		return err
	}
	all := true
	out := struct {
		OK    bool        `json:"ok"`
		Packs []validated `json:"packs"`
	}{Packs: make([]validated, 0, len(results))}
	for _, c := range results {
		v := newValidated(c)
		all = all && v.OK
		out.Packs = append(out.Packs, v)
		if asJSON {
			continue
		}
		if v.OK {
			fmt.Fprintf(t.stdout, "ok    %s: %s %s %s\n", oneLine(v.Path), v.Kind, v.ID, v.Version)
		} else {
			fmt.Fprintf(t.stdout, "FAIL  %s: %s (%s)\n", oneLine(v.Path), v.Error, v.Code)
		}
	}
	out.OK = all
	if asJSON {
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(t.stdout, "%s\n", data)
	}
	if !all {
		return errReported
	}
	return nil
}

// preview writes the Markdown of every pack of paths (writePreview); it fails when one did not pass, after writing them all.
func (t tool) preview(paths []string) error {
	results, err := t.check(paths)
	if err != nil {
		return err
	}
	if err := writePreview(t.stdout, results); err != nil {
		return err
	}
	for _, c := range results {
		if c.err != nil {
			return errReported
		}
	}
	return nil
}

// index is `kando-plugin index`, as `kando plugin index` (publish.IndexKey, publish.Index, index.Sign): the key's secret only from publish.IndexKeyEnv, refused unless it is the key Kando checks the index with.
func (t tool) index(folder, base, out string, asJSON bool) error {
	key, err := publish.IndexKey(t.getenv(publish.IndexKeyEnv), t.indexPublicKey)
	if err != nil {
		return err
	}
	if base == "" {
		return usagef("index needs --base, the https address the pack files are published under")
	}
	idx, err := publish.Index(folder, base, t.now(), t.roots)
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
	signer := pkg.KeyID(key.Public().(ed25519.PublicKey))
	if asJSON {
		data, err := json.MarshalIndent(struct {
			Out     string        `json:"out"`
			Signer  string        `json:"signer"`
			Entries []index.Entry `json:"entries"`
		}{out, signer, idx.Entries}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(t.stdout, "%s\n", data)
		return nil
	}
	for _, e := range idx.Entries {
		source := string(e.Source)
		if source == "" {
			source = "unsigned"
		}
		fmt.Fprintf(t.stdout, "%s\t%s\t%s\t%d\t%s\n", e.ID, e.Kind, e.Version, e.Size, source)
	}
	fmt.Fprintf(t.stderr, "Wrote %s: %d packs, signed by key %s.\n", oneLine(out), len(idx.Entries), signer)
	return nil
}

// errorCodes are the stable codes of why a pack did not pass, by the error it wraps, in the order they are looked for.
var errorCodes = []struct {
	err  error
	code string
}{
	{signature.ErrRevoked, "revoked"},
	{signature.ErrRollback, "rollback"},
	{signature.ErrRuntime, "needs-newer-kando"},
	{signature.ErrScope, "scope"},
	{signature.ErrUntrusted, "untrusted"},
	{plugins.ErrTampered, "tampered"},
	{plugins.ErrBadManifest, "bad-manifest"},
	{plugins.ErrUnknownKind, "unknown-kind"},
	{plugins.ErrUnsupportedKind, "unsupported-kind"},
	{plugins.ErrSchema, "schema"},
	{plugins.ErrBadID, "bad-id"},
	{plugins.ErrReservedID, "reserved-id"},
	{plugins.ErrBadName, "bad-name"},
	{plugins.ErrReservedName, "reserved-name"},
	{plugins.ErrBadSite, "bad-site"},
	{plugins.ErrSiteKnown, "site-known"},
	{plugins.ErrInsecureURL, "insecure-url"},
	{plugins.ErrUnknownPerm, "unknown-permission"},
	{plugins.ErrBadCapabilities, "bad-capabilities"},
	{plugins.ErrBadField, "bad-field"},
	{plugins.ErrBadShape, "bad-shape"},
	{plugins.ErrUnsafePath, "unsafe-path"},
	{plugins.ErrBadArchive, "bad-archive"},
	{plugins.ErrTooLarge, "too-large"},
	{plugins.ErrMismatch, "mismatch"},
	{index.ErrBadEntry, "bad-entry"},
	{os.ErrNotExist, "not-found"},
}

// errorCode is the stable code of why a pack did not pass (errorCodes), "error" for another error.
func errorCode(err error) string {
	for _, c := range errorCodes {
		if errors.Is(err, c.err) {
			return c.code
		}
	}
	return "error"
}

// oneLine is s on one line, its control characters as spaces, so a path or a message from a pack cannot write lines of its own into the output.
func oneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f || r == 0x2028 || r == 0x2029 }), " ")
}
