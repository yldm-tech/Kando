// Package theme is the phone app's look: the design's grouped palette (a grey page under white cards, #F2F2F7 and #FFFFFF; black under #1C1C1E in the dark) in Kando's accent, in the system's appearance or the one the user chose, and the dark one the player always shows.
package theme

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Accent is Kando's blue, the desktop app's accent too (kando/apps/kando/theme).
var Accent = core.RGB(0x0A66D8)

// The roles the design adds to NSGo's palette, read through Of.
const (
	roleBar      = "bar"
	roleBarLine  = "barLine"
	roleTileInk  = "tileInk"
	roleWarnText = "warnText"
)

// Kando is NSGo's default theme in the design's palette: the page is the grouped list's grey and a card (Surface) is white, black under #1C1C1E in the dark; ink, muted text, hairlines and fills are the system's; danger is the destructive red and warning the system's orange, with warnText the orange that reads as text.
func Kando() *core.Theme {
	t := core.DefaultTheme()
	for _, p := range []*core.Palette{&t.Light, &t.Dark} {
		p.Accent, p.AccentText = Accent, core.RGB(0xFFFFFF)
		p.AccentHover, p.AccentPressed = ui.Mix(Accent, core.RGB(0x000000), 0.10), ui.Mix(Accent, core.RGB(0x000000), 0.20)
		p.Selection, p.Focus = Accent.Faded(0.22), Accent.Faded(0.55)
		p.Danger, p.Warning = core.RGB(0xD70015), BadgeOrange
		p.SetColor("accentFill", Accent)
	}
	l, d := &t.Light, &t.Dark
	l.Background, l.Surface, l.Text, l.TextMuted = core.RGB(0xF2F2F7), core.RGB(0xFFFFFF), core.RGB(0x1D1D1F), core.RGB(0x6A6A70)
	l.SurfacePressed, l.Border = core.RGBA(0x767680, 0.16), core.RGBA(0x3C3C43, 0.2)
	l.SetColor("line", core.RGBA(0x3C3C43, 0.2))
	l.SetColor("inset", core.RGBA(0x767680, 0.12))
	l.SetColor("faint", Chevron)
	l.SetColor("raised", core.RGB(0xFFFFFF))
	l.SetColor(roleBar, core.RGBA(0xF9F9FB, 0.92))
	l.SetColor(roleBarLine, core.RGBA(0x000000, 0.14))
	l.SetColor(roleTileInk, core.RGB(0x1D1D1F))
	l.SetColor(roleWarnText, core.RGB(0x8A4B00))
	d.Background, d.Surface, d.Text, d.TextMuted = core.RGB(0x000000), core.RGB(0x1C1C1E), core.RGB(0xF5F5F7), core.RGB(0xA1A1A6)
	d.SurfacePressed, d.Border = core.RGBA(0x767680, 0.32), core.RGBA(0x545458, 0.65)
	d.SetColor("line", core.RGBA(0x545458, 0.65))
	d.SetColor("inset", core.RGBA(0x767680, 0.24))
	d.SetColor("faint", core.RGBA(0xEBEBF5, 0.45))
	d.SetColor("raised", core.RGB(0x2C2C2E))
	d.SetColor(roleBar, core.RGBA(0x1C1C1E, 0.92))
	d.SetColor(roleBarLine, core.RGBA(0x545458, 0.65))
	// The ink tile is a lighter grey in the dark, where the ink's black would be lost on the card.
	d.SetColor(roleTileInk, core.RGB(0x636366))
	d.SetColor(roleWarnText, core.RGB(0xFFB340))
	return t
}

// Apply gives cx and everything it builds Kando's theme; the app's root calls it.
func Apply(cx *core.Scope) { cx.SetTheme(Kando()) }

// Dark shows cx and everything it builds in the dark palette, whatever the system's appearance: the player calls it at its root.
func Dark(cx *core.Scope) { cx.SetSubtreeAppearance(core.AppearanceDark) }

// Colors are the roles the design adds to NSGo's (ui.ColorsOf), in the appearance cx shows; read them in a binding.
type Colors struct{ cx *core.Scope }

// Of is the design's own roles under cx.
func Of(cx *core.Scope) Colors { return Colors{cx} }

func (c Colors) role(name string) core.Color { return c.cx.Palette().Colors[name] }

// Bar is the frosted fill of the tab bar and the bars over a page, drawn over BarBlur.
func (c Colors) Bar() core.Color { return c.role(roleBar) }

// BarLine is the hairline over a drawn tab bar.
func (c Colors) BarLine() core.Color { return c.role(roleBarLine) }

// TileInk is the settings rows' ink tile: the ink in the light, a grey that stands out on the card in the dark.
func (c Colors) TileInk() core.Color { return c.role(roleTileInk) }

// WarnText is warning's orange where it is text, dark enough to read on the page and on a card.
func (c Colors) WarnText() core.Color { return c.role(roleWarnText) }

// BarBlur is what a bar's fill frosts behind it.
var BarBlur = []core.Filter{core.Blur(24), core.Saturate(1.8)}

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
	// The count badges: red, or orange where a job failed. BadgeOnAccent is a badge's fill on the accent, as on a selected sidebar row.
	BadgeRed      = core.RGB(0xFF3B30)
	BadgeOrange   = core.RGB(0xFF9500)
	BadgeOnAccent = core.RGBA(0xFFFFFF, 0.25)
	// The stats' numbers: the streak's orange and the words' green, dark enough to read on a card.
	Streak    = core.RGB(0xE07B00)
	StatGreen = core.RGB(0x248A3D)
	// A sheet's grabber and the shadow it casts upward.
	Grabber     = core.RGBA(0x808080, 0.4)
	SheetShadow = core.RGBA(0x000000, 0.2)
	// TabInactive is a tab's icon and name while another is selected.
	TabInactive = core.RGB(0x8E8E93)
	// Chevron is the arrow at the end of a row that leads to a page, and a disabled bar button's text.
	Chevron = core.RGB(0xC7C7CC)
	// Offline is the dot of a computer that is not reachable.
	Offline = core.RGB(0xC7C7CC)
	// The settings rows' icon tiles, each row's own color.
	TileBlue   = core.RGB(0x0A66D8)
	TileIndigo = core.RGB(0x5E5CE6)
	TilePurple = core.RGB(0x5856D6)
	TileTeal   = core.RGB(0x30B0C7)
	TileViolet = core.RGB(0xAF52DE)
	TileOrange = core.RGB(0xFF9500)
	TileGreen  = core.RGB(0x34C759)
	TileRed    = core.RGB(0xFF3B30)
	TileSky    = core.RGB(0x0A84FF)
	TileGrey   = core.RGB(0x8E8E93)
	// The laptop drawn for a computer: its body's gradient and its screen's.
	LaptopTop, LaptopBottom = core.RGB(0x3A3A3F), core.RGB(0x1D1D1F)
	ScreenTop, ScreenBottom = core.RGB(0x2F7DF0), core.RGB(0x6AA8FF)
)
