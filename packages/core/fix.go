package core

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fix is what the user changed in one line, From to To, which the other lines of the film that hold From may want too: a name the translation wrote wrongly, a word the speech recognizer misheard.
type Fix struct {
	From, To string
}

// fixShortest is how many characters From must have for a fix to be looked for in other lines: one character changed, a punctuation mark, is no fix of a word.
const fixShortest = 2

// FixOf is the fix an edit of a line from before to after made: the part of before that changed, without what the two start and end with alike. In a language written with spaces the part takes in the whole words it touches, so "Jon" to "John" is a fix of the word, not the letter added. It reports false for no change, for a change that only adds words, and for one of fewer than two characters.
func FixOf(before, after string) (Fix, bool) {
	a, b := []rune(before), []rune(after)
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	q := 0
	for q < len(a)-p && q < len(b)-p && a[len(a)-1-q] == b[len(b)-1-q] {
		q++
	}
	// Widen the change to the words it starts and ends in.
	at := func(r []rune, i int) bool { return i >= 0 && i < len(r) && wordRune(r[i]) }
	for p > 0 && at(a, p-1) && (at(a, p) || at(b, p)) {
		p--
	}
	for q > 0 && at(a, len(a)-q) && (at(a, len(a)-q-1) || at(b, len(b)-q-1)) {
		q--
	}
	from, to := string(a[p:len(a)-q]), string(b[p:len(b)-q])
	if utf8.RuneCountInString(strings.TrimSpace(from)) < fixShortest || from == to {
		return Fix{}, false
	}
	return Fix{From: from, To: to}, true
}

// In reports whether text holds the fix's From where Apply would replace it.
func (f Fix) In(text string) bool { return len(f.matches(text)) > 0 }

// Apply is text with To in place of every From it holds. From must stand as whole words in a language written with spaces: fixing "Jon" leaves "Jonathan" alone. Case counts, as for a name.
func (f Fix) Apply(text string) string {
	at := f.matches(text)
	if len(at) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, i := range at {
		out.WriteString(text[last:i])
		out.WriteString(f.To)
		last = i + len(f.From)
	}
	out.WriteString(text[last:])
	return out.String()
}

// matches are where in text From stands as Apply replaces it, by byte offset, without overlaps.
func (f Fix) matches(text string) []int {
	if f.From == "" {
		return nil
	}
	first, _ := utf8.DecodeRuneInString(f.From)
	last, _ := utf8.DecodeLastRuneInString(f.From)
	var out []int
	for from := 0; from <= len(text); {
		i := strings.Index(text[from:], f.From)
		if i < 0 {
			break
		}
		start, end := from+i, from+i+len(f.From)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if (start == 0 || !wordRune(first) || !wordRune(before)) && (end == len(text) || !wordRune(last) || !wordRune(after)) {
			out = append(out, start)
			from = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		from = start + size
	}
	return out
}

// wordRune reports whether r is a letter or a digit of a script written with spaces between words, so a word of it goes on as long as such runes do.
func wordRune(r rune) bool {
	return (unicode.IsLetter(r) || unicode.IsDigit(r)) && !unspacedScript(r)
}
