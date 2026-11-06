package cheatsheet

import "github.com/egoist/mygo/ui"

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

func keyName(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + (key - ui.KeyA)))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + (key - ui.Key0)))
	}
	switch key {
	case ui.KeySpace:
		return "空格"
	case ui.KeyEnter:
		return "↩"
	case ui.KeyEscape:
		return "esc"
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
	}
	return "?"
}
