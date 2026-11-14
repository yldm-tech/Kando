// Package testlocale pins the language tests read the interface in, which would otherwise follow the languages of the machine running them. NSGo has no process-wide language: a message says the language of the app its scope is in, so a test passes Current to the app it starts (Environment), and code outside a scope says Messages.
package testlocale

import (
	"sync/atomic"
	"testing"

	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

var current atomic.Value

// Pin makes tests read the interface in tag from now on. A TestMain calls it once, with locales.Default, so the tests read the interface as Kando shows it by default.
func Pin(tag string) { current.Store(tag) }

// Use makes tests read the interface in tag until t ends, for a test of another language: apps it starts after the call, and Messages. An app already running keeps its language; switch it with core.Scope.SetLocale. The package's TestMain must have called Pin.
func Use(t testing.TB, tag string) {
	t.Helper()
	prev, _ := current.Load().(string)
	if prev == "" {
		t.Fatal("testlocale.Use before testlocale.Pin")
	}
	current.Store(tag)
	t.Cleanup(func() { current.Store(prev) })
}

// Current is the language Pin or Use set, locales.Default before either.
func Current() string {
	tag, _ := current.Load().(string)
	if tag == "" {
		return locales.Default
	}
	return tag
}

// Messages are the catalogs' messages in Current, for what a test checks outside an app.
func Messages() locales.Messages { return locales.In(Current()) }

// Environment returns env in the locale Current, for nsgotest.Environment: the app started with it shows Current until it forces a locale of its own (core.Scope.SetLocale), as Kando does from its interface language setting.
func Environment(env core.Environment) core.Environment {
	env.Locale = Current()
	return env
}
