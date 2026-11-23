package script

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// TestScriptVerify checks what Verify takes and refuses: a plugin of the official root, one whose certificate covers another publisher or every publisher, one under kando. that another root signed, one whose manifest asks for more than its package signed, one revoked, one older than a build accepted, and one of a root not trusted.
func TestScriptVerify(t *testing.T) {
	official := newSigner(t, AppIDPrefix+"kando.*")
	roots := official.roots()
	trust := trustOf(t, roots)
	p := official.pack(t, fixture{id: "kando.anki"})
	v, err := Verify(trust, roots, p)
	if err != nil || v.Source != plugins.SourceOfficial || v.Signer != pkg.KeyID(official.release.Public().(ed25519Public)) || v.Hash != p.Hash {
		t.Fatalf("verified %+v, %v", v, err)
	}

	// The official root's certificate for another publisher makes its plugins verified, not Kando's own; one of its certificates scoped to acme.* signs no kando. id.
	acme := certified(t, official, AppIDPrefix+"acme.*")
	if v, err := Verify(trust, roots, acme.pack(t, fixture{id: "acme.anki"})); err != nil || v.Source != plugins.SourceVerified {
		t.Errorf("another publisher under the official root: %+v, %v", v, err)
	}
	// tsx/pkg will not sign a kando. id under a certificate for acme.* alone (Build checks what the certificate covers), so the closest is a certificate for acme.* that names the kando. id itself: it is not scoped to kando.*, and is refused.
	named := certified(t, official, AppIDPrefix+"acme.*", AppIDPrefix+"kando.other")
	_, err = Verify(trust, roots, named.pack(t, fixture{id: "kando.other"}))
	wantErr(t, "a kando. id under a certificate for acme.*", err, ErrScope)

	dev := newSigner(t, AppIDPrefix+"acme.*")
	both := Roots{Official: official.public(), Dev: dev.public()}
	if v, err := Verify(trustOf(t, both), both, dev.pack(t, fixture{id: "acme.tool"})); err != nil || v.Source != plugins.SourceDeveloper {
		t.Errorf("a developer plugin: %+v, %v", v, err)
	}
	wide := newSigner(t, AppIDPrefix+"*")
	wideRoots := Roots{Official: official.public(), Dev: wide.public()}
	_, err = Verify(trustOf(t, wideRoots), wideRoots, wide.pack(t, fixture{id: "acme.tool"}))
	wantErr(t, "a certificate of every publisher", err, ErrScope)
	kando := newSigner(t, AppIDPrefix+"kando.*")
	kandoRoots := Roots{Official: official.public(), Dev: kando.public()}
	_, err = Verify(trustOf(t, kandoRoots), kandoRoots, kando.pack(t, fixture{id: "kando.anki"}))
	wantErr(t, "kando. by a developer", err, ErrScope)

	more := official.pack(t, fixture{id: "kando.anki", caps: `{"host": ["kando.toast", "kando.saved.read"]}`, signed: &pkg.Capabilities{Host: []string{"kando.toast"}}})
	_, err = Verify(trust, roots, more)
	wantErr(t, "a manifest asking for more", err, plugins.ErrTampered)
	extra := official.pack(t, fixture{id: "kando.anki", signed: &pkg.Capabilities{Host: []string{"kando.toast"}, Clipboard: true}})
	_, err = Verify(trust, roots, extra)
	wantErr(t, "a package signing the clipboard", err, plugins.ErrBadCapabilities)

	revoked := official.pack(t, fixture{id: "kando.revoked"})
	list, err := pkg.Revoke(official.root, 1, time.Now().Add(time.Hour), nil, []string{revoked.Script.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	rtrust := trustOf(t, roots)
	if err := rtrust.UpdateRevocations(list); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(rtrust, roots, revoked)
	wantErr(t, "revoked", err, ErrRevoked)

	newer := official.pack(t, fixture{id: "kando.anki", version: "2.0.0", build: 5})
	nv, err := Verify(trust, roots, newer)
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Accept(nv.Package); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(trust, roots, official.pack(t, fixture{id: "kando.anki", build: 3}))
	wantErr(t, "an older build", err, ErrRollback)

	stranger := newSigner(t, AppIDPrefix+"kando.*")
	_, err = Verify(trust, roots, stranger.pack(t, fixture{id: "kando.anki", build: 9}))
	wantErr(t, "a root not trusted", err, ErrUntrusted)
}

// TestScriptProbe checks the sandbox that reads what a plugin's code says of itself: it takes a plugin that says what its manifest says, and refuses one that says something else, one that says nothing, and one that does not export a command's function; what the plugin stores as it loads is gone after, in its probe's partition and its app ID's.
func TestScriptProbe(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	probe := func(f fixture) error {
		t.Helper()
		p := s.pack(t, f)
		v, err := Verify(trust, roots, p)
		if err != nil {
			t.Fatal(err)
		}
		e.do(func() { err = Probe(e.root, v) })
		return err
	}
	commands := `"contributes": {"commands": [{"id": "sync", "export": "run", "title": {"en": "Sync"}}]}, "events": ["film.opened"], "optional": ["kando.saved.read"], "settings": [{"key": "deck", "type": "text", "default": "Kando", "title": {"en": "Deck"}}]`
	if err := probe(fixture{id: "acme.tool", extra: commands, code: "export function run() { return 1 }"}); err != nil {
		t.Errorf("a plugin that says what it is: %v", err)
	}
	wantErr(t, "another event", probe(fixture{id: "acme.tool", extra: commands, code: "export function run() {}",
		said: `{"capabilities": {"host": ["kando.toast"]}, ` + strings.Replace(commands, "film.opened", "job.completed", 1) + `}`}), plugins.ErrTampered)
	wantErr(t, "another key", probe(fixture{id: "acme.tool", said: `{"capabilities": {"host": ["kando.toast"]}, "credentials": [{"id": "x"}]}`}), plugins.ErrTampered)
	wantErr(t, "no export", probe(fixture{id: "acme.tool", extra: commands}), plugins.ErrTampered)
	wantErr(t, "nothing said", probe(fixture{id: "acme.tool", said: `undefined`}), ErrNoManifest)
	wantErr(t, "a throw as it loads", probe(fixture{id: "acme.tool", code: `throw new Error("no")`}), ErrNoManifest)

	// What the plugin stores as it loads goes with its probe: its partition is cleared, and the partition of its app ID, which tsx would give it without one, holds nothing.
	random := probePartition
	probePartition = func() string { return "pluginprobe.test" }
	t.Cleanup(func() { probePartition = random })
	if err := probe(fixture{id: "acme.tool", code: `import { Storage } from "nsgo"; Storage.setItem("k", "v");`}); err != nil {
		t.Fatal(err)
	}
	for _, partition := range []string{"pluginprobe.test", AppIDPrefix + "acme.tool"} {
		var keys []string
		e.do(func() {
			inner := e.root.Child()
			inner.Run(func() {
				inner.ProvideStorage(partition, 0)
				keys = inner.Storage().Keys()
			})
			inner.Dispose()
		})
		if len(keys) != 0 {
			t.Errorf("partition %s holds %q", partition, keys)
		}
	}
}

// TestScriptSignedManifest checks a package that signed its manifest (pkg.Contents.Extra): Verify compares what it signed with the manifest, so for a manifest whose every Kando compares it Probe runs none of its code, whose kandoManifest need not say what it is, while for one that runs on a Kando that still asks the code Probe asks it, and Load checks the functions its contributions call; a manifest that is not what it signed, a signed entry with another key or that is no object, and a signed entry changed after signing are refused. A package that did not sign it, as every one published, is the kandoManifest's of TestScriptProbe.
func TestScriptSignedManifest(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	commands := `"contributes": {"commands": [{"id": "sync", "export": "run", "title": {"en": "Sync"}}]}, "optional": ["kando.saved.read"]`
	verify := func(f fixture) (Verified, error) {
		t.Helper()
		return Verify(trust, roots, s.pack(t, f))
	}

	// Its code throws as it loads and says nothing of itself: Probe never runs it, as every Kando its manifest runs on compares what it signed.
	v, err := verify(fixture{id: "acme.tool", extra: commands, signManifest: true, kando: ">=" + signature.ManifestComparedSince, said: "undefined", code: `throw new Error("ran before the user agreed")`})
	if err != nil {
		t.Fatal(err)
	}
	if v.Package.Extra == nil {
		t.Fatal("the package signed no manifest")
	}
	e.do(func() { err = Probe(e.root, v) })
	if err != nil {
		t.Errorf("a package that signed its manifest: %v", err)
	}
	v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true, kando: ">=" + signature.ManifestComparedSince, said: "undefined", code: "export function run() { return 1 }"})
	if err != nil {
		t.Fatal(err)
	}
	var pl *Plugin
	e.do(func() {
		if err = Probe(e.root, v); err == nil {
			pl, err = Load(e.root, v, plugins.Grant{}, core.NewHost())
		}
	})
	if err != nil || pl == nil {
		t.Fatalf("a package that signed its manifest whose kandoManifest says nothing: %v", err)
	}
	e.do(func() { pl.p.Close() })

	// The same package for a manifest that runs on a Kando that takes the signed manifest but still asks the code (signature.ComparesManifest): Probe asks it as that Kando does, so a kandoManifest that says nothing fails here, and one that says it passes.
	for _, kando := range []string{">=" + signature.ManifestEntrySince, ""} {
		v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true, kando: kando, said: "undefined", code: "export function run() { return 1 }"})
		if err != nil {
			t.Fatal(err)
		}
		e.do(func() { err = Probe(e.root, v) })
		wantErr(t, "kando "+kando+", a kandoManifest that says nothing", err, ErrNoManifest)
	}
	v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true, kando: ">=" + signature.ManifestEntrySince, code: "export function run() { return 1 }"})
	if err != nil {
		t.Fatal(err)
	}
	e.do(func() { err = Probe(e.root, v) })
	if err != nil {
		t.Errorf("kando >=%s, a kandoManifest that says it: %v", signature.ManifestEntrySince, err)
	}

	// A command whose function it does not export is refused as it loads, as Probe did not run it.
	v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true, kando: ">=" + signature.ManifestComparedSince})
	if err != nil {
		t.Fatal(err)
	}
	e.do(func() {
		if err = Probe(e.root, v); err == nil {
			_, err = Load(e.root, v, plugins.Grant{}, core.NewHost())
		}
	})
	wantErr(t, "no export", err, plugins.ErrTampered)

	// The manifest says more than the package signed of it, or the package signed another key or no object.
	_, err = verify(fixture{id: "acme.tool", extra: commands, entry: `{"capabilities": {"host": ["kando.toast"]}}`, code: "export function run() {}"})
	wantErr(t, "a manifest that is not the one signed", err, plugins.ErrTampered)
	_, err = verify(fixture{id: "acme.tool", entry: `{"capabilities": {"host": ["kando.toast"]}, "panels": [{"id": "x"}]}`})
	wantErr(t, "another key signed", err, plugins.ErrTampered)
	_, err = verify(fixture{id: "acme.tool", entry: `["capabilities"]`})
	wantErr(t, "no object signed", err, plugins.ErrTampered)

	// A byte of the signed manifest changed after signing breaks the signature, as a byte of the code does.
	p := s.pack(t, fixture{id: "acme.tool", signManifest: true})
	path := filepath.Join(p.Dir, p.Script.Package)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.LastIndex(string(data), "kando.toast")
	if i < 0 {
		t.Fatal("the package holds no signed manifest")
	}
	copy(data[i:], "kando.saved")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(trust, roots, p)
	wantErr(t, "a signed manifest changed", err, ErrUntrusted)
}

// TestScriptLimitsAreBounded checks that every Limits Kando runs a plugin under is hardened and bounds the stack, nodes, timers, requests, bodies and storage: a zero field would be no limit at all.
func TestScriptLimitsAreBounded(t *testing.T) {
	for name, l := range map[string]any{"run": *Limits(), "probe": *probeLimits(), "sandbox": *Sandbox("a.b", "k", plugins.Grant{}, nil, nil, nil).Limits, "probe sandbox": *probeSandbox("p").Limits} {
		v := reflect.ValueOf(l)
		if !v.FieldByName("Harden").Bool() {
			t.Errorf("%s: not hardened", name)
		}
		for _, f := range []string{"Load", "Call", "Stack", "Nodes", "Timers", "Requests", "Body", "Storage", "Memory"} {
			if v.FieldByName(f).Int() <= 0 {
				t.Errorf("%s: %s is %v", name, f, v.FieldByName(f))
			}
		}
	}
}

// TestScriptGrantIsMadeFromNothing checks that the sandbox's grant holds nothing but the permissions and HTTP rules granted, whatever pkg.Capabilities may hold, is never nil, and that each sandbox is in a partition of its id and signer.
func TestScriptGrantIsMadeFromNothing(t *testing.T) {
	grant := plugins.Grant{Perms: []plugins.Perm{plugins.PermToast}, Network: []pkg.HTTPRule{{Host: "127.0.0.1", Methods: []string{"POST"}, Paths: []string{"/"}}}}
	for name, s := range map[string]any{"granted": Sandbox("acme.tool", "k1", grant, nil, nil, nil), "nothing": Sandbox("acme.tool", "k1", plugins.Grant{}, nil, nil, nil), "probe": probeSandbox("p")} {
		sb := reflect.ValueOf(s)
		if sb.FieldByName("Capabilities").IsNil() == false {
			t.Errorf("%s: Capabilities set", name)
		}
		g := sb.FieldByName("Grant")
		if g.IsNil() {
			t.Fatalf("%s: a nil grant, which tsx reads as everything declared", name)
		}
		c := g.Elem()
		for i := range c.NumField() {
			f := c.Type().Field(i).Name
			if f != "Host" && f != "Network" && !c.Field(i).IsZero() {
				t.Errorf("%s: the grant holds %s", name, f)
			}
		}
	}
	if s := Sandbox("acme.tool", "k1", grant, nil, nil, nil); s.Storage != "plugin.acme.tool.k1" || len(s.Grant.Host) != 1 || len(s.Grant.Network.HTTP) != 1 {
		t.Errorf("sandbox %+v", s)
	}
}

// TestScriptRegexpIsBounded checks that a backtracking pattern, which would hold the engine for about twenty minutes at n=32, fails within its call's budget: tsx gives the regular expressions a sandboxed call compiles the call's budget as their match timeout (Limits' Call, half a second), and regexp2 reads its clock every 100ms, so the match fails within a second.
func TestScriptRegexpIsBounded(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.re", code: `export function re(n) { return /(a+)+\1$/.test("a".repeat(n) + "!") }`}), plugins.Grant{})
	r := &Runner{Post: e.post, Watch: DefaultWatch}
	start := time.Now()
	got := wait(t, e.call(r, pl, "re", 32), 30*time.Second)
	if got.err == nil {
		t.Fatalf("the match ended: %v", got.v)
	}
	if d := got.at.Sub(start); d > slow(time.Second) {
		t.Errorf("the match failed after %v", d)
	}
}

// TestScriptWatchdogStopsASlicedLoop checks that a call that keeps the engine busy in pieces under the call budget, awaiting a timer between them, which tsx never cuts, is stopped by the watchdog's heartbeats within the pieces they take, long before its Deadline, and reported slow.
func TestScriptWatchdogStopsASlicedLoop(t *testing.T) {
	// piece is how long the loop keeps the engine busy at a time: well under tsx's call budget of half a second, and longer than Stall and a Beat, so the heartbeat posted at most a Beat into each piece waits past Stall and counts. The budget is wall time (tsx/sandbox.go within, a time.AfterFunc), and a piece that a loaded runner holds past it is cut by tsx, which ends the call with ErrBudget (nsgo#1018) before the watchdog stops it, as a macOS runner once held pieces of 430 ms of DefaultWatch's Beat of 100 ms and Stall of 250 ms. So the watchdog here runs at half that scale, with the same Stalls, Heap and Deadline: a heartbeat every 50 ms that counts past 125 ms, which pieces of 265 ms hold with 90 ms to spare for a late tick (DefaultWatch's 430 ms pieces had 80) while staying 235 ms under the budget rather than 70.
	const piece = 265 * time.Millisecond
	w := DefaultWatch
	w.Beat, w.Stall = 50*time.Millisecond, 125*time.Millisecond
	if piece-w.Beat <= w.Stall {
		t.Fatalf("a piece of %v does not hold a heartbeat past Stall %v", piece, w.Stall)
	}
	if budget := Limits().Call; budget-piece < 200*time.Millisecond {
		t.Fatalf("a piece of %v is not well under the call budget of %v", piece, budget)
	}
	// Each heartbeat that counts takes a piece, and Stalls in a row stop the loop once the last one is past Stall, the watchdog interrupting the piece it waits in (tsx.Plugin.Interrupt) rather than waiting for its end. A heartbeat that falls due just as a piece ends can be posted after the one before it ran, find the engine between two pieces and start the count again, as it should, the window having answered it; a busy runner that delivers the tick late makes that likelier. The bound allows one such restart, at worst just before the last stall would have counted, so 2*Stalls-1 pieces, each with a Beat for the gaps between pieces and a late tick. The pieces end 15, 30, 45, 10 and 25 ms past a tick (265 ms rather than a multiple of 10, which would end the fifth piece on one), so a restart takes a tick at least 10 ms late.
	bound := time.Duration(2*w.Stalls-1) * (piece + w.Beat)
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.spin", code: fmt.Sprintf(`export async function spin() {
  for (;;) { const t = Date.now(); while (Date.now() - t < %d) {} await new Promise(r => setTimeout(r, 0)) }
}`, piece.Milliseconds())}), plugins.Grant{})
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: w, Stopped: func(id, hash, reason string) { stopped <- reason }}
	start := time.Now()
	got := wait(t, e.call(r, pl, "spin"), 30*time.Second)
	if !errors.Is(got.err, ErrStopped) {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	if d := got.at.Sub(start); d > slow(bound) {
		t.Errorf("stopped after %v, past %v", d, slow(bound))
	}
	if reason := <-stopped; reason != plugins.ReasonSlow {
		t.Errorf("reported %q", reason)
	}
	var closed bool
	e.do(func() { closed = pl.Closed() })
	if !closed {
		t.Error("not closed")
	}
}

// TestScriptWatchdogCutsTheRunningPiece checks that the watchdog, once it trips inside one long piece of synchronous JS, cuts that piece at once (tsx.Plugin.Interrupt) rather than waiting for it to end, as closing the plugin between two pieces would: the call ends with ErrStopped, reported slow, well before the piece's 450 ms. The piece runs on the wall clock and stays under tsx's half second call budget, so tsx does not cut it first; the bound is not widened under the race detector, which slows neither the piece nor the interrupt that ends it.
func TestScriptWatchdogCutsTheRunningPiece(t *testing.T) {
	const piece = 450 * time.Millisecond
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.spin", code: fmt.Sprintf(`export async function spin() { const t = Date.now(); while (Date.now() - t < %d) {} }`, piece.Milliseconds())}), plugins.Grant{})
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: Watch{Beat: 20 * time.Millisecond, Stall: time.Hour, Stalls: 3, Heap: DefaultWatch.Heap, Deadline: 100 * time.Millisecond}, Stopped: func(id, hash, reason string) { stopped <- reason }}
	start := time.Now()
	got := wait(t, e.call(r, pl, "spin"), 10*time.Second)
	if !errors.Is(got.err, ErrStopped) {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	if d := got.at.Sub(start); d >= 300*time.Millisecond {
		t.Errorf("the call ended after %v, not before its %v piece", d, piece)
	}
	if reason := <-stopped; reason != plugins.ReasonSlow {
		t.Errorf("reported %q", reason)
	}
}

// TestScriptWatchdogStopsGrowingMemory checks that a call that keeps what it makes, in pieces under the budget, is stopped once the heap grew past the threshold, within two seconds, and reported for its memory; and that a plugin whose module grows a little at each short call is stopped before its next call once it is past it.
func TestScriptWatchdogStopsGrowingMemory(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	watch := DefaultWatch
	watch.Heap = 128 << 20
	// The memory check is what this test is about: the collections of a heap this large, slower still under the race detector on a busy runner, can hold the engine's goroutine past three heartbeats, which would stop the plugin as slow first. TestScriptWatchdogStopsASlicedLoop covers the heartbeats.
	watch.Stall = time.Hour
	stopped := make(chan string, 4)
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	// A piece has to stay well under the call budget (tsx's half second), or tsx cuts it and the call fails before the watchdog sees the heap: a slow Windows runner took more than that for 100000 objects. Fewer still under the race detector, which slows each one down.
	piece := 20000
	if raceOn {
		piece = 10000
	}
	hog := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.hog", code: fmt.Sprintf(`export async function hog() {
  const keep = []
  for (;;) { for (let i = 0; i < %d; i++) keep.push({ i, s: "x" + i }); await new Promise(r => setTimeout(r, 0)) }
}`, piece)}), plugins.Grant{})
	start := time.Now()
	got := wait(t, e.call(r, hog, "hog"), 60*time.Second)
	if !errors.Is(got.err, ErrStopped) {
		t.Fatalf("the call ended with %v", got.err)
	}
	if d := got.at.Sub(start); d > slow(2*time.Second) {
		t.Errorf("stopped after %v", d)
	}
	if reason := <-stopped; reason != plugins.ReasonMemory {
		t.Errorf("reported %q", reason)
	}

	grow := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.grow", code: fmt.Sprintf(`const keep = []
export function grow() { for (let i = 0; i < %d; i++) keep.push({ i, s: "y" + i }); return keep.length }`, piece/2)}), plugins.Grant{})
	var err error
	for i := 0; i < 4000 && !errors.Is(err, ErrStopped); i++ {
		err = wait(t, e.call(r, grow, "grow"), 30*time.Second).err
	}
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("the growing module was never stopped: %v", err)
	}
	if reason := <-stopped; reason != plugins.ReasonMemory {
		t.Errorf("reported %q", reason)
	}
}

// TestScriptRunnerMarksCallsInFlight checks that a call is marked in flight while it runs, and its mark is gone once it ends, by its end or by the plugin's closing.
func TestScriptRunnerMarksCallsInFlight(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.hang", code: `export function hang() { return new Promise(() => {}) }
export function quick() { return 2 }`}), plugins.Grant{})
	root := t.TempDir()
	r := &Runner{Post: e.post, Root: root, Watch: DefaultWatch}
	hung := e.call(r, pl, "hang")
	queued := e.call(r, pl, "quick")
	var busy bool
	e.do(func() { busy = r.Busy() })
	if _, err := os.Stat(filepath.Join(root, ".inflight", "acme.hang")); err != nil || !busy {
		t.Fatalf("no mark while the call runs: %v, busy %t", err, busy)
	}
	e.do(func() { r.Close(pl) })
	if got := wait(t, hung, time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("the hung call ended with %v", got.err)
	}
	if got := wait(t, queued, time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("the queued call ended with %v", got.err)
	}
	if marks := plugins.InFlight(root); len(marks) != 0 {
		t.Errorf("marks left: %v", marks)
	}
}

// TestScriptBreaker checks that a plugin whose calls failed three times is stopped, and that a failure it reports on purpose, with a code, does not count, whether an async export rejects with it or a synchronous one throws it.
func TestScriptBreaker(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.fail", code: `export function fail() { throw new Error("broken") }
export async function busy() { const e = new Error("Anki is not open"); e.code = "unreachable"; throw e }
export function busyNow() { const e = new Error("Anki is not open"); e.code = "unreachable"; throw e }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: DefaultWatch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	for i := range 6 {
		name := "busy"
		if i%2 == 1 {
			name = "busyNow"
		}
		var coded *core.Error
		if got := wait(t, e.call(r, pl, name), time.Second); !errors.As(got.err, &coded) || coded.Code != "unreachable" || coded.Message != "Anki is not open" {
			t.Fatalf("%s: a coded failure: %v", name, got.err)
		}
	}
	for i := range 3 {
		if got := wait(t, e.call(r, pl, "fail"), time.Second); got.err == nil || errors.Is(got.err, ErrStopped) {
			t.Fatalf("call %d: %v", i, got.err)
		}
	}
	if reason := <-stopped; reason != plugins.ReasonFailed {
		t.Errorf("reported %q", reason)
	}
	if got := wait(t, e.call(r, pl, "busy"), time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("a call after it was stopped: %v", got.err)
	}
}

// TestScriptRunnerEndsACallCutAfterAnAwait checks that a call whose piece after an await runs past Limits' half second ends with tsx.ErrBudget soon after it, as one cut before its first await does, rather than waiting for the watchdog's Deadline, a minute (nsgo#1018), and that the runner goes on to the next call.
func TestScriptRunnerEndsACallCutAfterAnAwait(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.late", code: `export async function late() {
  await new Promise(r => setTimeout(r, 0))
  const t = Date.now(); while (Date.now() - t < 2000) {}
  return 1
}
export function ping() { return 1 }`}), plugins.Grant{})
	watch := DefaultWatch
	// tsx's budget is what this test is about: the heartbeats must not stop the plugin first.
	watch.Stall = time.Hour
	r := &Runner{Post: e.post, Watch: watch}
	start := time.Now()
	got := wait(t, e.call(r, pl, "late"), 30*time.Second)
	if !errors.Is(got.err, tsx.ErrBudget) {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	// The piece would run 2 s; the budget cuts it at half a second.
	if d := got.at.Sub(start); d > slow(1500*time.Millisecond) {
		t.Errorf("the call ended after %v", d)
	}
	if got := wait(t, e.call(r, pl, "ping"), time.Second); got.err != nil {
		t.Errorf("a call after: %v", got.err)
	}
}

// TestScriptRunnerStopsAPluginTsxStopped checks that a plugin tsx stopped at its node quota, whose every call after fails at once with the quota's coded error, which the breaker does not count, is stopped and reported as failed by the call that ran into the quota, and that the calls after it end with ErrStopped.
func TestScriptRunnerStopsAPluginTsxStopped(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	if nodeQuota != tsx.DefaultLimits.Nodes || Limits().Nodes != nodeQuota {
		t.Fatalf("plugins run under a quota of %d nodes, Limits gives %d, not tsx.DefaultLimits' %d", nodeQuota, Limits().Nodes, tsx.DefaultLimits.Nodes)
	}
	// The plugin is held to 2000 nodes rather than tsx.DefaultLimits' 200 000, which hold about 3.7 GiB of the Go heap and took a shared CI runner up to a minute to make, the Go heap still holding them as the next test starts. Here the third piece runs into the quota, so only three pieces of 1000 elements have to stay under Limits' half second: one past it would end the call with ErrBudget (nsgo#1018), rather than with the quota's error.
	quota := nodeQuota
	nodeQuota = 2000
	t.Cleanup(func() { nodeQuota = quota })
	// The elements stay counted for as long as the plugin runs, so pieces well under the call budget reach the quota.
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.flood", code: `import { View } from "nsgo"
const keep = []
export async function flood() {
  for (;;) { for (let i = 0; i < 1000; i++) keep.push(<View />); await new Promise(r => setTimeout(r, 0)) }
}
export function ping() { return 1 }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	watch := DefaultWatch
	// The quota is what this test is about: the watchdog must not stop it first, as slow on a busy runner or one under the race detector, or for the heap the plugin takes.
	watch.Stall, watch.Heap = time.Hour, 0
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	got := wait(t, e.call(r, pl, "flood"), 60*time.Second)
	if core.ErrorCode(got.err) != core.CodeQuotaExceeded {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonFailed {
			t.Errorf("reported %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the plugin tsx stopped was not stopped")
	}
	var closed bool
	e.do(func() { closed = pl.Closed() })
	if !closed {
		t.Error("not closed")
	}
	if got := wait(t, e.call(r, pl, "ping"), time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("a call after it was stopped: %v", got.err)
	}
}

// TestScriptRunnerStopsAPluginAtItsMemoryQuota checks that a plugin tsx stopped at its memory quota (Limits.Memory) is stopped and reported for its memory by the call that ran into it, before the watchdog's heap threshold, and that the calls after it end with ErrStopped.
func TestScriptRunnerStopsAPluginAtItsMemoryQuota(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	// One string past the quota, which tsx checks before it allocates: a loop of smaller ones ran past the call's budget under the race detector before it reached the quota.
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.big", code: `export function big() { return "x".repeat(128 << 20).length }
export function ping() { return 1 }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	watch := DefaultWatch
	// The quota is what this test is about: the watchdog must not stop it first, as slow on a busy runner, or for the heap the plugin takes.
	watch.Stall, watch.Heap = time.Hour, 0
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	var coded *core.Error
	if got := wait(t, e.call(r, pl, "big"), 30*time.Second); !errors.As(got.err, &coded) || coded.Code != core.CodeQuotaExceeded || coded.Resource != "memory" {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonMemory {
			t.Errorf("reported %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the plugin tsx stopped was not stopped")
	}
	if got := wait(t, e.call(r, pl, "ping"), time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("a call after it was stopped: %v", got.err)
	}
}

// TestScriptRunnerStopsAPluginTsxStoppedBetweenCalls checks that a plugin tsx stopped between its calls, at its memory quota in a timer a call set going, is stopped and reported for its memory as tsx stops it (tsx.Sandbox.OnStop), not only once a later call fails.
func TestScriptRunnerStopsAPluginTsxStoppedBetweenCalls(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.later", code: `const kept = []
export function later() { setTimeout(() => { kept.push("x".repeat(128 << 20)) }, 20); return 1 }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	watch := DefaultWatch
	// The quota is what this test is about: the watchdog must not stop it first.
	watch.Stall, watch.Heap = time.Hour, 0
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	if got := wait(t, e.call(r, pl, "later"), 5*time.Second); got.err != nil {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonMemory {
			t.Errorf("reported %q", reason)
		}
	case <-time.After(slow(5 * time.Second)):
		t.Fatal("the plugin tsx stopped in its timer was not stopped")
	}
	var closed, busy bool
	e.do(func() { closed, busy = pl.Closed(), r.Busy() })
	if !closed || busy {
		t.Errorf("closed %t, a call under way %t", closed, busy)
	}
}

// TestScriptRunnerSparesAPluginThatReportsAQuota checks that a plugin that rejects with a quota code of its own, as for a host's quota, is not taken for one tsx stopped.
func TestScriptRunnerSparesAPluginThatReportsAQuota(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.quota", code: `export async function full() { const e = new Error("used up"); e.code = "ERR_NSH_QUOTA_EXCEEDED"; throw e }
export function ping() { return 1 }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: DefaultWatch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	for range 5 {
		if got := wait(t, e.call(r, pl, "full"), time.Second); core.ErrorCode(got.err) != core.CodeQuotaExceeded {
			t.Fatalf("a coded failure: %v", got.err)
		}
	}
	if got := wait(t, e.call(r, pl, "ping"), time.Second); got.err != nil {
		t.Errorf("a call after: %v", got.err)
	}
	select {
	case reason := <-stopped:
		t.Fatalf("stopped for %s", reason)
	default:
	}
}

// TestScriptRunnerRunsNoJSOnAFailure checks that the runner, as it tells whether tsx stopped a plugin whose call failed, runs none of the plugin's JS: a plugin can put a getter on its own exports object, which reading a name of it runs on the engine's goroutine, outside any call's budget or watchdog, where a throw would take the engine down and a loop would hang it. The plugin makes its exports object's prototype a Proxy that throws on any name it does not own, so a lookup of any name it does not export runs its JS, and puts a throwing enumerable getter on it, so an enumeration of its exports does too.
func TestScriptRunnerRunsNoJSOnAFailure(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.trap", code: `export async function trap() {
  const exports = globalThis.__nsgo_exports
  Object.defineProperty(exports, "trap2", { get() { throw new Error("ran") }, enumerable: true })
  Object.setPrototypeOf(exports, new Proxy(Object.getPrototypeOf(exports) ?? Object.create(null), { get(target, k, receiver) { if (typeof k === "string") throw new Error("ran " + k); return Reflect.get(target, k, receiver) } }))
  throw new Error("broken")
}
export function ping() { return 1 }`}), plugins.Grant{})
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: DefaultWatch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	if got := wait(t, e.call(r, pl, "trap"), time.Second); !contains(got.err, "broken") {
		t.Fatalf("the call ended with %v", got.err)
	}
	if got := wait(t, e.call(r, pl, "ping"), time.Second); got.err != nil {
		t.Errorf("a call after: %v", got.err)
	}
	select {
	case reason := <-stopped:
		t.Fatalf("stopped for %s", reason)
	default:
	}
}

// TestScriptPackFolder checks that a developer's folder is compiled and signed with the developer key into a plugin that Stage, Verify and Probe take, as a developer plugin.
func TestScriptPackFolder(t *testing.T) {
	secret, public, err := NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, pub, err := DevKey(secret)
	if err != nil || !pub.Equal(public) {
		t.Fatal(err)
	}
	src := t.TempDir()
	fields := `"capabilities": {"host": ["kando.toast"]}, "contributes": {"commands": [{"id": "hi", "export": "hi", "title": {"en": "Say hi"}}]}`
	manifest := `{"schema": 2, "id": "acme.hello", "kind": "script", "version": "0.1.0", "license": "MIT", "name": {"en": "Hello"}, ` + fields + `}`
	code := fmt.Sprintf("export function kandoManifest() { return {%s} }\nexport function hi() { return \"hi\" }\n", fields)
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(src, Entry), []byte(code), 0o644)
	dst := t.TempDir()
	if err := PackFolder(src, dst, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	staged, p, err := plugins.Stage(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)
	roots := Roots{Official: officialRoot, Dev: public}
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil || v.Source != plugins.SourceDeveloper {
		t.Fatalf("verified %+v, %v", v, err)
	}
	e := newEngine(t)
	e.do(func() { err = Probe(e.root, v) })
	if err != nil {
		t.Fatal(err)
	}
	if v.Package.Extra != nil {
		t.Errorf("a manifest that runs on every Kando signed: %s", v.Package.Extra)
	}

	// A manifest no Kando it runs on refuses a signed manifest for is signed as nsgo publish -extra signs it, and Probe still asks its code, as the oldest Kando it runs on does.
	os.WriteFile(filepath.Join(src, plugins.ManifestName), []byte(strings.Replace(manifest, `"license"`, `"kando": ">=`+signature.ManifestEntrySince+`", "license"`, 1)), 0o644)
	dst = t.TempDir()
	if err := PackFolder(src, dst, key); err != nil {
		t.Fatal(err)
	}
	staged, p, err = plugins.Stage(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)
	if v, err = Verify(trustOf(t, roots), roots, p); err != nil {
		t.Fatal(err)
	}
	if v.Package.Extra == nil {
		t.Error("a manifest for Kando " + signature.ManifestEntrySince + " and newer signed none")
	}
	e.do(func() { err = Probe(e.root, v) })
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, Entry), []byte("export function hi() { return \"hi\" }\n"), 0o644)
	dst = t.TempDir()
	if err := PackFolder(src, dst, key); err != nil {
		t.Fatal(err)
	}
	staged, p, err = plugins.Stage(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staged)
	if v, err = Verify(trustOf(t, roots), roots, p); err != nil {
		t.Fatal(err)
	}
	e.do(func() { err = Probe(e.root, v) })
	wantErr(t, "a signed manifest without kandoManifest for Kando "+signature.ManifestEntrySince, err, ErrNoManifest)

	if err := PackFolder(t.TempDir(), t.TempDir(), key); !errors.Is(err, plugins.ErrBadManifest) {
		t.Errorf("an empty folder: %v", err)
	}
}

// wantErr fails t unless err is target.
func wantErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("%s: err = %v, want %v", what, err, target)
	}
}

// TestScriptReachesNoSecretsNorHost checks that a plugin, whatever its package declares, reads no secret, its partition's or the host's (HostPartition), calls no host handler it was not granted, and fetches no URL past its rules.
func TestScriptReachesNoSecretsNorHost(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	p := s.pack(t, fixture{id: "acme.peek", caps: `{"host": ["kando.toast", "kando.saved.read"]}`, code: `import { Secrets, host } from "nsgo"
export async function secret() { return await Secrets.get("token") }
export async function saved() { return await host.call("kando.saved.read", {}) }
export async function fetchIt() { return (await fetch("https://example.com/")).status }`})
	host := core.NewHost()
	host.Handle("kando.saved.read", func(c *core.HostCall) { c.Resolve("the saved lines") })
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var pl *Plugin
	e.do(func() { pl, err = Load(e.root, v, plugins.Grant{Perms: []plugins.Perm{plugins.PermToast}}, host) })
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Post: e.post, Watch: DefaultWatch}
	for _, name := range []string{"secret", "saved", "fetchIt"} {
		if got := wait(t, e.call(r, pl, name), 5*time.Second); core.ErrorCode(got.err) != core.CodeAccessDenied {
			t.Errorf("%s: %v, %v", name, got.v, got.err)
		}
	}
}

// TestScriptProbeComparesTheManifestAsWritten checks that a plugin whose texts Validate cleans, a title ending in a space and one with an emoji joined by a zero-width joiner, says what its manifest file says and is taken.
func TestScriptProbeComparesTheManifestAsWritten(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	commands := `"contributes": {"commands": [{"id": "a", "export": "run", "title": {"en": "Sync "}}, {"id": "b", "export": "run", "title": {"en": "Family 👨‍👩‍👧"}}]}`
	p := s.pack(t, fixture{id: "acme.tool", extra: commands, code: "export function run() {}"})
	if title := p.Contributes.Commands[1].Title["en"]; title == "Family 👨‍👩‍👧" {
		t.Fatalf("Validate left the title as it was: %q", title)
	}
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(t)
	e.do(func() { err = Probe(e.root, v) })
	if err != nil {
		t.Errorf("a plugin with texts Validate cleans: %v", err)
	}
}

// keep holds what TestScriptRunnerSparesAnInnocentPlugin allocates for the app, as long as the test runs.
var keep [][]byte

// TestScriptRunnerSparesAnInnocentPlugin checks that a plugin whose own calls keep nothing is not stopped for memory the app takes between its calls, well past the threshold.
func TestScriptRunnerSparesAnInnocentPlugin(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.ping", code: `export function ping() { return 1 }`}), plugins.Grant{})
	watch := DefaultWatch
	watch.Heap = 64 << 20
	stopped := make(chan string, 1)
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, hash, reason string) { stopped <- reason }}
	t.Cleanup(func() { keep = nil })
	for range 4 {
		keep = append(keep, make([]byte, 64<<20))
		for i := range keep[len(keep)-1] {
			keep[len(keep)-1][i] = 1
		}
		if got := wait(t, e.call(r, pl, "ping"), 10*time.Second); got.err != nil {
			t.Fatalf("an innocent call: %v", got.err)
		}
	}
	select {
	case reason := <-stopped:
		t.Fatalf("stopped for %s", reason)
	default:
	} // What a collection costs the watchdog with this heap, which it runs only once the heap looks past its threshold.
	at := time.Now()
	live := LiveHeapBytes()
	t.Logf("a collection of a %d MiB heap took %v", live>>20, time.Since(at))
}

// certified is a new release key that the root of s certifies for apps.
func certified(t *testing.T, s signer, apps ...string) signer {
	t.Helper()
	release, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cert, err := pkg.Certify(s.root, release.Public().(ed25519Public), apps, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return signer{root: s.root, release: release, cert: cert}
}

// TestScriptIndexKeyIsItsOwn checks that the community index is checked with a key of its own, a valid Ed25519 public key that is not the offline root's, so the root's secret never has to sign an index.
func TestScriptIndexKeyIsItsOwn(t *testing.T) {
	if IndexKey == OfficialRoot {
		t.Fatal("IndexKey is OfficialRoot")
	}
	index, err := pkg.DecodePublicKey(IndexKey)
	if err != nil {
		t.Fatalf("IndexKey: %v", err)
	}
	if pkg.KeyID(index) == pkg.KeyID(officialRoot) {
		t.Fatal("IndexKey has the official root's key ID")
	}
}

// testBgWatch is DefaultWatch with its background watchdog at a quarter of its scale: a reading every 50 ms and buckets of 1 s, the same Drain and TotalDrain, so a plugin that keeps the engine busy all the time is stopped in about 1.25 s and a burst of 0.75 s at full load, DefaultWatch's 3 s, fills its bucket to 0.6 s. The call watchdog is DefaultWatch's.
func testBgWatch() Watch {
	w := DefaultWatch
	w.BgBeat, w.Burst, w.TotalBurst = 50*time.Millisecond, time.Second, time.Second
	return w
}

// testOwnBgWatch is testBgWatch without the bucket of all plugins, for a test of a plugin's own bucket, which a plugin alone past TotalDrain would otherwise fill too.
func testOwnBgWatch() Watch {
	w := testBgWatch()
	w.TotalBurst = 0
	return w
}

// bgHog is the code of a plugin whose timers keep the engine busy in pieces of 400 ms, each set going from the last by setTimeout(0), from as soon as hog runs: the entry module runs it when it ends in "hog()".
const bgHog = `function hog() { setTimeout(function spin() { const t = Date.now(); while (Date.now() - t < 400) {} setTimeout(spin, 0) }, 0) }
`

// stoppedPlugin is a plugin the runner stopped, and why.
type stoppedPlugin struct{ id, reason string }

// bgRunner is a runner on e with watch that reports each plugin it stops on the channel it returns, and holds plugins, as the app does as it loads them.
func bgRunner(e *engine, watch Watch, plugins ...*Plugin) (*Runner, chan stoppedPlugin) {
	stopped := make(chan stoppedPlugin, 16)
	r := &Runner{Post: e.post, Watch: watch, Stopped: func(id, _, reason string) { stopped <- stoppedPlugin{id, reason} }}
	e.do(func() {
		for _, p := range plugins {
			r.Hold(p)
		}
	})
	return r, stopped
}

// wantStopped waits up to limit for the runner to stop a plugin, and fails t unless it is id, stopped as slow.
func wantStopped(t *testing.T, stopped <-chan stoppedPlugin, limit time.Duration, id string) {
	t.Helper()
	select {
	case got := <-stopped:
		if got.id != id || got.reason != plugins.ReasonSlow {
			t.Fatalf("stopped %s as %s, want %s as slow", got.id, got.reason, id)
		}
	case <-time.After(limit):
		t.Fatalf("%s was not stopped within %v", id, limit)
	}
}

// wantNoStop fails t if the runner stops a plugin within d.
func wantNoStop(t *testing.T, stopped <-chan stoppedPlugin, d time.Duration) {
	t.Helper()
	select {
	case got := <-stopped:
		t.Fatalf("stopped %s as %s", got.id, got.reason)
	case <-time.After(d):
	}
}

// wantResponsive fails t if a heartbeat posted to e waits more than 50 ms, as it would behind a plugin's piece of JS still running.
func wantResponsive(t *testing.T, e *engine) {
	t.Helper()
	for range 10 {
		posted := time.Now()
		e.do(func() {})
		if waited := time.Since(posted); waited > 50*time.Millisecond {
			t.Fatalf("a heartbeat waited %v after the plugin was stopped: its JS still runs", waited)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestScriptBackgroundStopsAHog checks the background watchdog: a plugin never called, whose entry module set timers going that keep the engine busy, is stopped as slow, once, its runtime interrupted so the engine answers again at once, and closed.
func TestScriptBackgroundStopsAHog(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.hog", code: bgHog + "hog()"}), plugins.Grant{})
	_, stopped := bgRunner(e, testOwnBgWatch(), pl)
	wantStopped(t, stopped, slow(5*time.Second), "acme.hog")
	wantResponsive(t, e)
	wantNoStop(t, stopped, 300*time.Millisecond)
	var closed bool
	e.do(func() { closed = pl.Closed() })
	if !closed {
		t.Error("not closed")
	}
}

// TestScriptBackgroundStopsAHogAfterItsCall checks that a plugin whose call sets its timers going and returns is not stopped by the call, which succeeds, but by the background watchdog after it.
func TestScriptBackgroundStopsAHogAfterItsCall(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.hog", code: bgHog + "export function start() { hog(); return 1 }"}), plugins.Grant{})
	r, stopped := bgRunner(e, testOwnBgWatch(), pl)
	if got := wait(t, e.call(r, pl, "start"), time.Second); got.err != nil || got.v != int64(1) {
		t.Fatalf("the call ended with %v, %v", got.v, got.err)
	}
	wantStopped(t, stopped, slow(5*time.Second), "acme.hog")
}

// TestScriptBackgroundSparesACallBehindAnotherPlugin checks that a call whose heartbeats wait behind another plugin's JS in the background is not stopped for it: the wait other plugins' JS took is not the call's. The call succeeds, and the only plugin stopped is the one that kept the engine busy. The call watchdog is at TestScriptWatchdogStopsASlicedLoop's scale, where three heartbeats behind the hog's pieces of 400 ms would stop the call in about 1.2 s, and the hog's bucket holds 2 s, so it is stopped well after.
func TestScriptBackgroundSparesACallBehindAnotherPlugin(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	hog := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.hog", code: bgHog + "hog()"}), plugins.Grant{})
	work := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.work", code: `export async function work() { for (let i = 0; i < 20; i++) await new Promise(r => setTimeout(r, 50)); return 1 }`}), plugins.Grant{})
	w := testBgWatch()
	w.Beat, w.Stall = 50*time.Millisecond, 125*time.Millisecond
	w.Burst = 2 * time.Second
	r, stopped := bgRunner(e, w, hog, work)
	if got := wait(t, e.call(r, work, "work"), slow(20*time.Second)); got.err != nil {
		t.Fatalf("the call behind the hog ended with %v", got.err)
	}
	wantStopped(t, stopped, slow(5*time.Second), "acme.hog")
	wantNoStop(t, stopped, 300*time.Millisecond)
}

// TestScriptBackgroundCountsAPausedCall checks that while the app waits on a call's behalf (PauseFor), the plugin's own timers that keep the engine busy are the background watchdog's: the call watchdog, which counts no heartbeat while paused (and none here at all), does not stop it, the background watchdog does.
func TestScriptBackgroundCountsAPausedCall(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	pl := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.hog", code: bgHog + "export function ask() { hog(); return new Promise(() => {}) }"}), plugins.Grant{})
	w := testOwnBgWatch()
	w.Stall = time.Hour
	r, stopped := bgRunner(e, w, pl)
	done := e.call(r, pl, "ask")
	var resume func()
	e.do(func() { resume = r.PauseFor(pl) })
	defer resume()
	wantStopped(t, stopped, slow(5*time.Second), "acme.hog")
	if got := wait(t, done, time.Second); !errors.Is(got.err, ErrStopped) {
		t.Errorf("the paused call ended with %v", got.err)
	}
}

// TestScriptBackgroundSparesLightWork checks what the background watchdog leaves alone: a plugin that works 5 ms of every 100 from a timer, a burst of 0.75 s at full load, which DefaultWatch's 3 s are at testBgWatch's scale, as a plugin indexing a film as it opens, and a call running pieces of 200 ms for 3 s, which is the call watchdog's to judge (its heartbeats count nothing here). Each runs on an engine of its own, side by side.
func TestScriptBackgroundSparesLightWork(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	cases := []struct {
		id, code, call string
	}{
		{"acme.light", `setInterval(() => { const t = Date.now(); while (Date.now() - t < 5) {} }, 100)`, ""},
		{"acme.burst", `export function index() {
  const end = Date.now() + 750
  setTimeout(function piece() { const t = Date.now(); while (Date.now() - t < 150) {} if (Date.now() < end) setTimeout(piece, 0) }, 0)
  return 1
}`, "index"},
		{"acme.sliced", `export async function slices() {
  const end = Date.now() + 3000
  while (Date.now() < end) { const t = Date.now(); while (Date.now() - t < 200) {} await new Promise(r => setTimeout(r, 0)) }
  return 1
}`, "slices"},
	}
	w := testBgWatch()
	w.Stall = time.Hour
	done := make(chan error, len(cases))
	for _, c := range cases {
		e := newEngine(t)
		pl := e.load(t, trust, roots, s.pack(t, fixture{id: c.id, code: c.code}), plugins.Grant{})
		r, stopped := bgRunner(e, w, pl)
		go func() {
			if c.call != "" {
				select {
				case got := <-e.call(r, pl, c.call):
					if got.err != nil {
						done <- fmt.Errorf("%s: the call ended with %v", c.id, got.err)
						return
					}
				case <-time.After(slow(10 * time.Second)):
					done <- fmt.Errorf("%s: the call did not end", c.id)
					return
				}
			}
			select {
			case got := <-stopped:
				done <- fmt.Errorf("%s: stopped as %s", got.id, got.reason)
			case <-time.After(3 * time.Second):
				done <- nil
			}
		}()
	}
	for range cases {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

// bgPanelFixture is a plugin with a panel whose timer keeps the engine busy in pieces of 300 ms, longer than a heartbeat may wait at TestScriptWatchdogStopsASlicedLoop's scale, and one that does nothing.
var bgPanelFixture = fixture{id: "acme.board", extra: `"contributes": {"panels": [{"id": "busy", "export": "Busy", "title": {"en": "Busy"}}, {"id": "quiet", "export": "Quiet", "title": {"en": "Quiet"}}]}`,
	code: `import { View } from "nsgo"
export function Busy() { setInterval(() => { const t = Date.now(); while (Date.now() - t < 300) {} }, 10); return <View /> }
export function Quiet() { return <View /> }
`}

// bgShowPanel loads the plugin f on e and shows its panel i under r, counting the builds of its view in builds.
func bgShowPanel(t *testing.T, e *engine, r *Runner, s signer, f fixture, i int, builds *atomic.Int32) *Plugin {
	t.Helper()
	roots := s.roots()
	p := s.pack(t, f)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var pl *Plugin
	e.do(func() {
		pl, err = Load(e.root, v, plugins.Grant{}, core.NewHost())
		if err != nil {
			return
		}
		e.tree.Mount(func(cx *core.Scope) core.Node {
			return r.Panel(pl, v, plugins.Grant{}, p.Contributes.Panels[i], func() *core.Host {
				if builds != nil {
					builds.Add(1)
				}
				return core.NewHost()
			}, func(cx *core.Scope, err error, _ func()) core.Node {
				t.Errorf("the panel failed: %v", err)
				return core.View{}
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

// TestScriptBackgroundSparesACallBehindAPanel checks that a call whose heartbeats wait behind another plugin's panel is not stopped for it, the panel's JS being its plugin's (Runner.othersJS), and that the plugin of the busy panel, which no call of its own watches, is stopped by the background watchdog, its view's runtime interrupted so the engine answers again. Its panel's own watchdog is kept from stopping it first.
func TestScriptBackgroundSparesACallBehindAPanel(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	work := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.work", code: `export async function work() { for (let i = 0; i < 20; i++) await new Promise(r => setTimeout(r, 50)); return 1 }`}), plugins.Grant{})
	w := testBgWatch()
	w.Beat, w.Stall = 50*time.Millisecond, 125*time.Millisecond
	w.Burst = 2 * time.Second
	r, stopped := bgRunner(e, w, work)
	r.PanelWatch = testPanelWatch
	r.PanelWatch.Windows = 1000
	bgShowPanel(t, e, r, s, bgPanelFixture, 0, nil)
	if got := wait(t, e.call(r, work, "work"), slow(20*time.Second)); got.err != nil {
		t.Fatalf("the call behind the panel ended with %v", got.err)
	}
	wantStopped(t, stopped, slow(5*time.Second), "acme.board")
	wantResponsive(t, e)
}

// TestScriptPanelSparesAQuietPanelBehindAnotherPlugin checks that a quiet panel is not suspected while another plugin keeps the engine busy in the background: what the other plugin's JS took of the engine is taken out of the engine's load the panel's watchdog measures, so the view is never taken down for a probe and stays its first build. The other plugin's bucket is kept from stopping it meanwhile.
func TestScriptPanelSparesAQuietPanelBehindAnotherPlugin(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	e := newEngine(t)
	hog := e.load(t, trustOf(t, roots), roots, s.pack(t, fixture{id: "acme.hog", code: bgHog + "hog()"}), plugins.Grant{})
	w := testBgWatch()
	w.Burst, w.TotalBurst = 0, 0
	r, stopped := bgRunner(e, w, hog)
	r.PanelWatch = testPanelWatch
	var builds atomic.Int32
	bgShowPanel(t, e, r, s, bgPanelFixture, 1, &builds)
	// Three windows of 300 ms suspect a view, and a probe of 300 ms takes it down and builds it again: 3 s hold a few.
	wantNoStop(t, stopped, slow(3*time.Second))
	if n := builds.Load(); n != 1 {
		t.Errorf("the quiet panel was built %d times: it was taken down for the other plugin's load", n)
	}
}

// TestScriptBackgroundStopsTheLargestOfMany checks the bucket of all plugins: three plugins that each work 16 ms of every 100, under Drain, together over TotalDrain, have one of them stopped, after which the other two, together back under TotalDrain, are left alone; and with a plugin that works 3 ms of every 100 and two of 16, over a TotalDrain of 0.3, the one stopped is one of the two, never the light one, which is under totalShare.
func TestScriptBackgroundStopsTheLargestOfMany(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	worker := func(ms int) string {
		return fmt.Sprintf(`setInterval(() => { const t = Date.now(); while (Date.now() - t < %d) {} }, 100)`, ms)
	}
	for _, c := range []struct {
		name       string
		ms         []int
		totalDrain float64
	}{
		{"three of 16%", []int{16, 16, 16}, 0.4},
		{"one of 3% and two of 16%", []int{3, 16, 16}, 0.3},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEngine(t)
			var pls []*Plugin
			heavy := map[string]bool{}
			for i, ms := range c.ms {
				id := fmt.Sprintf("acme.w%d", i)
				pls = append(pls, e.load(t, trust, roots, s.pack(t, fixture{id: id, code: worker(ms)}), plugins.Grant{}))
				heavy[id] = ms > 10
			}
			w := testBgWatch()
			w.TotalDrain, w.TotalBurst = c.totalDrain, 200*time.Millisecond
			_, stopped := bgRunner(e, w, pls...)
			var got stoppedPlugin
			select {
			case got = <-stopped:
			case <-time.After(slow(20 * time.Second)):
				t.Fatal("no plugin was stopped")
			}
			if !heavy[got.id] || got.reason != plugins.ReasonSlow {
				t.Fatalf("stopped %s as %s", got.id, got.reason)
			}
			wantNoStop(t, stopped, 3*time.Second)
			e.do(func() {
				for _, p := range pls {
					if p.Closed() != (p.ID == got.id) {
						t.Errorf("%s closed %t", p.ID, p.Closed())
					}
				}
			})
		})
	}
}

// TestScriptBackgroundEndsWithThePlugins checks that the background watchdog's goroutine runs while a plugin is held, and ends once every plugin held was closed, and starts again for a plugin held after.
func TestScriptBackgroundEndsWithThePlugins(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	a := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.a"}), plugins.Grant{})
	b := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.b"}), plugins.Grant{})
	r, _ := bgRunner(e, testBgWatch(), a, b)
	exited := func() <-chan struct{} {
		var ch chan struct{}
		e.do(func() {
			r.bg.mu.Lock()
			defer r.bg.mu.Unlock()
			if r.bg.running {
				ch = r.bg.exited
			}
		})
		return ch
	}
	// The goroutine starts from a post, which bgRunner's do ran before it returned.
	running := exited()
	if running == nil {
		t.Fatal("no background watchdog while two plugins are held")
	}
	e.do(func() { r.Close(a) })
	select {
	case <-running:
		t.Fatal("the background watchdog ended while a plugin is held")
	case <-time.After(200 * time.Millisecond):
	}
	e.do(func() { r.Close(b) })
	select {
	case <-running:
	case <-time.After(time.Second):
		t.Fatal("the background watchdog runs on with no plugin held")
	}
	if exited() != nil {
		t.Fatal("the runner says its background watchdog runs")
	}
	c := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.c"}), plugins.Grant{})
	e.do(func() { r.Hold(c) })
	again := exited()
	if again == nil {
		t.Fatal("no background watchdog for a plugin held after")
	}
	e.do(func() { r.Close(c) })
	select {
	case <-again:
	case <-time.After(time.Second):
		t.Fatal("the background watchdog runs on with no plugin held")
	}
}

// TestScriptOthersJSCountsFromWhenAPluginJoins checks that what other plugins ran, which spares a call or a panel the engine's time they took, moves only by what they run while it is measured: a plugin that ran 300 ms of JS as it loaded adds nothing as it joins the registry in the middle of a measure, and one that leaves takes nothing it ran away.
func TestScriptOthersJSCountsFromWhenAPluginJoins(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	trust := trustOf(t, roots)
	e := newEngine(t)
	quiet := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.quiet", code: `export function work() { return 1 }`}), plugins.Grant{})
	busy := e.load(t, trust, roots, s.pack(t, fixture{id: "acme.busy", code: `const t = Date.now(); while (Date.now() - t < 300) {}
export function work() { return 1 }`}), plugins.Grant{})
	if ran, _ := busy.p.JSTime(); ran < 250*time.Millisecond {
		t.Fatalf("the busy plugin ran %v as it loaded, want 300 ms", ran)
	}
	w := testBgWatch()
	w.BgBeat = 0
	r, _ := bgRunner(e, w, quiet)
	before := r.othersJS(quiet)
	e.do(func() { r.Hold(busy) })
	joined := r.othersJS(quiet)
	if d := joined - before; d > 50*time.Millisecond {
		t.Errorf("a plugin joining added %v of what it ran before it joined", d)
	}
	e.do(func() { r.Close(busy) })
	if left := r.othersJS(quiet); left < joined {
		t.Errorf("a plugin leaving took %v away", joined-left)
	}
}
