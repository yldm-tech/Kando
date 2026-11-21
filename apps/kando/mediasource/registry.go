package mediasource

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// ErrKindExists is Register's error for a kind whose ID the Registry already holds.
var ErrKindExists = errors.New("mediasource: a kind of that ID is registered")

// Registry is the kinds of media source Kando knows: the built-in ones, registered as the app starts, and those of script plugins, registered as a plugin loads and taken off as it stops or is removed. The interface reads it alone, so a plugin's kind is shown as a built-in one is. The zero Registry is empty and ready; its methods run on any goroutine.
type Registry struct {
	mu    sync.RWMutex
	kinds []Kind
}

// Register adds k, which must be valid (Kind.Validate) and of an ID not registered yet (ErrKindExists).
func (r *Registry) Register(k Kind) error {
	if err := k.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.kinds, func(have Kind) bool { return have.ID == k.ID }) {
		return fmt.Errorf("%w: %q", ErrKindExists, k.ID)
	}
	k.Fields = slices.Clone(k.Fields)
	r.kinds = append(r.kinds, k)
	return nil
}

// Unregister takes off the kind of ID id, reporting whether there was one. The sources of that kind stay in the settings, so the films imported from them keep their source; they cannot be opened until the kind is registered again.
func (r *Registry) Unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.kinds)
	r.kinds = slices.DeleteFunc(r.kinds, func(k Kind) bool { return k.ID == id })
	return len(r.kinds) != n
}

// UnregisterPlugin takes off every kind the plugin of ID plugin contributed, as it stops or is removed, and says how many.
func (r *Registry) UnregisterPlugin(plugin string) int {
	if plugin == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.kinds)
	r.kinds = slices.DeleteFunc(r.kinds, func(k Kind) bool { return k.Origin.Plugin == plugin })
	return n - len(r.kinds)
}

// Kind is the kind of ID id, reporting false when none is registered.
func (r *Registry) Kind(id string) (Kind, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, k := range r.kinds {
		if k.ID == id {
			return k, true
		}
	}
	return Kind{}, false
}

// Kinds are the kinds registered, the built-in ones first and then the plugins', each in the order they were registered, as the form that adds a source lists them.
func (r *Registry) Kinds() []Kind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Kind, 0, len(r.kinds))
	for _, k := range r.kinds {
		if k.Origin.Builtin() {
			out = append(out, k)
		}
	}
	for _, k := range r.kinds {
		if !k.Origin.Builtin() {
			out = append(out, k)
		}
	}
	return out
}
