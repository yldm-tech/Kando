package script

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Limits are what every plugin Kando loads runs under: tsx.DefaultLimits, which keep it hardened (no eval, no Function, frozen prototypes) and bound its stack, nodes and the rest, with what a plugin of functions needs (docs/plugin-system.md 3.3). Its entry module has a second to run and each call into it half of one, so a plugin that computes for longer splits its work with await; the engine's goroutine, the window's, is never held longer by one piece. Every field is set from DefaultLimits, never written as a literal, since a field left zero is no limit at all and Harden's zero is off.
func Limits() *tsx.Limits {
	l := tsx.DefaultLimits
	l.Load = time.Second
	l.Call = 500 * time.Millisecond
	l.Timers = 64
	l.Requests = 4
	l.Body = 4 << 20
	l.Storage = 1 << 20
	l.Nodes = nodeQuota
	return &l
}

// nodeQuota is Limits' Nodes, tsx.DefaultLimits', which only the test of a plugin stopped at its quota lowers: reaching 200 000 elements takes the engine about a second, and on a busy machine some piece of it can run past Limits' half second, which tsx cuts without ending the call (nsgo#1018, see that test).
var nodeQuota = tsx.DefaultLimits.Nodes

// probeLimits are what a plugin runs under while Kando reads what it says of itself, before the user agreed to anything: tighter still, a node, a timer and a request at most (Nodes 1 rather than 0, which would be no limit) and 4 KiB of storage.
func probeLimits() *tsx.Limits {
	l := tsx.DefaultLimits
	l.Load = time.Second
	l.Call = 250 * time.Millisecond
	l.Nodes, l.Timers, l.Requests, l.Storage = 1, 1, 1, 4<<10
	return &l
}

// Partition is the plugin id's partition of the app's storage and secrets, under the release key signer that signed it: a plugin of the same id another key signed keeps none of what it kept (docs/plugin-system.md 6.3).
func Partition(id, signer string) string { return "plugin." + id + "." + signer }

// HostPartition is the partition of the app's secrets where Kando keeps the plugin id's secrets for it, which the plugin's own code cannot reach: its secrets reach Partition's alone, and it is never granted them.
func HostPartition(id, signer string) string { return "pluginhost." + id + "." + signer }

// Sandbox is the one sandbox a plugin runs in: Limits, its own host, the partition of its id and signer, and a grant made from nothing, its permissions and its HTTP rules alone, never a copy of what the package declares with parts taken out, so no capability the package signed (secrets, openUrl, a web view) is ever granted (docs/plugin-system.md 3.3). The grant is never nil: tsx reads a nil grant as everything the package declares.
func Sandbox(id, signer string, grant plugins.Grant, host *core.Host) tsx.Sandbox {
	g := &pkg.Capabilities{}
	for _, p := range grant.Perms {
		g.Host = append(g.Host, string(p))
	}
	if len(grant.Network) > 0 {
		g.Network = &pkg.Network{HTTP: slices.Clone(grant.Network)}
	}
	return tsx.Sandbox{Limits: Limits(), Grant: g, Host: host, Storage: Partition(id, signer)}
}

// probeSandbox is the sandbox Probe reads a plugin in: probeLimits, no host, an empty grant and a partition of its own, random, which Probe clears as it is done, as tsx would otherwise give the plugin its app ID's partition, which nothing clears.
func probeSandbox(partition string) tsx.Sandbox {
	return tsx.Sandbox{Limits: probeLimits(), Grant: &pkg.Capabilities{}, Storage: partition}
}

// Load runs the plugin v under cx with what grant allows, through its own host, which must be no other plugin's (Sandbox), for a Runner to call, and checks that it exports every function its contributions call (Manifest.ExportsNeeded), which Probe did not for a package that signed its manifest: plugins.ErrTampered otherwise. It notes what the Go heap held before, which the watchdog measures the plugin's growth from. Call it on the engine's goroutine; it runs the plugin's entry module, a second at most.
func Load(cx *core.Scope, v Verified, grant plugins.Grant, host *core.Host) (*Plugin, error) {
	heap := HeapBytes()
	p, err := tsx.Load(cx, v.Package, Sandbox(v.ID, v.Signer, grant, host))
	if err != nil {
		return nil, err
	}
	if err := checkExports(p, v.Manifest); err != nil {
		p.Close()
		return nil, err
	}
	return &Plugin{ID: v.ID, Hash: v.Hash, p: p, heap: heap}, nil
}

// manifestExport is the function a plugin's code says what it is with, when its package did not sign its manifest (signature.SignedManifest).
const manifestExport = "kandoManifest"

// Probe checks, before the plugin v runs with what the user granted, that what it may reach and adds is what its manifest, as its file says it (Verified.Manifest), says: capabilities, optional, contributes, credentials, settings and events, key for key, and nothing else (signature.CheckManifest). A package that signed its manifest (pkg.Contents.Extra) had it compared as it verified (Verify), so its code does not run here and nothing is left to check: the functions its contributions call are checked as it loads (Load). For one that did not, as every package nsgo publish signs, the signature does not cover kando-plugin.json, so Probe runs it under cx in a sandbox that grants nothing (probeSandbox), calls its kandoManifest, compares what it returns with the manifest and closes it, and checks that it exports every function its contributions call (Manifest.ExportsNeeded). Its entry module runs there before the user agreed to anything, which is why the sandbox grants nothing and its partition is thrown away. A plugin that does not export kandoManifest, or fails as it loads or says it, is ErrNoManifest; one that says something else plugins.ErrTampered. Call it on the engine's goroutine.
func Probe(cx *core.Scope, v Verified) error {
	if v.Package.Extra != nil {
		return nil
	}
	m := v.Manifest
	partition := probePartition()
	defer clearStorage(cx, partition)
	p, err := tsx.Load(cx, v.Package, probeSandbox(partition))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoManifest, err)
	}
	defer p.Close()
	if err := checkExports(p, m); err != nil {
		return err
	}
	var said any
	called := false
	p.Call(manifestExport, nil, func(result any, err error) {
		called = true
		said = result
		if err != nil {
			said = err
		}
	})
	switch r := said.(type) {
	case error:
		return fmt.Errorf("%w: %w", ErrNoManifest, r)
	case map[string]any:
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("%w: %w", plugins.ErrTampered, err)
		}
		return signature.CheckManifest(data, m)
	}
	if !called {
		return fmt.Errorf("%w: kandoManifest does not return at once", ErrNoManifest)
	}
	return fmt.Errorf("%w: kandoManifest returns no object", ErrNoManifest)
}

// checkExports checks that the plugin p exports every function the contributions of its manifest m call (Manifest.ExportsNeeded): plugins.ErrTampered otherwise.
func checkExports(p *tsx.Plugin, m plugins.Manifest) error {
	exports := p.Exports()
	for _, e := range m.ExportsNeeded() {
		if !slices.Contains(exports, e) {
			return fmt.Errorf("%w: it does not export %s", plugins.ErrTampered, e)
		}
	}
	return nil
}

// probePartition names the partition of a probe: pluginprobe. and 16 random hex digits.
var probePartition = func() string {
	var b [8]byte
	rand.Read(b[:])
	return "pluginprobe." + hex.EncodeToString(b[:])
}

// clearStorage clears the storage partition name.
func clearStorage(cx *core.Scope, name string) {
	inner := cx.Child()
	inner.Run(func() {
		inner.ProvideStorage(name, 0)
		inner.Storage().Clear()
	})
	inner.Dispose()
}

// ClearStorage clears the plugin id's storage partition of signer, as it is uninstalled or another key signs it. Call it on the engine's goroutine.
func ClearStorage(cx *core.Scope, id, signer string) { clearStorage(cx, Partition(id, signer)) }
