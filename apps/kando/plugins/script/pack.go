package script

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Entry is the TSX file a developer's plugin folder starts from, beside its kando-plugin.json.
const Entry = "plugin.tsx"

// ErrNoEntry is a developer's plugin folder without its Entry.
var ErrNoEntry = errors.New("the folder has no plugin.tsx")

// NewDevKey makes a developer key, the root and the release key of this computer's developer plugins in one (docs/plugin-system.md 6.4): its secret, as pkg encodes it, for the app to keep in the system's secure store, and its public key, the root developer mode trusts.
func NewDevKey() (secret string, public ed25519.PublicKey, err error) {
	key, err := pkg.GenerateKey()
	if err != nil {
		return "", nil, err
	}
	return pkg.EncodeSecretKey(key), key.Public().(ed25519.PublicKey), nil
}

// DevKey is the developer key secret encodes, and its public key.
func DevKey(secret string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	key, err := pkg.DecodeSecretKey(secret)
	if err != nil {
		return nil, nil, err
	}
	return key, key.Public().(ed25519.PublicKey), nil
}

// PackFolder makes a script plugin of the developer's folder src, which holds its kando-plugin.json, without its script, and its TSX code starting at Entry: it compiles the code (bundler.Compile, as nsgo publish does), signs it with the developer key, as root and release key at once, certified for the publisher of the plugin's id alone, and writes the plugin into the empty folder dst: the package, <name>.nsgopkg, and the manifest with its script naming it, as Stage then takes it. The package's build is the time it was made, so each one is newer than the last. Nothing of it is checked here beyond what signing needs: the app checks it as it checks any plugin (plugins.Stage, Verify, Probe).
func PackFolder(src, dst string, key ed25519.PrivateKey) error {
	data, err := os.ReadFile(filepath.Join(src, plugins.ManifestName))
	if err != nil {
		return fmt.Errorf("%w: %v", plugins.ErrBadManifest, err)
	}
	m, err := plugins.Parse(data)
	if err != nil {
		return err
	}
	if m.Kind != plugins.KindScript {
		return fmt.Errorf("%w: %s", plugins.ErrBadField, m.Kind)
	}
	if _, err := os.Stat(filepath.Join(src, Entry)); err != nil {
		return ErrNoEntry
	}
	source, err := bundler.Compile(os.DirFS(src), Entry)
	if err != nil {
		return err
	}
	publisher, name, ok := strings.Cut(m.ID, ".")
	if !ok {
		return fmt.Errorf("%w: %q", plugins.ErrBadID, m.ID)
	}
	now := time.Now()
	public := key.Public().(ed25519.PublicKey)
	cert, err := pkg.Certify(key, public, []string{AppIDPrefix + publisher + ".*"}, now.Add(-time.Hour), now.AddDate(1, 0, 0))
	if err != nil {
		return err
	}
	declared := m.Declared()
	caps := pkg.Capabilities{}
	for _, p := range declared.Perms {
		caps.Host = append(caps.Host, string(p))
	}
	if len(declared.Network) > 0 {
		caps.Network = &pkg.Network{HTTP: declared.Network}
	}
	app := pkg.App{ID: AppIDPrefix + m.ID, Name: m.Name["en"], Version: m.Version, Build: uint64(now.Unix())}
	// TODO(nsgo): nsgo publish cannot sign pkg.Contents.Extra, so no release signs a plugin's manifest and its code must say it (kandoManifest); a developer's package signs none either, so it is checked as the plugin will be once published. Once nsgo publish can, sign signature.SignedManifest here and in .github/scripts/official-plugins.sh for a manifest signature.SignsManifest takes.
	signed, err := pkg.Build(pkg.Contents{App: app, Source: source, Capabilities: caps}, key, cert)
	if err != nil {
		return err
	}
	file := strings.ReplaceAll(name, ".", "-") + ".nsgopkg"
	sum := sha256.Sum256(signed)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: %v", plugins.ErrBadManifest, err)
	}
	raw["script"], err = json.Marshal(plugins.Script{Package: file, SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dst, file), signed, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, plugins.ManifestName), append(manifest, '\n'), 0o644)
}
