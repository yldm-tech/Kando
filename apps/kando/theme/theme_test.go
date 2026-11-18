package theme

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// A subtree under Dark reads the dark palette and the rest the light one, both in Kando's accent.
func TestKandoLightAndDark(t *testing.T) {
	var light, dark core.Palette
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		Apply(cx)
		return core.View{Children: []core.Node{
			core.Func(func(cx *core.Scope) core.Node { light = cx.Palette(); return nil }),
			core.Func(func(cx *core.Scope) core.Node { Dark(cx); dark = cx.Palette(); return nil }),
		}}
	})
	if light.Background != lightBackground || light.Text != lightText || light.TextMuted != lightMuted {
		t.Errorf("light palette %v on %v, muted %v", light.Text, light.Background, light.TextMuted)
	}
	if dark.Background == light.Background || dark.Text == light.Text {
		t.Errorf("the dark subtree reads the light palette")
	}
	for _, p := range []core.Palette{light, dark} {
		if p.Accent != Accent || p.Colors["accentFill"] != Accent {
			t.Errorf("accent %v, accentFill %v, want %v", p.Accent, p.Colors["accentFill"], Accent)
		}
	}
	if got := ui.Alpha(light.Colors["line"], 1); got != ui.Alpha(lightText, 1) {
		t.Errorf("hairlines %v are not drawn in the text's color %v", got, lightText)
	}
}

// With the system's contrast turned up the light theme's secondary text is darker, and it goes back as contrast is turned down.
func TestMutedTextFollowsTheContrast(t *testing.T) {
	env := core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		Apply(cx)
		return core.View{TestID: "muted", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)}, StyleFunc: func() core.Style { return core.Style{Background: cx.Palette().TextMuted} }}
	}, nsgotest.Environment(env))
	muted := func() core.Color { return app.Find("id:muted").Style.Background }
	if got := muted(); got != lightMuted {
		t.Fatalf("muted text %#x, want %#x", uint32(got), uint32(lightMuted))
	}
	env.IncreaseContrast = true
	app.SetEnvironment(env)
	if got := muted(); got != lightMutedContrast {
		t.Fatalf("muted text %#x with more contrast, want %#x", uint32(got), uint32(lightMutedContrast))
	}
	env.IncreaseContrast = false
	app.SetEnvironment(env)
	if got := muted(); got != lightMuted {
		t.Fatalf("muted text %#x once contrast is back, want %#x", uint32(got), uint32(lightMuted))
	}
}

// Kando's own roles are set in both palettes, and a binding that reads one turns with the system's appearance: the light window's white card turns the dark one's as the environment turns dark, and back.
func TestRolesFollowTheAppearance(t *testing.T) {
	for name := range lightRoles {
		if _, ok := darkRoles[name]; !ok {
			t.Errorf("role %s has no dark color", name)
		}
	}
	for name := range darkRoles {
		if _, ok := lightRoles[name]; !ok {
			t.Errorf("role %s has no light color", name)
		}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		Apply(cx)
		k := Of(cx)
		return core.View{TestID: "card", Style: core.Style{Width: core.Pt(10), Height: core.Pt(10)},
			StyleFunc: func() core.Style { return core.Style{Background: k.Card(), BorderColor: k.Shade(0.08)} }}
	})
	card := func() *nsgotest.View { return app.Find("id:card") }
	if got := card().Style.Background; got != lightRoles[roleCard] {
		t.Fatalf("card %#x in the light, want %#x", uint32(got), uint32(lightRoles[roleCard]))
	}
	if got := card().Style.BorderColor; got != core.RGBA(0x000000, 0.08) {
		t.Fatalf("hairline %#x in the light, want black at 8%%", uint32(got))
	}
	app.SetEnvironment(core.Environment{Dark: true})
	if got := card().Style.Background; got != darkRoles[roleCard] {
		t.Fatalf("card %#x in the dark, want %#x", uint32(got), uint32(darkRoles[roleCard]))
	}
	if got := card().Style.BorderColor; got != core.RGBA(0xFFFFFF, 0.08) {
		t.Fatalf("hairline %#x in the dark, want white at 8%%", uint32(got))
	}
	app.SetEnvironment(core.Environment{})
	if got := card().Style.Background; got != lightRoles[roleCard] {
		t.Fatalf("card %#x back in the light, want %#x", uint32(got), uint32(lightRoles[roleCard]))
	}
}
