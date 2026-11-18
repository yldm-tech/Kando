// Package theme is the phone app's look: NSGo's default theme in Kando's accent, in the system's appearance, and the dark one the player always shows.
package theme

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Accent is Kando's blue, the desktop app's accent too (kando/apps/kando/theme).
var Accent = core.RGB(0x0A66D8)

// Kando is NSGo's default theme with Kando's accent in both palettes.
func Kando() *core.Theme {
	t := core.DefaultTheme()
	for _, p := range []*core.Palette{&t.Light, &t.Dark} {
		p.Accent, p.AccentText = Accent, core.RGB(0xFFFFFF)
		p.AccentHover, p.AccentPressed = ui.Mix(Accent, core.RGB(0x000000), 0.10), ui.Mix(Accent, core.RGB(0x000000), 0.20)
		p.Selection, p.Focus = Accent.Faded(0.22), Accent.Faded(0.55)
		p.SetColor("accentFill", Accent)
	}
	return t
}

// Apply gives cx and everything it builds Kando's theme; the app's root calls it.
func Apply(cx *core.Scope) { cx.SetTheme(Kando()) }

// Dark shows cx and everything it builds in the dark palette, whatever the system's appearance: the player calls it at its root.
func Dark(cx *core.Scope) { cx.SetSubtreeAppearance(core.AppearanceDark) }

// The design's fixed colors, the desktop app's too (kando/apps/kando/theme tokens.go): the same in both appearances, because each is drawn on a picture, on the dark player or as a mark whose meaning is its color. A change here goes there too.
var (
	// The source marks: YouTube's, Bilibili's, another web page's and a file's on the computer.
	SourceYouTube  = core.RGB(0xE62117)
	SourceBilibili = core.RGB(0x00A1D6)
	SourceOther    = core.RGB(0x5E5CE6)
	SourceLocal    = core.RGB(0x8E8E93)
	// SubtitleYellow is the original line's color under the translation.
	SubtitleYellow = core.RGB(0xFFD60A)
	// AI is the accent of what an AI service does.
	AI = core.RGB(0x5E5CE6)
	// The status dots: ready or done, something missing or waiting, failed.
	StatusOK     = core.RGB(0x30D158)
	StatusWarn   = core.RGB(0xFF9F0A)
	StatusFailed = core.RGB(0xFF453A)
)
