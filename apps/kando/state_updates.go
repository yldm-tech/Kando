package main

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/updater"
)

// updatesUI is how Kando checks for a newer version of itself when the user asks (updates.go); the checks in the background are the updater's own.
type updatesUI struct {
	// check checks at once and shows what it finds (updater.Check); tests replace it.
	check func(cx *core.Scope)
}

func newUpdatesUI() updatesUI { return updatesUI{check: updater.Check} }
