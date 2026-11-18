// Package lookup is the word lookup of the player's lines: it cuts a line into the words a click looks up (Tokens), writes what the AI service set in 「AI 服务」 is asked about a word and reads its answer (Prompt, Request, Parse), and keeps the answers on this computer (Cache), so a word looked up once shows again at once, also offline. Kando bundles no dictionary: the meaning is the AI service's. It knows nothing of the interface.
package lookup

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

// Token is a run of a line: a word a click looks up, or what lies between words, as spaces and punctuation.
type Token struct {
	Text string
	Word bool
}

// Tokens cuts line into its words and what lies between them, which joined again are the line. A word is a run of letters, marks and digits with at least one letter, an apostrophe or a hyphen inside it joining its parts, as "don't" and "well-known". Letters of the scripts written without spaces between words (Han, Hiragana, Katakana, Thai, Lao, Khmer, Myanmar) are no word's, as a click could not tell where their words end without a dictionary.
func Tokens(line string) []Token {
	var out []Token
	runes := []rune(line)
	add := func(text string, word bool) {
		if text == "" {
			return
		}
		if n := len(out); n > 0 && !out[n-1].Word && !word {
			out[n-1].Text += text
			return
		}
		out = append(out, Token{Text: text, Word: word})
	}
	for i := 0; i < len(runes); {
		if !wordRune(runes[i]) {
			add(string(runes[i]), false)
			i++
			continue
		}
		j := i
		for j < len(runes) {
			switch {
			case wordRune(runes[j]):
				j++
				continue
			case joiner(runes[j]) && j+1 < len(runes) && wordRune(runes[j+1]) && j > i:
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

// wordRune reports whether r is part of a word: a letter, a mark or a digit of a script written with spaces between words.
func wordRune(r rune) bool {
	if !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r) {
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

// Entry is what the AI service said of a word: the word as a dictionary lists it, its reading and its meaning, in the language it was asked to answer in.
type Entry struct {
	Word, Reading, Meaning string
}

// Prompt is the instruction a lookup is sent with: give the word's dictionary form, how it is said and what it means in the line, in language, as JSON (Parse).
func Prompt(language string) string {
	if strings.TrimSpace(language) == "" {
		language = "简体中文"
	}
	return "你是给看外语影片的学习者用的词典。用户消息里是一部影片的一行字幕和其中的一个词。" +
		"用" + language + "解释这个词：先给出它的词典形式（原形），再给出读音（英语等用国际音标，日语用假名，其他语言用通行的注音，没有就留空），" +
		"最后用一两句简短地写出词性和它在这一行里的意思，必要时补一个常见的其他意思。" +
		`只输出一个 JSON 对象：{"word": "词典形式", "reading": "读音", "meaning": "释义"}。`
}

// Request is the message a lookup of word, as it reads in line, of the film titled film, is sent with.
func Request(word, line, film string) string {
	var b strings.Builder
	if film = strings.TrimSpace(film); film != "" {
		b.WriteString("影片：" + film + "\n")
	}
	b.WriteString("字幕：" + strings.TrimSpace(line) + "\n")
	b.WriteString("要查的词：" + strings.TrimSpace(word))
	return b.String()
}

// ErrNoMeaning is what Parse returns for a reply that gives no meaning.
var ErrNoMeaning = errors.New("the AI service gave no meaning")

// Parse reads the AI service's reply to a lookup of word: the JSON object Prompt asks for, or, from a model that answered in plain text, the text as the meaning. The word is word itself when the reply names no dictionary form.
func Parse(word, reply string) (Entry, error) {
	reply = strings.TrimSpace(reply)
	var parsed struct {
		Word    string `json:"word"`
		Reading string `json:"reading"`
		Meaning string `json:"meaning"`
	}
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start >= 0 && end > start && json.Unmarshal([]byte(reply[start:end+1]), &parsed) == nil {
		e := Entry{Word: strings.TrimSpace(parsed.Word), Reading: strings.TrimSpace(parsed.Reading), Meaning: strings.TrimSpace(parsed.Meaning)}
		if e.Word == "" {
			e.Word = strings.TrimSpace(word)
		}
		if e.Meaning == "" {
			return Entry{}, ErrNoMeaning
		}
		return e, nil
	}
	if reply == "" {
		return Entry{}, ErrNoMeaning
	}
	return Entry{Word: strings.TrimSpace(word), Meaning: reply}, nil
}
