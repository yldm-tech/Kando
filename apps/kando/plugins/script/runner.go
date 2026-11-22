package script

import (
	"cmp"
	"errors"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// Runner calls the plugins' functions one call at a time, all plugins together, so a window slow to answer, or memory that grows, is the call's that is running (docs/plugin-system.md 6.6). tsx cuts each piece of synchronous JS at Limits' half second, but not a call that awaits between pieces, which can keep the window busy and its memory growing for ever; nor does a cut stop the plugin, whose module keeps what it holds. While a call runs, a watchdog posts a heartbeat to the engine's goroutine every Beat and reads the Go heap: when Stalls heartbeats in a row waited more than Stall, but for what other plugins' JS ran while each waited (tsx.Plugin.JSTime, which the registry of the plugins held reads, background.go), so another plugin's load neither counts against the call nor starts its count again, the call ran past Deadline, or the heap grew past Heap during the call, the watchdog interrupts the plugin from its own goroutine (tsx.Plugin.Interrupt), which cuts the piece of JS running at once and drops its timers, then the runner closes it on the engine's goroutine, ends the call with ErrStopped and reports it (Stopped). The heap is the whole app's, so growth is only taken as the plugin's once a collection confirms it is live (runtime.GC, run only then): during a call, growth past Heap since the call started; across calls, a plugin whose own calls grew the heap by more than Heap in all, while the heap after a collection is still more than Heap over what it was as the plugin loaded, which a module that keeps what each short call makes reaches and a plugin that runs while the app itself grows does not. That check runs as a call ends, never before one, so a plugin is not stopped for what others did before it ran. While the app waits for the user on a plugin's behalf, or does work of its own for the plugins, as loading one, it pauses the watchdog of the call running (Pause): the deadline does not run, and the window's pauses are not the call's. It also stops a plugin whose calls failed too often: breakerFails of the last breakerCalls, a failure being an error that is not a core.Error with a code, which a plugin throws or rejects with on purpose, as for a service that does not answer; and a plugin whose runtime tsx itself stopped, as at its node or memory quota, as soon as tsx says so (halt). Outside its calls, and while a call of it is paused, a plugin's JS is the background watchdog's (background.go), which stops a plugin whose timers, promise continuations or panels' views keep the window busy too long, alone or with other plugins. Each call is marked in flight in the plugins folder (plugins.MarkInFlight) until it ends, so a call that hung or crashed Kando turns its plugin off as Kando starts again.
//
// The runner does not set tsx's Limits.CallTotal, nor call through Plugin.CallWithBudget: CallTotal holds what the entry module sets going to Limits.Load, a second, and what a call sets going, its timers included, to the call's deadline even after the call returned, which plugins may rely on running on; and a call's budget can only be extended, ahead of its deadline and between two pieces of JS, never paused, while Deadline is paused (Pause, PauseFor) for as long as the app serves the plugin, as for a sound read or an LLM answer of up to two minutes. Nor does it measure a call by its own tsx.Plugin.JSTime: that is the time inside the plugin's JS entries, which leaves out what a call keeps the window from outside them, as the getters of its exports object, which tsx runs as it looks an export up (Plugin.Call, Plugin.Exports), and the Go functions it calls; a heartbeat that waits is the window not answering, whatever holds it. Other plugins' JSTime it does read, to take out of a heartbeat's wait what was theirs: what it leaves out of them stays the call's, which only errs towards stopping the call, as before. The background watchdog measures by JSTime alone, as no call holds the window there to post heartbeats for.
//
// TODO(nsgo#1060): Limits keeps tsx.DefaultLimits' Memory (64 MiB, nsgo#760), but tsx counts only what the built-ins that allocate by a size hold (tsx/memory_goja.go:20, tsx/sandbox.go:49), not the objects, closures and arrays a bundle builds, nor what concat, flat, slice, spreads, replace and string concatenation copy, so a bundle written to exhaust memory still can and the watchdog's heap threshold stays the bound for that; drop it once tsx bounds a runtime's whole heap https://github.com/nsgo-dev/nsgo/issues/1060
type Runner struct {
	// Post runs fn on the engine's goroutine later, from any goroutine, without blocking.
	Post func(fn func())
	// Root is the plugins folder, where the in-flight marks are kept; "" keeps none.
	Root string
	// Beat, Stall, Stalls, Heap and Deadline are the call watchdog's, BgBeat, Drain, Burst, TotalDrain and TotalBurst the background watchdog's (DefaultWatch).
	Watch
	// Stopped hears, on the engine's goroutine, that the runner stopped the plugin id, for its content hash, and why (plugins.ReasonSlow, ReasonMemory, ReasonFailed).
	Stopped func(id, hash, reason string)

	// PanelWatch is the watchdog of the panels' views (Panel), DefaultPanelWatch when zero.
	PanelWatch PanelWatch

	queue   []*call
	running *call
	// marks are the in-flight marks of the panels' views to write, in order, on one goroutine (mark), which closes marksDone as it ends; drained is set once Drain stopped it, for good. marksMu guards them.
	marksMu   sync.Mutex
	marks     chan func()
	marksDone chan struct{}
	drained   bool

	// bg is the registry of the plugins held and the background watchdog (background.go).
	bg background
}

// Watch is when the watchdog stops a plugin.
type Watch struct {
	// Beat is how often a heartbeat is posted; Stall how long one may wait before it counts, and Stalls how many that waited too long in a row stop the plugin.
	Beat, Stall time.Duration
	Stalls      int
	// Heap is how many bytes the Go heap may grow past what it was before the plugin loaded while a call of it runs.
	Heap uint64
	// Deadline is the longest a call may take, its awaits included, but for the time the app waits on its behalf (Pause, PauseFor); Paused is the longest the app may wait on its behalf in all during one call, so a call that awaits host calls in a loop does not hold every other plugin's calls for ever.
	Deadline time.Duration
	Paused   time.Duration
	// BgBeat is how often the background watchdog reads what each plugin's JS ran outside the calls watched, zero for no background watchdog (background.go). Drain is the share of the time gone by that a plugin's bucket leaks, and Burst how much of its JS it holds before the plugin is stopped, zero for no such bucket; TotalDrain and TotalBurst are the same for the bucket of all plugins together.
	BgBeat            time.Duration
	Drain, TotalDrain float64
	Burst, TotalBurst time.Duration
}

// DefaultWatch is the watchdog's: a heartbeat every 100 ms, three in a row that waited over 250 ms, 512 MiB of heap, a minute per call and five minutes of the app's waiting on its behalf; outside the calls, a reading every half second, each plugin's bucket leaking 20% of the time and holding 4 s, all plugins' bucket leaking 40% and holding 4 s.
var DefaultWatch = Watch{Beat: 100 * time.Millisecond, Stall: 250 * time.Millisecond, Stalls: 3, Heap: 512 << 20, Deadline: time.Minute, Paused: 5 * time.Minute,
	BgBeat: 500 * time.Millisecond, Drain: 0.2, Burst: 4 * time.Second, TotalDrain: 0.4, TotalBurst: 4 * time.Second}

// The circuit breaker: a plugin of which breakerFails of the last breakerCalls calls failed is stopped. A failure a plugin reports on purpose, an Error with a code that an export throws or rejects with, does not count.
const (
	breakerCalls = 50
	breakerFails = 3
)

// Plugin is a plugin loaded (Load) for the runner to call: its id and the content hash it was loaded from, which the health and the in-flight marks are kept for. runner is the Runner that holds it (Hold), which hears when tsx stops it (halt).
type Plugin struct {
	ID, Hash string
	p        *tsx.Plugin
	runner   *Runner
	heap     uint64
	grown    uint64
	closed   bool
	outcomes []bool
}

// Exports are the functions the plugin exports.
func (p *Plugin) Exports() []string { return p.p.Exports() }

// Closed reports whether the plugin was closed.
func (p *Plugin) Closed() bool { return p.closed }

// call is one call waiting or running.
type call struct {
	p    *Plugin
	name string
	args []any
	done func(any, error)
	// ended is set once done heard it; stop ends the watchdog. heap is the Go heap as the call started.
	ended bool
	stop  chan struct{}
	heap  uint64
	// mu guards what follows, which the engine's goroutine (Pause) and the watchdog share: how many pauses hold the call, since when, how long it was paused in all, an epoch that moves at each pause and resume, so a heartbeat posted before is not counted, and whether the call is over, after which a pause or resume no longer tells the background watchdog anything.
	mu        sync.Mutex
	paused    int
	pausedAt  time.Time
	pausedFor time.Duration
	epoch     int
	over      bool
}

// HeapBytes is what the Go heap holds now, the measure Heap is in.
func HeapBytes() uint64 {
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return s[0].Value.Uint64()
}

// Hold has r stop p as soon as tsx stops its runtime (halt), as in a timer between its calls, rather than when a call of it fails next, and watch its JS outside its calls (background.go) until it is closed. Call and Panel hold the plugin they are given; the app holds each plugin as it loads it. Holding it again does nothing more; a plugin closed is not held. Call it on the engine's goroutine.
func (r *Runner) Hold(p *Plugin) {
	p.runner = r
	if !p.closed {
		r.hold(p)
	}
}

// Call calls the function name of p with args once the calls before it have ended, and has done hear what it returned, or why it failed, on the engine's goroutine. Call it on the engine's goroutine.
func (r *Runner) Call(p *Plugin, name string, args []any, done func(result any, err error)) {
	r.Hold(p)
	if p.closed {
		done(nil, ErrStopped)
		return
	}
	r.queue = append(r.queue, &call{p: p, name: name, args: args, done: done})
	if r.running == nil {
		r.next()
	}
}

// Busy reports whether a call runs or waits.
func (r *Runner) Busy() bool { return r.running != nil || len(r.queue) > 0 }

// next starts the next call waiting, if none runs.
func (r *Runner) next() {
	for r.running == nil && len(r.queue) > 0 {
		c := r.queue[0]
		r.queue = r.queue[1:]
		if c.p.closed {
			c.done(nil, ErrStopped)
			continue
		}
		r.running = c
		c.heap = HeapBytes()
		if r.Root != "" {
			plugins.MarkInFlight(r.Root, c.p.ID, c.p.Hash)
		}
		c.stop = make(chan struct{})
		r.bgWatch(c.p, true)
		go r.watch(c, time.Now())
		c.p.p.Call(c.name, c.args, func(v any, err error) { r.end(c, v, err) })
	}
}

// end ends the call c with what it returned or why it failed, unless it ended already, as when the plugin was stopped; it then counts it for the breaker, stops a plugin tsx stopped (halt), and starts the next call.
func (r *Runner) end(c *call, v any, err error) {
	if c.ended {
		return
	}
	c.ended = true
	close(c.stop)
	c.mu.Lock()
	c.over = true
	watched := c.paused == 0
	c.mu.Unlock()
	if watched {
		r.bgWatch(c.p, false)
	}
	if r.Root != "" {
		plugins.ClearInFlight(r.Root, c.p.ID)
	}
	if r.running == c {
		r.running = nil
	}
	ours := errors.Is(err, ErrStopped)
	tripped := !ours && c.p.count(err)
	kept := !ours && r.keeps(c)
	c.done(v, err)
	switch {
	case r.halt(c.p):
	case tripped && !c.p.closed:
		r.stop(c.p, plugins.ReasonFailed)
	case kept && !c.p.closed:
		r.stop(c.p, plugins.ReasonMemory)
	}
	r.next()
}

// keeps adds what the call c grew the heap by to its plugin's growth, and reports whether the plugin keeps more than Heap: its calls grew the heap by more in all, and after a collection the heap is still more than Heap over what it was as the plugin loaded. A plugin whose growth was garbage starts counting again.
func (r *Runner) keeps(c *call) bool {
	if r.Heap == 0 {
		return false
	}
	if now := HeapBytes(); now > c.heap {
		c.p.grown += now - c.heap
	}
	if c.p.grown <= r.Heap {
		return false
	}
	if LiveHeapBytes() > c.p.heap+r.Heap {
		return true
	}
	c.p.grown = 0
	return false
}

// overCall reports whether the heap grew by more than Heap since the call c started, and still has after a collection.
func (r *Runner) overCall(c *call) bool {
	return r.Heap > 0 && HeapBytes() > c.heap+r.Heap && LiveHeapBytes() > c.heap+r.Heap
}

// LiveHeapBytes is what the Go heap holds once a collection took what nothing reaches. A collection takes tens of milliseconds on a heap of hundreds of megabytes, so the watchdog runs one only once the heap already looks past its threshold.
func LiveHeapBytes() uint64 {
	runtime.GC()
	return HeapBytes()
}

// Pause pauses the watchdog of the call running while the app waits on its behalf, as for the user's answer, or does work of its own on the engine's goroutine, as loading a plugin: the call's deadline does not run, and the heartbeats posted meanwhile do not count against it. What the plugin's JS runs meanwhile, as in its timers, is the background watchdog's. resume ends the pause; it may be called once. With no call running it does nothing. Call it on the engine's goroutine.
func (r *Runner) Pause() (resume func()) {
	return r.pause(r.running)
}

// PauseFor is Pause for a wait on behalf of the plugin p alone, as for a host call it made: it pauses the call running only when it is p's. A host call a plugin made outside its own call, as from a timer after the call ended, pauses no other plugin's call. Call it on the engine's goroutine.
func (r *Runner) PauseFor(p *Plugin) (resume func()) {
	if c := r.running; c != nil && c.p == p {
		return r.pause(c)
	}
	return func() {}
}

// pause pauses the watchdog of the call c, nil for none.
func (r *Runner) pause(c *call) (resume func()) {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	if c.paused == 0 {
		c.pausedAt = time.Now()
		if !c.over {
			r.bgWatch(c.p, false)
		}
	}
	c.paused++
	c.epoch++
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.paused--
			if c.paused == 0 {
				c.pausedFor += time.Since(c.pausedAt)
				if !c.over {
					r.bgWatch(c.p, true)
				}
			}
			c.epoch++
			c.mu.Unlock()
		})
	}
}

// halt stops p, and reports whether it did, once tsx stopped its runtime for a reason of its own (tsx.Plugin.Err), as at its node quota (Limits.Nodes) or its memory quota (Limits.Memory): every call of it after would fail at once with that quota's core.Error, whose code the breaker does not count, so without this the plugin would stay on, each call failing. Its memory quota is plugins.ReasonMemory, any other reason plugins.ReasonFailed; a plugin that runs, or that Kando stopped or closed itself (ErrStopped, tsx.ErrPluginClosed), it leaves alone. It reads what tsx keeps of the runtime and runs none of the plugin's JS, which a lookup of a name of its exports object would. The runner asks as each call ends and as tsx stops the runtime (Load's tsx.Sandbox.OnStop), so a plugin stopped between its calls, as in a timer, is stopped then. Call it on the engine's goroutine.
func (r *Runner) halt(p *Plugin) bool {
	if p.closed {
		return false
	}
	err := p.p.Err()
	if err == nil || errors.Is(err, ErrStopped) || errors.Is(err, tsx.ErrPluginClosed) {
		return false
	}
	r.stop(p, cmp.Or(quotaReason(err), plugins.ReasonFailed))
	return true
}

// quotaReason is the reason of plugins a runtime tsx stopped at a quota with err is stopped for, "" when err is no such quota's: a quota tsx stops a runtime at fails with a core.Error that names the quota's resource, which an error a plugin throws or rejects with never does, as tsx keeps only its code and message (tsx/view.go rejection), so a plugin that reports a quota of its own is not taken for one tsx stopped. Its memory quota is ReasonMemory, any other ReasonFailed.
func quotaReason(err error) string {
	var coded *core.Error
	if !errors.As(err, &coded) || coded.Code != core.CodeQuotaExceeded || coded.Resource == "" {
		return ""
	}
	if coded.Resource == "memory" {
		return plugins.ReasonMemory
	}
	return plugins.ReasonFailed
}

// count counts the outcome of a call of p, err nil for one that succeeded, and reports whether the breaker trips.
func (p *Plugin) count(err error) bool {
	var coded *core.Error
	failed := err != nil && !(errors.As(err, &coded) && coded.Code != "")
	p.outcomes = append(p.outcomes, failed)
	if len(p.outcomes) > breakerCalls {
		p.outcomes = p.outcomes[len(p.outcomes)-breakerCalls:]
	}
	n := 0
	for _, f := range p.outcomes {
		if f {
			n++
		}
	}
	return n >= breakerFails
}

// Close closes p, ending its calls with ErrStopped, without reporting it: the app closes a plugin it turns off, replaces or quits with. It is no longer held (Hold); once no plugin is, the background watchdog's goroutine ends. Call it on the engine's goroutine.
func (r *Runner) Close(p *Plugin) {
	if p.closed {
		return
	}
	p.closed = true
	r.release(p)
	if c := r.running; c != nil && c.p == p {
		r.end(c, nil, ErrStopped)
	}
	var keep []*call
	for _, c := range r.queue {
		if c.p == p {
			c.done(nil, ErrStopped)
			continue
		}
		keep = append(keep, c)
	}
	r.queue = keep
	// tsx fails the calls still waiting on the plugin as it closes it, saying it was closed; they have ended with ErrStopped already, so what tsx says is dropped.
	p.p.Close()
}

// stop closes p for reason and reports it.
func (r *Runner) stop(p *Plugin, reason string) {
	if p.closed {
		return
	}
	r.Close(p)
	if r.Stopped != nil {
		r.Stopped(p.ID, p.Hash, reason)
	}
}

// watch is the watchdog of the call c, which started at start, until it ends.
func (r *Runner) watch(c *call, start time.Time) {
	w := r.Watch
	tick := time.NewTicker(w.Beat)
	defer tick.Stop()
	var posted time.Time
	// postedOthers is what other plugins' JS ran in all (othersJS) as the heartbeat waiting was posted: a heartbeat counts only for what it waited past that, so another plugin's JS does not count against the call, and does not start its count again either, which would let another plugin hide this one's stalls.
	var postedOthers time.Duration
	waiting, counted, stalls, postedEpoch := false, false, 0, 0
	// trip interrupts the plugin unless the call ended meanwhile, which cuts the JS running at once rather than between two pieces, its timers dropped and its call failed with ErrStopped, then closes and reports it on the engine's goroutine.
	trip := func(reason string) {
		select {
		case <-c.stop:
			return
		default:
		}
		c.p.p.Interrupt(ErrStopped)
		r.Post(func() { r.stop(c.p, reason) })
	}
	for {
		select {
		case <-c.stop:
			return
		case now := <-tick.C:
			c.mu.Lock()
			paused, epoch, ran := c.paused > 0, c.epoch, c.pausedFor
			if paused {
				ran += now.Sub(c.pausedAt)
			}
			if paused || postedEpoch != epoch {
				// Paused, or a pause came and went since the heartbeat was posted: what the window waited for meanwhile is not the call's.
				stalls, counted = 0, waiting
			}
			c.mu.Unlock()
			if w.Paused > 0 && ran > w.Paused {
				// The app waited on the call's behalf too long in all, as for a call that awaits host calls in a loop.
				trip(plugins.ReasonSlow)
				return
			}
			if paused {
				continue
			}
			if r.overCall(c) {
				trip(plugins.ReasonMemory)
				return
			}
			if w.Deadline > 0 && now.Sub(start)-ran > w.Deadline {
				trip(plugins.ReasonSlow)
				return
			}
			others := r.othersJS(c.p)
			c.mu.Lock()
			switch {
			case waiting && !counted && now.Sub(posted)-(others-postedOthers) > w.Stall:
				// A heartbeat still waiting past Stall, but for what other plugins' JS took meanwhile, counts once, as soon as it is past, so an engine that never gets to it stops the plugin too.
				counted = true
				stalls++
			case !waiting:
				waiting, counted, posted, postedEpoch, postedOthers = true, false, now, epoch, others
				r.Post(func() {
					others := r.othersJS(c.p)
					c.mu.Lock()
					defer c.mu.Unlock()
					if !counted && c.epoch == postedEpoch && c.paused == 0 {
						switch waited := time.Since(posted); {
						case waited <= w.Stall:
							stalls = 0
						case waited-(others-postedOthers) > w.Stall:
							stalls++
						}
					}
					waiting = false
				})
			}
			over := stalls >= w.Stalls
			c.mu.Unlock()
			if over {
				trip(plugins.ReasonSlow)
				return
			}
		}
	}
}
