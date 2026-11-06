// Package cheatsheet draws the list of keyboard shortcuts that ⌘/ opens over the window.
package cheatsheet

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Group is a heading of the cheat sheet and the shortcuts under it.
type Group struct {
	Title string
	Keys  []Key
}

// Key is one shortcut: what it does and the keys that run it.
type Key struct {
	Label string
	Mods  ui.Modifiers
	Key   ui.Key
}

// The sheet's measures, as the prototype's dialog has them: at most 860 wide, padded 20 by 22, the groups in columns at least 240 wide, 22 apart, and rows of groups 18 apart.
const (
	sheetWidth  = 860
	sheetPadY   = 20
	sheetPadX   = 22
	columnWidth = 240
	columnGap   = 22
	groupGap    = 18
	sheetRadius = 24
)

// glassTint is the dark glass's color over what the blur shows through.
var glassTint = ui.RGBA(28, 28, 32, 0.86)

// Show draws the cheat sheet over the window in the dark theme while *open: dark glass with the groups in as many columns as fit. Escape, ⌘/, the close button or a click outside it sets *open to false.
func Show(c *ui.Context, dark *ui.Theme, open *bool, groups []Group) {
	ui.Themed(c, dark).Absolute().Children(func() {
		w, _ := c.Size()
		width := min(sheetWidth, w-40)
		columns := max(1, int((width-2*sheetPadX+columnGap)/(columnWidth+columnGap)))
		panel := breeze.Modal(c, open, func() {
			// The glass: the panel blurs what is under it, and the sheet inside tints the blur dark.
			ui.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(ui.Stretch).Radius(sheetRadius).Background(glassTint).Padding(sheetPadY, sheetPadX, 18, sheetPadX).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(0, 0, 14, 0).Children(func() {
					breeze.Text(c, "快捷键", breeze.TextSubtitle).Bold().Grow(1)
					breeze.Text(c, "单键只在没有输入框获焦时生效", breeze.TextSecondary).FontSize(12)
					if flatIconButton(c, iconClose, "关闭").Clicked() {
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
		panel.Width(width).Padding(0).Gap(0).Material(glass.Blur{Radius: 36})
		// The window's ⌘/ that opened the sheet waits behind the modal, and nothing in the sheet takes the focus, so the sheet takes the key as an overlay to close again.
		if panel.OverlayShortcut(ui.Cmd, ui.KeySlash) {
			*open = false
		}
	})
}

// group draws a group's heading and a row per shortcut: what it does, then its keys.
func group(c *ui.Context, g Group) {
	ui.Column(c).Gap(6).MinWidth(0).Children(func() {
		breeze.Text(c, g.Title, breeze.TextSecondary).FontSize(11).Bold()
		for _, k := range g.Keys {
			ui.Row(c).Gap(10).AlignItems(ui.Center).MinHeight(22).Children(func() {
				breeze.Text(c, k.Label).Grow(1).MinWidth(0)
				shortcutLabel(c, k.Mods, k.Key)
			})
		}
	})
}
