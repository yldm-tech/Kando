package main

import (
	"testing"

	"kando/apps/mobile/look"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

// TestScreenshotsMaterial is the Material pieces an Android phone draws, laid out on a page: the 28pt medium large title, a group of rows with switches on and off (the system's, so Material 3's only on an Android target), a 20pt medium section heading of content, as the Library's, and the 22pt regular title of a sub-page's toolbar. nsgo test -target reports the target's own OS, so the app's Android screens cannot show on a Mac; this scene draws the same pieces by hand, in the light and in the dark.
func TestScreenshotsMaterial(t *testing.T) {
	screenshots(t)
	for _, dark := range []bool{false, true} {
		name := "android-look"
		if dark {
			name += "-dark"
		}
		t.Run(name, func(t *testing.T) {
			app := nsgotest.Start(t, materialLook, nsgotest.Size(390, 844), nsgotest.Environment(core.Environment{Locale: lang, Dark: dark}))
			app.WaitFor("id:android-look", wait)
			app.Screenshot(lang + "/" + name)
		})
	}
}

// materialLook is the scene of TestScreenshotsMaterial.
func materialLook(cx *core.Scope) core.Node {
	theme.Apply(cx)
	colors := ui.ColorsOf(cx)
	context, remind := core.NewSignal(cx, true), core.NewSignal(cx, false)
	text := func(s string) func() string { return func() string { return s } }
	row := func(i int, title string, tile core.Color, on *core.Signal[bool]) core.Node {
		return look.Row{Index: func() int { return i }, Title: text(title), Leading: look.ColorTile(cx, lucide.Bell, func() core.Color { return tile }),
			Trailing: look.Switch{Label: text(title), Checked: on.Get, OnChange: on.Set}}
	}
	return core.View{TestID: "android-look", StyleFunc: ui.Filled(colors.Background), Style: core.Style{FlexGrow: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Edges{Top: 56}}, Children: []core.Node{
			core.Text{Content: msgs.SettingsTitle(), Style: look.DisplayAndroid.Merge(core.Style{Padding: core.Edges{Left: 16, Right: 16, Bottom: 4}}), StyleFunc: ui.Colored(colors.Text)},
		}},
		core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				look.GroupHeader(cx, text(msgs.SettingsThisPhone())),
				look.Group{Children: []core.Node{
					row(0, msgs.ComputerContext(), theme.TileOrange, context),
					row(1, msgs.SettingsRemind(), theme.TileRed, remind),
				}},
			}},
			core.Text{Content: msgs.FilmsAll(), Style: look.HeadingAndroid, StyleFunc: ui.Colored(colors.Text)},
		}},
		// The toolbar of a page Settings opens, as NSGo's Header.java lays it out with the title the app gives it (look.NavPage): its 24pt arrow in a 48pt target, the title after it.
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, MinHeight: core.Pt(58), Margin: core.Edges{Top: 28}, Padding: core.Edges{Left: 8}},
			StyleFunc: ui.Filled(theme.Of(cx).Bar), Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.Icon(lucide.ArrowLeft, 24, colors.Text)}},
				core.Text{Content: msgs.SettingsSubtitleStyle(), Style: look.SubPageTitle.Merge(core.Style{Margin: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Text)},
			}},
	}}
}
