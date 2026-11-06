package settings

import (
	"testing"

	ailogo "github.com/yldm-tech/ai-logo/mygo"
)

func TestProviderMarksResolve(t *testing.T) {
	if providerMark("custom") != nil {
		t.Fatal("the generic preset has a brand mark")
	}
	for preset, logo := range providerLogos {
		if providerMark(preset) == nil {
			t.Fatalf("%s (%s) has no mark", preset, logo)
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
