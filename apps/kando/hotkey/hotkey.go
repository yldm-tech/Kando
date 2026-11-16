// Package hotkey is how Kando writes a shortcut and finds one by a search: the command palette, the cheat sheet and Settings › 快捷键 all write a key with Label and search their rows with Match, so a key reads the same in all three and a query finds the same rows in each.
package hotkey

import (
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

// Label is key, in NSGo's form ("mod+shift+n", "space", "escape"), as the platform the app runs on writes it under cx (core.Scope.KeyLabel), and Esc for Escape alone on every platform, as the prototype writes it: macOS's menu glyph ⎋ reads like an undo arrow on a keycap. "" is none.
func Label(cx *core.Scope, key string) string {
	switch key {
	case "":
		return ""
	case "escape":
		return "Esc"
	}
	return cx.KeyLabel(key)
}

// Match reports whether query finds a row that does label, under group, on the key written keys (Label): every word of the query, ignoring case, is in one of the three. rank orders the rows found, lower first: 0 for a row whose label starts with the whole query, 1 for any other. An empty query finds every row at rank 1.
func Match(query, label, group, keys string) (rank int, ok bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 1, true
	}
	l := strings.ToLower(label)
	haystack := l + "\n" + strings.ToLower(group) + "\n" + strings.ToLower(keys)
	for _, word := range strings.Fields(q) {
		if !strings.Contains(haystack, word) {
			return 0, false
		}
	}
	if strings.HasPrefix(l, q) {
		return 0, true
	}
	return 1, true
}
