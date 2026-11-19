// Package index is Kando's community plugin index (docs/plugin-system.md, P5): a list of packs, data packs and script plugins, each named by the SHA-256 and size of its file and the https address it is downloaded from, signed as a whole with an Ed25519 key of Kando's, so a host serving it that was broken into cannot add, change or take away an entry. It reads and checks an index (Parse), keeps the last one in the plugins folder and asks for a newer one at most daily or when the user asks (Source.Refresh), downloads a pack the index lists, checking its size and hash (Source.Download), and builds and signs an index from a folder of packs (Build, Sign) for `kando plugin index`. Listing a pack grants it nothing: a downloaded pack goes through the same Stage, Validate, Verify and install confirmation as a file the user picked. It imports no interface package.
package index

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Format is what an index file says it is, and Version the version of its format this Kando reads; an index of a newer version is refused (ErrVersion), so an entry this version would read wrong is never shown.
const (
	Format  = "kando-plugins-index"
	Version = 1
)

// MaxBytes is the most bytes an index file may have, and MaxEntries the most entries; MaxLanguages the most languages an entry may name.
const (
	MaxBytes     = 4 << 20
	MaxEntries   = 5000
	MaxLanguages = 16
)

// signContext is the context an index's signature is made in, so it can never be taken for a signature of another kind made with the same key (tsx/pkg signs its packages, certificates and revocation lists each in its own context the same way).
const signContext = "kando-plugins-index-v1"

// Why an index or a download is refused, wrapped with the details (errors.Is finds them). They are English and never shown as they are: the app maps each to a message of its catalogs.
var (
	// ErrNotConfigured is a Kando that has no public key of the index to check it with.
	ErrNotConfigured = errors.New("the plugin index is not configured")
	// ErrSignature is an index that is not signed, or not by the index key, or whose content does not match its signature.
	ErrSignature = errors.New("the plugin index is not signed by Kando's index key")
	// ErrFormat is an index file that is not JSON, has a key this version does not know, or does not say it is an index.
	ErrFormat = errors.New("the plugin index cannot be read")
	// ErrVersion is an index of a newer format than this Kando reads.
	ErrVersion = errors.New("the plugin index is made for a newer version of Kando")
	// ErrTooLarge is an index over MaxBytes or MaxEntries, or a download over the size its entry says.
	ErrTooLarge = errors.New("the plugin index or the download is too large")
	// ErrBadEntry is an entry of the index whose fields are not valid, wrapping the plugins package's error where there is one.
	ErrBadEntry = errors.New("an entry of the plugin index is not valid")
	// ErrRollback is an index older than the one Kando already has, as a host that was broken into could serve to bring back a version taken out.
	ErrRollback = errors.New("the plugin index is older than the one Kando has")
	// ErrChecksum is a download whose size or SHA-256 is not the one its entry says.
	ErrChecksum = errors.New("the downloaded file is not the one the plugin index lists")
	// ErrHTTP is an answer of the server other than the file, wrapped with its status.
	ErrHTTP = errors.New("the server did not send the file")
	// ErrRedirect is a redirect to another host, or away from https.
	ErrRedirect = errors.New("the server redirected to another host")
	// ErrNotListed is a pack the index does not list, or a downloaded pack that is not the one its entry names: another id, kind or version, or a script plugin from another source.
	ErrNotListed = errors.New("the pack is not the one the plugin index lists")
)

// Index is the list of packs, as of Generated, which only rises: an index older than the one Kando has is refused (ErrRollback).
type Index struct {
	Format    string    `json:"format"`
	Version   int       `json:"version"`
	Generated time.Time `json:"generated"`
	Entries   []Entry   `json:"entries"`
}

// Entry is a pack the index lists: its id, kind, name and summary (its manifest's description), version, publisher (the segment of its id before the first dot), license and homepage, as its manifest says them; the size, SHA-256 and https address of its file; the languages it is for, where its kind has some; the oldest Kando it runs on, for a script plugin that names one; and for a script plugin where its signature comes from, official or verified, which the package's certificate must say too once Kando verified it.
type Entry struct {
	ID        string         `json:"id"`
	Kind      plugins.Kind   `json:"kind"`
	Name      plugins.Text   `json:"name"`
	Summary   plugins.Text   `json:"summary,omitempty"`
	Version   string         `json:"version"`
	Publisher string         `json:"publisher"`
	License   string         `json:"license"`
	Homepage  string         `json:"homepage,omitempty"`
	Size      int64          `json:"size"`
	SHA256    string         `json:"sha256"`
	URL       string         `json:"url"`
	Languages []string       `json:"languages,omitempty"`
	MinKando  string         `json:"minKando,omitempty"`
	Source    plugins.Source `json:"source,omitempty"`
}

// signedFile is an index file: the index's JSON in base64 (Body), the ID of the key that signed it (Signer, pkg.KeyID) and the signature, with the format and version outside the signature too, so a Kando can tell an index of a newer format before it checks anything else.
type signedFile struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	Signer    string `json:"signer"`
	Signature string `json:"signature"`
	Body      string `json:"body"`
}

// DecodeKey reads the public key of the index, the base64 of its 32 bytes as `go tool nsgo keygen` prints it; "" is ErrNotConfigured.
func DecodeKey(text string) (ed25519.PublicKey, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrNotConfigured
	}
	return pkg.DecodePublicKey(text)
}

// framed is what the signature of body signs: the context, a zero byte and body's SHA-256, as tsx/pkg frames its own signatures.
// TODO(nsgo#868): tsx/pkg's signed envelope (signed, signBody, framed) is not exported; sign and verify the index with it under a context of Kando's once it is, and drop this copy and signedFile's own signature fields https://github.com/nsgo-dev/nsgo/issues/868
func framed(body []byte) []byte {
	sum := sha256.Sum256(body)
	return append(append([]byte(signContext), 0), sum[:]...)
}

// Sign checks idx (Check) and writes it as an index file signed with key.
func Sign(idx Index, key ed25519.PrivateKey) ([]byte, error) {
	if err := idx.Check(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(idx)
	if err != nil {
		return nil, err
	}
	f := signedFile{Format: Format, Version: Version, Signer: pkg.KeyID(key.Public().(ed25519.PublicKey)),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, framed(body))), Body: base64.StdEncoding.EncodeToString(body)}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%w: the index is %d bytes, over %d", ErrTooLarge, len(data), MaxBytes)
	}
	return data, nil
}

// Parse reads the index file data once key verifies its signature, and checks every entry of it (Check). Nothing of the index is read before its signature is checked but its format and version.
func Parse(data []byte, key ed25519.PublicKey) (Index, error) {
	if key == nil {
		return Index{}, ErrNotConfigured
	}
	if len(data) > MaxBytes {
		return Index{}, fmt.Errorf("%w: the index is over %d bytes", ErrTooLarge, MaxBytes)
	}
	var f signedFile
	if err := decodeStrict(data, &f); err != nil {
		return Index{}, err
	}
	if f.Format != Format {
		return Index{}, fmt.Errorf("%w: it is %q", ErrFormat, f.Format)
	}
	if f.Version != Version {
		return Index{}, fmt.Errorf("%w: version %d", ErrVersion, f.Version)
	}
	if f.Signer != pkg.KeyID(key) {
		return Index{}, fmt.Errorf("%w: signed by key %q", ErrSignature, f.Signer)
	}
	body, err := base64.StdEncoding.DecodeString(f.Body)
	if err != nil {
		return Index{}, fmt.Errorf("%w: the body is not base64", ErrFormat)
	}
	signature, err := base64.StdEncoding.DecodeString(f.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, framed(body), signature) {
		return Index{}, ErrSignature
	}
	var idx Index
	if err := decodeStrict(body, &idx); err != nil {
		return Index{}, err
	}
	if idx.Format != f.Format || idx.Version != f.Version {
		return Index{}, fmt.Errorf("%w: the signed index is %q version %d", ErrFormat, idx.Format, idx.Version)
	}
	if err := idx.Check(); err != nil {
		return Index{}, err
	}
	return idx, nil
}

// decodeStrict decodes the JSON data into v, refusing a key v does not have and anything after the value.
func decodeStrict(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("%w: data after the index", ErrFormat)
	}
	return nil
}

// Check checks the index: its format and version, when it was made, at most MaxEntries entries, each valid (Entry.Check) and no id twice. It cleans the entries' texts in place.
func (idx Index) Check() error {
	if idx.Format != Format || idx.Version != Version {
		return fmt.Errorf("%w: %q version %d", ErrFormat, idx.Format, idx.Version)
	}
	if idx.Generated.IsZero() {
		return fmt.Errorf("%w: no time it was made", ErrFormat)
	}
	if len(idx.Entries) > MaxEntries {
		return fmt.Errorf("%w: %d entries, over %d", ErrTooLarge, len(idx.Entries), MaxEntries)
	}
	seen := make(map[string]bool, len(idx.Entries))
	for i := range idx.Entries {
		e := &idx.Entries[i]
		if err := e.Check(); err != nil {
			return fmt.Errorf("entry %d (%q): %w", i, e.ID, err)
		}
		if seen[e.ID] {
			return fmt.Errorf("%w: %q is listed twice", ErrBadEntry, e.ID)
		}
		seen[e.ID] = true
	}
	return nil
}

// sha256Pattern is a SHA-256 in lowercase hex.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Check checks the entry e by the rules its pack's manifest is held to (plugins.Validate) where it repeats them: a kind this Kando installs, the id of that kind, a name and summary of a pack's, the version, license and homepage; and by its own: the publisher is the id's first segment, the file is at least a byte and at most the largest pack of its kind Stage takes, its SHA-256 is lowercase hex, its address is https, its languages are canonical tags, its oldest Kando is a MAJOR.MINOR.PATCH, and only a script plugin names where its signature comes from, official or verified. It cleans the texts in place.
func (e *Entry) Check() error {
	bad := func(err error) error { return fmt.Errorf("%w: %w", ErrBadEntry, err) }
	if !e.Kind.Installable() {
		return fmt.Errorf("%w: kind %q", ErrBadEntry, e.Kind)
	}
	if err := plugins.CheckID(e.ID, e.Kind); err != nil {
		return bad(err)
	}
	if publisher, _, _ := strings.Cut(e.ID, "."); e.Publisher != publisher {
		return fmt.Errorf("%w: publisher %q of %q", ErrBadEntry, e.Publisher, e.ID)
	}
	if err := plugins.CheckName(e.Name); err != nil {
		return bad(err)
	}
	if err := plugins.CheckDescription(e.Summary); err != nil {
		return bad(err)
	}
	if err := plugins.CheckVersion(e.Version); err != nil {
		return bad(err)
	}
	if err := plugins.CheckLicense(&e.License); err != nil {
		return bad(err)
	}
	if e.Homepage != "" {
		if err := plugins.CheckHTTPS(e.Homepage); err != nil {
			return bad(err)
		}
	}
	if e.Size < 1 || e.Size > plugins.ZipMax(e.Kind) {
		return fmt.Errorf("%w: size %d", ErrBadEntry, e.Size)
	}
	if !sha256Pattern.MatchString(e.SHA256) {
		return fmt.Errorf("%w: sha256 %q", ErrBadEntry, e.SHA256)
	}
	if err := plugins.CheckHTTPS(e.URL); err != nil {
		return bad(err)
	}
	if len(e.Languages) > MaxLanguages {
		return fmt.Errorf("%w: %d languages", ErrBadEntry, len(e.Languages))
	}
	for i, lang := range e.Languages {
		if err := plugins.CheckLanguage(lang); err != nil {
			return bad(err)
		}
		if slices.Contains(e.Languages[:i], lang) {
			return fmt.Errorf("%w: language %q twice", ErrBadEntry, lang)
		}
	}
	if e.MinKando != "" {
		if !minKandoPattern.MatchString(e.MinKando) {
			return fmt.Errorf("%w: minKando %q", ErrBadEntry, e.MinKando)
		}
		if e.Kind != plugins.KindScript {
			return fmt.Errorf("%w: minKando of a %s pack", ErrBadEntry, e.Kind)
		}
	}
	switch {
	case e.Kind == plugins.KindScript && (e.Source == plugins.SourceOfficial || e.Source == plugins.SourceVerified):
	case e.Kind != plugins.KindScript && e.Source == plugins.SourceLocal:
	default:
		return fmt.Errorf("%w: source %q of a %s pack", ErrBadEntry, e.Source, e.Kind)
	}
	return nil
}

// RunsOn reports whether the pack of e runs on Kando version: always when it names no oldest version, and for a version that is not MAJOR.MINOR.PATCH, as a build of the source; else when version is that one or newer (plugins.Manifest.RunsOn).
func (e Entry) RunsOn(version string) bool {
	if e.MinKando == "" {
		return true
	}
	return plugins.Manifest{Kando: ">=" + e.MinKando}.RunsOn(version)
}

// Lookup is the entry of id, false when the index lists none.
func (idx Index) Lookup(id string) (Entry, bool) {
	i := slices.IndexFunc(idx.Entries, func(e Entry) bool { return e.ID == id })
	if i < 0 {
		return Entry{}, false
	}
	return idx.Entries[i], true
}

// Matches reports whether the pack p, as Stage read the downloaded file, is the one e lists: the same id, kind and version. The file's SHA-256 already matched e's; this catches an entry whose fields say one pack while its file is another.
func (e Entry) Matches(p plugins.Pack) error {
	if p.ID != e.ID || p.Kind != e.Kind || p.Version != e.Version {
		return fmt.Errorf("%w: the file is %s %s %s, the index lists %s %s %s", ErrNotListed, p.Kind, p.ID, p.Version, e.Kind, e.ID, e.Version)
	}
	return nil
}

// Newer reports whether version a is newer than version b: their dotted parts compared one by one, as numbers where both are, a part that is not after one that is; with every part the same, one without a suffix after a hyphen is newer than one with, and two suffixes compare as text. What follows a plus is left out. Versions are a manifest's, letters, digits, dots, plus signs and hyphens (plugins.CheckVersion).
func Newer(a, b string) bool { return compareVersions(a, b) > 0 }

func compareVersions(a, b string) int {
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	ac, ap, aPre := strings.Cut(a, "-")
	bc, bp, bPre := strings.Cut(b, "-")
	as, bs := strings.Split(ac, "."), strings.Split(bc, ".")
	for i := range max(len(as), len(bs)) {
		x, y := "0", "0"
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := comparePart(x, y); c != 0 {
			return c
		}
	}
	switch {
	case aPre && !bPre:
		return -1
	case !aPre && bPre:
		return 1
	}
	return strings.Compare(ap, bp)
}

// comparePart compares two parts of a version: as numbers when both are, a number before a word, else as text.
func comparePart(x, y string) int {
	xn, xerr := strconv.ParseUint(x, 10, 64)
	yn, yerr := strconv.ParseUint(y, 10, 64)
	switch {
	case xerr == nil && yerr == nil:
		if xn == yn {
			return 0
		}
		if xn < yn {
			return -1
		}
		return 1
	case xerr == nil:
		return -1
	case yerr == nil:
		return 1
	}
	return strings.Compare(x, y)
}

// minKandoPattern is an oldest Kando: MAJOR.MINOR.PATCH in digits alone, as a manifest's kando field names it after its >=.
var minKandoPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// FromPack is the entry of the pack p, read and checked from its file (plugins.Stage, and for a script plugin its signature verified, which gave source), whose file is size bytes with SHA-256 sum, downloaded from url: its id, kind, name, description, version, publisher, license and homepage as its manifest says them; the languages its kind is for (a glossary's from and to, a dictionary's, a word list's), and a script plugin's oldest Kando.
func FromPack(p plugins.Pack, url string, size int64, sum [sha256.Size]byte, source plugins.Source) Entry {
	publisher, _, _ := strings.Cut(p.ID, ".")
	e := Entry{ID: p.ID, Kind: p.Kind, Name: p.Name, Summary: p.Description, Version: p.Version, Publisher: publisher, License: p.License, Homepage: p.Homepage,
		Size: size, SHA256: hex.EncodeToString(sum[:]), URL: url, Source: source}
	add := func(langs ...string) {
		for _, l := range langs {
			if l != "" && !slices.Contains(e.Languages, l) {
				e.Languages = append(e.Languages, l)
			}
		}
	}
	switch {
	case p.Glossary != nil:
		add(p.Glossary.From, p.Glossary.To)
	case p.Dictionary != nil:
		add(p.Dictionary.From...)
		add(p.Dictionary.To...)
	case p.Wordlist != nil:
		add(p.Wordlist.Language)
	}
	if need, ok := strings.CutPrefix(p.Kando, ">="); ok {
		e.MinKando = need
	}
	return e
}
