package film

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// The breeze widgets the player waits for. Each helper has the signature of the widget it stands in for, so swapping it for breeze's is a rename once a breeze release carries the widget.

// flatIconButton stands in for breeze.FlatIconButton of mygo-breeze-ui PR #15 (B2), the button with no face at rest the player's controls use. Until then it is breeze.IconButton, which has the platform's bordered face.
func flatIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	return breeze.IconButton(c, icon, label)
}

// flatToggle stands in for breeze.FlatToggle of mygo-breeze-ui PR #15 (B2). Until then it is breeze.Toggle without a label, holding icon, named label for assistive technology.
func flatToggle(c *ui.Context, on *bool, icon *ui.SVG, label string) ui.Element {
	b := breeze.Toggle(c, on, "").Label(label).Tooltip(label)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).Shrink(0) })
	return b
}

// iconLabelButton stands in for breeze.IconLabelButton of mygo-breeze-ui PR #14 (B7). Until then it is breeze.Button, showing the label without the icon.
func iconLabelButton(c *ui.Context, _ *ui.SVG, label string) ui.Element {
	return breeze.Button(c, label)
}

// shortcutText stands in for breeze.ShortcutText of mygo-breeze-ui PR #17 (B9), which writes a shortcut the way the platform's menus do. Until then every platform gets macOS's form: the modifiers in the order ⌃⌥⇧⌘ before the key, as in ⇧⌘T.
func shortcutText(_ *ui.Context, mods ui.Modifiers, key ui.Key) string {
	s := ""
	for _, m := range []struct {
		mod   ui.Modifiers
		glyph string
	}{{ui.Ctrl, "⌃"}, {ui.Alt, "⌥"}, {ui.Shift, "⇧"}, {ui.Super, "⌘"}} {
		if mods&m.mod != 0 {
			s += m.glyph
		}
	}
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return s + string(rune('A'+(key-ui.KeyA)))
	case key == ui.KeyLeft:
		return s + "←"
	case key == ui.KeyRight:
		return s + "→"
	case key == ui.KeySpace:
		return s + "空格"
	}
	return s
}
