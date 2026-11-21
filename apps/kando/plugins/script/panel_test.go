package script

import (
	"errors"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
)

// panelFixture is a plugin with three panels: deck, which shows the parameters its view was given, broken, which throws as it renders, and spin, which never returns.
var panelFixture = fixture{id: "acme.panel", extra: `"contributes": {"panels": [{"id": "deck", "export": "Deck", "title": {"en": "Deck"}}, {"id": "broken", "export": "Broken", "title": {"en": "Broken"}}, {"id": "spin", "export": "Spin", "title": {"en": "Spin"}}]}`,
	code: `import { host, mount, Text } from "nsgo"
export function Deck() { return <Text testID="deck">{"deck " + host.params.panel + " " + host.params.export}</Text> }
export function Broken() { throw new Error("broken panel") }
export function Spin() { for (;;) {} }
if (host.params.panel === "deck") mount(Deck)
if (host.params.panel === "broken") mount(Broken)
if (host.params.panel === "spin") mount(Spin)
`}

// TestScriptPanel checks a panel's view (Runner.Panel): it shows the export its parameters name, apart from the plugin loaded for its functions, which mounts nothing; one that throws shows the fallback in its place, with what it threw; each build, a retry's too, has a host of its own; and each failure, a retry's too, counts for the plugin's breaker, which turns it off at the third.
func TestScriptPanel(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, panelFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan string, 1)
	var failed []error
	hosts := 0
	start := func(id string) *nsgotest.App {
		return nsgotest.Start(t, func(cx *core.Scope) core.Node {
			r := &Runner{Post: cx.Post, Root: t.TempDir(), Watch: DefaultWatch, Stopped: func(_, _, reason string) { stopped <- reason }}
			// The panel's marks are written off the engine's goroutine; wait for the writer before the TempDir's removal, which runs after this cleanup, or a mark written or cleared meanwhile leaves .inflight not empty on Windows.
			t.Cleanup(r.Drain)
			pl, err := Load(cx, v, plugins.Grant{}, core.NewHost())
			if err != nil {
				t.Fatal(err)
			}
			i := 0
			for i < len(p.Contributes.Panels) && p.Contributes.Panels[i].ID != id {
				i++
			}
			return core.View{Children: []core.Node{
				core.Text{Content: "host", TestID: "host"},
				r.Panel(pl, v, plugins.Grant{}, p.Contributes.Panels[i], func() *core.Host { hosts++; return core.NewHost() }, func(cx *core.Scope, err error, retry func()) core.Node {
					failed = append(failed, err)
					return base.Button{TestID: "retry", Label: "retry", OnPress: func(core.PressEvent) { retry() }, Children: []core.Node{core.Text{Content: "retry"}}}
				}),
			}}
		})
	}
	app := start("deck")
	if got := app.Find("id:deck").Text; got != "deck deck Deck" {
		t.Fatalf("the panel shows %q\n%s", got, app.Dump())
	}
	if len(failed) > 0 || app.Find("id:host").Text != "host" {
		t.Fatalf("failed %v", failed)
	}
	hosts = 0
	app = start("broken")
	if !app.Exists("id:retry") || len(failed) != 1 || !strings.Contains(failed[0].Error(), "broken panel") {
		t.Fatalf("a panel that throws: %v\n%s", failed, app.Dump())
	}
	if app.Find("id:host").Text != "host" {
		t.Fatal("the host's own view is gone")
	}
	for range 2 {
		app.Press("id:retry")
		app.Settle()
	}
	if hosts != 3 {
		t.Errorf("%d hosts for three builds: a retry reuses the host of the build it replaces", hosts)
	}
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonFailed || len(failed) != 3 {
			t.Errorf("stopped for %q after %d failures", reason, len(failed))
		}
	default:
		t.Fatalf("not stopped after %d failures", len(failed))
	}
}

// flipFixture is a plugin whose panel's root is a function, which a timer of the panel's makes render again as two nodes in place of one.
var flipFixture = fixture{id: "acme.flip", extra: `"contributes": {"panels": [{"id": "flip", "export": "Flip", "title": {"en": "Flip"}}]}`,
	code: `import { host, mount, signal, Text } from "nsgo"
export function Flip() {
  const n = signal(0)
  setTimeout(() => n.set(1), 10)
  return () => n() === 0 ? <Text testID="flip">zero</Text> : [<Text testID="flip">one</Text>, <Text testID="flip">two</Text>]
}
if (host.params.panel === "flip") mount(Flip)
`}

// TestScriptPanelShowsWhatItsModuleMounts checks that a panel's view shows what the entry module mounts once, even after a root that is a function renders again: a view that also rendered the export (tsx.View.Export) would show the mounted copy next to it then, as tsx leaves what the entry mounted running.
func TestScriptPanelShowsWhatItsModuleMounts(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, flipFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var failed []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		r := &Runner{Post: cx.Post, Watch: DefaultWatch}
		pl, err := Load(cx, v, plugins.Grant{}, core.NewHost())
		if err != nil {
			t.Fatal(err)
		}
		return r.Panel(pl, v, plugins.Grant{}, p.Contributes.Panels[0], core.NewHost, func(cx *core.Scope, err error, _ func()) core.Node {
			failed = append(failed, err)
			return core.Text{TestID: "failed", Content: "failed"}
		})
	})
	if n := len(app.QueryMounted("id:flip")); n != 1 || len(failed) > 0 {
		t.Fatalf("%d nodes before the timer, failed %v\n%s", n, failed, app.Dump())
	}
	app.Advance(50 * time.Millisecond)
	app.Settle()
	if n := len(app.QueryMounted("id:flip")); n != 2 {
		t.Fatalf("%d nodes after the timer, not the two it renders\n%s", n, app.Dump())
	}
}

// TestScriptPanelPastItsBudget checks that a panel whose render runs past Limits' budget is cut there and shows the fallback, the window's goroutine held no longer than that.
func TestScriptPanelPastItsBudget(t *testing.T) {
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, panelFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	start := time.Now()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		r := &Runner{Post: cx.Post, Watch: DefaultWatch}
		pl, err := Load(cx, v, plugins.Grant{}, core.NewHost())
		if err != nil {
			t.Fatal(err)
		}
		return r.Panel(pl, v, plugins.Grant{}, p.Contributes.Panels[2], core.NewHost, func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{TestID: "failed", Content: "failed"}
		})
	})
	if !app.Exists("id:failed") || !errors.Is(failed, tsx.ErrBudget) && !contains(failed, "budget") {
		t.Fatalf("a panel past its budget: %v", failed)
	}
	if took := time.Since(start); took > slow(3*time.Second) {
		t.Errorf("it held the window %v", took)
	}
}

// watchFixture is a plugin with four panels for the watchdog: busy, which keeps the engine busy from a timer, 80 ms of every 90; grow, which keeps what a timer makes, an array of 64 Ki numbers every 20 ms, copied with concat, which tsx's memory quota (Limits.Memory) does not count, the engine idle in between; quiet, which does nothing; and big, which keeps a string of 1 MiB every 20 ms, made with repeat, which the quota counts.
var watchFixture = fixture{id: "acme.watch", extra: `"contributes": {"panels": [{"id": "busy", "export": "Busy", "title": {"en": "Busy"}}, {"id": "grow", "export": "Grow", "title": {"en": "Grow"}}, {"id": "quiet", "export": "Quiet", "title": {"en": "Quiet"}}, {"id": "big", "export": "Big", "title": {"en": "Big"}}]}`,
	code: `import { host, mount, View } from "nsgo"
const kept = []
const half = []
for (let i = 0; i < 1 << 15; i++) half.push(i)
export function Busy() { setInterval(() => { const t = Date.now(); while (Date.now() - t < 80) {} }, 10); return <View /> }
export function Grow() { setInterval(() => { kept.push(half.concat(half)) }, 20); return <View /> }
export function Quiet() { return <View /> }
export function Big() { let n = 0; setInterval(() => { kept.push("x".repeat(1 << 20) + n++) }, 20); return <View /> }
if (host.params.panel === "busy") mount(Busy)
if (host.params.panel === "grow") mount(Grow)
if (host.params.panel === "quiet") mount(Quiet)
if (host.params.panel === "big") mount(Big)
`}

// testPanelWatch is a panel's watchdog on the scale of a test: windows of 300 ms.
var testPanelWatch = PanelWatch{Window: 300 * time.Millisecond, Share: 0.6, Windows: 3, Probe: 300 * time.Millisecond, Idle: 0.3, Retries: 3}

// showPanel shows the panel i of watchFixture on an engine of its own, under a runner with watch and a heap threshold of heap, and returns the engine and what the runner reports it stopped the plugin for. The panel failing fails the test.
func showPanel(t *testing.T, i int, heap uint64, watch PanelWatch) (*engine, <-chan string) {
	t.Helper()
	return showPanelFailing(t, i, heap, watch, func(err error) { t.Errorf("the panel failed: %v", err) })
}

// showPanelFailing is showPanel with failed hearing what the panel fails with.
func showPanelFailing(t *testing.T, i int, heap uint64, watch PanelWatch, failed func(error)) (*engine, <-chan string) {
	t.Helper()
	s := newSigner(t, AppIDPrefix+"acme.*")
	roots := s.roots()
	p := s.pack(t, watchFixture)
	v, err := Verify(trustOf(t, roots), roots, p)
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(t)
	stopped := make(chan string, 1)
	w := DefaultWatch
	w.Heap = heap
	r := &Runner{Post: e.post, Watch: w, PanelWatch: watch, Stopped: func(_, _, reason string) { stopped <- reason }}
	e.do(func() {
		pl, err := Load(e.root, v, plugins.Grant{}, core.NewHost())
		if err != nil {
			t.Fatal(err)
		}
		e.tree.Mount(func(cx *core.Scope) core.Node {
			return r.Panel(pl, v, plugins.Grant{}, p.Contributes.Panels[i], core.NewHost, func(cx *core.Scope, err error, _ func()) core.Node {
				failed(err)
				return core.View{}
			})
		})
	})
	return e, stopped
}

// TestScriptPanelWatchdogStopsABusyPanel checks that a panel whose timers keep the engine busy is stopped as slow, once taking it down for the probe took the load away.
func TestScriptPanelWatchdogStopsABusyPanel(t *testing.T) {
	_, stopped := showPanel(t, 0, DefaultWatch.Heap, testPanelWatch)
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonSlow {
			t.Errorf("stopped as %q", reason)
		}
	case <-time.After(slow(5 * time.Second)):
		t.Fatal("a busy panel was not stopped")
	}
}

// TestScriptPanelWatchdogStopsAGrowingPanel checks that a panel that keeps what it makes is stopped for its memory once its live heap passed the threshold and taking it down for the probe let it go.
func TestScriptPanelWatchdogStopsAGrowingPanel(t *testing.T) {
	// The engine's load is not watched here: under the race detector the strings take long enough to make the engine look busy.
	watch := testPanelWatch
	watch.Windows = 1000
	_, stopped := showPanel(t, 1, 64<<20, watch)
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonMemory {
			t.Errorf("stopped as %q", reason)
		}
	case <-time.After(slow(10 * time.Second)):
		t.Fatal("a growing panel was not stopped")
	}
}

// TestScriptPanelStopsAtItsMemoryQuota checks that a panel tsx stopped at its memory quota (Limits.Memory) shows the fallback and stops its plugin for its memory at once, as a call that ran into it does, though the quota's error has a code the breaker does not count.
func TestScriptPanelStopsAtItsMemoryQuota(t *testing.T) {
	var failed []error
	// Neither the engine's load nor the heap is watched: the quota is what this test is about.
	watch := testPanelWatch
	watch.Windows = 1000
	e, stopped := showPanelFailing(t, 3, 0, watch, func(err error) { failed = append(failed, err) })
	select {
	case reason := <-stopped:
		if reason != plugins.ReasonMemory {
			t.Errorf("stopped as %q", reason)
		}
	case <-time.After(slow(10 * time.Second)):
		t.Fatal("a panel past its memory quota was not stopped")
	}
	e.do(func() {
		if len(failed) != 1 || !contains(failed[0], "memory") {
			t.Errorf("the panel failed with %v", failed)
		}
	})
}

// TestScriptPanelWatchdogSparesAQuietPanel checks that a quiet panel is not stopped while the engine is busy with work of the app's own: taking it down for the probe leaves the load where it was, so it is built again and left alone.
func TestScriptPanelWatchdogSparesAQuietPanel(t *testing.T) {
	e, stopped := showPanel(t, 2, DefaultWatch.Heap, testPanelWatch)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			e.do(func() {
				t0 := time.Now()
				for time.Since(t0) < 80*time.Millisecond {
				}
			})
			time.Sleep(10 * time.Millisecond)
		}
	}()
	select {
	case reason := <-stopped:
		t.Fatalf("a quiet panel was stopped as %q while the app was busy", reason)
	case <-time.After(slow(4 * time.Second)):
	}
}

// TestScriptMarksStopWithTheRunner checks the writer of the panels' in-flight marks: Drain waits until it wrote every mark asked before, a build's clearing included, and nothing is written after it, however many marks are asked for then.
func TestScriptMarksStopWithTheRunner(t *testing.T) {
	root := t.TempDir()
	r := &Runner{Root: root}
	p := &Plugin{ID: "acme.marks", Hash: "h1"}
	for range 30 {
		r.mark(p, true)
		r.mark(p, false)
	}
	r.mark(p, true)
	r.Drain()
	if got := plugins.InFlight(root); got["acme.marks"] != "h1" {
		t.Fatalf("after Drain the marks are %v: the last one asked was not written", got)
	}
	for range 30 {
		r.mark(p, true)
	}
	r.Drain()
	time.Sleep(50 * time.Millisecond)
	if got := plugins.InFlight(root); len(got) != 0 {
		t.Errorf("marks written after Drain: %v", got)
	}
}
