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

// watchFixture is a plugin with three panels for the watchdog: busy, which keeps the engine busy from a timer, 80 ms of every 90; grow, which keeps what a timer makes, a string of 1 MiB every 20 ms, the engine idle in between; and quiet, which does nothing.
var watchFixture = fixture{id: "acme.watch", extra: `"contributes": {"panels": [{"id": "busy", "export": "Busy", "title": {"en": "Busy"}}, {"id": "grow", "export": "Grow", "title": {"en": "Grow"}}, {"id": "quiet", "export": "Quiet", "title": {"en": "Quiet"}}]}`,
	code: `import { host, mount, View } from "nsgo"
const kept = []
export function Busy() { setInterval(() => { const t = Date.now(); while (Date.now() - t < 80) {} }, 10); return <View /> }
export function Grow() { let n = 0; setInterval(() => { kept.push("x".repeat(1 << 20) + n++) }, 20); return <View /> }
export function Quiet() { return <View /> }
if (host.params.panel === "busy") mount(Busy)
if (host.params.panel === "grow") mount(Grow)
if (host.params.panel === "quiet") mount(Quiet)
`}

// testPanelWatch is a panel's watchdog on the scale of a test: windows of 300 ms.
var testPanelWatch = PanelWatch{Window: 300 * time.Millisecond, Share: 0.6, Windows: 3, Probe: 300 * time.Millisecond, Idle: 0.3, Retries: 3}

// showPanel shows the panel i of watchFixture on an engine of its own, under a runner with watch and a heap threshold of heap, and returns the engine and what the runner reports it stopped the plugin for.
func showPanel(t *testing.T, i int, heap uint64, watch PanelWatch) (*engine, <-chan string) {
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
				t.Errorf("the panel failed: %v", err)
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
