package main

import (
	"slices"
	"testing"

	"kando/apps/kando/locales"
)

// TestAppRegistersTheBundleLanguages checks that the catalogs the app is built with are the languages of its interface (locales.Interface), which nsgo build declares to the platform, so NSGo's own texts, the About panel and the open panel speak the languages the interface does instead of staying English.
func TestAppRegistersTheBundleLanguages(t *testing.T) {
	want := slices.Clone(locales.Interface)
	slices.Sort(want[1:])
	if got := locales.Catalog.Languages(); !slices.Equal(got, want) {
		t.Fatalf("languages = %q, want %q", got, want)
	}
}
