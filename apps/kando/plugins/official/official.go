// Package official publishes what Kando's plugin root vouches for (docs/plugin-system.md 6.4, 15): Kando's official script plugins, whose sources are the folders beside this file (ankiconnect/, deepl/, webdav/, alist/, jellyfin/, emby/, each a kando-plugin.json without its script, the plugin.tsx it starts from and the nsgo.yaml `go tool nsgo publish` reads), wrapped into the .kandoplugin files Kando installs once nsgo publish signed them (Wrap); and the revocation list of the plugins' root, which a release checks before it attaches it for Kando to fetch (CheckRevocations). Signing itself is nsgo's, on the owner's computer, with keys that never reach the repository or CI. It reads the plugin root and the app-ID prefix from package signature, their one home, and like it never links the TSX runtime or an interface, so a release builds it in seconds; it is not part of the app.
package official

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// RevocationsFile is the name the revocation list is published under, as an asset of each release beside the appcasts (script.RevocationsURL).
const RevocationsFile = "kando-plugin-revocations.json"

// manifestName is the manifest's file name in a plugin (plugins.ManifestName).
const manifestName = "kando-plugin.json"

// ErrMismatch is a package that is not the plugin its manifest says: another app ID or version.
var ErrMismatch = errors.New("the package is not the plugin its manifest says")

// Ext is the extension of the file Wrap makes.
const Ext = ".kandoplugin"

// FileName is the name of the .kandoplugin of the plugin id at version: id-version.kandoplugin. A published file is never replaced in place, as a Kando that cached an older index checks it by its SHA-256, so each version has a name of its own.
func FileName(id, version string) string { return id + "-" + version + Ext }

// Wrap makes the .kandoplugin of a plugin from manifest, its source kando-plugin.json, and the package nsgo publish signed of it, data, named pkgName in the file: a zip of the manifest with its script naming the package by name and SHA-256, and the package, as plugins.Stage takes it. The package must be the manifest's plugin: its app ID ai.yldm.kando.plugin.<id> and its version; and if it signed its manifest (pkg.Contents.Extra), what it signed must be the manifest's (signature.CheckManifest) and every Kando the manifest says it runs on must take it (signature.SignsManifest). What the package signed it may reach is checked as Kando installs it (script.Verify), not here. It also returns the plugin's id and version.
func Wrap(manifest []byte, pkgName string, data []byte) (out []byte, id, version string, err error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(manifest, &raw); err != nil {
		return nil, "", "", fmt.Errorf("%s: %w", manifestName, err)
	}
	var m struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, "", "", fmt.Errorf("%s: %w", manifestName, err)
	}
	if m.Kind != "script" || m.ID == "" || m.Version == "" {
		return nil, "", "", fmt.Errorf("%s: not a script plugin with an id and a version", manifestName)
	}
	if path.Base(pkgName) != pkgName || path.Ext(pkgName) != ".nsgopkg" {
		return nil, "", "", fmt.Errorf("the package's name %q is not a file name ending in .nsgopkg", pkgName)
	}
	info, err := pkg.Inspect(data)
	if err != nil {
		return nil, "", "", err
	}
	if info.App.ID != signature.AppIDPrefix+m.ID || info.App.Version != m.Version {
		return nil, "", "", fmt.Errorf("%w: %s %s, where the manifest is %s%s %s", ErrMismatch, info.App.ID, info.App.Version, signature.AppIDPrefix, m.ID, m.Version)
	}
	if info.Extra != nil {
		if err := checkSignedManifest(manifest, info.Extra); err != nil {
			return nil, "", "", err
		}
	}
	sum := sha256.Sum256(data)
	raw["script"], err = json.Marshal(map[string]string{"package": pkgName, "sha256": hex.EncodeToString(sum[:])})
	if err != nil {
		return nil, "", "", err
	}
	text, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, "", "", err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		data []byte
	}{{manifestName, append(text, '\n')}, {pkgName, data}} {
		// A fixed time, so the same package wraps into the same file.
		fw, err := w.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return nil, "", "", err
		}
		if _, err := fw.Write(f.data); err != nil {
			return nil, "", "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", "", err
	}
	return buf.Bytes(), m.ID, m.Version, nil
}

// ErrSignedManifest is a package that signed its manifest (pkg.Contents.Extra) for a plugin whose manifest says it runs on a Kando older than signature.ManifestEntrySince, which refuses such a package.
var ErrSignedManifest = errors.New("the package signs its manifest, which a Kando the manifest says it runs on refuses")

// checkSignedManifest checks the manifest a package signed, signed, against manifest, the plugin's source kando-plugin.json: ErrSignedManifest when a Kando it says it runs on refuses a package that signed it, ErrMismatch when it is not what it signed.
func checkSignedManifest(manifest, signed []byte) error {
	m, err := plugins.Parse(manifest)
	if err != nil {
		return fmt.Errorf("%s: %w", manifestName, err)
	}
	if !signature.SignsManifest(m) {
		return fmt.Errorf("%w: kando %q, where it takes %s or newer", ErrSignedManifest, m.Kando, signature.ManifestEntrySince)
	}
	if err := signature.CheckManifest(signed, m); err != nil {
		return fmt.Errorf("%w: %w", ErrMismatch, err)
	}
	return nil
}

// VerifyPackage checks that the package data chains to root, as Kando's trust does before anything else (pkg.Trust.Verify), with no revocation list.
func VerifyPackage(data []byte, root ed25519.PublicKey) (*pkg.Package, error) {
	t := pkg.NewTrust()
	t.AddRoot(root)
	return t.Verify(data)
}

// Why a revocation list is not published.
var (
	ErrExpired = errors.New("the revocation list has expired")
	ErrOlder   = errors.New("the revocation list is older than the one the latest release published")
)

// CheckRevocations checks the revocation list data before a release publishes it: root signed it, it has not expired at now, and it is not older than previous, the list the latest release published (nil for none): its sequence is above previous's, or it is previous itself, unchanged, as most releases publish the list again. It returns what the list says. A list of the same sequence with other contents is older too: Kando keeps the first it saw of a sequence (pkg.ErrStale).
func CheckRevocations(data, previous []byte, root ed25519.PublicKey, now time.Time) (pkg.RevocationsInfo, error) {
	info, err := verifyRevocations(data, root)
	if err != nil {
		return pkg.RevocationsInfo{}, err
	}
	if !info.NotAfter.IsZero() && !now.Before(info.NotAfter) {
		return info, fmt.Errorf("%w: list %d expired %s", ErrExpired, info.Sequence, info.NotAfter.UTC().Format(time.DateOnly))
	}
	if previous == nil {
		return info, nil
	}
	prev, err := verifyRevocations(previous, root)
	if err != nil {
		return info, fmt.Errorf("the list the latest release published: %w", err)
	}
	switch {
	case info.Sequence < prev.Sequence:
		return info, fmt.Errorf("%w: list %d, where it published %d", ErrOlder, info.Sequence, prev.Sequence)
	case info.Sequence == prev.Sequence && !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(previous)):
		return info, fmt.Errorf("%w: another list %d than the one it published; sign it with a higher -sequence", ErrOlder, info.Sequence)
	}
	return info, nil
}

// verifyRevocations is what the revocation list data says, once root's signature on it verified.
func verifyRevocations(data []byte, root ed25519.PublicKey) (pkg.RevocationsInfo, error) {
	list, err := pkg.ParseRevocations(data)
	if err != nil {
		return pkg.RevocationsInfo{}, err
	}
	t := pkg.NewTrust()
	t.AddRoot(root)
	if err := t.UpdateRevocations(list); err != nil {
		return pkg.RevocationsInfo{}, err
	}
	return list.Info()
}
