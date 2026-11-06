package cheatsheet

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// shortcutLabel draws the shortcut of mods and key on one keycap, as macOS shows a shortcut in a cheat sheet: 22 points high and at least as wide, padded 8 either side, corners of 6, the text 12 points, on a plate of white at 12% in the dark theme (black at 5% in the light one) with a 1-point line inside its bottom. It stands in for breeze.ShortcutLabel of mygo-breeze-ui PR #17 (B9), which has the same signature and draws the same keycap on macOS; swap it for that once a breeze release carries it. Until then every platform gets the macOS keycap.
func shortcutLabel(c *ui.Context, mods ui.Modifiers, key ui.Key) ui.Element {
	t := c.Theme()
	fill, line := ui.RGBA(0, 0, 0, 0.05), ui.RGBA(0, 0, 0, 0.08)
	if t.Dark {
		fill, line = ui.RGBA(255, 255, 255, 0.12), ui.RGBA(0, 0, 0, 0.3)
	}
	text := shortcutText(c, mods, key)
	plate := ui.Row(c).Justify(ui.Center).AlignItems(ui.Center).Shrink(0).MinHeight(22).MinWidth(22).Padding(0, 8).Radius(6).Background(fill).InsetShadow(0, -1, 0, 0, line).Role(ui.RoleText).Label(text)
	plate.Children(func() {
		ui.Text(c, text).SingleLine().TextColor(t.Text).FontSize(t.Rem(12.0 / 13)).Role(ui.RoleNone)
	})
	return plate
}

// flatIconButton is a button that shows only icon, without a face at rest. It stands in for breeze.FlatIconButton of mygo-breeze-ui PR #15 (B2), which has the same signature; swap it for that once a breeze release carries it. Until then it is breeze's IconButton, which has a face.
func flatIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	return breeze.IconButton(c, icon, label)
}

// shortcutText writes a shortcut the way macOS menus do, modifiers in the order ⌃⌥⇧⌘ before the key, as in ⇧⌘L. It stands in for breeze.ShortcutText of mygo-breeze-ui PR #17 (B9), which has the same signature and also writes Windows's and GNOME's forms; swap it for that once a breeze release carries it. Until then every platform gets the macOS form.
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
	return s + keyName(key)
}

// keyName is how macOS menus name key, as breeze.ShortcutText names it on macOS, or "" for a key they do not name.
func keyName(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + (key - ui.KeyA)))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + (key - ui.Key0)))
	}
	switch key {
	case ui.KeySpace:
		return "Space"
	case ui.KeyEnter:
		return "↩"
	case ui.KeyEscape:
		return "⎋"
	case ui.KeyTab:
		return "⇥"
	case ui.KeyBackspace:
		return "⌫"
	case ui.KeyDelete:
		return "⌦"
	case ui.KeyLeft:
		return "←"
	case ui.KeyRight:
		return "→"
	case ui.KeyUp:
		return "↑"
	case ui.KeyDown:
		return "↓"
	case ui.KeyComma:
		return ","
	case ui.KeyPeriod:
		return "."
	case ui.KeySlash:
		return "/"
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	case ui.KeyBracketLeft:
		return "["
	case ui.KeyBracketRight:
		return "]"
	case ui.KeyBackslash:
		return "\\"
	case ui.KeySemicolon:
		return ";"
	case ui.KeyQuote:
		return "'"
	case ui.KeyBackquote:
		return "`"
	}
	return ""
}
