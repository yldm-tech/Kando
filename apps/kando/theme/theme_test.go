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
