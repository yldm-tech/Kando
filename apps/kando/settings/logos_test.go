package settings

import (
	"testing"

	ailogo "github.com/yldm-tech/ai-logo/nsgo"
)

// TestProviderMarksResolve checks that every brand preset draws a mark, that the generic one draws none, and that a brand the collection has in color parses in color. AI Logo's NSGo module lists a logo's variants from the mono mark up (Mono, Color, Text, …), where the MyGo module listed them from the brand mark down, so the test looks Color up rather than relying on its place.
func TestProviderMarksResolve(t *testing.T) {
	if providerMark("custom") != nil {
		t.Fatal("the generic preset has a brand mark")
	}
	for preset, logo := range providerLogos {
		if providerMark(preset) == nil {
			t.Fatalf("%s (%s) has no mark", preset, logo)
		}
		if providerMark(preset) != providerMark(preset) {
			t.Fatalf("%s is parsed again on each call", preset)
		}
		variants, err := ailogo.Variants(logo)
		if err != nil {
			t.Fatal(err)
		}
		for _, variant := range variants {
			if variant != ailogo.Color {
				continue
			}
			if _, err := ailogo.Parse(logo, ailogo.Color); err != nil {
				t.Fatalf("%s color mark: %v", logo, err)
			}
		}
	}
}
