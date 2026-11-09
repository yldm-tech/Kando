// Package testlocale pins the language the catalogs speak in tests, which would otherwise follow the languages of the machine running them.
package testlocale

import (
	"sync/atomic"
	"testing"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/i18n"
)

var current atomic.Value

// Pin makes the catalogs speak tag from now on, in place of the app's language. A TestMain calls it once, with locales.Default, so the tests read the interface as Kando shows it by default.
func Pin(tag string) {
	current.Store(tag)
	i18n.SetLocaleFunc(func() string { return current.Load().(string) })
}

// Use makes the catalogs speak tag until t ends, for a test of another language. The package's TestMain must have called Pin.
func Use(t testing.TB, tag string) {
	t.Helper()
	prev, _ := current.Load().(string)
	if prev == "" {
		t.Fatal("testlocale.Use before testlocale.Pin")
	}
	current.Store(tag)
	t.Cleanup(func() { current.Store(prev) })
}

// Current is the language Pin or Use set.
func Current() string {
	tag, _ := current.Load().(string)
	if tag == "" {
		return locales.Default
	}
	return tag
}
