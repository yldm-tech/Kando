// Package cheatsheet draws the list of keyboard shortcuts that ⌘/ opens over the window.
package cheatsheet

import (
	"strings"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Group is a heading of the cheat sheet and the lines under it.
type Group struct {
	Title string
	Keys  []Key
}

// Key is one line of the sheet: what it does and the shortcuts that run it, on one keycap, as 后退 / 前进 5 秒 shows ← →.
type Key struct {
	Label  string
	Combos []Combo
}

// Combo is one shortcut: its modifiers and its key.
type Combo struct {
	Mods ui.Modifiers
	Key  ui.Key
}

// The sheet's measures, as the prototype's dialog has them: 26 below the window's top, at most 860 wide and 32 narrower than the window, padded 20 22 18, the header 14 above the groups, the groups in columns at least 240 wide, 22 apart, rows of groups 18 apart, and lines 22 high, 6 apart. The close button is a 28 circle.
const (
	sheetTop    = 26
	sheetWidth  = 860
	sheetInset  = 32
	sheetPadTop = 20
	sheetPadX   = 22
	sheetPadBot = 18
	sheetRadius = 24
	headerGap   = 14
	columnWidth = 240
	columnGap   = 22
	groupGap    = 18
	lineGap     = 6
	lineHeight  = 22
	closeSize   = 28
)

var (
	// scrim dims the window under the sheet.
	scrim = ui.RGBA(0, 0, 0, 0.25)
	// glassTint is the dark glass's color over what the blur shows through.
	glassTint = ui.RGBA(28, 28, 32, 0.86)
	// rim is the glass's hairline edge, and shadow the shadow it casts.
	rim    = ui.RGBA(255, 255, 255, 0.2)
	shadow = ui.RGBA(0, 0, 0, 0.5)
	// ink is the sheet's text, and closeFill the close button's round plate.
	ink       = ui.RGBA(255, 255, 255, 1)
	closeFill = ui.RGBA(255, 255, 255, 0.12)
)

// Show draws the cheat sheet over the window in the dark theme while *open: the window dimmed, and near its top dark glass with the groups in as many columns as fit. Escape, ⌘/, the close button or a click outside it sets *open to false.
func Show(c *ui.Context, dark *ui.Theme, open *bool, groups []Group) {
	ui.Themed(c, dark).Absolute().Children(func() {
		w, h := c.Size()
		width := min(sheetWidth, w-sheetInset)
		columns := max(1, int((width-2*sheetPadX+columnGap)/(columnWidth+columnGap)))
		panel := ui.DialogBase(c, open, func(back, p ui.Element) {
			back.Background(scrim).Justify(ui.Start).Padding(sheetTop, 0, 0, 0)
			// The glass: the panel blurs what is under it, and the sheet inside tints the blur dark.
			p.Width(width).MaxHeight(h-2*sheetTop).Padding(0).Radius(sheetRadius).Material(glass.Blur{Radius: 36}).Shadow(0, 30, 80, 0, shadow)
			ui.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(ui.Stretch).Radius(sheetRadius).Background(glassTint).InsetShadow(0, 0, 0, 0.5, rim).
				Padding(sheetPadTop, sheetPadX, sheetPadBot, sheetPadX).TextColor(ink).Children(func() {
				// The header: the title 17 bold, the hint 12 at 60%.
				ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(0, 0, headerGap, 0).Children(func() {
					breeze.Text(c, locales.CheatsheetTitle(), breeze.TextSubtitle).FontSize(17).Bold().Grow(1)
					ui.Text(c, locales.CheatsheetHint()).FontSize(12).Opacity(0.6)
					if breeze.FlatIconButton(c, iconClose, locales.CheatsheetClose()).Size(closeSize, closeSize).Radius(closeSize / 2).Background(closeFill).TextColor(ink).Clicked() {
						*open = false
					}
				})
				ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
					ui.Grid(c).Columns(columns).GapX(columnGap).GapY(groupGap).AlignItems(ui.Start).Children(func() {
						for _, g := range groups {
							group(c, g)
						}
					})
				})
			})
		})
		// The window's ⌘/ that opened the sheet waits behind the dialog, and nothing in the sheet takes the focus, so the sheet takes the key as an overlay to close again.
		if panel.OverlayShortcut(ui.Cmd, ui.KeySlash) {
			*open = false
		}
	})
}

// group draws a group's heading, 11 bold at 55% on a line 16 high (the browser's normal line of PingFang), and a line per entry: what it does at 90%, then its keycap, its text 600.
func group(c *ui.Context, g Group) {
	ui.Column(c).Gap(lineGap).MinWidth(0).Children(func() {
		ui.Text(c, g.Title).FontSize(11).LineHeight(16.0 / 11).Bold().Opacity(0.55)
		for _, k := range g.Keys {
			ui.Row(c).Gap(10).AlignItems(ui.Center).MinHeight(lineHeight).Children(func() {
				breeze.Text(c, k.Label).Opacity(0.9).Grow(1).MinWidth(0)
				keycap(c, k.Combos).FontWeight(600)
			})
		}
	})
}

// keycap draws combos as the platform shows a shortcut. On macOS several share one keycap, as the prototype draws ⌘← ⌘→; elsewhere they stand side by side, each the platform's own. Escape alone reads Esc on every platform, as the prototype writes it: macOS's menu glyph ⎋ on a keycap reads like an undo arrow.
func keycap(c *ui.Context, combos []Combo) ui.Element {
	if len(combos) == 1 {
		return oneKeycap(c, combos[0])
	}
	if breeze.PlatformOf(c) == breeze.Mac {
		texts := make([]string, len(combos))
		for i, k := range combos {
			texts[i] = comboText(c, k)
		}
		return breeze.Keycaps(c, strings.Join(texts, " "))
	}
	return ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, k := range combos {
			oneKeycap(c, k)
		}
	})
}

// oneKeycap draws k as breeze.ShortcutLabel does, and Escape alone as Esc, spoken as Escape.
func oneKeycap(c *ui.Context, k Combo) ui.Element {
	if isEscape(k) {
		return breeze.Keycaps(c, "Esc").Label("Escape")
	}
	return breeze.ShortcutLabel(c, k.Mods, k.Key)
}

func isEscape(k Combo) bool { return k.Key == ui.KeyEscape && k.Mods == 0 }

// comboText is k as the platform's menus write it, and Esc for Escape alone.
func comboText(c *ui.Context, k Combo) string {
	if isEscape(k) {
		return "Esc"
	}
	return breeze.ShortcutText(c, k.Mods, k.Key)
}
