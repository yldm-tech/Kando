package theme

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The names of Kando's own roles in the palettes' Colors, past NSGo's (ui.ColorsOf): the library's, the sidebar's, the settings window's and the onboarding sheet's colors that NSGo's roles do not give.
const (
	roleShade       = "kandoShade"
	roleCard        = "kandoCard"
	roleCardHover   = "kandoCardHover"
	roleGlyph       = "kandoGlyph"
	roleWindow      = "kandoWindow"
	rolePanel       = "kandoPanel"
	roleRail        = "kandoRail"
	roleGutter      = "kandoGutter"
	roleStrong      = "kandoStrong"
	roleTile        = "kandoTile"
	roleThumb       = "kandoThumb"
	rolePlaceholder = "kandoPlaceholder"
	roleIdle        = "kandoIdle"
	roleAccentInk   = "kandoAccentInk"
	roleSuccessInk  = "kandoSuccessInk"
	roleDangerInk   = "kandoDangerInk"
	roleWarnInk     = "kandoWarnInk"
	roleWarnMark    = "kandoWarnMark"
	roleOnWarn      = "kandoOnWarn"
	roleRetryInk    = "kandoRetryInk"
	roleWarnFill    = "kandoWarnFill"
	roleWarnRing    = "kandoWarnRing"
	roleWarnText    = "kandoWarnText"
	roleOKFill      = "kandoOKFill"
	roleOKRing      = "kandoOKRing"
	roleOKText      = "kandoOKText"
)

// lightRoles are the prototype's colors, which the light window has always shown.
var lightRoles = map[string]core.Color{
	roleShade:       core.RGB(0x000000),
	roleCard:        core.RGB(0xFFFFFF),
	roleCardHover:   core.RGB(0xFAFAFC),
	roleGlyph:       Accent,
	roleWindow:      core.RGB(0xF5F5F7),
	rolePanel:       core.RGB(0xF5F5F7),
	roleRail:        core.RGBA(0xECECF0, 0.92),
	roleGutter:      core.RGB(0xF0F2F6),
	roleStrong:      core.RGB(0x3A3A3F),
	roleTile:        core.RGB(0xF0F1F4),
	roleThumb:       core.RGB(0xDCDDE2),
	rolePlaceholder: core.RGB(0x8A8A90),
	roleIdle:        core.RGB(0xC7C7CC),
	roleAccentInk:   core.RGB(0x0052B0),
	roleSuccessInk:  core.RGB(0x1B6B30),
	roleDangerInk:   core.RGB(0xC4291C),
	roleWarnInk:     core.RGB(0xB25000),
	roleWarnMark:    core.RGB(0xFF9F0A),
	roleOnWarn:      core.RGB(0xFFFFFF),
	roleRetryInk:    core.RGB(0xA35200),
	roleWarnFill:    core.RGB(0xFFF7E3),
	roleWarnRing:    core.RGBA(0xB47800, 0.25),
	roleWarnText:    core.RGB(0x5A3A00),
	roleOKFill:      core.RGB(0xEAF7EE),
	roleOKRing:      core.RGBA(0x1E823C, 0.25),
	roleOKText:      core.RGB(0x14532D),
}

// darkText is the text of NSGo's dark page, which the dark message bars' text is over their faint wash.
var darkText = core.DefaultTheme().Dark.Text

// darkRoles are the same roles over NSGo's dark page (#1F1F1F): the hairlines and washes white where they are black on the light page, the cards a step lighter than the page, the settings window a step darker than its cards, and the inks lighter so they read on the dark.
var darkRoles = map[string]core.Color{
	roleShade:       core.RGB(0xFFFFFF),
	roleCard:        core.RGB(0x2C2C2E),
	roleCardHover:   core.RGB(0x343437),
	roleGlyph:       core.RGB(0x0A84FF),
	roleWindow:      core.RGB(0x1C1C1E),
	rolePanel:       core.RGB(0x2A2A2C),
	roleRail:        core.RGBA(0x2A2A2C, 0.92),
	roleGutter:      core.RGB(0x1A1A1C),
	roleStrong:      core.RGB(0xC7C7CC),
	roleTile:        core.RGB(0x3A3A3C),
	roleThumb:       core.RGB(0x3A3A3E),
	rolePlaceholder: core.RGB(0x8E8E93),
	roleIdle:        core.RGB(0x5A5A5E),
	roleAccentInk:   core.RGB(0x6CB4FF),
	roleSuccessInk:  core.RGB(0x63D58A),
	roleDangerInk:   core.RGB(0xFF6B5E),
	roleWarnInk:     core.RGB(0xFFA94D),
	roleWarnMark:    core.RGB(0xFF9F0A),
	roleOnWarn:      core.RGB(0x1F1F1F),
	roleRetryInk:    core.RGB(0xFFA94D),
	roleWarnFill:    core.RGBA(0xFFB340, 0.10),
	roleWarnRing:    core.RGBA(0xFFB340, 0.24),
	roleWarnText:    darkText,
	roleOKFill:      core.RGBA(0x4CD07A, 0.10),
	roleOKRing:      core.RGBA(0x4CD07A, 0.24),
	roleOKText:      darkText,
}

// Colors are Kando's own roles where a component is, as ui.Colors are NSGo's: each reads the palette of the component's subtree, so a StyleFunc or a ui.Colored that calls one follows the appearance as it changes. The methods are funcs of no arguments, so they go where ui takes a func() core.Color.
type Colors struct{ cx *core.Scope }

// Of is Kando's roles where cx is.
func Of(cx *core.Scope) Colors { return Colors{cx} }

func (c Colors) role(name string) core.Color { return c.cx.Palette().Colors[name] }

// Shade is the ink of the hairlines, washes, hover fills and rings drawn over the page at alpha: black on the light page, white on the dark one.
func (c Colors) Shade(alpha float32) core.Color { return ui.Alpha(c.role(roleShade), alpha) }

// Card is a card's face on the page or the window: white, or a step lighter than the dark page. A translucent one (Card().Faded) is the frosted pills and the sidebar's card. CardHover is a pressable card under the pointer.
func (c Colors) Card() core.Color      { return c.role(roleCard) }
func (c Colors) CardHover() core.Color { return c.role(roleCardHover) }

// Glyph is the accent's color for an icon or mark on the page, as a sidebar row's, the drop zone's and a watched film's check and an empty library's play mark: Kando's blue on the light page; on the dark one Apple's lighter blue for the dark (systemBlue), as Kando's blue is too deep to stand out from the dark there. It is not AccentInk, the accent's text: #0A84FF is 3.8:1 on a dark card, under the 4.5:1 small text wants, so the text takes a lighter blue there, as macOS's links do beside its tinted icons.
func (c Colors) Glyph() core.Color { return c.role(roleGlyph) }

// Window is the settings window's and the onboarding sheet's ground under their cards; Panel a card a step off the page, as a processing job's; Rail the settings window's sidebar; Gutter the main window's sidebar where the window shows no material.
func (c Colors) Window() core.Color { return c.role(roleWindow) }
func (c Colors) Panel() core.Color  { return c.role(rolePanel) }
func (c Colors) Rail() core.Color   { return c.role(roleRail) }
func (c Colors) Gutter() core.Color { return c.role(roleGutter) }

// Strong is text and glyphs between the text's color and the muted one: the sidebar's status line, ⋯, and what to do about a failure. Placeholder is the onboarding's placeholder text and dimmest words.
func (c Colors) Strong() core.Color      { return c.role(roleStrong) }
func (c Colors) Placeholder() core.Color { return c.role(rolePlaceholder) }

// Tile is a small gray tile behind a mark, as a tool's in Settings › 通用; Thumb a thumbnail's face before its picture is there.
func (c Colors) Tile() core.Color  { return c.role(roleTile) }
func (c Colors) Thumb() core.Color { return c.role(roleThumb) }

// Idle is a status dot of something not tried or not there.
func (c Colors) Idle() core.Color { return c.role(roleIdle) }

// The inks of words in a color: AccentInk the accent's blue as text on a tint, SuccessInk green, DangerInk red, WarnInk orange, RetryInk the orange of a job waiting out a rate limit.
func (c Colors) AccentInk() core.Color  { return c.role(roleAccentInk) }
func (c Colors) SuccessInk() core.Color { return c.role(roleSuccessInk) }
func (c Colors) DangerInk() core.Color  { return c.role(roleDangerInk) }
func (c Colors) WarnInk() core.Color    { return c.role(roleWarnInk) }
func (c Colors) RetryInk() core.Color   { return c.role(roleRetryInk) }

// WarnMark is the bright orange of a warning's round mark, a white ! on it (ui.Colors.OnAccent), as the design draws a subscription's in the sidebar and on its page: a fill, never text, which it is too light to be.
func (c Colors) WarnMark() core.Color { return c.role(roleWarnMark) }

// OnWarn is text on WarnInk used as a fill, as a warning badge's mark: white on the light window's dark orange, dark on the dark window's light orange.
func (c Colors) OnWarn() core.Color { return c.role(roleOnWarn) }

// The library's message bar, amber for a warning and green for news that something worked: its fill, its hairline and its text. On the dark page the fill is a faint wash of the color over the page and the text the page's own, as Slack's and Linear's dark banners are: an opaque olive or dark green under tinted text read as muddy there. The mark keeps the color.
func (c Colors) WarnFill() core.Color { return c.role(roleWarnFill) }
func (c Colors) WarnRing() core.Color { return c.role(roleWarnRing) }
func (c Colors) WarnText() core.Color { return c.role(roleWarnText) }
func (c Colors) OKFill() core.Color   { return c.role(roleOKFill) }
func (c Colors) OKRing() core.Color   { return c.role(roleOKRing) }
func (c Colors) OKText() core.Color   { return c.role(roleOKText) }
