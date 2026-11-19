package script

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// The tests sign their plugins with roots and release keys they make and throw away: Kando's official root's secret is never needed, as a Trust is given the test's root in its place (Roots).

// signer is a throwaway root and a release key it certified for apps.
type signer struct {
	root, release ed25519.PrivateKey
	cert          *pkg.Certificate
}

// newSigner makes a root and a release key it certifies for apps, the patterns of app IDs.
func newSigner(t testing.TB, apps ...string) signer {
	t.Helper()
	root, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	release, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cert, err := pkg.Certify(root, release.Public().(ed25519.PublicKey), apps, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return signer{root: root, release: release, cert: cert}
}

// public is the signer's root's public key.
func (s signer) public() ed25519.PublicKey { return s.root.Public().(ed25519.PublicKey) }

// roots are Roots with s's root as the official one.
func (s signer) roots() Roots { return Roots{Official: s.public()} }

// trust is a Trust in a new folder that trusts roots.
func trustOf(t testing.TB, roots Roots) *pkg.Trust {
	t.Helper()
	trust, err := OpenTrust(t.TempDir(), roots)
	if err != nil {
		t.Fatal(err)
	}
	return trust
}

// fixture is a script plugin to sign: its id, version and build, the manifest's capabilities, optional, contributes, settings and events as JSON (caps etc.), and its code, whose kandoManifest returns said (the manifest's fields when said is "").
type fixture struct {
	id, version string
	build       uint64
	caps        string
	extra       string
	code        string
	said        string
	// signed are the capabilities the package signs, the manifest's when nil.
	signed *pkg.Capabilities
}

// manifestFields is the fixture's manifest fields that kandoManifest repeats, as JSON object members.
func (f fixture) manifestFields() string {
	fields := `"capabilities": ` + f.caps
	if f.extra != "" {
		fields += ", " + f.extra
	}
	return fields
}

// pack signs the fixture with s into a plugin folder, validates it and returns it.
func (s signer) pack(t testing.TB, f fixture) plugins.Pack {
	t.Helper()
	if f.version == "" {
		f.version = "1.0.0"
	}
	if f.build == 0 {
		f.build = 1
	}
	if f.caps == "" {
		f.caps = `{"host": ["kando.toast"]}`
	}
	said := f.said
	if said == "" {
		said = "{" + f.manifestFields() + "}"
	}
	source := "export function kandoManifest() { return " + said + "; }\n" + f.code
	code, err := bundler.Compile(fstest.MapFS{Entry: {Data: []byte(source)}}, Entry)
	if err != nil {
		t.Fatal(err)
	}
	caps := f.signed
	if caps == nil {
		m, err := plugins.Parse([]byte(`{"schema": 2, "id": "a.b", "kind": "script", "script": {"package": "p.nsgopkg", "sha256": ""}, ` + f.manifestFields() + `}`))
		if err != nil {
			t.Fatal(err)
		}
		d := m.Declared()
		caps = &pkg.Capabilities{}
		for _, p := range d.Perms {
			caps.Host = append(caps.Host, string(p))
		}
		if len(d.Network) > 0 {
			caps.Network = &pkg.Network{HTTP: d.Network}
		}
	}
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: AppIDPrefix + f.id, Name: "Fixture", Version: f.version, Build: f.build}, Source: code, Capabilities: *caps}, s.release, s.cert)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	manifest := `{"schema": 2, "id": "` + f.id + `", "kind": "script", "version": "` + f.version + `", "license": "MIT", "name": {"en": "Fixture"},
  "script": {"package": "plugin.nsgopkg", "sha256": "` + hex.EncodeToString(sum[:]) + `"}, ` + f.manifestFields() + `}`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.nsgopkg"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := plugins.Validate(dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, manifest)
	}
	return p
}

// engine is a graph of its own on a goroutine of its own, the engine's in the app, with the system's clock, so a plugin's timers and the watchdog's heartbeats run as they do in the app. Its graph has a tree, which tsx.Load needs (nsgo#838).
type engine struct {
	root  *core.Scope
	tree  *core.Tree
	posts chan func()
}

// newEngine starts an engine, which stops at the end of the test.
func newEngine(t testing.TB) *engine {
	t.Helper()
	e := &engine{posts: make(chan func(), 1<<14)}
	ready := make(chan struct{})
	go func() {
		e.root = core.NewRoot(core.RootOptions{Post: e.post})
		// TODO(nsgo#838): tsx.Load fails with a nil pointer on a graph without a tree, which its documentation does not say; drop the tree once Load needs none https://github.com/nsgo-dev/nsgo/issues/838
		e.tree = core.NewTree(e.root, nil)
		close(ready)
		for fn := range e.posts {
			if fn == nil {
				e.root.Dispose()
				return
			}
			fn()
		}
	}()
	<-ready
	t.Cleanup(func() {
		done := make(chan struct{})
		e.posts <- func() { close(done) }
		<-done
		e.posts <- nil
	})
	return e
}

// post runs fn on the engine later.
func (e *engine) post(fn func()) { e.posts <- fn }

// do runs fn on the engine and waits for it.
func (e *engine) do(fn func()) {
	done := make(chan struct{})
	e.post(func() {
		defer close(done)
		fn()
	})
	<-done
}

// result is what a call ended with, and when.
type result struct {
	v   any
	err error
	at  time.Time
}

// call calls name of p through r and returns a channel that hears its end.
func (e *engine) call(r *Runner, p *Plugin, name string, args ...any) <-chan result {
	out := make(chan result, 1)
	e.post(func() {
		r.Call(p, name, args, func(v any, err error) { out <- result{v, err, time.Now()} })
	})
	return out
}

// wait is the end of a call, failing t after limit.
func wait(t testing.TB, c <-chan result, limit time.Duration) result {
	t.Helper()
	select {
	case r := <-c:
		return r
	case <-time.After(limit):
		t.Fatalf("the call did not end within %v", limit)
	}
	return result{}
}

// load verifies p against trust and roots and loads it on e with grant, through a new host.
func (e *engine) load(t testing.TB, trust *pkg.Trust, roots Roots, p plugins.Pack, grant plugins.Grant) *Plugin {
	t.Helper()
	v, err := Verify(trust, roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var pl *Plugin
	e.do(func() { pl, err = Load(e.root, v, grant, core.NewHost()) })
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

// contains reports whether err's text has s.
func contains(err error, s string) bool { return err != nil && strings.Contains(err.Error(), s) }

// ed25519Public names a release key's public key.
type ed25519Public = ed25519.PublicKey

// slow is d, five times longer under the race detector, which slows the plugins' JS down as much.
func slow(d time.Duration) time.Duration {
	if raceOn {
		return 5 * d
	}
	return d
}
