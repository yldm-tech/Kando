// Package theme is the window's light and dark themes in Kando's accent.
package theme

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Accent is Kando's blue, used in place of the desktop's accent color in both appearances.
var Accent = core.RGB(0x0A66D8)

// The light theme's own colors: a white page, near-black text and a darker gray than NSGo's for secondary text.
var (
	lightBackground = core.RGB(0xFFFFFF)
	lightText       = core.RGB(0x1D1D1F)
	lightMuted      = core.RGB(0x6A6A70)
)

// Kando is NSGo's default theme in Kando's colors: the light palette is the window's, which the app shows everywhere (nsgo.Appearance), and the dark one is the player's and the cheat sheet's, which they force on their subtree with Dark. Every color the app sets on a theme is set here; a component reads them through ui.ColorsOf or core.Scope.Palette inside a StyleFunc, so they follow the subtree's theme and appearance.
func Kando() *core.Theme {
	t := core.DefaultTheme()
	l := &t.Light
	l.Background, l.Surface = lightBackground, lightBackground
	l.Text, l.TextMuted = lightText, lightMuted
	// TODO(nsgo#141): keep the platform's darker secondary text while the user asks for more contrast, as MyGo's Preferences().HighContrast let the light palette do; core.Env reports no contrast preference https://github.com/nsgo-dev/nsgo/issues/141
	// The roles NSGo derives from the text's color, at its opacities, derived from Kando's darker text.
	l.SurfaceHover, l.SurfacePressed = lightText.Faded(0.08), lightText.Faded(0.14)
	l.Border, l.Scrollbar = lightText.Faded(0.16), lightText.Faded(0.30)
	for name, c := range map[string]core.Color{"line": lightText.Faded(0.12), "faint": lightText.Faded(0.38), "track": lightText.Faded(0.25), "fieldBorder": l.Border, "light": lightText} {
		l.SetColor(name, c)
	}
	for _, p := range []*core.Palette{&t.Light, &t.Dark} {
		p.Accent, p.AccentText = Accent, core.RGB(0xFFFFFF)
		p.AccentHover, p.AccentPressed = ui.Mix(Accent, core.RGB(0x000000), 0.10), ui.Mix(Accent, core.RGB(0x000000), 0.20)
		p.Selection, p.Focus = Accent.Faded(0.22), Accent.Faded(0.55)
		p.SetColor("accentFill", Accent)
	}
	return t
}

// Apply gives cx and everything it builds Kando's theme. The root of each window calls it once: the main window's view and the settings window's.
func Apply(cx *core.Scope) { cx.SetTheme(Kando()) }

// Dark shows cx and everything it builds in the dark palette, whatever the window shows: the player and the cheat sheet call it at their root.
func Dark(cx *core.Scope) { cx.SetSubtreeAppearance(core.AppearanceDark) }

// Appearance is the one the app runs in, light whatever the system's, as nsgo.Appearance and core.Scope.SetAppearance take it.
const Appearance = core.AppearanceLight
