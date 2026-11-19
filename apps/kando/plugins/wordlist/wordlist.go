// Package wordlist reads the word list packs of Kando's plugins (plugins.KindWordlist): the words of a language, each with one of the pack's tiers, such as a JLPT or HSK level, which the player draws a line in its tone under the words of its lines by. A list's file is a table of tab-separated values, a word and the id of its tier on each line; it is read into memory once, when the list is first wanted. It imports no interface package. docs/plugin-system.md is the design.
package wordlist

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kando/apps/kando/plugins"
)

// ErrEmpty is a word list none of whose lines names a word and one of the pack's tiers, which Open refuses.
var ErrEmpty = errors.New("wordlist: the word list has no word of one of its tiers")

// wordMax is the most bytes of a word kept; longer ones are left out, as no line has such a word.
const wordMax = 200

// List is a word list read into memory: each word, as Key writes it, with the index of its tier among tiers. It is not changed once read, so any goroutine may read it.
type List struct {
	words map[string]int
	tiers []plugins.Tier
}

// Key is how a list finds a word: trimmed and in lower case, so "River" and "river" are one word.
func Key(word string) string { return strings.ToLower(strings.TrimSpace(word)) }

// Open reads the file of the word list pack p (plugins.Pack.Wordlist).
func Open(p plugins.Pack) (*List, error) {
	if p.Wordlist == nil {
		return nil, fmt.Errorf("wordlist: %s is not a word list", p.ID)
	}
	f, err := os.Open(filepath.Join(p.Dir, p.Wordlist.File))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, p.Wordlist.Tiers)
}

// Parse reads a word list's file of tiers: on each line a word, a tab and the id of its tier, columns after that left out. Lines that start with # are comments; empty lines, lines without a tab and lines of a tier not among tiers are left out, and a word listed again keeps its first tier. ErrEmpty when no line is left.
func Parse(r io.Reader, tiers []plugins.Tier) (*List, error) {
	index := make(map[string]int, len(tiers))
	for i, t := range tiers {
		index[t.ID] = i
	}
	l := &List{words: map[string]int{}, tiers: tiers}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "#") {
			continue
		}
		word, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		tier, _, _ := strings.Cut(rest, "\t")
		i, known := index[strings.TrimSpace(tier)]
		k := Key(strings.TrimPrefix(word, "\ufeff"))
		if !known || k == "" || len(k) > wordMax {
			continue
		}
		if _, seen := l.words[k]; !seen {
			l.words[k] = i
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(l.words) == 0 {
		return nil, ErrEmpty
	}
	return l, nil
}

// Tier is the tier of word, found as Key writes it, and whether the list has it.
func (l *List) Tier(word string) (plugins.Tier, bool) {
	i, ok := l.words[Key(word)]
	if !ok {
		return plugins.Tier{}, false
	}
	return l.tiers[i], true
}
