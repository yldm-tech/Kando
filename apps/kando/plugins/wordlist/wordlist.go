// Package wordlist reads the word list packs of Kando's plugins (plugins.KindWordlist): the words of a language, each with one of the pack's tiers, such as a JLPT or HSK level, which the player draws a line in its tone under the words of its lines by, and, when the pack gives it, the word's rank by frequency, which the card of a word looked up shows with its tier. A list's file is a table of tab-separated values, a word, the id of its tier and, optionally, its rank on each line; it is read into memory once, when the list is first wanted. It imports no interface package. docs/plugin-system.md is the design.
package wordlist

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kando/apps/kando/plugins"
)

// ErrEmpty is a word list none of whose lines names a word and one of the pack's tiers, which Open refuses.
var ErrEmpty = errors.New("wordlist: the word list has no word of one of its tiers")

// wordMax is the most bytes of a word kept; longer ones are left out, as no line has such a word.
const wordMax = 200

// RankMax is the highest rank by frequency a word may have; a rank that is not a whole number from 1 to RankMax is left out, and the word kept without one.
const RankMax = 1_000_000

// List is a word list read into memory: each word, as Key writes it, with the index of its tier among tiers and its rank by frequency, 0 for none. It is not changed once read, so any goroutine may read it.
type List struct {
	words map[string]entry
	tiers []plugins.Tier
}

// entry is a word of a list: the index of its tier and its rank, 0 for none.
type entry struct {
	tier int
	rank int
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

// Parse reads a word list's file of tiers: on each line a word, a tab and the id of its tier, then, optionally, a tab and the word's rank by frequency (1 the most frequent, at most RankMax; one that is not such a whole number is left out), columns after that left out. Lines that start with # are comments; empty lines, lines without a tab and lines of a tier not among tiers are left out, and a word listed again keeps its first tier and rank. ErrEmpty when no line is left.
func Parse(r io.Reader, tiers []plugins.Tier) (*List, error) {
	index := make(map[string]int, len(tiers))
	for i, t := range tiers {
		index[t.ID] = i
	}
	l := &List{words: map[string]entry{}, tiers: tiers}
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
		tier, rest, _ := strings.Cut(rest, "\t")
		rank, _, _ := strings.Cut(rest, "\t")
		i, known := index[strings.TrimSpace(tier)]
		k := Key(strings.TrimPrefix(word, "\ufeff"))
		if !known || k == "" || len(k) > wordMax {
			continue
		}
		if _, seen := l.words[k]; !seen {
			l.words[k] = entry{tier: i, rank: parseRank(rank)}
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

// parseRank is the rank by frequency a list's third column gives, 0 when it gives none: a whole number from 1 to RankMax.
func parseRank(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > RankMax {
		return 0
	}
	return n
}

// Tier is the tier of word, found as Key writes it, and whether the list has it.
func (l *List) Tier(word string) (plugins.Tier, bool) {
	e, ok := l.words[Key(word)]
	if !ok {
		return plugins.Tier{}, false
	}
	return l.tiers[e.tier], true
}

// Rank is the rank by frequency of word, found as Key writes it, 1 the most frequent: 0 when the list does not have the word or gives it no rank.
func (l *List) Rank(word string) int {
	return l.words[Key(word)].rank
}
