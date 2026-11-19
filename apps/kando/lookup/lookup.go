// Package lookup is the word lookup of the player's lines: it cuts a line into the words a click looks up (Tokens), writes what the AI service set in 「AI 服务」 is asked about a word and reads its answer (Prompt, Request, Parse), and keeps the answers on this computer (Cache), so a word looked up once shows again at once, also offline. Kando bundles no dictionary of meanings: the meaning is the AI service's. It knows nothing of the interface.
package lookup

import (
	"encoding/json"
	"errors"
	"strings"

	"kando/packages/core"
	"kando/packages/core/segment"
)

// Token is a run of a line: a word a click looks up, or what lies between words (core.WordToken).
type Token = core.WordToken

// Tokens cuts line into its words and what lies between them, its Japanese and Chinese by dictionary (segment.Words): while that dictionary is still being read in the background they are no words yet, and loaded, when not nil, is called on another goroutine once it is read.
func Tokens(line string, loaded func()) []Token { return segment.Words(line, loaded) }

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
