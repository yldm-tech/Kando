package script

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
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

// TestScriptSignedManifest checks a package that signed its manifest (pkg.Contents.Extra): Verify compares what it signed with the manifest, so Probe runs none of its code, which need not say what it is (kandoManifest), and Load checks the functions its contributions call; a manifest that is not what it signed, a signed entry with another key or that is no object, and a signed entry changed after signing are refused. A package that did not sign it, as every one published, is the kandoManifest's of TestScriptProbe.
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

	// Its code throws as it loads and says nothing of itself: Probe never runs it.
	v, err := verify(fixture{id: "acme.tool", extra: commands, signManifest: true, said: "undefined", code: `throw new Error("ran before the user agreed")`})
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
	v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true, said: "undefined", code: "export function run() { return 1 }"})
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
		t.Fatalf("a package that signed its manifest without kandoManifest: %v", err)
	}
	e.do(func() { pl.p.Close() })

	// A command whose function it does not export is refused as it loads, as Probe did not run it.
	v, err = verify(fixture{id: "acme.tool", extra: commands, signManifest: true})
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
	for name, l := range map[string]any{"run": *Limits(), "probe": *probeLimits(), "sandbox": *Sandbox("a.b", "k", plugins.Grant{}, nil).Limits, "probe sandbox": *probeSandbox("p").Limits} {
		v := reflect.ValueOf(l)
		if !v.FieldByName("Harden").Bool() {
			t.Errorf("%s: not hardened", name)
		}
		for _, f := range []string{"Load", "Call", "Stack", "Nodes", "Timers", "Requests", "Body", "Storage"} {
			if v.FieldByName(f).Int() <= 0 {
				t.Errorf("%s: %s is %v", name, f, v.FieldByName(f))
			}
		}
	}
}

// TestScriptGrantIsMadeFromNothing checks that the sandbox's grant holds nothing but the permissions and HTTP rules granted, whatever pkg.Capabilities may hold, is never nil, and that each sandbox is in a partition of its id and signer.
func TestScriptGrantIsMadeFromNothing(t *testing.T) {
	grant := plugins.Grant{Perms: []plugins.Perm{plugins.PermToast}, Network: []pkg.HTTPRule{{Host: "127.0.0.1", Methods: []string{"POST"}, Paths: []string{"/"}}}}
	for name, s := range map[string]any{"granted": Sandbox("acme.tool", "k1", grant, nil), "nothing": Sandbox("acme.tool", "k1", plugins.Grant{}, nil), "probe": probeSandbox("p")} {
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
	if s := Sandbox("acme.tool", "k1", grant, nil); s.Storage != "plugin.acme.tool.k1" || len(s.Grant.Host) != 1 || len(s.Grant.Network.HTTP) != 1 {
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
	// piece is how long the loop keeps the engine busy at a time: under tsx's call budget of half a second, and longer than Stall and a Beat, so the heartbeat posted at most a Beat into each piece waits past Stall and counts.
	const piece = 430 * time.Millisecond
	w := DefaultWatch
	if piece-w.Beat <= w.Stall {
		t.Fatalf("a piece of %v does not hold a heartbeat past Stall %v", piece, w.Stall)
	}
	// Each heartbeat that counts takes a piece, and Stalls in a row stop the loop as the piece the last one waited in ends. A heartbeat that falls due just as a piece ends can be posted after the one before it ran, find the engine between two pieces and start the count again, as it should, the window having answered it; a busy runner that delivers the tick late makes that likelier. The bound allows one such restart, at worst just before the last stall would have counted, so 2*Stalls-1 pieces, each with a Beat for the gaps between pieces and a late tick. The pieces end 30, 60, 90, 20 and 50 ms past a tick (430 ms where the 450 ms of docs/plugin-system.md ended the second piece on one), so a restart takes a tick at least 20 ms late.
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
