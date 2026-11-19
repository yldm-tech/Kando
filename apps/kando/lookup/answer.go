package lookup

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"kando/packages/core"
	"kando/packages/core/segment"
)

// Dictionary is a dictionary that answers a lookup offline, as a dictionary pack does (plugins/dict.Dict): its id, and the entry of word, looked up by base, its dictionary form, first when known, with reading, the word's reading when known, picking homographs.
type Dictionary interface {
	ID() string
	LookUp(word, base, reading string) (Entry, bool, error)
}

// AI is the AI service a lookup asks (media.ChatTranslator).
type AI interface {
	Chat(ctx context.Context, system, user string) (string, error)
}

// ErrNoAnswer is a lookup no dictionary answered while no AI service can.
var ErrNoAnswer = errors.New("no dictionary has the word and no AI service can answer")

// Ask is how a lookup of Query, in a line of the film titled Film, is answered (Answer). Dicts are the dictionaries of the word's language (WordLanguage) whose definitions are in the language the lookup answers in, asked first, in order; Others those whose definitions are in another, asked only when no AI service can answer or the one set gave no answer. AI is the AI service, nil when none can answer. Failed, when set, is told of a dictionary whose lookup failed, as when its index is damaged, so the caller can open it again; the lookup goes on to the next one.
type Ask struct {
	Query  Query
	Film   string
	Dicts  []Dictionary
	Others []Dictionary
	AI     AI
	Failed func(d Dictionary, err error)
}

// Answers reports whether anything can answer the lookup: a dictionary, or the AI service.
func (k Ask) Answers() bool {
	return k.AI != nil || len(k.Dicts) > 0 || len(k.Others) > 0
}

// Answer is the entry of the word looked up and the id of the dictionary it came from, "" when the AI service gave it: the dictionaries of Dicts first, then the AI service, then those of Others, when no AI service can answer or the one set gave no answer, whose error it is when none of them has the word; ErrNoAnswer when none of them answered and no AI service can. A Japanese word with kana or Han characters is looked up by its dictionary form and reading as well (segment.Annotated), when a dictionary is asked, which reads the larger Japanese dictionary the first time, so Answer runs off the UI's goroutine.
func (k Ask) Answer(ctx context.Context) (Entry, string, error) {
	word := strings.TrimSpace(k.Query.Word)
	annotated := false
	base, reading := "", ""
	find := func(dicts []Dictionary) (Entry, string, bool) {
		if len(dicts) == 0 {
			return Entry{}, "", false
		}
		if !annotated {
			annotated = true
			if WordLanguage(k.Query) == "ja" && japanese(word) {
				for _, m := range segment.Annotated("ja", k.Query.Line) {
					if m.Word && m.Text == word {
						base, reading = m.Base, m.Reading
						break
					}
				}
			}
		}
		for _, d := range dicts {
			e, ok, err := d.LookUp(k.Query.Word, base, reading)
			if err != nil {
				if k.Failed != nil {
					k.Failed(d, err)
				}
				continue
			}
			if ok {
				return e, d.ID(), true
			}
		}
		return Entry{}, "", false
	}
	if e, id, ok := find(k.Dicts); ok {
		return e, id, nil
	}
	if k.AI == nil {
		if e, id, ok := find(k.Others); ok {
			return e, id, nil
		}
		return Entry{}, "", ErrNoAnswer
	}
	reply, err := k.AI.Chat(ctx, Prompt(k.Query.Language), Request(k.Query.Word, k.Query.Line, k.Film))
	if err == nil {
		var e Entry
		if e, err = Parse(k.Query.Word, reply); err == nil {
			return e, "", nil
		}
	}
	// The AI service gave no answer: a dictionary in another language still may, unless the lookup was given up.
	if ctx.Err() == nil {
		if e, id, ok := find(k.Others); ok {
			return e, id, nil
		}
	}
	return Entry{}, "", err
}

// japanese reports whether s has kana or Han characters, which only a Japanese word's dictionary form and reading are found for.
func japanese(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) }) >= 0
}

// WordLanguage is the language of the word looked up as q, as its dictionaries are chosen by: the language its film is spoken in, else what its script says, Japanese for kana, or for Han characters in a line with kana, Chinese for other Han characters; "" when neither tells, which every dictionary is for.
func WordLanguage(q Query) string {
	if lang := core.BaseLanguage(q.Spoken); lang != "" {
		return lang
	}
	kana := func(s string) bool {
		return strings.IndexFunc(s, func(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) }) >= 0
	}
	switch {
	case kana(q.Word):
		return "ja"
	case strings.IndexFunc(q.Word, func(r rune) bool { return unicode.Is(unicode.Han, r) }) < 0:
		return ""
	case kana(q.Line):
		return "ja"
	}
	return "zh"
}
