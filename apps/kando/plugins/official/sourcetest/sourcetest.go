// Package sourcetest runs one of Kando's official media-source plugins (docs/media-sources.md 11.5) in its tests as Kando runs it: the plugin's folder signed with a key the test makes, which stands in for Kando's official root, wrapped as a release wraps it (official.Wrap), staged and verified as an official plugin (plugins.Stage, script.Verify), loaded in Kando's sandbox (script.Load) on an engine of its own and called through Kando's runner; its sources read by Kando's own mediasource.PluginDriver, and its kando.source.fetch sent by Kando's own mediasource.Fetcher to the test's fake server, with the source's secrets put in as its manifest says. It is only for the plugins' tests, which import it; nothing of the app does.
package sourcetest

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official"
	"kando/apps/kando/plugins/script"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
)

// Folder is the folder of the test source file that calls it, a plugin's folder, found from the file's own path rather than the working directory, which nsgo test -target runs the test binary elsewhere than.
func Folder(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("no path of the test's source")
	}
	return filepath.Dir(file)
}

// engine is a graph of its own on a goroutine of its own, the engine's in the app, as plugins/script's tests run one.
type engine struct {
	root  *core.Scope
	posts chan func()
}

func newEngine(t testing.TB) *engine {
	t.Helper()
	e := &engine{posts: make(chan func(), 1<<14)}
	ready := make(chan struct{})
	go func() {
		e.root = core.NewRoot(core.RootOptions{Post: e.post})
		// tsx.Load needs a tree: on a scope without one it fails with tsx.ErrNoTree.
		core.NewTree(e.root, nil)
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

func (e *engine) post(fn func()) { e.posts <- fn }

func (e *engine) do(fn func()) {
	done := make(chan struct{})
	e.post(func() {
		defer close(done)
		fn()
	})
	<-done
}

// Plugin is an official media-source plugin signed, installed and running as Kando runs it, granted what it declares.
type Plugin struct {
	// Verified is the plugin as Kando verified it, its manifest included.
	Verified script.Verified

	e  *engine
	r  *script.Runner
	pl *script.Plugin

	mu       sync.Mutex
	fetchers map[string]*mediasource.Fetcher
	sent     []Sent
	seq      int
	// stopped are the reasons the runner stopped the plugin for, which no test expects (engine's goroutine).
	stopped []string
}

// Sent is a request the plugin sent through kando.source.fetch, as it sent it, before Kando put any secret in.
type Sent struct {
	Source string
	mediasource.FetchRequest
}

// Load signs the plugin folder dir with a key the test makes, which stands in for Kando's official root (script.PackFolder certifies it for the publisher kando), wraps the package as the release does (official.Wrap), stages the .kandoplugin as Kando installs it (plugins.Stage), verifies it as an official plugin (script.Verify) of the id want, and runs it in Kando's sandbox granted everything it declares, its host answering kando.source.fetch as Kando does.
func Load(t testing.TB, dir, want string) *Plugin {
	t.Helper()
	secret, public, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := script.DevKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	packed := t.TempDir()
	if err := script.PackFolder(dir, packed, key); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, plugins.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	pkgPath, _ := filepath.Glob(filepath.Join(packed, "*.nsgopkg"))
	if len(pkgPath) != 1 {
		t.Fatalf("packed %v", pkgPath)
	}
	data, err := os.ReadFile(pkgPath[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := official.VerifyPackage(data, public); err != nil {
		t.Fatal(err)
	}
	zipped, id, version, err := official.Wrap(manifest, filepath.Base(pkgPath[0]), data)
	if err != nil {
		t.Fatal(err)
	}
	if id != want {
		t.Fatalf("wrapped %s %s, want %s", id, version, want)
	}
	file := filepath.Join(t.TempDir(), official.FileName(id, version))
	if err := os.WriteFile(file, zipped, 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "plugins")
	_, p, err := plugins.Stage(root, file)
	if err != nil {
		t.Fatal(err)
	}
	roots := script.Roots{Official: public}
	trust, err := script.OpenTrust(root, roots)
	if err != nil {
		t.Fatal(err)
	}
	v, err := script.Verify(trust, roots, p)
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != plugins.SourceOfficial {
		t.Fatalf("the plugin comes from %q, not Kando", v.Source)
	}
	e := newEngine(t)
	e.do(func() { err = script.Probe(e.root, v) })
	if err != nil {
		t.Fatalf("what the code says of itself is not its manifest: %v", err)
	}
	out := &Plugin{Verified: v, e: e, fetchers: map[string]*mediasource.Fetcher{}}
	// The runner's deadline and pauses are Kando's (script.DefaultWatch); only the heartbeat is lenient, as the race detector slows the engine down.
	w := script.DefaultWatch
	w.Stall = time.Minute
	out.r = &script.Runner{Post: e.post, Watch: w, Stopped: func(id, hash, reason string) { out.stopped = append(out.stopped, reason) }}
	t.Cleanup(func() {
		var stopped []string
		e.do(func() { stopped = out.stopped })
		if len(stopped) > 0 {
			t.Errorf("the runner stopped the plugin: %v", stopped)
		}
	})
	host := core.NewHost()
	host.Handle(string(plugins.PermSourceFetch), out.fetch)
	d := v.Manifest.Declared()
	grant := plugins.Grant{Perms: d.Perms, Network: d.Network}
	e.do(func() { out.pl, err = script.Load(e.root.Child(), v, grant, host) })
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Kind is the plugin's kind of media source id, as Kando registers it, and its declaration in the manifest.
func (p *Plugin) Kind(t testing.TB, id string) (mediasource.Kind, plugins.MediaSource) {
	t.Helper()
	m := p.Verified.Manifest
	if m.Contributes != nil {
		for _, src := range m.Contributes.Sources {
			if src.ID == id {
				return mediasource.PluginKind(m.ID, src, nil), src
			}
		}
	}
	t.Fatalf("the plugin contributes no source %q", id)
	return mediasource.Kind{}, plugins.MediaSource{}
}

// Options are what a test changes of how Kando reaches a source's server: Roots trusts a test's TLS server's certificate (nil for the system's roots), Offline is offline mode.
type Options struct {
	Roots   *x509.CertPool
	Offline bool
}

// Open opens a source of the plugin's kind kind as Kando does, the user having filled in config (its endpoint among them) and secrets in its form: a driver that asks the plugin's list, stat and open and reads files itself, whose kando.source.fetch goes to the endpoint with the secrets, by the rules of mediasource.Fetcher. Each source opened is one of its own to the plugin, by an id of its own.
func (p *Plugin) Open(t testing.TB, kind string, config, secrets map[string]string, o Options) *mediasource.PluginDriver {
	t.Helper()
	k, src := p.Kind(t, kind)
	endpoint, err := mediasource.ParseEndpoint(config[mediasource.EndpointKey])
	if err != nil {
		t.Fatal(err)
	}
	f := &mediasource.Fetcher{Endpoint: endpoint, Source: src, Config: config, Secrets: secrets, Pin: config[mediasource.PinKey], Offline: o.Offline, Roots: o.Roots, Log: func(string, ...any) {}}
	f.Clone() // makes the note of refused certificates its clones share
	p.mu.Lock()
	p.seq++
	id := fmt.Sprintf("s_%016x", p.seq)
	p.fetchers[id] = f
	p.mu.Unlock()
	d := &mediasource.PluginDriver{
		Call:    p.Call,
		Source:  mediasource.PluginSourceArg(k, id, mediasource.Source{ID: id, Kind: k.ID, Config: config}),
		Fetcher: f,
		Done: func() {
			p.mu.Lock()
			delete(p.fetchers, id)
			p.mu.Unlock()
		},
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// Call calls the plugin's export with args through Kando's runner, waiting 15 s at most as Kando does for a source's list, stat and open (callWait, longer under the race detector); a rejection with a code is a *mediasource.PluginError, as Kando makes it.
func (p *Plugin) Call(ctx context.Context, export string, args ...any) (any, error) {
	type answer struct {
		v   any
		err error
	}
	got := make(chan answer, 1)
	p.e.post(func() {
		p.r.Call(p.pl, export, args, func(v any, err error) { got <- answer{v, coded(err)} })
	})
	timer := time.NewTimer(callWait)
	defer timer.Stop()
	select {
	case r := <-got:
		return r.v, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, &mediasource.PluginError{Code: "timeout"}
	}
}

// coded is err as Kando gives a source's failure to the app: a rejection with a code as that code, anything else as failed.
func coded(err error) error {
	var c *core.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &c) && c.Code != "":
		return &mediasource.PluginError{Code: c.Code, Message: c.Message}
	}
	return &mediasource.PluginError{Code: "failed", Message: err.Error()}
}

// fetch answers kando.source.fetch as Kando does (the app's pluginsSourceFetch): the request of one of the plugin's sources open now, sent by its fetcher off the engine's goroutine, refused with denied, too-large, unavailable or timeout as Kando refuses it.
func (p *Plugin) fetch(c *core.HostCall) {
	source, r, ok := mediasource.PluginRequest(c.Data)
	if !ok {
		c.Reject(&core.Error{Code: "bad-request", Message: "bad-request"})
		return
	}
	p.mu.Lock()
	base := p.fetchers[source]
	p.sent = append(p.sent, Sent{Source: source, FetchRequest: r})
	p.mu.Unlock()
	if base == nil {
		c.Reject(&core.Error{Code: "denied", Message: "denied"})
		return
	}
	f := base.Clone()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		got, err := f.Fetch(ctx, r)
		p.e.post(func() {
			var cert *mediasource.CertError
			refuse := func(code string) { c.Reject(&core.Error{Code: code, Message: code}) }
			switch {
			case errors.Is(err, mediasource.ErrFetchDenied):
				refuse("denied")
			case errors.Is(err, mediasource.ErrFetchTooLarge):
				refuse("too-large")
			case errors.Is(err, media.ErrOffline), errors.As(err, &cert):
				refuse("unavailable")
			case errors.Is(err, context.DeadlineExceeded):
				refuse("timeout")
			case err != nil:
				refuse("unavailable")
			default:
				c.Resolve(mediasource.PluginReply(got))
			}
		})
	}()
}

// Sent are the requests the plugin sent through kando.source.fetch since the last Sent, as it sent them.
func (p *Plugin) Sent() []Sent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := slices.Clone(p.sent)
	p.sent = nil
	return out
}

// Closed reports whether Kando's runner stopped the plugin.
func (p *Plugin) Closed() bool {
	var closed bool
	p.e.do(func() { closed = p.pl.Closed() })
	return closed
}
