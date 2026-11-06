// Package cheatsheet draws the list of keyboard shortcuts that ⌘/ opens over the window.
package cheatsheet

import (
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

// Show draws the cheat sheet over the window in the dark theme while *open. Escape or a click outside it sets *open to false.
func Show(c *ui.Context, dark *ui.Theme, open *bool, groups []Group) {
	ui.Themed(c, dark).Absolute().Children(func() {
		breeze.Modal(c, open, func() {
			breeze.Text(c, "键盘快捷键", breeze.TextTitle)
			ui.Row(c).Wrap().Gap(24).AlignItems(ui.Start).Children(func() {
				for _, g := range groups {
					ui.Column(c).Width(240).Gap(6).Children(func() {
						breeze.Text(c, g.Title, breeze.TextHeading)
						for _, k := range g.Keys {
							ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
								breeze.Text(c, k.Label).Grow(1)
								breeze.Text(c, shortcutText(c, k.Mods, k.Key), breeze.TextSecondary).Shrink(0)
							})
						}
					})
				}
			})
		}).Width(560)
	})
}
