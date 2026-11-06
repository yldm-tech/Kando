package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// shortcuts is the 快捷键 page: the app's shortcut table, read only, grouped as the cheat sheet groups it and filtered by the page's own search field.
func (s screen) shortcuts(c *ui.Context, t *ui.Theme) {
	query := s.ShortcutQuery()
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		c.Key("shortcut-search")
		breeze.SearchField(c, query).Label("搜索命令或按键").Placeholder("搜索命令或按键")
		breeze.Text(c, "单个按键只在没有输入框获得焦点时生效。", breeze.TextCaption, breeze.TextSecondary).Padding(0, 10).Wrap()
	})
	groups, order := map[string][]Shortcut{}, []string{}
	for _, row := range s.Shortcuts() {
		if !shortcutMatches(c, row, *query) {
			continue
		}
		if _, ok := groups[row.Group]; !ok {
			order = append(order, row.Group)
		}
		groups[row.Group] = append(groups[row.Group], row)
	}
	if len(order) == 0 {
		breeze.Text(c, "没有匹配的快捷键", breeze.TextSecondary).Padding(0, 10)
		return
	}
	for _, group := range order {
		preferencesGroup(c, group, "", nil, func() {
			for _, row := range groups[group] {
				breeze.ActionRow(c, row.Label, "", func() {
					shortcutLabel(c, row.Mods, row.Key)
				})
			}
		})
	}
}

// shortcutMatches reports whether the search query finds row by what it does, its group or its keys, ignoring case.
func shortcutMatches(c *ui.Context, row Shortcut, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(row.Group+" "+row.Label+" "+shortcutText(c, row.Mods, row.Key)), q)
}

// shortcutLabel is a shortcut's keys in a keycap. It stands in for breeze.ShortcutLabel of mygo-breeze-ui PR #17 (B9), which has the same signature and draws each platform's keycaps; swap it for that once a breeze release carries it. Until then every platform gets one macOS keycap holding the whole shortcut, 22 points high, padded 0 8 with corners of 6, as B9 draws it on macOS.
func shortcutLabel(c *ui.Context, mods ui.Modifiers, key ui.Key) ui.Element {
	t := c.Theme()
	text := shortcutText(c, mods, key)
	// The text color at 5%, 12% in the dark theme: B9's black 5% and white 12%.
	fill := t.Text.Alpha(0.05)
	if t.Dark {
		fill = t.Text.Alpha(0.12)
	}
	keycap := ui.Row(c).MinWidth(22).Height(22).Padding(0, 8).Radius(6).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Background(fill).Label(text)
	keycap.Children(func() {
		ui.Text(c, text).FontSize(12).SingleLine()
	})
	return keycap
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
