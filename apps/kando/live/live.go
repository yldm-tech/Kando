// Package live connects Kando's state to NSGo's reactive graph. Kando keeps its state the way it did under MyGo: plain fields on the app, its areas' structs and the nodra store, changed in place by the handlers and by background jobs reporting through the app's update. NSGo builds a component once and updates only the bindings that read a signal that changed, so this package puts one revision signal over all of that state: a binding reads the state through Func, which subscribes it to the revision, and whatever changes the state says so with Changed, which runs every such binding again. A binding whose value came out the same changes nothing on screen, and nothing is built again, so the keyboard focus, scroll positions and what a text field holds stay where they are.
//
// Every change has to be followed by Changed, on the graph's goroutine:
//   - a handler a component runs (a press, a typed character, a menu item) is wrapped in Do or On, which run it and then Changed;
//   - a background goroutine hands its report to Rev.Post, which runs it on the graph's goroutine and then Changed;
//   - the nodra store reaches the revision through nodra.InvalidateTarget, as Rev is an InvalidationTarget.
//
// A getter that Func wraps only reads: it never changes the state or calls Changed, which would run the bindings again while they run.
package live

import (
	"sync"

	"github.com/nsgo-dev/nsgo/core"
)

// State is what a component reads the app's state through and tells about changes. Each area's App interface embeds it, and the root package's app implements it with its Rev.
type State interface {
	// Track subscribes the running binding to every later Changed. Outside a binding it does nothing.
	Track()
	// Changed runs again every binding that called Track. It runs on the graph's goroutine, from a handler or a report Rev.Post ran.
	Changed()
}

// Func returns get as a binding of s: each call tracks s first, so a ...Func field, a Show's When or an Each's items that call it run again after every Changed.
func Func[T any](s State, get func() T) func() T {
	return func() T {
		s.Track()
		return get()
	}
}

// Memo is Func behind a memo of cx's: what reads it runs again only when get's value changes, not after every Changed, for a value many bindings read or one that is costly to follow, as a list's items are.
func Memo[T comparable](cx *core.Scope, s State, get func() T) func() T {
	return core.NewMemo(cx, Func(s, get)).Get
}

// Do returns fn as a handler that tells s once it ran: the OnPress of a ui.Button, a menu item's OnSelect.
func Do(s State, fn func()) func() {
	return func() {
		fn()
		s.Changed()
	}
}

// On is Do for a handler that takes a value: a field's OnChange, a core.View's or base.Button's OnPress with its core.PressEvent, a Sidebar's OnSelect.
func On[T any](s State, fn func(T)) func(T) {
	return func(v T) {
		fn(v)
		s.Changed()
	}
}

// Rev is the app's revision: the one signal State's methods read and write. Its zero value is ready for use and does nothing until Attach gives it the graph, so the app can be made, and its settings and library read, before nsgo.Run starts the engine.
type Rev struct {
	mu      sync.Mutex
	cx      *core.Scope
	sig     *core.Signal[struct{}]
	pending []func()
}

// Attach makes the revision a signal of cx's graph, cx being the app's own scope or its main view's, and runs the reports Post held while there was no graph. It runs once, on the graph's goroutine; a second call does nothing.
func (r *Rev) Attach(cx *core.Scope) {
	r.mu.Lock()
	if r.cx != nil {
		r.mu.Unlock()
		return
	}
	r.cx = cx
	// No equal function: every Set notifies, as the value itself never changes.
	r.sig = core.NewSignalFunc(cx, struct{}{}, nil)
	pending := r.pending
	r.pending = nil
	r.mu.Unlock()
	for _, fn := range pending {
		cx.Post(func() {
			fn()
			r.Changed()
		})
	}
}

// Track subscribes the running binding to the revision.
func (r *Rev) Track() {
	if r.sig != nil {
		r.sig.Get()
	}
}

// Changed runs again every binding that read the revision.
func (r *Rev) Changed() {
	if r.sig != nil {
		r.sig.Set(struct{}{})
	}
}

// Post runs fn on the graph's goroutine, then Changed. Any goroutine may call it; before Attach it holds fn until the graph exists. fn does not run once the scope Attach was given is disposed, as when the app quits.
func (r *Rev) Post(fn func()) {
	r.mu.Lock()
	cx := r.cx
	if cx == nil {
		r.pending = append(r.pending, fn)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	cx.Post(func() {
		fn()
		r.Changed()
	})
}

// Invalidate is Changed from any goroutine, through Post: nodra.InvalidateTarget calls it when the store publishes.
func (r *Rev) Invalidate() { r.Post(func() {}) }

// Attached reports whether Attach gave the revision its graph.
func (r *Rev) Attached() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cx != nil
}
