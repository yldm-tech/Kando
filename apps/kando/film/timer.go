package film

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Timer runs one function after a while under an owner, each Set replacing the one before. A timer set again and again, as on every report of the player or every move of the pointer, leaves the owner as it was, as Scope.AfterFunc keeps nothing on its owner once its timer has fired or been stopped.
type Timer struct {
	owner *core.Scope
	// stop stops the timer set last, nil once it ran or was stopped; gen numbers the Sets, so a function that runs after another Set leaves that one's time alone.
	stop func() bool
	gen  int
	at   time.Time
}

// NewTimer is a Timer whose function runs under owner and stops when owner is disposed.
func NewTimer(owner *core.Scope) *Timer { return &Timer{owner: owner} }

// Set runs fn after d, unless Set or Stop is called first.
func (t *Timer) Set(d time.Duration, fn func()) {
	t.Stop()
	t.at = t.owner.Now().Add(d)
	t.gen++
	gen := t.gen
	// AfterFunc takes the owner running now, so the timer is set under the Timer's owner whatever handler sets it.
	t.owner.Run(func() {
		t.stop = t.owner.AfterFunc(d, func() {
			if t.gen == gen {
				t.stop, t.at = nil, time.Time{}
			}
			fn()
		})
	})
}

// Stop keeps the function set last from running.
func (t *Timer) Stop() {
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
	t.at = time.Time{}
}

// At is when the function set last runs, or zero once it ran or was stopped.
func (t *Timer) At() time.Time { return t.at }
