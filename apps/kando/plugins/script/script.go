// Package script runs Kando's script plugins (docs/plugin-system.md, P3): the trust their signatures chain to (OpenTrust, Verify, which package signature holds and this package re-exports), the sandbox that reads what a plugin's signed code says of itself before the user agreed to anything (Probe), the one constructor of the sandbox a plugin runs in (Sandbox, Limits), loading it (Load), and the runner that calls it one call at a time and stops a plugin that keeps the window from answering or holds too much memory (Runner). It imports no interface package; the app gives each plugin its own host (core.Host) and checks every call against what the user granted.
package script

import (
	"crypto/ed25519"
	"errors"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// The keys, the trust and the verification of a script plugin's signature are package signature's, which builds without the TSX runtime for the command line and the plugin directory's CI tool; they are re-exported here so the app keeps one name for each.
const (
	OfficialRoot = signature.OfficialRoot
	IndexKey     = signature.IndexKey
	AppIDPrefix  = signature.AppIDPrefix
)

// Roots are the roots a Trust of Kando's trusts (signature.Roots); Verified is a script plugin whose package verified (signature.Verified).
type (
	Roots    = signature.Roots
	Verified = signature.Verified
)

// Why a script plugin is refused, wrapped with the details (errors.Is finds them). They are English and never shown as they are: the app maps each to a message of its catalogs. The first five are its signature's (package signature).
var (
	ErrUntrusted = signature.ErrUntrusted
	ErrRevoked   = signature.ErrRevoked
	ErrRollback  = signature.ErrRollback
	ErrRuntime   = signature.ErrRuntime
	ErrScope     = signature.ErrScope
	// ErrNoManifest is a plugin whose code does not say what it is (kandoManifest), or fails as it loads or as it says it.
	ErrNoManifest = errors.New("the plugin's code does not say what it is")
	// ErrStopped is a call cut short because the plugin was stopped: the watchdog stopped it, or it was closed.
	ErrStopped = errors.New("the plugin was stopped")
)

// RevocationsURL is where the revocation list of the plugins' roots is published, beside the appcasts of Kando's latest release (nsgo.yaml's release, whose feed NSGo's updater reads): a file go tool nsgo revoke writes, which the app fetches once a day (docs/plugin-system.md 6.4). Until one is published the address answers 404, which is no list.
const RevocationsURL = "https://github.com/yldm-tech/Kando/releases/latest/download/kando-plugin-revocations.json"

// officialRoot is OfficialRoot decoded.
var officialRoot = signature.DefaultRoots(nil).Official

// DefaultRoots are the official root and the developer root dev (signature.DefaultRoots).
func DefaultRoots(dev ed25519.PublicKey) Roots { return signature.DefaultRoots(dev) }

// TrustDir is the folder of the plugins folder root where the trust keeps the revocation lists and the builds accepted (signature.TrustDir).
func TrustDir(root string) string { return signature.TrustDir(root) }

// OpenTrust opens the trust kept in the plugins folder root and trusts roots (signature.OpenTrust).
func OpenTrust(root string, roots Roots) (*pkg.Trust, error) { return signature.OpenTrust(root, roots) }

// Verify verifies the package of the script plugin p (signature.Verify). Call it off the engine's goroutine: it hashes the package.
func Verify(trust *pkg.Trust, roots Roots, p plugins.Pack) (Verified, error) {
	return signature.Verify(trust, roots, p)
}

// Publisher is the publisher of the plugin id (signature.Publisher).
func Publisher(id string) string { return signature.Publisher(id) }
