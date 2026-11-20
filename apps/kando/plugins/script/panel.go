package script

import (
	"runtime"
	"sync"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// The host parameters a panel's view reads (core.Scope.HostParams, host.params in TSX): the id of the panel it shows and the export that draws it (plugins.Panel). They are set only in a panel's view, never as the plugin is loaded for its functions or read by Probe. The view renders the export itself (tsx.View.Export), so a plugin only exports the component; one export may read them to tell apart the panels it draws:
//
//	import { Text } from "nsgo"
//	export function Deck() { return <Text>…</Text> }
//
// A plugin written for the hosts that rendered only what the entry module mounts mounts the export itself when they name its panel, which still shows the panel once: the view replaces what the entry module mounted with the export it renders.
//
//	if (host.params.panel === "deck") mount(Deck)
//
// TODO(nsgo): tsx replaces what the entry module mounted with View.Export's component by taking its nodes out of the root, not by disposing the scope mount ran it under (tsx/sandbox.go:222-224 removeChild, tsx/dom.go:241 "it stays until its owner goes", tsx/dom.go:728 mount's owner), so in the view of a plugin that still mounts its export a hidden copy of it keeps its effects and timers until the view goes; nothing to change here once tsx disposes it
const (
	PanelParam  = "panel"
	ExportParam = "export"
)

// PanelWatch is when the watchdog of a panel's view suspects it. A view stays open for as long as the user keeps it, its JS running in pieces on the engine's goroutine between the window's own work, and tsx says neither when a piece of it runs nor how long, so the watchdog reads what the engine as a whole does: heartbeats posted through each window and whether each waited, which says how busy the engine was (share), and the Go heap. It suspects the view when the engine was busy more than Share of each of Windows windows of Window in a row, or the live heap grew by more than Runner.Watch.Heap since the view was built; it then takes the view down for Probe, which stops its timers and lets what it holds go, and stops the plugin only when that took the load away: the engine busy less than Idle of the probe, or the live heap back under the threshold. Otherwise the load was the app's, or another plugin's, and the view is built again with the counts started afresh; a view suspected Retries times in a row in vain is left alone until it is built again, so an app busy for long does not take it down over and over.
//
// TODO(nsgo#836): tsx gives no time a sandboxed runtime's JS ran, nor a hook around its timers, frames and events (tsx/timers.go:44-58 runs a timer through Scope.AfterFunc and runtime.call with no callback, tsx/sandbox.go:311-329 times each call only to cut it), so the watchdog suspects a view from the whole engine's load and confirms it by taking the view down; measure the view's own time once tsx says it https://github.com/nsgo-dev/nsgo/issues/836
type PanelWatch struct {
	Window  time.Duration
	Share   float64
	Windows int
	Probe   time.Duration
	Idle    float64
	Retries int
}

// DefaultPanelWatch is a panel's watchdog: busy more than 60% of three windows of 5 s in a row, confirmed by a probe of 2 s busy less than 30%, three suspicions in vain at most.
var DefaultPanelWatch = PanelWatch{Window: 5 * time.Second, Share: 0.6, Windows: 3, Probe: 2 * time.Second, Idle: 0.3, Retries: 3}

// Panel is the view of the panel of the plugin p, whose package v loaded it with grant: its package run anew in a runtime of its own, in the same sandbox its functions run in (Sandbox: Limits, the grant made from nothing, the partition of its id and signer), with a host host makes for each build, whose parameters it sets to name the panel (PanelParam, ExportParam), and the export the panel names rendered as its view (tsx.View.Export): one the package does not export as a component fails as a throw does. Each build has a new host, a retry's too, as a core.Host serves one app at a time and keeps the events sent to a view gone for the next; the host p was loaded with is never one, as p would stop hearing its events. The view is under a core.Catch: what the module throws as it loads, as it renders or as it handles an event later, a piece of it cut at Limits' budget, or a quota it runs past, shows fallback in its place, retry building the view again, and counts for p's breaker as a failed call does, a view taken down without failing as a call that succeeded, so a panel that keeps failing turns its plugin off (Stopped); it never reaches what holds the view. While it shows, its watchdog (PanelWatch) stops the plugin for a view that keeps the window busy (plugins.ReasonSlow) or holds memory (plugins.ReasonMemory). Each build is marked in flight (plugins.MarkInFlight) unless a call of p's is under way, which is marked already, written off the engine's goroutine, so a panel that crashed Kando as it loaded turns its plugin off at the next start. Build it on the engine's goroutine.
func (r *Runner) Panel(p *Plugin, v Verified, grant plugins.Grant, panel plugins.Panel, host func() *core.Host, fallback func(cx *core.Scope, err error, retry func()) core.Node) core.Node {
	return panelView{r: r, p: p, v: v, grant: grant, panel: panel, host: host, fallback: fallback}
}

// panelView is a panel's view as Runner.Panel builds it.
type panelView struct {
	r        *Runner
	p        *Plugin
	v        Verified
	grant    plugins.Grant
	panel    plugins.Panel
	host     func() *core.Host
	fallback func(cx *core.Scope, err error, retry func()) core.Node
}

// Render implements core.Node: the view while its watchdog does not hold it down, nothing while it does.
func (pv panelView) Render(cx *core.Scope) core.Node {
	r, p := pv.r, pv.p
	down := core.NewSignal(cx, false)
	w := &panelWatchdog{r: r, p: p, watch: r.panelWatch(), down: down.Set, stop: make(chan struct{}), heap: HeapBytes()}
	cx.Cleanup(w.end)
	// The watchdog starts once the view is built: no goroutine starts in a build.
	r.Post(w.start)
	return core.Show{When: func() bool { return !down.Get() }, Then: pv.catch, Else: func(*core.Scope) core.Node { return core.View{} }}
}

// catch is the view under its core.Catch.
func (pv panelView) catch(*core.Scope) core.Node {
	r, p := pv.r, pv.p
	// build is the latest build of this Catch, which its fallback marks as failed.
	var build *panelBuild
	return core.Catch{
		Try: func(cx *core.Scope) core.Node {
			host := pv.host()
			_ = host.SetParams(map[string]any{PanelParam: pv.panel.ID, ExportParam: pv.panel.Export})
			b := &panelBuild{marked: r.Root != "" && (r.running == nil || r.running.p != p)}
			build = b
			if b.marked {
				r.mark(p, true)
			}
			// A build taken down without failing, as another panel was chosen, counts as a success; one that failed was counted by its fallback, which runs before this post.
			cx.Cleanup(func() {
				r.Post(func() {
					if !b.failed {
						r.Succeeded(p)
					}
				})
			})
			node := tsx.View{Package: pv.v.Package, Export: pv.panel.Export, Sandbox: Sandbox(pv.v.ID, pv.v.Signer, pv.grant, host)}.Render(cx)
			if b.marked {
				b.marked = false
				r.mark(p, false)
			}
			return node
		},
		Fallback: func(cx *core.Scope, err error, retry func()) core.Node {
			if b := build; b != nil {
				b.failed = true
				if b.marked {
					b.marked = false
					r.mark(p, false)
				}
			}
			// Counted after the build: stopping the plugin changes the app's state, which a build does not.
			r.Post(func() { r.Failed(p, err) })
			return pv.fallback(cx, err, retry)
		},
	}
}

// panelBuild is one build of a panel's view: whether its in-flight mark is still to clear, and whether it failed.
type panelBuild struct {
	marked bool
	failed bool
}

// Failed counts err, a failure of the plugin p outside its calls, as its panel's view failing, for its breaker as a failed call is counted (a core.Error with a code is not a failure), and stops it when the breaker trips. Call it on the engine's goroutine.
func (r *Runner) Failed(p *Plugin, err error) {
	if p.closed {
		return
	}
	if p.count(err) {
		r.stop(p, plugins.ReasonFailed)
	}
}

// Succeeded counts a success of the plugin p outside its calls, as its panel's view taken down without failing, for its breaker, so failures far apart do not add up to a stop. Call it on the engine's goroutine.
func (r *Runner) Succeeded(p *Plugin) {
	if !p.closed {
		p.count(nil)
	}
}

// panelWatch is the runner's PanelWatch, DefaultPanelWatch when it has none.
func (r *Runner) panelWatch() PanelWatch {
	if r.PanelWatch.Window > 0 {
		return r.PanelWatch
	}
	return DefaultPanelWatch
}

// mark writes, or with on false clears, the in-flight mark of p off the engine's goroutine, in the order asked, on the runner's one writer; nothing once the runner was drained (Drain).
func (r *Runner) mark(p *Plugin, on bool) {
	root, id, hash := r.Root, p.ID, p.Hash
	fn := func() { plugins.ClearInFlight(root, id) }
	if on {
		fn = func() { plugins.MarkInFlight(root, id, hash) }
	}
	r.marksMu.Lock()
	defer r.marksMu.Unlock()
	if r.drained {
		return
	}
	if r.marks == nil {
		r.marks = make(chan func(), 64)
		r.marksDone = make(chan struct{})
		go func(marks <-chan func(), done chan<- struct{}) {
			defer close(done)
			for fn := range marks {
				fn()
			}
		}(r.marks, r.marksDone)
	}
	select {
	case r.marks <- fn:
	default:
		// A writer this far behind is stuck on the disk; a mark lost is one chance less to turn a plugin off.
	}
}

// Drain stops the writer of the panels' in-flight marks once it wrote what was asked of it, and waits for it: the marks of a build are asked for together, on the engine's goroutine, so a build's mark is never left without its clearing. No mark is written after it returns. Call it as the app quits, or a test ends, once no panel is built any more.
func (r *Runner) Drain() {
	r.marksMu.Lock()
	r.drained = true
	marks, done := r.marks, r.marksDone
	r.marks = nil
	r.marksMu.Unlock()
	if marks != nil {
		close(marks)
		<-done
	}
}

// panelWatchdog watches a panel's view while it shows (PanelWatch). heap is the Go heap as the view was built; down holds the view down.
type panelWatchdog struct {
	r     *Runner
	p     *Plugin
	watch PanelWatch
	down  func(bool)
	stop  chan struct{}
	heap  uint64

	mu    sync.Mutex
	ended bool
}

// end ends the watchdog, as the view goes.
func (w *panelWatchdog) end() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.ended {
		w.ended = true
		close(w.stop)
	}
}

// start starts the watchdog's goroutine unless the view went already.
func (w *panelWatchdog) start() {
	w.mu.Lock()
	ended := w.ended
	w.mu.Unlock()
	if !ended {
		go w.run()
	}
}

// panelSamples is how many heartbeats the watchdog posts in a window, and panelWaited how long one may wait before the engine counts as busy when it was posted.
const (
	panelSamples = 50
	panelWaited  = 4 * time.Millisecond
)

// share posts heartbeats for d, one at a time, panelSamples of them spread over it, and returns how busy the engine was over it: each heartbeat that waited more than panelWaited counts the time from its post to the next as busy, as the engine was at work when it was posted, so the share is about how much of d the engine was busy. It is false when the view went meanwhile.
func (w *panelWatchdog) share(d time.Duration) (float64, bool) {
	every := max(d/panelSamples, time.Millisecond)
	start := time.Now()
	var busy time.Duration
	done := make(chan time.Duration, 1)
	for time.Since(start) < d {
		posted := time.Now()
		w.r.Post(func() { done <- time.Since(posted) })
		var waited time.Duration
		select {
		case <-w.stop:
			return 0, false
		case waited = <-done:
		}
		if rest := every - time.Since(posted); rest > 0 {
			select {
			case <-w.stop:
				return 0, false
			case <-time.After(rest):
			}
		}
		if waited > panelWaited {
			busy += time.Since(posted)
		}
	}
	return float64(busy) / float64(time.Since(start)), true
}

// over reports whether the live heap is more than Heap over what it was as the view was built: the heap first, a collection only once it looks over.
func (w *panelWatchdog) over() bool {
	h := w.r.Heap
	return h > 0 && HeapBytes() > w.heap+h && LiveHeapBytes() > w.heap+h
}

// freed reports whether, with the view down, the live heap is back within Heap of what it was as the view was built: two collections, as what a runtime held may wait for a second one once its finalizers ran.
func (w *panelWatchdog) freed() bool {
	runtime.GC()
	return LiveHeapBytes() <= w.heap+w.r.Heap
}

// run is the watchdog until the view goes or the plugin is stopped.
func (w *panelWatchdog) run() {
	busyWindows, vain := 0, 0
	for {
		share, ok := w.share(w.watch.Window)
		if !ok {
			return
		}
		if share > w.watch.Share {
			busyWindows++
		} else {
			busyWindows = 0
		}
		memory := w.over()
		if busyWindows < w.watch.Windows && !memory {
			continue
		}
		busyWindows = 0
		if vain >= w.watch.Retries {
			continue
		}
		// Suspected: the view is taken down for the probe, which measures from when it is down, past what the engine had queued before.
		taken := make(chan struct{}, 1)
		w.post(func() {
			w.down(true)
			taken <- struct{}{}
		})
		select {
		case <-w.stop:
			return
		case <-taken:
		}
		share, ok = w.share(w.watch.Probe)
		if !ok {
			return
		}
		switch {
		case memory && w.freed():
			w.post(func() { w.r.stop(w.p, plugins.ReasonMemory) })
			return
		case !memory && share < w.watch.Idle:
			w.post(func() { w.r.stop(w.p, plugins.ReasonSlow) })
			return
		}
		// The load stayed without the view: not its doing. It is built again, measured from now.
		vain++
		w.heap = LiveHeapBytes()
		w.post(func() { w.down(false) })
	}
}

// post runs fn on the engine's goroutine unless the view went.
func (w *panelWatchdog) post(fn func()) {
	w.r.Post(func() {
		w.mu.Lock()
		ended := w.ended
		w.mu.Unlock()
		if !ended {
			fn()
		}
	})
}
