package core

import (
	"strings"
	"unicode"
)

// WordToken is a run of a line: a word a tap or a click looks up, or what lies between words, as spaces and punctuation.
type WordToken struct {
	Text string
	Word bool
}

// WordTokens cuts line into its words and what lies between them, which joined again are the line. A word is a run of letters, marks and digits with at least one letter, an apostrophe or a hyphen inside it joining its parts, as "don't" and "well-known". Letters of the scripts written without spaces between words (Han, Hiragana, Katakana, Thai, Lao, Khmer, Myanmar) are no word's, as a click could not tell where their words end without a dictionary.
func WordTokens(line string) []WordToken {
	var out []WordToken
	runes := []rune(line)
	add := func(text string, word bool) {
		if text == "" {
			return
		}
		if n := len(out); n > 0 && !out[n-1].Word && !word {
			out[n-1].Text += text
			return
		}
		out = append(out, WordToken{Text: text, Word: word})
	}
	for i := 0; i < len(runes); {
		if !tokenRune(runes[i]) {
			add(string(runes[i]), false)
			i++
			continue
		}
		j := i
		for j < len(runes) {
			switch {
			case tokenRune(runes[j]):
				j++
				continue
			case joiner(runes[j]) && j+1 < len(runes) && tokenRune(runes[j+1]) && j > i:
				j++
				continue
			}
			break
		}
		text := string(runes[i:j])
		add(text, strings.IndexFunc(text, unicode.IsLetter) >= 0)
		i = j
	}
	return out
}

// tokenRune reports whether r is part of a word: a letter, a mark or a digit of a script written with spaces between words. The marks written among Japanese kana and Han characters that are of no script of their own, the long vowel mark (ー, ｰ) and the closing mark (〆), are of those scripts here.
func tokenRune(r rune) bool {
	if !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r) {
		return false
	}
	switch r {
	case 'ー', 'ｰ', '〆':
		return false
	}
	return !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

// joiner reports whether r joins the parts of one word: an apostrophe or a hyphen.
func joiner(r rune) bool {
	switch r {
	case '\'', '’', '-', '‐':
		return true
	}
	return false
}
