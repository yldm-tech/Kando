package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/i18n"
)

// TestAppRegistersTheBundleLanguages checks that main imports the locales catalog, so MyGo's own texts speak the same languages the bundle declares to AppKit (en and zh-Hans) instead of staying English while the About panel and the open panel follow the system.
func TestAppRegistersTheBundleLanguages(t *testing.T) {
	if got, want := i18n.Languages(), []string{"en", "zh-Hans"}; !slices.Equal(got, want) {
		t.Fatalf("languages = %q, want %q", got, want)
	}
}
