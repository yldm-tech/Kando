package dict

import "strings"

// toneMarks are the vowels of pinyin with the marks of tones 1 to 4.
var toneMarks = map[rune][4]rune{
	'a': {'ā', 'á', 'ǎ', 'à'}, 'e': {'ē', 'é', 'ě', 'è'}, 'i': {'ī', 'í', 'ǐ', 'ì'}, 'o': {'ō', 'ó', 'ǒ', 'ò'}, 'u': {'ū', 'ú', 'ǔ', 'ù'}, 'ü': {'ǖ', 'ǘ', 'ǚ', 'ǜ'},
	'A': {'Ā', 'Á', 'Ǎ', 'À'}, 'E': {'Ē', 'É', 'Ě', 'È'}, 'I': {'Ī', 'Í', 'Ǐ', 'Ì'}, 'O': {'Ō', 'Ó', 'Ǒ', 'Ò'}, 'U': {'Ū', 'Ú', 'Ǔ', 'Ù'}, 'Ü': {'Ǖ', 'Ǘ', 'Ǚ', 'Ǜ'},
}

// pinyin is CC-CEDICT's pinyin, its syllables numbered by tone ("zhong1 guo2", "lu:4"), written with tone marks ("zhōng guó", "lǜ"). A syllable without a tone number, or of tone 5, the neutral tone, has no mark.
func pinyin(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = syllable(w)
	}
	return strings.Join(words, " ")
}

// syllable is the pinyin syllable s with its tone number made a mark on its vowel: on a or e when it has one, on the o of ou, else on its last vowel.
func syllable(s string) string {
	s = strings.NewReplacer("u:", "ü", "U:", "Ü").Replace(s)
	n := len(s)
	if n < 2 || s[n-1] < '1' || s[n-1] > '5' {
		return s
	}
	tone := int(s[n-1] - '0')
	r := []rune(s[:n-1])
	if tone == 5 {
		return string(r)
	}
	at := -1
	for i, c := range r {
		switch c {
		case 'a', 'e', 'A', 'E':
			at = i
		}
		if at >= 0 {
			break
		}
	}
	if at < 0 {
		for i := 0; i+1 < len(r); i++ {
			if (r[i] == 'o' || r[i] == 'O') && (r[i+1] == 'u' || r[i+1] == 'U') {
				at = i
				break
			}
		}
	}
	if at < 0 {
		for i := len(r) - 1; i >= 0; i-- {
			if _, ok := toneMarks[r[i]]; ok {
				at = i
				break
			}
		}
	}
	if at < 0 {
		return string(r)
	}
	r[at] = toneMarks[r[at]][tone-1]
	return string(r)
}
