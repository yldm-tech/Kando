package settings

import (
	"testing"

	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
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

// TestMonoMarkFollowsTheAppearance checks that a service whose mark has no color version, OpenAI's, is drawn in the text's color: dark on the light window and light on the dark one, not black on both.
func TestMonoMarkFollowsTheAppearance(t *testing.T) {
	if _, err := ailogo.Parse("openai", ailogo.Color); err == nil {
		t.Fatal("OpenAI's mark has a color version; pick a mono-only brand for this test")
	}
	a := newFakeApp()
	a.section, a.provider = SectionAI, "openai"
	app := start(t, a)
	inks := func() []core.Color {
		var out []core.Color
		for _, op := range app.Find("id:settings-llm-logo").Ops {
			if op.Color != 0 {
				out = append(out, core.Color(op.Color))
			}
		}
		if len(out) == 0 {
			t.Fatalf("the mark draws nothing\n%s", app.Dump())
		}
		return out
	}
	for _, c := range inks() {
		if r, g, b, _ := c.Components(); r > 0.5 || g > 0.5 || b > 0.5 {
			t.Fatalf("the mark is drawn in %#x on the light window", uint32(c))
		}
	}
	app.SetEnvironment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, AppleKeyboard: true, Dark: true}))
	for _, c := range inks() {
		if r, g, b, _ := c.Components(); r < 0.7 || g < 0.7 || b < 0.7 {
			t.Fatalf("the mark is drawn in %#x on the dark window, want a light ink", uint32(c))
		}
	}
}
