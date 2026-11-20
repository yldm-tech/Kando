package script

import (
	"errors"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// Runner calls the plugins' functions one call at a time, all plugins together, so a window slow to answer, or memory that grows, is the call's that is running (docs/plugin-system.md 6.6). tsx cuts each piece of synchronous JS at Limits' half second, but not a call that awaits between pieces, which can keep the window busy and its memory growing for ever; nor does a cut stop the plugin, whose module keeps what it holds. While a call runs, a watchdog posts a heartbeat to the engine's goroutine every Beat and reads the Go heap: when Stalls heartbeats in a row waited more than Stall, the call ran past Deadline, or the heap grew past Heap during the call, the runner closes the plugin on the engine's goroutine, which tsx does between two pieces, ends the call with ErrStopped and reports it (Stopped). The heap is the whole app's, so growth is only taken as the plugin's once a collection confirms it is live (runtime.GC, run only then): during a call, growth past Heap since the call started; across calls, a plugin whose own calls grew the heap by more than Heap in all, while the heap after a collection is still more than Heap over what it was as the plugin loaded, which a module that keeps what each short call makes reaches and a plugin that runs while the app itself grows does not. That check runs as a call ends, never before one, so a plugin is not stopped for what others did before it ran. While the app waits for the user on a plugin's behalf, or does work of its own for the plugins, as loading one, it pauses the watchdog of the call running (Pause): the deadline does not run, and the window's pauses are not the call's. It also stops a plugin whose calls failed too often: breakerFails of the last breakerCalls, a failure being an error that is not a core.Error with a code, which a plugin throws or rejects with on purpose, as for a service that does not answer. Each call is marked in flight in the plugins folder (plugins.MarkInFlight) until it ends, so a call that hung or crashed Kando turns its plugin off as Kando starts again.
//
// TODO(nsgo#836): tsx has no total time per call nor a way for the host to interrupt a plugin; stop it from inside tsx then https://github.com/nsgo-dev/nsgo/issues/836
// TODO(nsgo#760): tsx.Limits bounds no memory; the watchdog's heap threshold stands in for it https://github.com/nsgo-dev/nsgo/issues/760
type Runner struct {
	// Post runs fn on the engine's goroutine later, from any goroutine, without blocking.
	Post func(fn func())
	// Root is the plugins folder, where the in-flight marks are kept; "" keeps none.
	Root string
	// Beat, Stall, Stalls, Heap and Deadline are the watchdog's (DefaultWatch).
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
}

// DefaultWatch is the watchdog's: a heartbeat every 100 ms, three in a row that waited over 250 ms, 512 MiB of heap, a minute per call and five minutes of the app's waiting on its behalf.
var DefaultWatch = Watch{Beat: 100 * time.Millisecond, Stall: 250 * time.Millisecond, Stalls: 3, Heap: 512 << 20, Deadline: time.Minute, Paused: 5 * time.Minute}

// The circuit breaker: a plugin of which breakerFails of the last breakerCalls calls failed is stopped. A failure a plugin reports on purpose, an Error with a code that an export throws or rejects with, does not count.
const (
	breakerCalls = 50
	breakerFails = 3
)

// Plugin is a plugin loaded (Load) for the runner to call: its id and the content hash it was loaded from, which the health and the in-flight marks are kept for.
type Plugin struct {
	ID, Hash string
	p        *tsx.Plugin
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
	// mu guards what follows, which the engine's goroutine (Pause) and the watchdog share: how many pauses hold the call, since when, how long it was paused in all, and an epoch that moves at each pause and resume, so a heartbeat posted before is not counted.
	mu        sync.Mutex
	paused    int
	pausedAt  time.Time
	pausedFor time.Duration
	epoch     int
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

// Call calls the function name of p with args once the calls before it have ended, and has done hear what it returned, or why it failed, on the engine's goroutine. Call it on the engine's goroutine.
func (r *Runner) Call(p *Plugin, name string, args []any, done func(result any, err error)) {
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
		go r.watch(c, time.Now())
		c.p.p.Call(c.name, c.args, func(v any, err error) { r.end(c, v, err) })
	}
}

// end ends the call c with what it returned or why it failed, unless it ended already, as when the plugin was stopped; it then counts it for the breaker and starts the next call.
func (r *Runner) end(c *call, v any, err error) {
	if c.ended {
		return
	}
	c.ended = true
	close(c.stop)
	if r.Root != "" {
		plugins.ClearInFlight(r.Root, c.p.ID)
	}
	if r.running == c {
		r.running = nil
	}
	tripped := !errors.Is(err, ErrStopped) && c.p.count(err)
	kept := !errors.Is(err, ErrStopped) && r.keeps(c)
	c.done(v, err)
	switch {
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

// Pause pauses the watchdog of the call running while the app waits on its behalf, as for the user's answer, or does work of its own on the engine's goroutine, as loading a plugin: the call's deadline does not run, and the heartbeats posted meanwhile do not count against it. resume ends the pause; it may be called once. With no call running it does nothing. Call it on the engine's goroutine.
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
			}
			c.epoch++
			c.mu.Unlock()
		})
	}
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

// Close closes p, ending its calls with ErrStopped, without reporting it: the app closes a plugin it turns off, replaces or quits with. Call it on the engine's goroutine.
func (r *Runner) Close(p *Plugin) {
	if p.closed {
		return
	}
	p.closed = true
	p.p.Close()
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
	waiting, counted, stalls, postedEpoch := false, false, 0, 0
	trip := func(reason string) {
		r.Post(func() {
			if !c.ended {
				r.stop(c.p, reason)
			}
		})
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
			c.mu.Lock()
			switch {
			case waiting && !counted && now.Sub(posted) > w.Stall:
				// A heartbeat still waiting past Stall counts once, as soon as it is past, so an engine that never gets to it stops the plugin too.
				counted = true
				stalls++
			case !waiting:
				waiting, counted, posted, postedEpoch = true, false, now, epoch
				r.Post(func() {
					c.mu.Lock()
					defer c.mu.Unlock()
					if !counted && c.epoch == postedEpoch && c.paused == 0 {
						if time.Since(posted) > w.Stall {
							stalls++
						} else {
							stalls = 0
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
