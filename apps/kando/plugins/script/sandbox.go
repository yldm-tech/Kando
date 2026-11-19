package script

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	"kando/apps/kando/plugins"

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
	return &l
}

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

// Load runs the plugin v under cx with what grant allows, through its own host, which must be no other plugin's (Sandbox), for a Runner to call. It notes what the Go heap held before, which the watchdog measures the plugin's growth from. Call it on the engine's goroutine; it runs the plugin's entry module, a second at most.
func Load(cx *core.Scope, v Verified, grant plugins.Grant, host *core.Host) (*Plugin, error) {
	heap := HeapBytes()
	p, err := tsx.Load(cx, v.Package, Sandbox(v.ID, v.Signer, grant, host))
	if err != nil {
		return nil, err
	}
	return &Plugin{ID: v.ID, Hash: v.Hash, p: p, heap: heap}, nil
}

// manifestExport is the function a plugin's code says what it is with.
const manifestExport = "kandoManifest"

// signedKeys are the keys of the manifest whose values kandoManifest returns, as the package's signature does not cover kando-plugin.json.
var signedKeys = []string{"capabilities", "optional", "contributes", "settings", "events"}

// Probe runs the plugin v under cx in a sandbox that grants nothing (probeSandbox), calls its kandoManifest and closes it, and checks that what it returned is what its manifest, as its file says it (Verified.Manifest), says of what it may reach and adds: capabilities, optional, contributes, settings and events, key for key, and nothing else; and that it exports every function m's commands call. Its entry module runs here before the user agreed to anything, which is why the sandbox grants nothing and its partition is thrown away. A plugin that does not export kandoManifest, or fails as it loads or says it, is ErrNoManifest; one that says something else plugins.ErrTampered. Call it on the engine's goroutine.
//
// TODO(nsgo#761): pkg.Contents has no app-defined signed entries; compare kandoManifest() instead https://github.com/nsgo-dev/nsgo/issues/761
func Probe(cx *core.Scope, v Verified) error {
	m := v.Manifest
	partition := probePartition()
	defer clearStorage(cx, partition)
	p, err := tsx.Load(cx, v.Package, probeSandbox(partition))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoManifest, err)
	}
	defer p.Close()
	exports := p.Exports()
	if m.Contributes != nil {
		for _, c := range m.Contributes.Commands {
			if !slices.Contains(exports, c.Export) {
				return fmt.Errorf("%w: it does not export %s", plugins.ErrTampered, c.Export)
			}
		}
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
		return compareManifest(r, m)
	}
	if !called {
		return fmt.Errorf("%w: kandoManifest does not return at once", ErrNoManifest)
	}
	return fmt.Errorf("%w: kandoManifest returns no object", ErrNoManifest)
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

// compareManifest checks that said, what kandoManifest returned, holds the signed keys of m, alike once both are read as JSON reads them and what is empty is left out, and no other key.
func compareManifest(said map[string]any, m plugins.Manifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var all map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for k := range said {
		if !slices.Contains(signedKeys, k) && normalize(said[k]) != nil {
			return fmt.Errorf("%w: kandoManifest says %s", plugins.ErrTampered, k)
		}
	}
	for _, k := range signedKeys {
		if !reflect.DeepEqual(normalize(said[k]), normalize(all[k])) {
			return fmt.Errorf("%w: kandoManifest's %s is not the manifest's", plugins.ErrTampered, k)
		}
	}
	return nil
}

// normalize is v as JSON decodes it, with empty arrays, empty objects and null left out of what holds them, so a key left out and one written empty are alike.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if n := normalize(e); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return v
}
