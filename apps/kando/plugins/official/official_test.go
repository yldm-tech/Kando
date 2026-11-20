package official_test

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// TestPluginOfficialAddresses checks that the revocation list's name the release publishes is the one Kando fetches.
func TestPluginOfficialAddresses(t *testing.T) {
	if !strings.HasSuffix(script.RevocationsURL, "/releases/latest/download/"+official.RevocationsFile) {
		t.Errorf("Kando fetches %s, the release publishes %s", script.RevocationsURL, official.RevocationsFile)
	}
}

// key is a throwaway key.
func key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	k, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// TestPluginOfficialWrap checks the .kandoplugin Wrap makes: the manifest naming the package by its SHA-256 and the package, nothing else, the same bytes each time; a package of another plugin or version is refused.
func TestPluginOfficialWrap(t *testing.T) {
	root := key(t)
	cert, err := pkg.Certify(root, root.Public().(ed25519.PublicKey), []string{"ai.yldm.kando.plugin.kando.*"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: "ai.yldm.kando.plugin.kando.demo", Name: "Demo", Version: "1.2.0", Build: 3}, Source: "1"}, root, cert)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := official.VerifyPackage(data, root.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := official.VerifyPackage(data, key(t).Public().(ed25519.PublicKey)); err == nil {
		t.Error("a package of another root verified")
	}
	manifest := []byte(`{"schema": 2, "id": "kando.demo", "kind": "script", "version": "1.2.0", "name": {"en": "Demo"}}`)
	out, id, version, err := official.Wrap(manifest, "demo.nsgopkg", data)
	if err != nil || id != "kando.demo" || version != "1.2.0" {
		t.Fatalf("%s %s: %v", id, version, err)
	}
	if again, _, _, _ := official.Wrap(manifest, "demo.nsgopkg", data); !bytes.Equal(again, out) {
		t.Error("the same package wraps into another file")
	}
	if official.FileName(id, version) != "kando.demo-1.2.0.kandoplugin" {
		t.Errorf("file name %s", official.FileName(id, version))
	}
	r, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	if len(files) != 2 || !bytes.Equal(files["demo.nsgopkg"], data) {
		t.Fatalf("the zip holds %d files", len(files))
	}
	var m struct {
		ID     string            `json:"id"`
		Script map[string]string `json:"script"`
	}
	if err := json.Unmarshal(files["kando-plugin.json"], &m); err != nil {
		t.Fatal(err)
	}
	info, _ := pkg.Inspect(data)
	if m.ID != "kando.demo" || m.Script["package"] != "demo.nsgopkg" || m.Script["sha256"] != info.SHA256 {
		t.Errorf("the manifest %+v, the package's sha256 %s", m, info.SHA256)
	}
	for name, bad := range map[string][]byte{
		"another version": []byte(`{"id": "kando.demo", "kind": "script", "version": "1.3.0"}`),
		"another id":      []byte(`{"id": "kando.other", "kind": "script", "version": "1.2.0"}`),
	} {
		if _, _, _, err := official.Wrap(bad, "demo.nsgopkg", data); !errors.Is(err, official.ErrMismatch) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, _, err := official.Wrap(manifest, "../demo.nsgopkg", data); err == nil {
		t.Error("a package name with a folder")
	}
}

// TestPluginOfficialWrapSignedManifest checks a package that signed its manifest (pkg.Contents.Extra): Wrap takes it when what it signed is the manifest's and every Kando the manifest says it runs on takes it, and refuses it for a manifest that says more than it signed or that runs on a Kando older than signature.ManifestEntrySince.
func TestPluginOfficialWrapSignedManifest(t *testing.T) {
	root := key(t)
	cert, err := pkg.Certify(root, root.Public().(ed25519.PublicKey), []string{"ai.yldm.kando.plugin.kando.*"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	manifest := func(kando, fields string) []byte {
		return []byte(`{"schema": 2, "id": "kando.demo", "kind": "script", "version": "1.2.0", "kando": "` + kando + `", "name": {"en": "Demo"}, "capabilities": {"host": ["kando.toast"]}` + fields + `}`)
	}
	m, err := plugins.Parse(manifest(">="+signature.ManifestEntrySince, ""))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := signature.SignedManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: "ai.yldm.kando.plugin.kando.demo", Name: "Demo", Version: "1.2.0", Build: 3}, Source: "1", Capabilities: pkg.Capabilities{Host: []string{"kando.toast"}}, Extra: entry}, root, cert)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := official.Wrap(manifest(">="+signature.ManifestEntrySince, ""), "demo.nsgopkg", data); err != nil {
		t.Errorf("a package that signed its manifest: %v", err)
	}
	if _, _, _, err := official.Wrap(manifest(">=1.0.0", ""), "demo.nsgopkg", data); err != nil {
		t.Errorf("a manifest of a newer Kando: %v", err)
	}
	if _, _, _, err := official.Wrap(manifest(">="+signature.ManifestEntrySince, `, "optional": ["kando.saved.read"]`), "demo.nsgopkg", data); !errors.Is(err, official.ErrMismatch) {
		t.Errorf("a manifest that says more than the package signed: %v", err)
	}
	for _, kando := range []string{">=0.37.0", ""} {
		if _, _, _, err := official.Wrap(manifest(kando, ""), "demo.nsgopkg", data); !errors.Is(err, official.ErrSignedManifest) {
			t.Errorf("a manifest of Kando %q: %v", kando, err)
		}
	}
}

// revoke is a revocation list root signs, number sequence, expiring in days.
func revoke(t *testing.T, root ed25519.PrivateKey, sequence uint64, days int, keys ...string) []byte {
	t.Helper()
	list, err := pkg.Revoke(root, sequence, time.Now().AddDate(0, 0, days), keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestPluginOfficialRevocations checks what a release publishes of the revocation list: one the root signed and that has not expired, the same as the latest release's or newer; never one another key signed, one expired, an older one, nor another list of the same sequence.
func TestPluginOfficialRevocations(t *testing.T) {
	root := key(t)
	public := root.Public().(ed25519.PublicKey)
	now := time.Now()
	list := revoke(t, root, 3, 90, "0123456789abcdef")
	info, err := official.CheckRevocations(list, nil, public, now)
	if err != nil || info.Sequence != 3 || len(info.Keys) != 1 {
		t.Fatalf("%+v, %v", info, err)
	}
	if _, err := official.CheckRevocations(list, list, public, now); err != nil {
		t.Errorf("the latest release's list again: %v", err)
	}
	if _, err := official.CheckRevocations(list, revoke(t, root, 2, 90), public, now); err != nil {
		t.Errorf("a newer list: %v", err)
	}
	if _, err := official.CheckRevocations(list, nil, public, now.AddDate(0, 0, 91)); !errors.Is(err, official.ErrExpired) {
		t.Errorf("expired: %v", err)
	}
	if _, err := official.CheckRevocations(list, revoke(t, root, 4, 90), public, now); !errors.Is(err, official.ErrOlder) {
		t.Errorf("older: %v", err)
	}
	if _, err := official.CheckRevocations(list, revoke(t, root, 3, 30), public, now); !errors.Is(err, official.ErrOlder) {
		t.Errorf("another list 3: %v", err)
	}
	if _, err := official.CheckRevocations(list, nil, key(t).Public().(ed25519.PublicKey), now); err == nil {
		t.Error("a list another root signed")
	}
	if _, err := official.CheckRevocations([]byte("{}"), nil, public, now); err == nil {
		t.Error("not a list")
	}
}
