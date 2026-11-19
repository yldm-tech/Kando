// Package script runs Kando's script plugins (docs/plugin-system.md, P3): the trust their signatures chain to (OpenTrust, Verify), the sandbox that reads what a plugin's signed code says of itself before the user agreed to anything (Probe), the one constructor of the sandbox a plugin runs in (Sandbox, Limits), loading it (Load), and the runner that calls it one call at a time and stops a plugin that keeps the window from answering or holds too much memory (Runner). It imports no interface package; the app gives each plugin its own host (core.Host) and checks every call against what the user granted.
package script

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/plugins"

	"github.com/dlclark/regexp2/v2"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func init() {
	// goja compiles every regular expression a plugin makes with regexp2, whose matches no time budget interrupts: a backtracking pattern would hold the engine's goroutine, the window's, for minutes. regexp2 reads its default timeout as it compiles, so it is set before any plugin loads; a match past it throws a RangeError in the plugin. Kando uses regexp2 nowhere else.
	// TODO(nsgo#759): tsx does not bound regexp2's matches; set MatchTimeout per runtime there and drop this process-wide default https://github.com/nsgo-dev/nsgo/issues/759
	regexp2.DefaultMatchTimeout = 100 * time.Millisecond
}

// OfficialRoot is the public key of Kando's plugin root, kept offline, which certifies the official release key for ai.yldm.kando.plugin.kando.* (docs/plugin-system.md 6.4, 15).
const OfficialRoot = "lGuPTFQh1ONLAwOz71PTtGYNDCJvNf3cHQXgg7tvZNk="

// IndexKey is the public key the community plugin index is signed with (plugins/index), apart from OfficialRoot, which certifies release keys and signs revocation lists: the base64 of an Ed25519 public key, as `go tool nsgo keygen` prints it. Empty, Kando reports the index as not configured and never asks for it. It holds OfficialRoot's value until a key of its own is made for it.
const IndexKey = OfficialRoot

// AppIDPrefix is what a script plugin's app ID is: AppIDPrefix and its id, so the publisher segment of its id is the one its certificate covers.
const AppIDPrefix = "ai.yldm.kando.plugin."

// officialPublisher is the publisher of Kando's own plugins, which only the official root's certificates may sign.
const officialPublisher = "kando"

// Why a script plugin is refused, wrapped with the details (errors.Is finds them, and the error of tsx/pkg's they wrap). They are English and never shown as they are: the app maps each to a message of its catalogs.
var (
	// ErrUntrusted is a package whose signature does not verify, whose certificate does not chain to a root Kando trusts or does not cover it, or whose files do not match their hashes.
	ErrUntrusted = errors.New("the plugin is not signed by a key Kando trusts")
	// ErrRevoked is a package, or a release key that signed it, a root's revocation list revokes.
	ErrRevoked = errors.New("the plugin was revoked")
	// ErrRollback is a package older than one of the same plugin accepted before.
	ErrRollback = errors.New("the plugin is older than a version installed before")
	// ErrRuntime is a package that needs a newer runtime than this Kando's.
	ErrRuntime = errors.New("the plugin needs a newer version of Kando")
	// ErrScope is a plugin whose id's publisher is not the one its certificate covers, as a plugin under kando. that Kando's official key did not sign.
	ErrScope = errors.New("the plugin's id is not one its signer may use")
	// ErrNoManifest is a plugin whose code does not say what it is (kandoManifest), or fails as it loads or as it says it.
	ErrNoManifest = errors.New("the plugin's code does not say what it is")
	// ErrStopped is a call cut short because the plugin was stopped: the watchdog stopped it, or it was closed.
	ErrStopped = errors.New("the plugin was stopped")
)

// officialRoot is OfficialRoot decoded.
var officialRoot = func() ed25519.PublicKey {
	k, err := pkg.DecodePublicKey(OfficialRoot)
	if err != nil {
		panic(err)
	}
	return k
}()

// Roots are the roots a Trust of Kando's trusts: the official root, and the developer root of this computer when developer mode is on (dev, nil otherwise).
type Roots struct {
	Official, Dev ed25519.PublicKey
}

// RevocationsURL is where the revocation list of the plugins' roots is published, beside the appcasts of Kando's latest release (nsgo.yaml's release, whose feed NSGo's updater reads): a file go tool nsgo revoke writes, which the app fetches once a day (docs/plugin-system.md 6.4). Until one is published the address answers 404, which is no list.
const RevocationsURL = "https://github.com/yldm-tech/Kando/releases/latest/download/kando-plugin-revocations.json"

// DefaultRoots are the official root and the developer root dev, nil while developer mode is off.
func DefaultRoots(dev ed25519.PublicKey) Roots { return Roots{Official: officialRoot, Dev: dev} }

// TrustDir is the folder of the plugins folder root where the trust keeps the revocation lists and the builds accepted (pkg.OpenTrust).
func TrustDir(root string) string { return filepath.Join(root, "trust") }

// OpenTrust opens the trust kept in the plugins folder root and trusts roots: the official root each time, as pkg.Trust keeps its roots in memory alone, and the developer root when there is one (docs/plugin-system.md 6.4).
func OpenTrust(root string, roots Roots) (*pkg.Trust, error) {
	t, err := pkg.OpenTrust(TrustDir(root))
	if err != nil {
		return nil, err
	}
	t.AddRoot(roots.Official)
	if roots.Dev != nil {
		t.AddRoot(roots.Dev)
	}
	return t, nil
}

// Verified is a script plugin whose package verified: its id and content hash, its manifest as written, the package, where it comes from (the root its certificate chains to) and the ID of the release key that signed it, which its storage partition is named after.
type Verified struct {
	ID, Hash string
	// Manifest is the plugin's manifest as its file says it, before Validate cleaned its texts, which is what its code repeats (Probe).
	Manifest plugins.Manifest
	Package  *pkg.Package
	Source   plugins.Source
	Signer   string
}

// Verify verifies the package of the script plugin p, read from its folder (docs/plugin-system.md 4.3, 6.4): it chains to one of roots and trust neither revokes it nor accepted a newer build of it (pkg.Trust.Verify); it is the package p's manifest names by SHA-256; it is the app ai.yldm.kando.plugin.<id> at the manifest's version; its certificate covers the publisher of its id, and only the official root's covers kando.; and what it signed it may reach is exactly what the manifest declares, permissions and HTTP rules, and nothing else. Call it off the engine's goroutine: it hashes the package.
func Verify(trust *pkg.Trust, roots Roots, p plugins.Pack) (Verified, error) {
	if p.Script == nil {
		return Verified{}, fmt.Errorf("%w: not a script plugin", plugins.ErrBadField)
	}
	data, err := readPackage(filepath.Join(p.Dir, p.Script.Package))
	if err != nil {
		return Verified{}, err
	}
	signed, err := trust.Verify(data)
	if err != nil {
		return Verified{}, verifyError(err)
	}
	if signed.SHA256 != p.Script.SHA256 {
		return Verified{}, fmt.Errorf("%w: the package is not the one the manifest names", plugins.ErrTampered)
	}
	if signed.App.ID != AppIDPrefix+p.ID || signed.App.Version != p.Version {
		return Verified{}, fmt.Errorf("%w: the package is %s %s", plugins.ErrTampered, signed.App.ID, signed.App.Version)
	}
	cert, err := signed.Certificate.Info()
	if err != nil {
		return Verified{}, fmt.Errorf("%w: %w", ErrUntrusted, err)
	}
	var official bool
	switch cert.Signer {
	case pkg.KeyID(roots.Official):
		official = true
	case keyID(roots.Dev):
	default:
		return Verified{}, fmt.Errorf("%w: root %s", ErrUntrusted, cert.Signer)
	}
	if err := checkScope(p.ID, signed.App.ID, cert.Apps, official); err != nil {
		return Verified{}, err
	}
	source := sourceOf(p.ID, official)
	raw, err := readManifest(p)
	if err != nil {
		return Verified{}, err
	}
	if err := checkSigned(signed.Info, raw); err != nil {
		return Verified{}, err
	}
	return Verified{ID: p.ID, Hash: p.Hash, Manifest: raw, Package: signed, Source: source, Signer: signed.KeyID}, nil
}

// readManifest reads the manifest of the plugin p as its file says it, not cleaned as Validate cleans what it shows: what the plugin's code says of itself is compared with that (Probe). It must be the manifest p was read from, its id, version and what it asks for.
func readManifest(p plugins.Pack) (plugins.Manifest, error) {
	path := filepath.Join(p.Dir, plugins.ManifestName)
	info, err := os.Lstat(path)
	if err != nil {
		return plugins.Manifest{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return plugins.Manifest{}, fmt.Errorf("%w: %s", plugins.ErrBadManifest, plugins.ManifestName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return plugins.Manifest{}, err
	}
	m, err := plugins.Parse(data)
	if err != nil {
		return plugins.Manifest{}, err
	}
	if m.ID != p.ID || m.Version != p.Version || m.Declared().Key() != p.Declared().Key() {
		return plugins.Manifest{}, fmt.Errorf("%w: the manifest changed since it was read", plugins.ErrTampered)
	}
	return m, nil
}

// keyID is k's ID, "" for none.
func keyID(k ed25519.PublicKey) string {
	if k == nil {
		return ""
	}
	return pkg.KeyID(k)
}

// readPackage reads the package at path, at most plugins.PackageMax bytes.
func readPackage(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", plugins.ErrUnsafePath, filepath.Base(path))
	}
	if info.Size() > plugins.PackageMax {
		return nil, fmt.Errorf("%w: the package is over %d MiB", plugins.ErrTooLarge, plugins.PackageMax>>20)
	}
	return os.ReadFile(path)
}

// verifyError is the error of this package's err of pkg.Trust.Verify stands for, wrapping it.
func verifyError(err error) error {
	switch {
	case errors.Is(err, pkg.ErrRevoked):
		return fmt.Errorf("%w: %w", ErrRevoked, err)
	case errors.Is(err, pkg.ErrRollback):
		return fmt.Errorf("%w: %w", ErrRollback, err)
	case errors.Is(err, pkg.ErrRuntime):
		return fmt.Errorf("%w: %w", ErrRuntime, err)
	}
	return fmt.Errorf("%w: %w", ErrUntrusted, err)
}

// checkScope checks that a pattern of apps covering app, the plugin id's app ID, covers no more than the publisher of id (ai.yldm.kando.plugin.<publisher>.*) or the app ID itself, so a certificate given to one publisher cannot sign another's ids; and that a plugin of the publisher kando comes from the official root.
func checkScope(id, app string, apps []string, official bool) error {
	publisher, _, _ := strings.Cut(id, ".")
	// Kando's own plugins come from the official root under a certificate for the whole of kando.*, never one that names an id of it alone.
	if publisher == officialPublisher && (!official || !slices.Contains(apps, AppIDPrefix+officialPublisher+".*")) {
		return fmt.Errorf("%w: %s is Kando's own", ErrScope, id)
	}
	if !slices.ContainsFunc(apps, func(p string) bool { return p == app || p == AppIDPrefix+publisher+".*" }) {
		return fmt.Errorf("%w: the certificate covers %s", ErrScope, strings.Join(apps, ", "))
	}
	return nil
}

// sourceOf is where the plugin id comes from, its certificate chaining to the official root when official is set, as checkScope let it: Kando's own (SourceOfficial) only for the publisher kando, whose certificates only the official root signs, scoped to ai.yldm.kando.plugin.kando.*; another publisher's that root certified is verified by Kando (SourceVerified); one of this computer's developer key is a developer's (SourceDeveloper). Never from the manifest: its author says nothing of who signed it (docs/plugin-system.md 6.4).
func sourceOf(id string, official bool) plugins.Source {
	publisher, _, _ := strings.Cut(id, ".")
	switch {
	case official && publisher == officialPublisher:
		return plugins.SourceOfficial
	case official:
		return plugins.SourceVerified
	}
	return plugins.SourceDeveloper
}

// Publisher is the publisher of the plugin id, the segment before its first dot, which its certificate covers (checkScope): what the install confirmation names it by, never its manifest's author.
func Publisher(id string) string {
	publisher, _, _ := strings.Cut(id, ".")
	return publisher
}

// checkSigned checks that the capabilities the package signed are those the manifest m declares: its permissions, those it needs and those it can do without, as a set, and its HTTP rules, and nothing else, not even one this runtime does not know (Info.Ignored).
func checkSigned(info pkg.Info, m plugins.Manifest) error {
	if len(info.Ignored) > 0 {
		return fmt.Errorf("%w: it signed %s", plugins.ErrBadCapabilities, strings.Join(info.Ignored, ", "))
	}
	c := info.Capabilities
	rest := c
	rest.Host, rest.Network = nil, nil
	if !reflect.ValueOf(rest).IsZero() {
		return fmt.Errorf("%w: it signed more than host and network", plugins.ErrBadCapabilities)
	}
	// The credentials are Kando's to send, not a capability of the package: what it signed is its permissions and HTTP rules alone.
	declared := m.Declared()
	declared.Credentials = nil
	signed := plugins.Grant{}
	for _, h := range c.Host {
		signed.Perms = append(signed.Perms, plugins.Perm(h))
	}
	if c.Network != nil {
		if len(c.Network.Hosts) > 0 {
			return fmt.Errorf("%w: it signed network hosts", plugins.ErrBadCapabilities)
		}
		signed.Network = c.Network.HTTP
	}
	if len(signed.Perms) != len(declared.Perms) || len(signed.Network) != len(declared.Network) || !signed.Within(declared) || !declared.Within(signed) {
		return fmt.Errorf("%w: what its code may reach is not what its manifest says", plugins.ErrTampered)
	}
	return nil
}
