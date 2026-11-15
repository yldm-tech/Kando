package film

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Timer runs one function after a while under an owner, each Set replacing the one before. Every Set runs in a scope of its own, which the next Set or Stop disposes, so a timer set again and again, as on every report of the player or every move of the pointer, leaves the owner as it was: Scope.AfterFunc alone adds a cleanup to its owner on each call, which stays until the owner goes.
type Timer struct {
	owner *core.Scope
	cur   *core.Scope
	at    time.Time
}

// NewTimer is a Timer whose function runs under owner and stops when owner is disposed.
func NewTimer(owner *core.Scope) *Timer { return &Timer{owner: owner} }

// Set runs fn after d, unless Set or Stop is called first.
func (t *Timer) Set(d time.Duration, fn func()) {
	t.Stop()
	t.at = t.owner.Now().Add(d)
	t.owner.Run(func() {
		cur := t.owner.Child()
		t.cur = cur
		cur.Run(func() {
			cur.AfterFunc(d, func() {
				if t.cur == cur {
					t.at = time.Time{}
				}
				fn()
			})
		})
	})
}

// Stop keeps the function set last from running.
func (t *Timer) Stop() {
	if t.cur != nil {
		t.cur.Dispose()
		t.cur = nil
	}
	t.at = time.Time{}
}

// At is when the function set last runs, or zero once it ran or was stopped.
func (t *Timer) At() time.Time { return t.at }
