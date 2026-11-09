package main

import (
	"slices"
	"testing"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/i18n"
)

// TestAppRegistersTheBundleLanguages checks that main imports the locales catalog, so MyGo's own texts speak the same languages the bundle declares to AppKit (the catalogs', locales.Interface) instead of staying English while the About panel and the open panel follow the system.
func TestAppRegistersTheBundleLanguages(t *testing.T) {
	want := slices.Clone(locales.Interface)
	slices.Sort(want[1:])
	if got := i18n.Languages(); !slices.Equal(got, want) {
		t.Fatalf("languages = %q, want %q", got, want)
	}
}
