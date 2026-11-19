package lookup

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// The most characters a lookup source's entry may have in its word, its reading and its meaning; more is cut off.
const (
	pluginWordMax    = 100
	pluginMeaningMax = 2000
)

// ErrBadAnswer is what a lookup source of a script plugin answered that is not an entry nor null.
var ErrBadAnswer = errors.New("the lookup source answered something that is not an entry")

// FromPlugin is the entry a lookup source of a script plugin answered for word as v, what its lookUp returned read as JSON: an object of word, reading and meaning, the meaning not empty, or null for a word it does not have (ok false). Each field is trimmed, cut to a size the card shows, and the word is word when it gave none.
func FromPlugin(word string, v any) (e Entry, ok bool, err error) {
	if v == nil {
		return Entry{}, false, nil
	}
	m, isMap := v.(map[string]any)
	if !isMap {
		return Entry{}, false, ErrBadAnswer
	}
	text := func(key string, max int) string {
		s, _ := m[key].(string)
		s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
		if utf8.RuneCountInString(s) > max {
			s = string([]rune(s)[:max]) + "…"
		}
		return s
	}
	e = Entry{Word: text("word", pluginWordMax), Reading: text("reading", pluginWordMax), Meaning: text("meaning", pluginMeaningMax)}
	if e.Meaning == "" {
		return Entry{}, false, nil
	}
	if e.Word == "" {
		e.Word = strings.TrimSpace(word)
	}
	return e, true, nil
}
