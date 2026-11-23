package script

import (
	"errors"
	"sync"
	"time"

	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/tsx"
)

// The background watchdog (Runner.Watch's BgBeat, Drain, Burst, TotalDrain, TotalBurst) stops a plugin whose JS keeps the engine's goroutine, the window's, busy outside the calls the call watchdog watches: from timers and promise continuations its entry module or a call that returned set going, from its panels' views, and during a call the app paused for it (Pause, PauseFor). The runner keeps a registry of the plugins it holds (Hold adds one, Close removes it), each with the JS time of its runtime for its functions (tsx.Plugin.JSTime) and of its panels' views (panelWatchdog.jsTime), and what of it ran while a call of it ran unpaused, which the call watchdog watches; the rest is the plugin's unwatched JS. One goroutine, running while the registry holds a plugin and BgBeat is set, reads every BgBeat what each plugin's unwatched JS grew by, and pours it into the plugin's bucket, which leaks Drain of the time gone by: a bucket past Burst stops its plugin as slow, the way the call watchdog does (Interrupt, then Close on the engine's goroutine). With DefaultWatch a plugin that keeps the engine busy all the time is stopped in about 5 s, one busy 40% of the time in about 20 s, one busy 20% or less never, and a burst of 3 s, as a plugin indexing a film as it opens, is spared. A second bucket takes what all plugins' unwatched JS grew by and leaks TotalDrain: past TotalBurst it stops the plugin that took the most of it since that bucket was last empty, if that is at least totalShare of the time, and takes that plugin's share out of the bucket, so plugins that each stay under Drain cannot keep the engine busy together while one that does next to nothing is never the one stopped. The time between two readings is the wall clock's, the JS a reading adds is at most that time, and a reading more than twice BgBeat after the last one, as after the computer slept, is dropped.
//
// The registry also tells the call watchdog and the panels' what other plugins' JS took of the engine while they measure (othersJS), so that a heartbeat waiting on another plugin's JS does not count against the call running, nor the engine busy with it against a panel.
//
// TODO(nsgo#1838): JSTime leaves out the Go functions a plugin calls, so a plugin that keeps the engine busy in them from a timer is not seen here, its host calls held to 60 a minute alone; count that time in the buckets too once tsx reports the engine time a runtime takes. https://github.com/nsgo-dev/nsgo/issues/1838

// totalShare is the least share of the time since the all-plugins bucket was last empty that the plugin with the most of it must have taken for the bucket to stop it: a plugin that works now and then, as one that refreshes a panel every second, takes under 5%.
const totalShare = 0.1

// background is the runner's registry of the plugins it holds and the state of its background watchdog. mu guards it, and is taken after a call's mu and before a panel watchdog's.
type background struct {
	mu      sync.Mutex
	plugins map[*Plugin]*held
	// running is set while the sampler's goroutine runs, posted once its start is posted and not yet run; wake tells the goroutine the registry may be empty, and exited is closed as the goroutine ends.
	running bool
	posted  bool
	wake    chan struct{}
	exited  chan struct{}
	// last is when the last reading was taken, zero before the first; total is the all-plugins bucket, and span the time since it was last empty.
	last  time.Time
	total time.Duration
	span  time.Duration
	// left is what the plugins that left the registry ran from when they joined it, for othersJS: each plugin counts from what its JS was as it joined (held.joined) and keeps what it ran once it leaves, so that what others ran over a span is what they ran in it, however the registry changed meanwhile. A plugin that left runs no call any more, so it is never the p othersJS leaves out.
	left time.Duration
}

// held is a plugin in the registry: its panels' watchdogs, and what the JS of their builds gone ran (panelsGone); whether a call of it runs unpaused (watching), since its JS ran watchFrom, and what its JS ran in all while one did (watched). The sampler keeps what its unwatched JS was at the last reading (seen, last), its bucket, what it poured into the all-plugins bucket since that was last empty (share), and whether it is being stopped.
type held struct {
	p          *Plugin
	joined     time.Duration
	panels     map[*panelWatchdog]struct{}
	panelsGone time.Duration
	watching   bool
	watchFrom  time.Duration
	watched    time.Duration
	seen       bool
	last       time.Duration
	bucket     time.Duration
	share      time.Duration
	stopping   bool
}

// js is how long the plugin's JS ran in all, its functions' and its panels' views'. It reads no field of the plugin the engine's goroutine writes.
func (h *held) js() time.Duration {
	d, _ := h.p.p.JSTime()
	d += h.panelsGone
	for w := range h.panels {
		ran, _ := w.jsTime()
		d += ran
	}
	return d
}

// unwatched is how long the plugin's JS ran while no call of it ran unpaused, its JS read as js.
func (h *held) unwatched(js time.Duration) time.Duration {
	d := js - h.watched
	if h.watching {
		d -= js - h.watchFrom
	}
	return d
}

// hold adds p to the registry unless it is there, and has the sampler run. Call it on the engine's goroutine.
func (r *Runner) hold(p *Plugin) {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	if r.bg.plugins == nil {
		r.bg.plugins = map[*Plugin]*held{}
	}
	if r.bg.plugins[p] != nil {
		return
	}
	h := &held{p: p, panels: map[*panelWatchdog]struct{}{}}
	h.joined = h.js()
	r.bg.plugins[p] = h
	if r.Watch.BgBeat > 0 && !r.bg.running && !r.bg.posted && r.Post != nil {
		// Hold is called as a panel's view is built, where no goroutine starts.
		r.bg.posted = true
		r.Post(r.bgStart)
	}
}

// release takes p out of the registry, its share out of the all-plugins bucket, and has the sampler end once no plugin is left.
func (r *Runner) release(p *Plugin) {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	h := r.bg.plugins[p]
	if h == nil {
		return
	}
	delete(r.bg.plugins, p)
	r.bgLeft(h)
	r.bgTakeOut(h)
	if len(r.bg.plugins) == 0 && r.bg.wake != nil {
		select {
		case r.bg.wake <- struct{}{}:
		default:
		}
	}
}

// bgStart starts the sampler's goroutine, unless it runs or no plugin is held any more.
func (r *Runner) bgStart() {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	r.bg.posted = false
	if r.bg.running || len(r.bg.plugins) == 0 {
		return
	}
	r.bg.running = true
	r.bg.wake = make(chan struct{}, 1)
	r.bg.exited = make(chan struct{})
	r.bg.last = time.Time{}
	go r.bgRun(r.bg.wake, r.bg.exited)
}

// bgRun is the sampler, until the registry is empty.
func (r *Runner) bgRun(wake <-chan struct{}, exited chan<- struct{}) {
	defer close(exited)
	tick := time.NewTicker(r.Watch.BgBeat)
	defer tick.Stop()
	for {
		var now time.Time
		select {
		case now = <-tick.C:
		case <-wake:
			now = time.Now()
		}
		stops, ok := r.bgSample(now)
		if !ok {
			return
		}
		for _, h := range stops {
			r.bgStop(h)
		}
	}
}

// bgSample takes a reading at now and returns the plugins to stop; false once the registry is empty, when the sampler ends. A plugin whose runtime was closed without the runner (tsx.ErrPluginClosed) leaves the registry here.
func (r *Runner) bgSample(now time.Time) (stops []*held, ok bool) {
	w := r.Watch
	b := &r.bg
	b.mu.Lock()
	defer b.mu.Unlock()
	for p, h := range b.plugins {
		// A plugin whose owner went without closing it through the runner, as a test's engine as it ends, runs no JS any more.
		if errors.Is(p.p.Err(), tsx.ErrPluginClosed) {
			delete(b.plugins, p)
			r.bgLeft(h)
			r.bgTakeOut(h)
		}
	}
	if len(b.plugins) == 0 {
		b.running, b.wake = false, nil
		b.total, b.span = 0, 0
		return nil, false
	}
	elapsed := now.Sub(b.last)
	first := b.last.IsZero()
	b.last = now
	// A reading of each plugin moves its own last reading on, whether the interval counts or not.
	grew := make(map[*held]time.Duration, len(b.plugins))
	for _, h := range b.plugins {
		u := h.unwatched(h.js())
		d := u - h.last
		h.last = u
		if !h.seen {
			h.seen = true
			continue
		}
		grew[h] = d
	}
	if first || elapsed <= 0 || elapsed > 2*w.BgBeat {
		// The first reading, or one long after the last, as the computer slept: what it read is where the next counts from.
		return nil, true
	}
	var all time.Duration
	for h, d := range grew {
		d = min(max(d, 0), elapsed)
		all += d
		h.share += d
		h.bucket = max(h.bucket+d-time.Duration(w.Drain*float64(elapsed)), 0)
	}
	b.total += min(all, elapsed) - time.Duration(w.TotalDrain*float64(elapsed))
	b.span += elapsed
	if b.total <= 0 {
		r.bgEmpty()
	}
	for h := range grew {
		if w.Burst > 0 && h.bucket > w.Burst && !h.stopping {
			h.stopping = true
			stops = append(stops, h)
			r.bgTakeOut(h)
		}
	}
	if w.TotalBurst > 0 && b.total > w.TotalBurst {
		var top *held
		for _, h := range b.plugins {
			if !h.stopping && (top == nil || h.share > top.share) {
				top = h
			}
		}
		if top != nil && float64(top.share) >= totalShare*float64(b.span) {
			top.stopping = true
			stops = append(stops, top)
			r.bgTakeOut(top)
		} else {
			// None took enough to be the one: the bucket stays full, and does not run up meanwhile.
			b.total = w.TotalBurst
		}
	}
	return stops, true
}

// bgTakeOut takes what h poured into the all-plugins bucket since it was last empty out of it, as h is stopped or released.
func (r *Runner) bgTakeOut(h *held) {
	b := &r.bg
	b.total -= h.share
	h.share = 0
	if b.total <= 0 {
		r.bgEmpty()
	}
}

// bgEmpty empties the all-plugins bucket, which starts its span again.
func (r *Runner) bgEmpty() {
	b := &r.bg
	b.total, b.span = 0, 0
	for _, h := range b.plugins {
		h.share = 0
	}
}

// bgStop stops the plugin h as slow, from the sampler's goroutine, as the call watchdog does (watch's trip): its runtimes, its functions' and its panels' views', are interrupted, which cuts the JS running at once and drops their timers, then it is closed and reported on the engine's goroutine.
func (r *Runner) bgStop(h *held) {
	r.bg.mu.Lock()
	panels := make([]*panelWatchdog, 0, len(h.panels))
	for w := range h.panels {
		panels = append(panels, w)
	}
	r.bg.mu.Unlock()
	h.p.p.Interrupt(ErrStopped)
	for _, w := range panels {
		w.interrupt()
	}
	p := h.p
	r.Post(func() { r.stop(p, plugins.ReasonSlow) })
}

// bgWatch notes that a call of p starts or stops running unpaused, on for one that starts: its JS from then on is the call watchdog's, until it stops.
func (r *Runner) bgWatch(p *Plugin, on bool) {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	h := r.bg.plugins[p]
	if h == nil || h.watching == on {
		return
	}
	js := h.js()
	if on {
		h.watchFrom = js
	} else {
		h.watched += js - h.watchFrom
	}
	h.watching = on
}

// bgPanel adds the watchdog w of a panel's view of p to p's JS, or, with on false as the view goes, keeps what it ran and lets it go. Call it on the engine's goroutine.
func (r *Runner) bgPanel(p *Plugin, w *panelWatchdog, on bool) {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	h := r.bg.plugins[p]
	if h == nil {
		return
	}
	if on {
		h.panels[w] = struct{}{}
		return
	}
	if _, ok := h.panels[w]; ok {
		ran, _ := w.jsTime()
		h.panelsGone += ran
		delete(h.panels, w)
	}
}

// othersJS is how long the JS of the plugins but p ran since each joined the registry, those that left it included, from any goroutine: the difference of two readings is what of the engine's time a heartbeat of p's call, or a panel of p, waited for that is not p's, even when a plugin joined or left between them.
func (r *Runner) othersJS(p *Plugin) time.Duration {
	r.bg.mu.Lock()
	defer r.bg.mu.Unlock()
	var d time.Duration
	for q, h := range r.bg.plugins {
		if q != p {
			d += h.js() - h.joined
		}
	}
	return d + r.bg.left
}

// bgLeft keeps what h ran since it joined the registry, as it leaves it, for othersJS.
func (r *Runner) bgLeft(h *held) {
	r.bg.left += h.js() - h.joined
}
